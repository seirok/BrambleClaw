package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	// 捕获标准输出
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runVersion(versionCmd, nil)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "brambleclaw 1.0.0") {
		t.Errorf("Expected version output to contain 'brambleclaw 1.0.0', got %s", output)
	}
}

func TestExecuteHelp(t *testing.T) {
	// 测试 help 命令
	rootCmd.SetArgs([]string{"--help"})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Execute()

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Errorf("Execute with --help failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "brambleclaw is a Go language implementation of an AI Agent framework") {
		t.Errorf("Expected help output to contain framework description, got %s", output)
	}
}
