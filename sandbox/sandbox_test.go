package sandbox_test

import (
	"brambleclaw/bus"
	"brambleclaw/internal"
	"brambleclaw/logger"
	"context"
	"testing"
	"time"
)

func TestSandboxFileTool(t *testing.T) {
	logger.SetLevel("debug")

	gateway, msgBus, _, mockChannel := testutil.CreateTestGatewayWithMockChannel()
	ctx := context.Background()

	// 启动 Gateway
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("启动 Gateway 失败: %v", err)
	}
	defer gateway.Stop()

	// 等待 Gateway 完全启动
	time.Sleep(100 * time.Millisecond)

	// 发送测试消息
	testMsg := &bus.InBoundMessage{
		ID:        "test-msg-1",
		SenderID:  "user123",
		ChatID:    "chat456",
		InChannel: "cli",
		Content:   "你好，你给我阐述一下理想主义者的特质(一段极其简单的话），然后把相关内容写到本地",
		TimeStamp: time.Now(),
	}

	// 发布消息到总线
	if err := msgBus.PublishInBoundMessage(ctx, testMsg); err != nil {
		t.Fatalf("发布消息失败: %v", err)
	}

	// 等待消息处理完成
	select {
	case <-mockChannel.OnSendCalled:
		// 消息已发送成功
	case <-time.After(300 * time.Second):
		t.Error("等待响应超时，消息可能未正确路由")
	}

	// 验证消息是否被发送
	messages := mockChannel.GetMessages()
	if len(messages) == 0 {
		t.Error("没有收到任何响应消息")
	}

	// 验证响应内容
	found := false
	for _, msg := range messages {
		if msg.ReplyTo == testMsg.ID {
			found = true
			if msg.ChatID != testMsg.ChatID {
				t.Errorf("ChatID 不匹配: 期望 %s, 得到 %s", testMsg.ChatID, msg.ChatID)
			}
			if msg.OutChannel != testMsg.InChannel {
				t.Errorf("OutChannel 不匹配: 期望 %s, 得到 %s", testMsg.InChannel, msg.OutChannel)
			}
			logger.L().Info().Msg(msg.Content)
			break
		}

	}
	if !found {
		t.Error("没有找到对应的回复消息")
	}
}
