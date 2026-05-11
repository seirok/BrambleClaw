package command

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/logger"
	"context"
	"fmt"
)

// ClearCommand clears the session by creating a new one
type ClearCommand struct{}

// Name returns command name
func (c *ClearCommand) Name() string { return "clear" }

// Description returns command description
func (c *ClearCommand) Description() string { return "创建新会话（保留旧会话记录）" }

// Usage returns usage info
func (c *ClearCommand) Usage() string { return "/clear" }

// Execute runs the command
func (c *ClearCommand) Execute(ctx context.Context, agent interface{}, msg interface{}, args []string) error {
	// Type assert agent to something that has ClearSession method and Name()
	type agentWithSession interface {
		ClearSession(sessionKey string) (int, error)
		Name() string
	}
	a, ok := agent.(agentWithSession)
	if !ok {
		return fmt.Errorf("agent does not support ClearSession")
	}

	// Type assert msg to bus.InBoundMessage
	m, ok := msg.(*bus.InBoundMessage)
	if !ok {
		return fmt.Errorf("invalid message type")
	}

	sessKey := util.BuildSessionKey(a.Name(), m.InChannel, m.ChatID)
	count, err := a.ClearSession(sessKey)

	var reply string
	if err != nil {
		reply = fmt.Sprintf("❌ 创建新会话失败: %v", err)
	} else {
		reply = fmt.Sprintf("✅ 已创建新会话，旧会话有 %d 条消息已保留。", count)
	}
	logger.L().Info().Str("command", "clear").Msg(reply)
	// fmt.Println(reply)
	return err
}
