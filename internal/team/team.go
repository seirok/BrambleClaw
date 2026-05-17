package team

import (
	"context"
	"sync/atomic"

	"neoclaw/internal/agent"
	"neoclaw/internal/logger"
	"neoclaw/internal/messages"
)

// HandleCommonResponse handles common response logic for all managers
// Returns true if the message was fully handled (caller should return)
// Returns false if the caller should continue with default logic
func HandleCommonResponse(
	ctx context.Context,
	msg messages.ChatMessage,
	managerCtx GroupChatManagerContext,
	findParticipant func(name string) int,
	onHandoff func(idx int), // called when handoff target is found
	onErrorSkip func(), // called when ErrorPolicySkip is triggered
) bool {
	if messages.IsStopMessage(msg) {
		return true
	}

	if messages.IsErrorMessage(msg) {
		switch managerCtx.ErrorPolicy {
		case ErrorPolicyTerminate:
			stopMsg := messages.NewStopMessage(
				"manager",
				"team terminated due to error from "+msg.GetSource()+": "+messages.GetErrorDetail(msg),
			)
			managerCtx.Runtime.Publish(ctx, managerCtx.OutputTopic, stopMsg)
			return true
		case ErrorPolicySkip:
			logger.L().Warn().
				Str("agent", msg.GetSource()).
				Str("error", messages.GetErrorDetail(msg)).
				Msg("Agent error, skipping to next participant")
			onErrorSkip()
			return true
		default:
			stopMsg := messages.NewStopMessage(
				"manager",
				"team terminated due to error from "+msg.GetSource()+": "+messages.GetErrorDetail(msg),
			)
			managerCtx.Runtime.Publish(ctx, managerCtx.OutputTopic, stopMsg)
			return true
		}
	}

	if messages.IsHandoffMessage(msg) {
		target := messages.GetHandoffTarget(msg)
		if idx := findParticipant(target); idx >= 0 {
			onHandoff(idx)
			return true
		}
	}

	return false
}

// ErrorPolicy 错误处理策略
type ErrorPolicy string

const (
	ErrorPolicyTerminate ErrorPolicy = "terminate"
	ErrorPolicySkip      ErrorPolicy = "skip"
)

// TaskResult 任务执行结果
type TaskResult struct {
	Messages []messages.ChatMessage
	Error    string // 非空表示团队有成员报错（快速检查字段）
}

// Team 团队接口，嵌套 ChatAgent 以支持 Team 作为 Agent 参与另一个 Team
type Team interface {
	agent.ChatAgent

	// Run 执行任务，收集所有消息后返回
	Run(ctx context.Context, task messages.ChatMessage) (*TaskResult, error)

	// RunStream 流式执行任务，逐条推送 StreamItem
	RunStream(ctx context.Context, task messages.ChatMessage) (<-chan agent.StreamItem, error)
}

// TerminationCondition 终止条件接口
type TerminationCondition interface {
	// ShouldTerminate 判断是否应该终止
	ShouldTerminate(msg messages.ChatMessage) bool
	// Reset 重置终止条件（用于新一轮对话）
	Reset()
}

// MaxTurnsTermination 最大消息数终止
type MaxTurnsTermination struct {
	maxTurns int
	count    atomic.Int32
}

// NewMaxTurnsTermination 创建最大消息数终止条件
func NewMaxTurnsTermination(maxTurns int) *MaxTurnsTermination {
	return &MaxTurnsTermination{maxTurns: maxTurns}
}

func (t *MaxTurnsTermination) ShouldTerminate(msg messages.ChatMessage) bool {
	return t.count.Add(1) >= int32(t.maxTurns)
}

func (t *MaxTurnsTermination) Reset() {
	t.count.Store(0)
}

// StopMessageTermination 遇到 StopMessage 时终止
type StopMessageTermination struct{}

// NewStopMessageTermination 创建 StopMessage 终止条件
func NewStopMessageTermination() *StopMessageTermination {
	return &StopMessageTermination{}
}

func (t *StopMessageTermination) ShouldTerminate(msg messages.ChatMessage) bool {
	return messages.IsStopMessage(msg)
}

func (t *StopMessageTermination) Reset() {}

// AllTermination 组合终止条件（任一条件满足即终止）
type AllTermination struct {
	conditions []TerminationCondition
}

// NewAllTermination 创建组合终止条件
func NewAllTermination(conditions ...TerminationCondition) *AllTermination {
	return &AllTermination{conditions: conditions}
}

func (t *AllTermination) ShouldTerminate(msg messages.ChatMessage) bool {
	for _, c := range t.conditions {
		if c.ShouldTerminate(msg) {
			return true
		}
	}
	return false
}

func (t *AllTermination) Reset() {
	for _, c := range t.conditions {
		c.Reset()
	}
}
