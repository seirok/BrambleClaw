package agent

import (
	"brambleclaw/tools"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatMsg struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolResult struct {
	Content string
	CallId  string
}

// Orchestrator 编排器
type Orchestrator struct {
	llmClient    *LLMClient
	toolRegistry *tools.ToolRegistry
}

// NewOrchestrator 创建编排器
func NewOrchestrator(llmClient *LLMClient, toolRegistry *tools.ToolRegistry) *Orchestrator {
	return &Orchestrator{
		llmClient:    llmClient,
		toolRegistry: toolRegistry,
	}
}

// prepareToolDefinitions 准备工具定义
func (o *Orchestrator) prepareToolDefinitions() []map[string]interface{} {
	toolList := o.toolRegistry.List()
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
func (o *Orchestrator) Run(ctx context.Context, messages []AgentMessage) (string, error) {
	// 准备工具定义
	toolDefs := o.prepareToolDefinitions()

	// Agent Message --> ChatMsg
	chatMsgs := make([]ChatMsg, len(messages))
	for i, msg := range messages {
		chatMsgs[i].Role = msg.Role
		content := ""
		for _, ct := range msg.Content {
			if textContent, ok := ct.(TextContent); ok {
				content += textContent.Text
			}
		}
		chatMsgs[i].Content = content
	}

	// Get LLM's response
	chatReq := ChatCompletionRequest{
		Model:    o.llmClient.config.Model,
		Messages: chatMsgs,
		Tools:    toolDefs,
	}
	response, err := o.llmClient.Chat(chatReq)
	if err != nil {
		return "", err
	}

	// Update Chat History
	if err = o.UpdateHistory(response, &chatMsgs); err != nil {
		return "", err
	}

	// Call Tools
	if len(response.Choices) == 0 { // 在 Go 中，对一个 nil 切片调用 len() 函数是安全且合法的
		return "", errors.New("empty response choices: choices is nil or empty")
	}
	toolCalls := response.Choices[0].Message.ToolCalls
	for len(toolCalls) > 0 {
		for _, call := range toolCalls {
			result, err := o.executeToolCall(ctx, call.Function, call.ID)
			if err != nil {
				return "", err
			}
			msgWithToolresult := ChatMsg{
				Role:       RoleTool,
				Content:    result.Content,
				ToolCallID: result.CallId,
			}
			chatMsgs = append(chatMsgs, msgWithToolresult)
		}

		chatReq = ChatCompletionRequest{
			Model:    o.llmClient.config.Model,
			Messages: chatMsgs,
			Tools:    toolDefs,
		}
		response, err = o.llmClient.Chat(chatReq)
		if err != nil {
			return "", err
		}

		// Update Chat History
		if err = o.UpdateHistory(response, &chatMsgs); err != nil {
			return "", err
		}

		// Get Tool Call
		if len(response.Choices) == 0 {
			return "", errors.New("empty response choices: choices is nil or empty")
		}
		toolCalls = response.Choices[0].Message.ToolCalls
	}

	// don't need to check again since before did
	return response.Choices[0].Message.Content, nil
}

// executeToolCalls 执行工具调用
func (o *Orchestrator) executeToolCall(ctx context.Context, toolCall ToolFunction, toolCallId string) (ToolResult, error) {
	// Get Tool
	toolName := toolCall.Name
	tool, ok := o.toolRegistry.Get(toolName)
	if !ok {
		return ToolResult{}, errors.New("tool not found")
	}

	// Execute Tool
	args := toolCall.Arguments
	result, err := tool.Execute(ctx, args)
	if err != nil {
		return ToolResult{}, err
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
