package channel

import (
	"brambleclaw/internal/bus"
	"brambleclaw/internal/config/structs" // 修改导入路径
	"brambleclaw/internal/logger"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
)

// DingtalkChannel 钉钉通道
type DingTalkChannel struct {
	base            *BaseChannelImpl       // 嵌入新的 Base 结构体
	dingtalkConfig  structs.DingTalkConfig // 修改类型
	streamClient    *client.StreamClient
	sessionWebhooks sync.Map
	running         bool
}

func NewDingTalkChannel(cfg structs.DingTalkConfig, messageBus *bus.MessageBus) (*DingTalkChannel, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("dingtalk client_id and client_secret are required")
	}

	baseCfg := &BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowFrom,
	}
	base := NewBaseChannelImpl("dingtalk", baseCfg, messageBus)

	return &DingTalkChannel{
		base:           base,
		dingtalkConfig: cfg,
	}, nil
}

// Start 启动通道
func (c *DingTalkChannel) Start(ctx context.Context) error {
	if !c.dingtalkConfig.Enabled {
		return nil
	}

	cred := client.NewAppCredentialConfig(c.dingtalkConfig.ClientID, c.dingtalkConfig.ClientSecret)
	c.streamClient = client.NewStreamClient(
		client.WithAppCredential(cred),
		client.WithAutoReconnect(true),
	)

	c.streamClient.RegisterChatBotCallbackRouter(c.onChatBotMessageReceived)
	// Start the stream client
	if err := c.streamClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start stream client: %w", err)
	}

	c.running = true
	return nil
}

// Stop 停止通道
func (c *DingTalkChannel) Stop(ctx context.Context) error {
	// 这里可以添加钉钉客户端的关闭或清理逻辑
	c.running = false
	return nil
}

// Send 发送消息
func (c *DingTalkChannel) Send(ctx context.Context, msg *bus.OutBoundMessage) error {
	replier := chatbot.NewChatbotReplier()

	// Convert string content to []byte for the API
	contentBytes := []byte(msg.Content)
	titleBytes := []byte("BrambleClaw")

	//
	sessionWebhookRaw, ok := c.sessionWebhooks.Load(msg.ChatID)
	if !ok {
		return fmt.Errorf("session webhook not loaded")
	}

	sessionWebhook, ok := sessionWebhookRaw.(string)
	if !ok {
		return fmt.Errorf("session webhook type wrong")
	}

	err := replier.SimpleReplyMarkdown(
		ctx,
		sessionWebhook,
		titleBytes,
		contentBytes,
	)
	if err != nil {
		return fmt.Errorf("failed to send reply: %w", err)
	}

	return nil
}

func (c *DingTalkChannel) onChatBotMessageReceived(
	ctx context.Context,
	data *chatbot.BotCallbackDataModel,
) ([]byte, error) {
	if data == nil {
		return nil, nil
	}

	// Extract message content from Text field
	content := strings.TrimSpace(data.Text.Content)
	if content == "" {
		// Try to extract from Content interface{} if Text is empty
		if contentMap, ok := data.Content.(map[string]any); ok {
			if textContent, ok := contentMap["content"].(string); ok {
				content = strings.TrimSpace(textContent)
			}
		}
	}

	if content == "" {
		return nil, nil // Ignore empty messages
	}

	senderID := strings.TrimSpace(data.SenderStaffId)
	if senderID == "" {
		senderID = strings.TrimSpace(data.SenderId)
	}
	//	senderNick := strings.TrimSpace(data.SenderNick)

	chatID := strings.TrimSpace(data.ConversationId)
	if chatID == "" && data.ConversationType == "1" {
		// Fallback for direct chats when conversation_id is absent.
		chatID = senderID
	}
	if chatID == "" {
		return nil, nil
	}
	c.sessionWebhooks.Store(chatID, data.SessionWebhook)

	inboundMsg := &bus.InBoundMessage{
		InChannel: "ding",
		SenderID:  senderID,
		ChatID:    chatID,
		Content:   content,
		TimeStamp: time.Now(),
	}

	if err := c.base.PublishInBoundMessage(ctx, inboundMsg); err != nil {
		logger.L().Error().Err(err).Str("chat_id", chatID).Str("sender_id", senderID).Msg("发送消息失败")
	}

	return []byte{}, nil
}

func (c *DingTalkChannel) Name() string {
	return c.base.Name()
}

func (c *DingTalkChannel) IsAllowed(id string) bool {
	return c.base.IsAllowed(id)
}
