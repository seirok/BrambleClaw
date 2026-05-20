package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"neoclaw/internal/audit"
	"neoclaw/internal/hook"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/logger"
	"neoclaw/internal/messages"
	"neoclaw/internal/sandbox"
	"neoclaw/internal/validation"
	"time"
)

// ToolExecuteEvent 工具执行事件（传递给 hook）
type ToolExecuteEvent struct {
	ToolName string      // 工具名称
	Data     interface{} // 数据（args 或 result 或 error）
}

type LLMProcessor interface {
	Chat(req ChatCompletionRequest) (*LLMResponse, error)
	Model() string
	// 注意：这里只定义 Orchestrator 真正用到的方法
}

// Orchestrator 编排器
type Orchestrator struct {
	llm         LLMProcessor
	tools       interfaces.Registry[interfaces.Tool]
	ctx         context.Context
	auditLogger *audit.AuditLogger
	validator   *validation.SchemaCache
	maxRetries  int
	enabled     bool
}

// AuditLogger 返回审计日志记录器
func (o *Orchestrator) AuditLogger() *audit.AuditLogger {
	return o.auditLogger
}

// NewOrchestrator 创建编排器
func NewOrchestrator(llm LLMProcessor, tools interfaces.Registry[interfaces.Tool], auditLogger *audit.AuditLogger, enabled bool, maxRetries int) *Orchestrator {
	return &Orchestrator{
		llm:         llm,
		tools:       tools,
		auditLogger: auditLogger,
		validator:   validation.NewSchemaCache(),
		enabled:     enabled,
		maxRetries:  maxRetries,
	}
}

// LLM 返回 LLM 处理器
func (o *Orchestrator) LLM() LLMProcessor { return o.llm }

// prepareToolDefinitions 准备工具定义
func (o *Orchestrator) prepareToolDefinitions() []map[string]interface{} {
	toolList := o.tools.List(o.ctx)
	defs := make([]map[string]interface{}, 0, len(toolList))

	for _, tool := range toolList {
		defs = append(defs, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name(),
				"description": tool.Description(),
				"parameters":  tool.Parameters(),
			},
		})
	}

	//jsonBytes, _ := json.MarshalIndent(defs, "", "  ")
	//logger.L().Debug().Msg(string(jsonBytes))

	return defs
}

func (o *Orchestrator) UpdateHistory(llmResp *LLMResponse, historyMsg *[]ChatMsg) error {
	// Validation Check
	if llmResp == nil {
		return errors.New("llmResp is nil")
	}
	if len(llmResp.Choices) == 0 {
		return errors.New("llmResp.Choices is empty or nil")
	}
	if historyMsg == nil {
		return errors.New("historyMsg is nil")
	}

	//
	rawMsg := llmResp.Choices[0].Message

	respMsg := ChatMsg{
		Role:      Role(rawMsg.Role),
		Content:   rawMsg.Content,
		ToolCalls: rawMsg.ToolCalls,
	}
	*historyMsg = append(*historyMsg, respMsg)
	return nil
}

// Run 运行编排器
func (o *Orchestrator) Run(ctx context.Context, messages []messages.BaseMessage) (*LLMResponse, error) {

	// Agent Message --> ChatMsg
	chatMsgs := make([]ChatMsg, len(messages))
	for i, msg := range messages {
		if am, ok := msg.(*AgentMessage); ok {
			chatMsgs[i].Role = am.Role
		}
		chatMsgs[i].Content = msg.ToText()
	}

	// Get LLM's response
	chatReq := ChatCompletionRequest{
		Model:    o.llm.Model(),
		Messages: chatMsgs,
		Tools:    o.prepareToolDefinitions(),
	}

	// 触发 LLM 请求前钩子
	if processedReq, err := hook.Emit(ctx, "hook.point.llm.request", chatReq); err != nil {
		return nil, err
	} else if processedReq != nil {
		if req, ok := processedReq.(ChatCompletionRequest); ok {
			chatReq = req
		}
	}

	response, err := o.llm.Chat(chatReq)
	if err != nil {
		// 触发 LLM 错误钩子
		hook.Emit(ctx, "hook.point.llm.error", err)
		return nil, err
	}

	// 触发 LLM 响应后钩子
	if processedResp, err := hook.Emit(ctx, "hook.point.llm.response", response); err != nil {
		return nil, err
	} else if processedResp != nil {
		if resp, ok := processedResp.(*LLMResponse); ok {
			response = resp
		}
	}

	// Update Chat History
	if err = o.UpdateHistory(response, &chatMsgs); err != nil {
		return nil, err
	}

	// Call Tools
	if len(response.Choices) == 0 { // 在 Go 中，对一个 nil 切片调用 len() 函数是安全且合法的
		return nil, errors.New("empty response choices: choices is nil or empty")
	}
	toolCalls := response.Choices[0].Message.ToolCalls
	// 追踪每个工具的连续校验失败次数
	validationRetries := make(map[string]int)
	for len(toolCalls) > 0 {
		for _, call := range toolCalls {
			result, err := o.executeToolCall(ctx, call.Function, call.ID)
			if err != nil {
				// 区分校验错误和执行错误
				var valErr *validation.ValidationError
				if errors.As(err, &valErr) {
					// 校验错误
					logger.L().Error().Err(err).Str("tool", call.Function.Name).Msg("Tool parameter validation failed")
					validationRetries[call.Function.Name]++

					var errorMsg string
					if validationRetries[call.Function.Name] >= o.maxRetries {
						// 达到最大重试次数
						errorMsg = fmt.Sprintf(
							"Tool %q parameter validation failed %d times. Last error: %s. The tool call will not be retried.",
							valErr.ToolName,
							validationRetries[call.Function.Name],
							valErr.SchemaError,
						)
						// 重置计数器
						validationRetries[call.Function.Name] = 0
					} else {
						// 使用 LLM 友好的错误消息
						errorMsg = valErr.ToLLMMessage()
					}

					msgWithToolError := ChatMsg{
						Role:       RoleTool,
						Content:    errorMsg,
						ToolCallID: call.ID,
					}
					chatMsgs = append(chatMsgs, msgWithToolError)
				} else {
					// 执行错误：原有格式
					logger.L().Error().Err(err).Str("tool", call.Function.Name).Msg("Tool execution failed")
					// 成功执行或其他错误，重置校验失败计数
					validationRetries[call.Function.Name] = 0

					msgWithToolError := ChatMsg{
						Role:       RoleTool,
						Content:    fmt.Sprintf("Error: tool execution failed - %s", err.Error()),
						ToolCallID: call.ID,
					}
					chatMsgs = append(chatMsgs, msgWithToolError)
				}
				continue
			}
			// 成功执行，重置校验失败计数
			validationRetries[call.Function.Name] = 0

			msgWithToolresult := ChatMsg{
				Role:       RoleTool,
				Content:    result.Content,
				ToolCallID: result.CallId,
			}
			chatMsgs = append(chatMsgs, msgWithToolresult)
		}

		chatReq = ChatCompletionRequest{
			Model:    o.llm.Model(),
			Messages: chatMsgs,
			Tools:    o.prepareToolDefinitions(),
		}
		response, err = o.llm.Chat(chatReq)
		if err != nil {
			return nil, err
		}

		// Update Chat History
		if err = o.UpdateHistory(response, &chatMsgs); err != nil {
			return nil, err
		}

		// Get Tool Call
		if len(response.Choices) == 0 {
			return nil, errors.New("empty response choices: choices is nil or empty")
		}
		toolCalls = response.Choices[0].Message.ToolCalls
	}

	// don't need to check again since before did
	return response, nil
}

