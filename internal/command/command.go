package command

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/agent"
	"brambleclaw/internal/bus"
	"context"
	"fmt"
)

type ClearCommand struct{}

func (c *ClearCommand) Name() string        { return "clear" }
func (c *ClearCommand) Description() string { return "清除当前会话历史" }
func (c *ClearCommand) Usage() string       { return "/clear" }

func (c *ClearCommand) Execute(ctx context.Context, a *agent.Agent, msg *bus.InBoundMessage, args []string) error {
	sessKey := util.BuildSessionKey(a.Name(), msg.InChannel, msg.ChatID)
	count, err := a.ClearSession(sessKey)

	var reply string
	if err != nil {
		reply = fmt.Sprintf("❌ 清理失败: %v", err)
	} else {
		reply = fmt.Sprintf("✅ 会话已重置，删除了 %d 条消息。", count)
	}
	fmt.Println(reply)
	return err
}
