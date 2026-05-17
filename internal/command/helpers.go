package command

import (
	"context"
	"fmt"
	"neoclaw/internal/bus"
	"neoclaw/internal/messages"
)

// publishReply 发布命令执行结果的反馈消息
func publishReply(ctx context.Context, agent interface{}, msg *bus.InBoundMessage, content string) error {
	type agentWithBus interface {
		Bus() *bus.MessageBus
		Name() string
	}
	a, ok := agent.(agentWithBus)
	if !ok {
		return fmt.Errorf("agent does not have Bus or Name method")
	}

	replyMsg := messages.NewTextMessage(a.Name(), content)
	outData := messages.ToOutBoundData(replyMsg, msg.ChatID, msg.InChannel, msg.ID)

	outbound := &bus.OutBoundMessage{
		ChatID:     outData.ChatID,
		OutChannel: outData.Channel,
		Content:    outData.Content,
		MsgType:    outData.MsgType,
		ReplyTo:    outData.ReplyTo,
		TimeStamp:  outData.TimeStamp,
	}

	return a.Bus().PublishOutBoundMessage(ctx, outbound)
}
