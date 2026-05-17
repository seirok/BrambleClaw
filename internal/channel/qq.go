package channel

import (
	"context"
	"fmt"
	"neoclaw/internal/bus"
	"neoclaw/internal/config/structs"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/event"
	"github.com/tencent-connect/botgo/openapi/options"
	"github.com/tencent-connect/botgo/token"
	"golang.org/x/oauth2"
)

type qqAPI interface {
	WS(ctx context.Context, params map[string]string, body string) (*dto.WebsocketAP, error)
	PostGroupMessage(
		ctx context.Context, groupID string, msg dto.APIMessage, opt ...options.Option,
	) (*dto.Message, error)
	PostC2CMessage(
		ctx context.Context, userID string, msg dto.APIMessage, opt ...options.Option,
	) (*dto.Message, error)
	PostMessage(
		ctx context.Context, channelID string, msg *dto.MessageToCreate, opt ...options.Option,
	) (*dto.Message, error)
	Transport(ctx context.Context, method, url string, body any) ([]byte, error)
}

type QQChannel struct {
	base           *BaseChannelImpl
	config         structs.QQConfig
	api            qqAPI
	tokenSource    oauth2.TokenSource
	ctx            context.Context
	cancel         context.CancelFunc
	sessionManager botgo.SessionManager
	downloadFn     func(urlStr, filename string) string

	// Chat routing: track whether a chatID is group, direct, or channel.
	chatType sync.Map // chatID → "group" | "direct" | "channel"

	// Passive reply: store last inbound message ID per chat.
	lastMsgID sync.Map // chatID → string

	// msg_seq: per-chat atomic counter for multi-part replies.
	msgSeqCounters sync.Map // chatID → *atomic.Uint64

	// Time-based dedup replacing the unbounded map.
	dedup   map[string]time.Time
	muDedup sync.Mutex

	// done is closed on Stop to shut down the dedup janitor.
	done     chan struct{}
	stopOnce sync.Once
}

var _ BaseChannel = (*QQChannel)(nil)

func NewQQChannel(cfg structs.QQConfig, messageBus *bus.MessageBus) (*QQChannel, error) {
	baseCfg := &BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowFrom,
	}
	base := NewBaseChannelImpl("qq", baseCfg, messageBus)

	return &QQChannel{
		base:   base,
		config: cfg,
		dedup:  make(map[string]time.Time),
		done:   make(chan struct{}),
	}, nil
}

func (c *QQChannel) Name() string {
	return c.base.Name()
}

func (c *QQChannel) IsAllowed(id string) bool {
	return c.base.IsAllowed(id)
}

func (c *QQChannel) Stop(ctx context.Context) error {
	c.stopOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		close(c.done)
	})
	return nil
}

func (c *QQChannel) Send(ctx context.Context, msg *bus.OutBoundMessage) error {
	if msg.ChatID == "" {
		return fmt.Errorf("qq send: chat ID is empty")
	}

	chatTypeVal, ok := c.chatType.Load(msg.ChatID)
	if !ok {
		return fmt.Errorf("qq send: unknown chat type for chatID %q", msg.ChatID)
	}
	chatType := chatTypeVal.(string)

	switch chatType {
	case "direct":
		return c.sendC2C(ctx, msg)
	case "group":
		return c.sendGroup(ctx, msg)
	case "channel":
		return c.sendChannel(ctx, msg)
	default:
		return fmt.Errorf("qq send: unsupported chat type %q", chatType)
	}
}

func (c *QQChannel) sendC2C(ctx context.Context, msg *bus.OutBoundMessage) error {
	lastID, _ := c.lastMsgID.Load(msg.ChatID)
	seq := c.nextMsgSeq(msg.ChatID)

	apimsg := &dto.MessageToCreate{
		Content: msg.Content,
		MsgID:   toStr(lastID),
		MsgSeq:  seq,
	}
	_, err := c.api.PostC2CMessage(ctx, msg.ChatID, apimsg)
	return err
}

func (c *QQChannel) sendGroup(ctx context.Context, msg *bus.OutBoundMessage) error {
	lastID, _ := c.lastMsgID.Load(msg.ChatID)
	seq := c.nextMsgSeq(msg.ChatID)

	apimsg := &dto.MessageToCreate{
		Content: msg.Content,
		MsgID:   toStr(lastID),
		MsgSeq:  seq,
	}
	_, err := c.api.PostGroupMessage(ctx, msg.ChatID, apimsg)
	return err
}

