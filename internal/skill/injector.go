package skill

import (
	"bytes"
	"context"
	"fmt"
	"neoclaw/internal/config/structs"
	"neoclaw/internal/logger"
	"os/exec"
	"regexp"
	"time"
)

var cmdRegex = regexp.MustCompile("!`([^`]+)`")

func InjectDynamicContext(ctx context.Context, content string, workingDir string, cfg *structs.SkillConfig) (string, error) {
	var err error
	result := cmdRegex.ReplaceAllStringFunc(content, func(m string) string {
		matches := cmdRegex.FindStringSubmatch(m)
		if len(matches) < 2 {
			return m
		}
		cmdStr := matches[1]

		// 执行命令
		cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.CommandTimeoutMs)*time.Millisecond)
		defer cancel()

		cmd := exec.CommandContext(cmdCtx, "cmd", "/c", cmdStr)
		cmd.Dir = workingDir

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if cmdErr := cmd.Run(); cmdErr != nil {
			logger.L().Warn().Err(cmdErr).Str("cmd", cmdStr).Str("stderr", stderr.String()).Msg("Dynamic command failed")
			return fmt.Sprintf("[COMMAND FAILED: %v]", cmdErr)
		}

		output := stdout.String()
		if len(output) > cfg.MaxCommandOutput {
			output = output[:cfg.MaxCommandOutput] + "... [TRUNCATED]"
		}
		return output
	})

	return result, err
}
