package channel

import (
	"brambleclaw/internal/bus"
	"brambleclaw/internal/config/structs"
	"context"
	"fmt"
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

	// Chat routing: track whether a chatID is group or direct.
	chatType sync.Map // chatID → "group" | "direct"

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
			InChannel: "feishu",
			SenderID:  senderID,
			ChatID:    senderID, //TODO
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
			ChatID:    senderID, //TODO
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
