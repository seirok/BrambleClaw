package channel

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/config/structs"
	"brambleclaw/internal/logger"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/sipeed/picoclaw/pkg/channels"
)

// errCodeTenantTokenInvalid is the Feishu API error code for an expired/revoked
// tenant_access_token. The Lark SDK's built-in retry does not clear its cache
// on this error, so we do it ourselves.
const errCodeTenantTokenInvalid = 99991663

type tokenCache struct {
	mu    sync.RWMutex
	store map[string]*tokenEntry
}

type tokenEntry struct {
	value    string
	expireAt time.Time
}

func newTokenCache() *tokenCache {
	return &tokenCache{store: make(map[string]*tokenEntry)}
}

func (c *tokenCache) Set(_ context.Context, key, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = &tokenEntry{value: value, expireAt: time.Now().Add(ttl)}
	return nil
}

func (c *tokenCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.store[key]
	if !ok {
		return "", nil
	}
	if e.expireAt.Before(time.Now()) {
		delete(c.store, key)
		return "", nil
	}
	return e.value, nil
}

// InvalidateAll removes all cached tokens, forcing fresh acquisition.
func (c *tokenCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.store)
}

type FeishuChannel struct {
	base       *BaseChannelImpl
	config     structs.FeishuConfig
	client     *lark.Client
	wsClient   *larkws.Client
	tokenCache *tokenCache // custom cache that supports invalidation

	botOpenID atomic.Value // stores string; populated lazily for @mention detection
	running   bool

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (c *FeishuChannel) Name() string {
	return c.base.Name()
}

func (c *FeishuChannel) Stop(ctx context.Context) error {
	c.running = false
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

func (c *FeishuChannel) Send(ctx context.Context, message *bus.OutBoundMessage) error {
	if message.ChatID == "" {
		return fmt.Errorf("chat ID is empty: %w", channels.ErrSendFailed)
	}

	// Build interactive card with markdown content
	cardContent, err := buildMarkdownCard(message.Content)
	if err != nil {
		// If card build fails, fall back to plain text
		return c.sendText(ctx, message.ChatID, message.Content)
	}

	// First attempt: try sending as interactive card
	err = c.sendCard(ctx, message.ChatID, cardContent)
	if err == nil {
		return nil
	}

	// Check if error is due to card table limit (error code 11310)
	// See: https://open.feishu.cn/document/server-docs/im-api/message-content-description/create_json
	errMsg := err.Error()
	isCardLimitError := strings.Contains(errMsg, "11310")

	if isCardLimitError {
		logger.L().Warn().Str("chat_id", message.ChatID).Err(err).Msg("feishu: Card send failed (table limit), falling back to text message")

		// Second attempt: fall back to plain text message
		textErr := c.sendText(ctx, message.ChatID, message.Content)
		if textErr == nil {
			return nil
		}
		// If text also fails, return the text error
		return textErr
	}

	// For other errors, return the original card error
	return err
}

func (c *FeishuChannel) IsAllowed(s string) bool {
	return c.base.IsAllowed(s)
}

func NewFeishuChannel(cfg structs.FeishuConfig, bus *bus.MessageBus) (*FeishuChannel, error) {
	baseCfg := &BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowFrom,
	}
	base := NewBaseChannelImpl("feishu", baseCfg, bus)
	// 在飞书（Lark）开放平台中，与 API 交互通常需要一个有效的 访问令牌 (Access Token) ，也称为 tenant_access_token 。
	// 这个令牌是有时效性的，通常在一段时间后会过期。为了避免每次 API 请求都重新获取令牌，从而提高效率和减少 API 调用次数，通常会使用缓存机制来存储和复用这个令牌
	tc := newTokenCache()
	opts := []lark.ClientOptionFunc{lark.WithTokenCache(tc)}
	if cfg.IsLark {
		opts = append(opts, lark.WithOpenBaseUrl(lark.LarkBaseUrl))
	}
	ch := &FeishuChannel{
		base:       base,
		config:     cfg,
		tokenCache: tc,
		client:     lark.NewClient(cfg.AppID, cfg.AppSecret, opts...),
	}

	return ch, nil
}

func (c *FeishuChannel) Start(ctx context.Context) error {
	if c.config.AppID == "" || c.config.AppSecret == "" {
		return fmt.Errorf("feishu app_id or app_secret is empty")
	}
	dispatcher := larkdispatcher.NewEventDispatcher(c.config.VerificationToken, c.config.EncryptKey).
		OnP2MessageReceiveV1(c.handleMessageReceive)

	domain := lark.FeishuBaseUrl
	c.wsClient = larkws.NewClient(
		c.config.AppID,
		c.config.AppSecret,
		larkws.WithEventHandler(dispatcher),
		larkws.WithDomain(domain),
	)
	wsClient := c.wsClient
	runCtx, runCancel := context.WithCancel(ctx)
	c.cancel = runCancel
	go func() {
		if err := wsClient.Start(runCtx); err != nil {
			logger.L().Error().Err(err).Msg("failed to start feishu client")
		}
	}()
	return nil
}

func (c *FeishuChannel) handleMessageReceive(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}

	message := event.Event.Message
	sender := event.Event.Sender

	chatID := util.StringValue(message.ChatId)
	if chatID == "" {
		return nil
	}

	senderID := extractFeishuSenderID(sender)
	if senderID == "" {
		senderID = "unknown"
	}

	messageType := util.StringValue(message.MessageType)
	messageID := util.StringValue(message.MessageId)
	rawContent := util.StringValue(message.Content)

	content := extractContent(messageType, rawContent)

	// TODO: 处理入站的媒体消息（例如图片、文件等），将其下载并存储起来

	if content == "" {
		content = "[empty message]"
	}

	metadata := map[string]string{}
	if messageID != "" {
		metadata["message_id"] = messageID
	}
	if messageType != "" {
		metadata["message_type"] = messageType
	}
	chatType := util.StringValue(message.ChatType)
	if chatType != "" {
		metadata["chat_type"] = chatType
	}
	if sender != nil && sender.TenantKey != nil {
		metadata["tenant_key"] = *sender.TenantKey
	}

	//
	inboundMsg := &bus.InBoundMessage{
		InChannel: "feishu",
		SenderID:  senderID,
		ChatID:    chatID,
		Content:   content,
		TimeStamp: time.Now(),
	}

	err := c.base.PublishInBoundMessage(ctx, inboundMsg)
	if err != nil {
		return err
	}

	return nil
}

