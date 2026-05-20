package tui

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"neoclaw/internal/events"
)

// getEventStyle 获取事件样式
func getEventStyle(point string) lipgloss.Style {
	switch {
	case strings.HasPrefix(point, "hook.point.llm."):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("69")) // blue
	case strings.HasPrefix(point, "hook.point.tool."):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // yellow
	case strings.HasPrefix(point, "hook.point.message."):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // gray
	case strings.HasPrefix(point, "hook.point.agent."):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("78")) // green
	case strings.HasPrefix(point, "hook.point.sandbox."):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("135")) // purple
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // gray
	}
}

// formatEventSummary 格式化事件摘要
func formatEventSummary(evt events.ThinkingEvent) string {
	switch {
	case strings.HasPrefix(evt.Point, "hook.point.llm."):
		return formatLLMSummary(evt)
	case strings.HasPrefix(evt.Point, "hook.point.tool."):
		return formatToolSummary(evt)
	case strings.HasPrefix(evt.Point, "hook.point.message."):
		return formatMessageSummary(evt)
	case strings.HasPrefix(evt.Point, "hook.point.agent."):
		return formatAgentSummary(evt)
	case strings.HasPrefix(evt.Point, "hook.point.sandbox."):
		return formatSandboxSummary(evt)
	default:
		return fmt.Sprintf("%s: %T", evt.Point, evt.Data)
	}
}

func formatLLMSummary(evt events.ThinkingEvent) string {
	switch evt.Point {
	case "hook.point.llm.request":
		req := evt.Data
		model := "unknown"
		msgCount := 0
		v := reflect.ValueOf(req)
		if v.Kind() == reflect.Struct {
			if modelField := v.FieldByName("Model"); modelField.IsValid() {
				model = modelField.String()
			}
			if msgsField := v.FieldByName("Messages"); msgsField.IsValid() {
				msgCount = msgsField.Len()
			}
		}
		return fmt.Sprintf("LLM → %s (%d messages)", model, msgCount)
	case "hook.point.llm.response":
		resp := evt.Data
		totalTokens := 0
		v := reflect.ValueOf(resp)
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
			v = v.Elem()
			if usageField := v.FieldByName("Usage"); usageField.IsValid() {
				if ptField := usageField.FieldByName("PromptTokens"); ptField.IsValid() {
					totalTokens += int(ptField.Int())
				}
				if ctField := usageField.FieldByName("CompletionTokens"); ctField.IsValid() {
					totalTokens += int(ctField.Int())
				}
			}
		}
		return fmt.Sprintf("LLM ← response (%d tokens)", totalTokens)
	case "hook.point.llm.error":
		if err, ok := evt.Data.(error); ok {
			return fmt.Sprintf("LLM ✗ %v", err)
		}
		return "LLM ✗ error"
	}
	return "LLM event"
}

// toolEventInfo 从 hook 数据中提取的工具事件信息
type toolEventInfo struct {
	Name string
	Data any
}

// extractToolEvent 从 evt.Data 提取工具名和数据。
// 支持 *ToolExecuteEvent 格式（新）和原始格式（旧，向后兼容）。
func extractToolEvent(data any) toolEventInfo {
	if data == nil {
		return toolEventInfo{}
	}

	v := reflect.ValueOf(data)
	// 处理指针
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		// 检查是否为 ToolExecuteEvent 结构
		nameField := v.FieldByName("ToolName")
		dataField := v.FieldByName("Data")
		if nameField.IsValid() && dataField.IsValid() {
			return toolEventInfo{
				Name: nameField.String(),
				Data: dataField.Interface(),
			}
		}
	}

	// 向后兼容：不是 ToolExecuteEvent 格式，返回原始数据
	return toolEventInfo{
		Name: "unknown",
		Data: data,
	}
}

func formatToolSummary(evt events.ThinkingEvent) string {
	evtInfo := extractToolEvent(evt.Data)
	switch evt.Point {
	case "hook.point.tool.pre-execute":
		if args, ok := evtInfo.Data.(string); ok {
			return fmt.Sprintf("[%s] ▶ %s", evtInfo.Name, truncate(args, 100))
		}
		return fmt.Sprintf("[%s] ▶ executing", evtInfo.Name)
	case "hook.point.tool.result":
		resultStr := fmt.Sprintf("%v", evtInfo.Data)
		return fmt.Sprintf("[%s] ◀ %s", evtInfo.Name, truncate(resultStr, 100))
	case "hook.point.tool.error":
		if err, ok := evtInfo.Data.(error); ok {
			return fmt.Sprintf("[%s] ✗ %v", evtInfo.Name, err)
		}
		return fmt.Sprintf("[%s] ✗ error", evtInfo.Name)
	}
	return "TOOL event"
}

func formatMessageSummary(evt events.ThinkingEvent) string {
	switch evt.Point {
	case "hook.point.message.pre-process":
		var content string
		v := reflect.ValueOf(evt.Data)
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
			v = v.Elem()
			if f := v.FieldByName("Content"); f.IsValid() {
				content = f.String()
			}
		}
		return fmt.Sprintf("MSG → processing: %s", truncate(content, 50))
	case "hook.point.message.pre-response":
		var content string
		v := reflect.ValueOf(evt.Data)
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
			v = v.Elem()
			if f := v.FieldByName("Content"); f.IsValid() {
				content = f.String()
			}
		}
		return fmt.Sprintf("MSG ← responding: %s", truncate(content, 50))
	case "hook.point.message.post-process":
		return "MSG ✔ processed"
	}
	return "MSG event"
}

func formatAgentSummary(evt events.ThinkingEvent) string {
	name := "unknown"
	v := reflect.ValueOf(evt.Data)
	if v.Kind() == reflect.Ptr {
		if nameMethod := v.MethodByName("Name"); nameMethod.IsValid() {
			results := nameMethod.Call(nil)
			if len(results) == 1 && results[0].Kind() == reflect.String {
				name = results[0].String()
			}
		}
	}
	switch evt.Point {
	case "hook.point.agent.create":
		return fmt.Sprintf("AGENT %s created", name)
	case "hook.point.agent.pre-start":
		return fmt.Sprintf("AGENT %s starting...", name)
	case "hook.point.agent.start":
		return fmt.Sprintf("AGENT %s started", name)
	case "hook.point.agent.pre-stop":
		return fmt.Sprintf("AGENT %s stopping...", name)
	case "hook.point.agent.stop":
		return fmt.Sprintf("AGENT %s stopped", name)
	}
	return "AGENT event"
}

func formatSandboxSummary(evt events.ThinkingEvent) string {
	return fmt.Sprintf("SANDBOX %s", strings.TrimPrefix(evt.Point, "hook.point.sandbox."))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	return s[:maxLen-3] + "..."
}
