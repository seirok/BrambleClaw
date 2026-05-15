package agent

import (
	"brambleclaw/internal/audit"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/messages"
	"brambleclaw/internal/sandbox"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

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
}

// AuditLogger 返回审计日志记录器
func (o *Orchestrator) AuditLogger() *audit.AuditLogger {
	return o.auditLogger
}

// NewOrchestrator 创建编排器
func NewOrchestrator(llm LLMProcessor, tools interfaces.Registry[interfaces.Tool], auditLogger *audit.AuditLogger) *Orchestrator {
	return &Orchestrator{
		llm:         llm,
		tools:       tools,
		auditLogger: auditLogger,
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
	for len(toolCalls) > 0 {
		for _, call := range toolCalls {
			result, err := o.executeToolCall(ctx, call.Function, call.ID)
			if err != nil {
				// Type 1: feed error back to LLM as tool result
				logger.L().Error().Err(err).Str("tool", call.Function.Name).Msg("Tool execution failed")
				msgWithToolError := ChatMsg{
					Role:       RoleTool,
					Content:    fmt.Sprintf("Error: tool execution failed - %s", err.Error()),
					ToolCallID: call.ID,
				}
				chatMsgs = append(chatMsgs, msgWithToolError)
				continue
			}
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
		hook.Emit(ctx, "hook.point.tool.error", fmt.Errorf("tool %s not found", toolName))
		return ToolResult{}, fmt.Errorf("tool %s not found", toolName)
	}

	// Execute Tool
	args := toolCall.Arguments

	// 触发工具执行前钩子
	if processedArgs, err := hook.Emit(ctx, "hook.point.tool.pre-execute", args); err != nil {
		return ToolResult{}, err
	} else if processedArgs != nil {
		if argStr, ok := processedArgs.(string); ok {
			args = argStr
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
		hook.Emit(ctx, "hook.point.tool.error", err)
		return ToolResult{}, err
	}

	// 触发工具执行后钩子
	if processedResult, err := hook.Emit(ctx, "hook.point.tool.result", result); err != nil {
		return ToolResult{}, err
	} else if processedResult != nil {
		result = processedResult
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
