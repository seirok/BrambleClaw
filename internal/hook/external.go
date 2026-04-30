package hook

import (
	"brambleclaw/internal/logger"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"brambleclaw/internal/config/structs"
)

// ExternalHookExecutor 外部 Hook 执行器
type ExternalHookExecutor struct {
	debug bool
}

// NewExternalHookExecutor 创建新的外部 Hook 执行器
func NewExternalHookExecutor() *ExternalHookExecutor {
	return &ExternalHookExecutor{
		debug: true,
	}
}

// SetDebugEnabled 设置调试模式
func (e *ExternalHookExecutor) SetDebugEnabled(enabled bool) {
	e.debug = enabled
}

// Execute 执行外部 Hook
func (e *ExternalHookExecutor) Execute(ctx context.Context, hook *ExternalHook, data interface{}) (*externalHookResult, error) {
	startTime := time.Now()

	// 1. 构建请求
	request, err := NewHookRequest(hook.Point(), data)
	if err != nil {
		return nil, fmt.Errorf("failed to create hook request: %w", err)
	}

	// 2. 序列化请求
	inputJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal hook request: %w", err)
	}

	if e.debug {
		logger.L().Debug().
			Str("hook_point", hook.Point()).
			Str("command", hook.config.Command).
			Str("script", hook.config.ScriptPath).
			Str("request_id", request.RequestID).
			Msg("Executing external hook")
	}

	// 3. 创建带超时的上下文
	timeout := hook.Timeout()
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 4. 准备命令
	args := append([]string{hook.config.ScriptPath}, hook.config.Args...)
	cmd := exec.CommandContext(execCtx, hook.config.Command, args...)

	// 5. 设置工作目录
	if workingDir := hook.WorkingDir(); workingDir != "" {
		cmd.Dir = workingDir
	}

	// 6. 设置环境变量
	cmd.Env = append(os.Environ(), hook.config.Env...)
	cmd.Env = append(cmd.Env, hook.defaults.Env...)

	// 7. 设置 stdin/stdout/stderr
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// 8. 执行命令
	runErr := cmd.Run()
	endTime := time.Now()

	// 9. 收集结果
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	executionInfo := ExecutionInfo{
		Duration:  endTime.Sub(startTime),
		StartTime: startTime,
		EndTime:   endTime,
		ExitCode:  exitCode,
		Stderr:    stderr,
		RequestID: request.RequestID,
	}

	if e.debug {
		logger.L().Debug().
			Str("hook_point", hook.Point()).
			Str("request_id", request.RequestID).
			Dur("duration", executionInfo.Duration).
			Int("exit_code", exitCode).
			Msg("External hook executed")
	}

	// 10. 处理执行错误
	if runErr != nil {
		// 检查是否是超时
		if execCtx.Err() == context.DeadlineExceeded {
			return &externalHookResult{
				executionInfo: executionInfo,
				success:       false,
				err:           fmt.Errorf("hook execution timeout after %v", timeout),
			}, nil
		}

		// 其他错误（非零退出码等）
		if exitCode != 0 {
			return &externalHookResult{
				executionInfo: executionInfo,
				success:       false,
				err:           fmt.Errorf("hook exited with code %d: %s", exitCode, stderr),
			}, nil
		}

		return &externalHookResult{
			executionInfo: executionInfo,
			success:       false,
			err:           fmt.Errorf("hook execution failed: %w", runErr),
		}, nil
	}

	// 11. 检查输出大小
	maxOutputSize := hook.MaxOutputSize()
	if maxOutputSize > 0 && len(stdout) > maxOutputSize {
		return &externalHookResult{
			executionInfo: executionInfo,
			success:       false,
			err:           fmt.Errorf("hook output exceeds max size: %d > %d", len(stdout), maxOutputSize),
		}, nil
	}

	// 12. 解析响应
	var response HookResponse
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		return &externalHookResult{
			executionInfo: executionInfo,
			success:       false,
			err:           fmt.Errorf("failed to parse hook response: %w, raw output: %s", err, stdout),
		}, nil
	}

	// 13. 验证决策类型
	if !response.Decision.IsValid() {
		return &externalHookResult{
			executionInfo: executionInfo,
			success:       false,
			err:           fmt.Errorf("invalid decision type: %s", response.Decision),
		}, nil
	}

	return &externalHookResult{
		response:      &response,
		executionInfo: executionInfo,
		success:       true,
	}, nil
}

// ValidateExternalHook 验证外部 Hook 配置是否有效
func ValidateExternalHook(config *structs.ExternalConfig) error {
	if config.Command == "" {
		return fmt.Errorf("external hook command is required")
	}

	if config.ScriptPath == "" {
		return fmt.Errorf("external hook script_path is required")
	}

	// 检查脚本文件是否存在
	if _, err := os.Stat(config.ScriptPath); os.IsNotExist(err) {
		return fmt.Errorf("external hook script not found: %s", config.ScriptPath)
	}

	return nil
}
