package hook

import (
	"context"
	"time"

	"neoclaw/internal/config/structs"
	"neoclaw/internal/interfaces"
)

// ExternalHook 外部脚本 Hook 实现
// 实现 interfaces.Service 接口以满足 Manager 约束
type ExternalHook struct {
	// 基础属性
	point    string
	priority Priority
	enabled  bool

	// 外部脚本配置
	config structs.ExternalConfig

	// 默认配置（从全局配置继承）
	defaults structs.HookDefaults
}

// NewExternalHook 创建新的外部 Hook
func NewExternalHook(point string, priority Priority, config structs.ExternalConfig, defaults structs.HookDefaults) *ExternalHook {
	return &ExternalHook{
		point:    point,
		priority: priority,
		enabled:  true,
		config:   config,
		defaults: defaults,
	}
}

// Point 返回钩子点名称
func (h *ExternalHook) Point() string {
	return h.point
}

// Priority 返回优先级
func (h *ExternalHook) Priority() Priority {
	return h.priority
}

// Enabled 返回是否启用
func (h *ExternalHook) Enabled() bool {
	return h.enabled
}

// SetEnabled 设置启用状态
func (h *ExternalHook) SetEnabled(enabled bool) {
	h.enabled = enabled
}

// Config 返回外部脚本配置
func (h *ExternalHook) Config() structs.ExternalConfig {
	return h.config
}

// Timeout 返回超时时间
func (h *ExternalHook) Timeout() time.Duration {
	timeoutMs := h.config.GetTimeoutMs(h.defaults.TimeoutMs)
	if timeoutMs <= 0 {
		timeoutMs = 5000 // 默认 5 秒
	}
	return time.Duration(timeoutMs) * time.Millisecond
}

// MaxOutputSize 返回最大输出大小
func (h *ExternalHook) MaxOutputSize() int {
	return h.config.GetMaxOutputSize(h.defaults.MaxOutputSize)
}

// WorkingDir 返回工作目录
func (h *ExternalHook) WorkingDir() string {
	return h.config.GetWorkingDir(h.defaults.WorkingDir)
}

// --- interfaces.Service 实现 ---

// 编译时检查接口实现
var _ interfaces.Service = (*ExternalHook)(nil)

// Name 实现 interfaces.Service
func (h *ExternalHook) Name() string {
	return h.point + ":" + h.config.ScriptPath
}

// Start 实现 interfaces.Service
// 外部 Hook 按需启动子进程，此处仅标记启用
func (h *ExternalHook) Start(ctx context.Context) error {
	h.enabled = true
	return nil
}

// Stop 实现 interfaces.Service
// 标记禁用，已运行的子进程由 ProcessManager 管理生命周期
func (h *ExternalHook) Stop(ctx context.Context) error {
	h.enabled = false
	return nil
}

// externalHookResult 外部 Hook 执行结果
type externalHookResult struct {
	// 原始响应对象
	response *HookResponse

	// 执行信息
	executionInfo ExecutionInfo

	// 是否成功
	success bool

	// 错误信息
	err error
}

// ExecutionInfo 执行元信息
type ExecutionInfo struct {
	// Duration 执行耗时
	Duration time.Duration

	// StartTime 开始时间
	StartTime time.Time

	// EndTime 结束时间
	EndTime time.Time

	// ExitCode 退出码
	ExitCode int

	// Stderr 标准错误输出
	Stderr string

	// RequestID 请求 ID
	RequestID string
}

// hookExecutor Hook 执行器接口
type hookExecutor interface {
	Execute(ctx context.Context, hook *ExternalHook, data interface{}) (*externalHookResult, error)
}
