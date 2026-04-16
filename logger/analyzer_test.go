package logger

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeLogs(t *testing.T) {
	// 创建临时测试目录和文件
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test.log")

	// 写入几行测试日志
	logContent := `{"level":"info","time":"2026-04-16T12:00:00Z","message":"test msg 1"}
{"level":"debug","time":"2026-04-16T12:00:01Z","message":"test msg 2"}
{"level":"error","time":"2026-04-16T12:00:02Z","message":"test msg 3"}
{"level":"warn","time":"2026-04-16T12:00:03Z","message":"test msg 4"}
`
	if err := os.WriteFile(logPath, []byte(logContent), 0644); err != nil {
		t.Fatalf("failed to write test log file: %v", err)
	}

	// 重定向 stdout 以捕获 AnalyzeLogs 的输出
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// 分析最近 2 行日志
	err := AnalyzeLogs(logPath, 2)

	// 恢复 stdout
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("AnalyzeLogs returned error: %v", err)
	}

	// 读取捕获的输出
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// 验证输出是否包含最后两行日志的特征（zerolog 格式化后的输出）
	if !strings.Contains(output, "test msg 3") {
		t.Errorf("Expected output to contain 'test msg 3', got: %s", output)
	}
	if !strings.Contains(output, "test msg 4") {
		t.Errorf("Expected output to contain 'test msg 4', got: %s", output)
	}
	// 不应该包含前两行
	if strings.Contains(output, "test msg 1") {
		t.Errorf("Expected output NOT to contain 'test msg 1', got: %s", output)
	}
}