// executeToolCalls 执行工具调用
func (o *Orchestrator) executeToolCall(ctx context.Context, toolCall ToolFunction, toolCallId string) (ToolResult, error) {
	// Get Tool
	toolName := toolCall.Name
	tool, ok := o.tools.Get(ctx, toolName)
	if ok != nil {
		hook.Emit(ctx, "hook.point.tool.error", &ToolExecuteEvent{ToolName: toolName, Data: fmt.Errorf("tool %s not found", toolName)})
		return ToolResult{}, fmt.Errorf("tool %s not found", toolName)
	}

	args := toolCall.Arguments

	// 参数校验
	if o.enabled {
		if valErr := validation.Validate(toolName, tool.Parameters(), args, o.validator); valErr != nil {
			// 触发校验错误钩子
			hook.Emit(ctx, "hook.point.tool.validation-error", valErr)

			// 审计记录
			if o.auditLogger != nil {
				o.auditLogger.LogToolCall(&audit.ToolCallAuditEvent{
					ToolName:   toolName,
					Arguments:  args,
					Result:     "",
					Success:    false,
					Error:      valErr.Error(),
					Duration:   0,
					SessionKey: sandbox.SessionKeyFromContext(ctx),
				})
			}

			return ToolResult{}, valErr
		}
	}

	// 触发工具执行前钩子
	toolEvent := &ToolExecuteEvent{ToolName: toolName, Data: args}
	if processedArgs, err := hook.Emit(ctx, "hook.point.tool.pre-execute", toolEvent); err != nil {
		return ToolResult{}, err
	} else if processedArgs != nil {
		if processedEvent, ok := processedArgs.(*ToolExecuteEvent); ok && processedEvent != nil {
			if argStr, ok := processedEvent.Data.(string); ok {
				args = argStr
			}
		}
	}

	// 记录开始时间
	startTime := time.Now()

	result, err := tool.Execute(ctx, args)
	duration := time.Since(startTime)

	// 审计记录（在 hook 之前，记录原始结果）
	if o.auditLogger != nil {
		resultStr := o.serializeResult(result)
		errorMsg := ""
		if err != nil {
			errorMsg = err.Error()
		}
		o.auditLogger.LogToolCall(&audit.ToolCallAuditEvent{
			ToolName:   toolName,
			Arguments:  args,
			Result:     resultStr,
			Success:    err == nil,
			Error:      errorMsg,
			Duration:   duration,
			SessionKey: sandbox.SessionKeyFromContext(ctx),
		})
	}

	if err != nil {
		// 触发工具错误钩子
		hook.Emit(ctx, "hook.point.tool.error", &ToolExecuteEvent{ToolName: toolName, Data: err})
		return ToolResult{}, err
	}

	// 触发工具执行后钩子
	if processedResult, err := hook.Emit(ctx, "hook.point.tool.result", &ToolExecuteEvent{ToolName: toolName, Data: result}); err != nil {
		return ToolResult{}, err
	} else if processedResult != nil {
		if ev, ok := processedResult.(*ToolExecuteEvent); ok && ev != nil {
			result = ev.Data
		}
	}

	// 转换结果为字符串
	resultStr := o.serializeResult(result)

	// Return Result
	toolResult := ToolResult{
		Content: resultStr,
		CallId:  toolCallId,
	}

	return toolResult, nil
}

// serializeResult 序列化工具执行结果为字符串
func (o *Orchestrator) serializeResult(result interface{}) string {
	switch v := result.(type) {
	case string:
		return v
	default:
		jsonData, err := json.Marshal(result)
		if err == nil {
			return string(jsonData)
		}
		return fmt.Sprintf("%v", result)
	}
}