func (c *QQChannel) sendChannel(ctx context.Context, msg *bus.OutBoundMessage) error {
	lastID, _ := c.lastMsgID.Load(msg.ChatID)
	seq := c.nextMsgSeq(msg.ChatID)

	// 1) 尝试被动回复（带 MsgID）
	if lastIDStr, ok := lastID.(string); ok && lastIDStr != "" {
		apimsg := &dto.MessageToCreate{
			Content: msg.Content,
			MsgID:   lastIDStr,
			MsgSeq:  seq,
		}
		if c.config.SendMarkdown {
			apimsg.MsgType = dto.MarkdownMsg
			apimsg.Markdown = &dto.Markdown{Content: msg.Content}
		}
		if _, err := c.api.PostMessage(ctx, msg.ChatID, apimsg); err == nil {
			return nil
		}
		// 被动回复失败（可能超时），降级为主动发送
	}

	// 2) 主动发送（不带 MsgID）
	apimsg := &dto.MessageToCreate{
		Content: msg.Content,
		MsgSeq:  c.nextMsgSeq(msg.ChatID),
	}
	if c.config.SendMarkdown {
		apimsg.MsgType = dto.MarkdownMsg
		apimsg.Markdown = &dto.Markdown{Content: msg.Content}
	}
	_, err := c.api.PostMessage(ctx, msg.ChatID, apimsg)
	return err
}

func (c *QQChannel) nextMsgSeq(chatID string) uint32 {
	val, ok := c.msgSeqCounters.Load(chatID)
	if !ok {
		return 1
	}
	counter := val.(*atomic.Uint64)
	return uint32(counter.Add(1))
}

func toStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func (c *QQChannel) Start(ctx context.Context) error {
	if c.config.AppID == "" || c.config.AppSecret == "" {
		return fmt.Errorf("QQ app_id and app_secret not configured")
	}

	//	botgo.SetLogger(newBotGoLogger("botgo"))
	logger.InfoC("qq", "Starting QQ bot (WebSocket mode)")

	// Reinitialize shutdown signal for clean restart.
	c.done = make(chan struct{})
	c.stopOnce = sync.Once{}

	// create token source
	credentials := &token.QQBotCredentials{
		AppID:     c.config.AppID,
		AppSecret: c.config.AppSecret,
	}
	c.tokenSource = token.NewQQBotTokenSource(credentials)

	// create child context
	c.ctx, c.cancel = context.WithCancel(ctx)

	// start auto-refresh token goroutine
	if err := token.StartRefreshAccessToken(c.ctx, c.tokenSource); err != nil {
		return fmt.Errorf("failed to start token refresh: %w", err)
	}

	// initialize OpenAPI client
	c.api = botgo.NewOpenAPI(c.config.AppID, c.tokenSource).WithTimeout(5 * time.Second)

	// register event handlers
	intent := event.RegisterHandlers(
		c.handleC2CMessage(),
		c.handleGroupATMessage(),
		c.handleChannelATMessage(),
	)

	// get WebSocket endpoint
	wsInfo, err := c.api.WS(c.ctx, nil, "")
	if err != nil {
		return fmt.Errorf("failed to get websocket info: %w", err)
	}

	logger.InfoCF("qq", "Got WebSocket info", map[string]any{
		"shards": wsInfo.Shards,
	})

	// create and save sessionManager
	c.sessionManager = botgo.NewSessionManager()

	// start WebSocket connection in goroutine to avoid blocking
	go func() {
		if err := c.sessionManager.Start(wsInfo, c.tokenSource, &intent); err != nil {
			logger.ErrorCF("qq", "WebSocket session error", map[string]any{
				"error": err.Error(),
			})

		}
	}()

	// Pre-register reasoning_channel_id as group chat if configured,
	// so outbound-only destinations are routed correctly.
	if c.config.ReasoningChannelID != "" {
		c.chatType.Store(c.config.ReasoningChannelID, "group")
	}

	return nil
}

