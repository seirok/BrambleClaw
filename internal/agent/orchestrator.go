package agent

import (
	"brambleclaw/internal/hook"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/messages"
	"brambleclaw/internal/tools"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type LLMProcessor interface {
	Chat(req ChatCompletionRequest) (*LLMResponse, error)
	Model() string
	// 注意：这里只定义 Orchestrator 真正用到的方法
}

// Orchestrator 编排器
type Orchestrator struct {
	llm   LLMProcessor
	tools interfaces.Registry[tools.Tool]
	ctx   context.Context
}

// NewOrchestrator 创建编排器
func NewOrchestrator(llm LLMProcessor, tools interfaces.Registry[tools.Tool]) *Orchestrator {
	return &Orchestrator{
		llm:   llm,
		tools: tools,
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
		Role:      Role(rawMsg.Role), // 如果 Role 是别名，记得转换
		Content:   rawMsg.Content,
		ToolCalls: rawMsg.ToolCalls, // 只要 ToolCall 的结构一致，可以直接赋值
	}
	*historyMsg = append(*historyMsg, respMsg)
	return nil
}

// Run 运行编排器
func (o *Orchestrator) Run(ctx context.Context, messages []messages.BaseMessage) (*LLMResponse, error) {
	// 准备工具定义
	toolDefs := o.prepareToolDefinitions()

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
		Tools:    toolDefs,
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
				return nil, err
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
			Tools:    toolDefs,
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
		return ToolResult{}, errors.New("tool not found")
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

	result, err := tool.Execute(ctx, args)
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
	resultStr := ""
	switch v := result.(type) {
	case string:
		resultStr = v
	default:
		jsonData, err := json.Marshal(result)
		if err == nil {
			resultStr = string(jsonData)
		} else {
			resultStr = fmt.Sprintf("%v", result)
		}
	}

	// Return Result
	toolResult := ToolResult{
		Content: resultStr,
		CallId:  toolCallId,
	}

	return toolResult, nil
}