func extractFeishuSenderID(sender *larkim.EventSender) string {
	if sender == nil || sender.SenderId == nil {
		return ""
	}

	if sender.SenderId.UserId != nil && *sender.SenderId.UserId != "" {
		return *sender.SenderId.UserId
	}
	if sender.SenderId.OpenId != nil && *sender.SenderId.OpenId != "" {
		return *sender.SenderId.OpenId
	}
	if sender.SenderId.UnionId != nil && *sender.SenderId.UnionId != "" {
		return *sender.SenderId.UnionId
	}

	return ""
}

// extractContent extracts text content from different message types.
func extractContent(messageType, rawContent string) string {
	if rawContent == "" {
		return ""
	}

	switch messageType {
	case larkim.MsgTypeText:
		var textPayload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(rawContent), &textPayload); err == nil {
			return textPayload.Text
		}
		return rawContent

	case larkim.MsgTypePost:
		// Pass raw JSON to LLM — structured rich text is more informative than flattened plain text
		return rawContent

	case larkim.MsgTypeInteractive:
		// Pass raw JSON to LLM — structured card is more informative than flattened text
		return rawContent

	case larkim.MsgTypeImage:
		// Image messages don't have text content
		return ""

	case larkim.MsgTypeFile, larkim.MsgTypeAudio, larkim.MsgTypeMedia:
		// File/audio/video messages may have a filename
		name := extractFileName(rawContent)
		if name != "" {
			return name
		}
		return ""

	default:
		return rawContent
	}
}

func extractFileName(content string) string { return util.ExtractJSONStringField(content, "file_name") }

func buildMarkdownCard(content string) (string, error) {
	card := map[string]any{
		"schema": "2.0",
		"body": map[string]any{
			"elements": []map[string]any{
				{
					"tag":     "markdown",
					"content": content,
				},
			},
		},
	}
	data, err := json.Marshal(card)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *FeishuChannel) sendText(ctx context.Context, chatID, text string) error {
	content, _ := json.Marshal(map[string]string{"text": text})

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeText).
			Content(string(content)).
			Build()).
		Build()

	resp, err := c.client.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("feishu send text: %w", channels.ErrTemporary)
	}

	if !resp.Success() {
		return fmt.Errorf("feishu text api error (code=%d msg=%s): %w", resp.Code, resp.Msg, channels.ErrTemporary)
	}

	logger.L().Debug().Msg("Feishu text message sent (fallback)")

	return nil
}

func (c *FeishuChannel) sendCard(ctx context.Context, chatID, cardContent string) error {
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeInteractive).
			Content(cardContent).
			Build()).
		Build()

	resp, err := c.client.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("feishu send card: %w", channels.ErrTemporary)
	}

	if !resp.Success() {
		c.invalidateTokenOnAuthError(resp.Code)
		return fmt.Errorf("feishu api error (code=%d msg=%s): %w", resp.Code, resp.Msg, channels.ErrTemporary)
	}
	return nil
}

func (c *FeishuChannel) invalidateTokenOnAuthError(code int) {
	if code == errCodeTenantTokenInvalid {
		c.tokenCache.InvalidateAll()
		logger.L().Error().Str("app_id", c.config.AppID).Msg("feishu: Invalidated cached token due to auth error")
	}
}