// handleC2CMessage handles QQ private messages.
func (c *QQChannel) handleC2CMessage() event.C2CMessageEventHandler {
	return func(event *dto.WSPayload, data *dto.WSC2CMessageData) error {
		// extract user info
		var senderID string
		if data.Author != nil && data.Author.ID != "" {
			senderID = data.Author.ID
		} else {
			//
			return nil
		}

		content := strings.TrimSpace(data.Content)
		//mediaPaths, attachmentNotes := c.extractInboundAttachments(senderID, data.ID, data.Attachments)
		//for _, note := range attachmentNotes {
		//	content = appendContent(content, note)
		//}
		//if content == "" && len(mediaPaths) == 0 {
		//	logger.DebugC("qq", "Received empty C2C message with no attachments, ignoring")
		//	return nil
		//}

		// Store chat routing context.
		c.chatType.Store(senderID, "direct")
		c.lastMsgID.Store(senderID, data.ID)

		// Reset msg_seq counter for new inbound message.
		c.msgSeqCounters.Store(senderID, new(atomic.Uint64))

		//
		inboundMsg := &bus.InBoundMessage{
			InChannel: "qq",
			SenderID:  senderID,
			ChatID:    senderID,
			Content:   content,
			TimeStamp: time.Now(),
		}

		err := c.base.PublishInBoundMessage(c.ctx, inboundMsg)
		if err != nil {
			return err
		}

		return nil
	}
}

func appendContent(content, suffix string) string {
	if suffix == "" {
		return content
	}
	if content == "" {
		return suffix
	}
	return content + "\n" + suffix
}

func (c *QQChannel) handleGroupATMessage() event.GroupATMessageEventHandler {
	return func(event *dto.WSPayload, data *dto.WSGroupATMessageData) error {
		// extract user info
		var senderID string
		if data.Author != nil && data.Author.ID != "" {
			senderID = data.Author.ID
		} else {
			logger.WarnC("qq", "Received group message with no sender ID")
			return nil
		}

		content := strings.TrimSpace(data.Content)
		//mediaPaths, attachmentNotes := c.extractInboundAttachments(data.GroupID, data.ID, data.Attachments)
		//for _, note := range attachmentNotes {
		//	content = appendContent(content, note)
		//}

		// GroupAT event means bot is always mentioned; apply group trigger filtering.
		cleaned := strings.TrimSpace(content)
		content = cleaned
		//if content == "" && len(mediaPaths) == 0 {
		//	logger.DebugC("qq", "Received empty group message with no attachments, ignoring")
		//	return nil
		//}

		// Store chat routing context using GroupID as chatID.
		c.chatType.Store(data.GroupID, "group")
		c.lastMsgID.Store(data.GroupID, data.ID)

		// Reset msg_seq counter for new inbound message.
		c.msgSeqCounters.Store(data.GroupID, new(atomic.Uint64))

		//
		inboundMsg := &bus.InBoundMessage{
			InChannel: "qq",
			SenderID:  senderID,
			ChatID:    data.GroupID,
			Content:   content,
			TimeStamp: time.Now(),
		}

		err := c.base.PublishInBoundMessage(c.ctx, inboundMsg)
		if err != nil {
			return err
		}

		return nil
	}
}

// handleChannelATMessage handles QQ channel (频道) @ messages.
func (c *QQChannel) handleChannelATMessage() event.ATMessageEventHandler {
	return func(ev *dto.WSPayload, data *dto.WSATMessageData) error {
		if data == nil {
			return nil
		}

		var senderID string
		if data.Author != nil && data.Author.ID != "" {
			senderID = data.Author.ID
		} else {
			logger.WarnC("qq", "Received channel AT message with no sender ID")
			return nil
		}

		content := strings.TrimSpace(data.Content)

		// Channel messages use ChannelID as the routing ChatID
		chatID := data.ChannelID
		if chatID == "" {
			logger.WarnC("qq", "Received channel AT message with no ChannelID")
			return nil
		}

		// Store chat routing context: "channel" type
		c.chatType.Store(chatID, "channel")
		c.lastMsgID.Store(chatID, data.ID)

		// Reset msg_seq counter for new inbound message
		c.msgSeqCounters.Store(chatID, new(atomic.Uint64))

		inboundMsg := &bus.InBoundMessage{
			InChannel: "qq",
			SenderID:  senderID,
			ChatID:    chatID,
			Content:   content,
			TimeStamp: time.Now(),
			Metadata: map[string]string{
				"guild_id":   data.GuildID,
				"channel_id": data.ChannelID,
				"message_id": data.ID,
			},
		}

		return c.base.PublishInBoundMessage(c.ctx, inboundMsg)
	}
}
