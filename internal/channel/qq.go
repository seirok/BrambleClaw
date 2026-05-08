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

	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/identity"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/media"
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
		mediaPaths, attachmentNotes := c.extractInboundAttachments(senderID, data.ID, data.Attachments)
		for _, note := range attachmentNotes {
			content = appendContent(content, note)
		}
		if content == "" && len(mediaPaths) == 0 {
			logger.DebugC("qq", "Received empty C2C message with no attachments, ignoring")
			return nil
		}

		logger.InfoCF("qq", "Received C2C message", map[string]any{
			"sender":      senderID,
			"length":      len(content),
			"media_count": len(mediaPaths),
		})

		// Store chat routing context.
		c.chatType.Store(senderID, "direct")
		c.lastMsgID.Store(senderID, data.ID)

		// Reset msg_seq counter for new inbound message.
		c.msgSeqCounters.Store(senderID, new(atomic.Uint64))

		metadata := map[string]string{
			"account_id": senderID,
		}

		//
		inboundMsg := &bus.InBoundMessage{
			InChannel: "feishu",
			SenderID:  senderID,
			ChatID:    chatID,
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

func (c *QQChannel) extractInboundAttachments(
	chatID, messageID string,
	attachments []*dto.MessageAttachment,
) ([]string, []string) {
	if len(attachments) == 0 {
		return nil, nil
	}

	scope := channels.BuildMediaScope("qq", chatID, messageID)
	mediaPaths := make([]string, 0, len(attachments))
	notes := make([]string, 0, len(attachments))

	storeMedia := func(localPath string, attachment *dto.MessageAttachment) string {
		if store := c.GetMediaStore(); store != nil {
			ref, err := store.Store(localPath, media.MediaMeta{
				Filename:      qqAttachmentFilename(attachment),
				ContentType:   attachment.ContentType,
				Source:        "qq",
				CleanupPolicy: media.CleanupPolicyDeleteOnCleanup,
			}, scope)
			if err == nil {
				return ref
			}
		}
		return localPath
	}

	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}

		filename := qqAttachmentFilename(attachment)
		if localPath := c.downloadAttachment(attachment.URL, filename); localPath != "" {
			mediaPaths = append(mediaPaths, storeMedia(localPath, attachment))
		} else if attachment.URL != "" {
			mediaPaths = append(mediaPaths, attachment.URL)
		}

		notes = append(notes, qqAttachmentNote(attachment))
	}

	return mediaPaths, notes
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
