package cli

import (
	"testing"
)

func TestAgentFlags(t *testing.T) {
	agentCmd.ParseFlags([]string{"-m", "hello", "-s", "new-task"})
	if agentMessage != "hello" {
		t.Errorf("Expected agentMessage to be 'hello', got %s", agentMessage)
	}
	if agentSession != "new-task" {
		t.Errorf("Expected agentSession to be 'new-task', got %s", agentSession)
	}
}
