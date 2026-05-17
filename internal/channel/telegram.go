package channel

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/sipeed/picoclaw/pkg/channels"

	"neoclaw/internal/bus"
	"neoclaw/internal/config/structs"
	"neoclaw/internal/logger"
)

type TelegramChannel struct {
	base   *BaseChannelImpl
	config structs.TelegramConfig
	bot    *telego.Bot
	bh     *th.BotHandler

	botUsername string

	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewTelegramChannel(cfg structs.TelegramConfig, bus *bus.MessageBus) (*TelegramChannel, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}

	baseCfg := &BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowFrom,
	}
	base := NewBaseChannelImpl("telegram", baseCfg, bus)

	var opts []telego.BotOption

	if cfg.Proxy != "" {
		proxyURL, err := url.Parse(cfg.Proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", cfg.Proxy, err)
		}
		opts = append(opts, telego.WithHTTPClient(&http.Client{
			Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		}))
	} else if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
		opts = append(opts, telego.WithHTTPClient(&http.Client{
			Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
		}))
	}

	if cfg.BaseURL != "" {
		opts = append(opts, telego.WithAPIServer(strings.TrimRight(cfg.BaseURL, "/")))
	}

	bot, err := telego.NewBot(cfg.Token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	return &TelegramChannel{
		base:   base,
		config: cfg,
		bot:    bot,
	}, nil
}

func (c *TelegramChannel) Name() string {
	return c.base.Name()
}

func (c *TelegramChannel) IsAllowed(s string) bool {
	return c.base.IsAllowed(s)
}

func (c *TelegramChannel) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config.Token == "" {
		return fmt.Errorf("telegram bot token not configured")
	}

	runCtx, runCancel := context.WithCancel(ctx)
	c.cancel = runCancel

	updates, err := c.bot.UpdatesViaLongPolling(runCtx, &telego.GetUpdatesParams{
		Timeout: 30,
	})
	if err != nil {
		c.cancel()
		return fmt.Errorf("failed to start long polling: %w", err)
	}

	bh, err := th.NewBotHandler(c.bot, updates)
	if err != nil {
		c.cancel()
		return fmt.Errorf("failed to create bot handler: %w", err)
	}
	c.bh = bh

	c.botUsername = c.bot.Username()

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.handleMessage(ctx, &message)
	}, th.AnyMessage())

	logger.L().Info().Str("username", c.botUsername).Msg("Telegram bot started (polling mode)")

	go func() {
		if err := bh.Start(); err != nil {
			logger.L().Error().Err(err).Msg("Telegram bot handler failed")
		}
	}()

	return nil
}

func (c *TelegramChannel) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bh != nil {
		_ = c.bh.StopWithContext(ctx)
	}
	if c.cancel != nil {
		c.cancel()
	}
	logger.L().Info().Msg("Telegram bot stopped")
	return nil
}

func (c *TelegramChannel) Send(ctx context.Context, msg *bus.OutBoundMessage) error {
	if msg.ChatID == "" {
		return fmt.Errorf("chat ID is empty: %w", channels.ErrSendFailed)
	}

	if msg.Content == "" {
		return nil
	}

	targetChatIDStr := msg.ChatID
	if msg.MsgType == "reasoning" && c.config.ReasoningChannelID != "" {
		targetChatIDStr = c.config.ReasoningChannelID
	}

	chatID, err := strconv.ParseInt(targetChatIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat ID %s: %w", targetChatIDStr, channels.ErrSendFailed)
	}

	content := markdownToTelegramHTML(msg.Content)

	chunks := splitTelegramMessage(content, 4096)

	for i, chunk := range chunks {
		tgMsg := tu.Message(tu.ID(chatID), chunk)
		tgMsg.WithParseMode(telego.ModeHTML)

		if i == 0 && msg.ReplyTo != "" {
			if mid, convErr := strconv.Atoi(msg.ReplyTo); convErr == nil {
				tgMsg.ReplyParameters = &telego.ReplyParameters{MessageID: mid}
			}
		}

		_, sendErr := c.bot.SendMessage(ctx, tgMsg)
		if sendErr != nil {
			if strings.Contains(sendErr.Error(), "Bad Request") || strings.Contains(sendErr.Error(), "can't parse") {
				logger.L().Warn().Err(sendErr).Msg("Telegram HTML parse failed, falling back to plain text")
				tgMsg.Text = msg.Content
				tgMsg.ParseMode = ""
				_, sendErr = c.bot.SendMessage(ctx, tgMsg)
				if sendErr != nil {
					return fmt.Errorf("telegram send: %w", channels.ErrTemporary)
				}
				continue
			}
			if strings.Contains(sendErr.Error(), "429") || strings.Contains(sendErr.Error(), "Too Many Requests") {
				return fmt.Errorf("telegram rate limited: %w", channels.ErrRateLimit)
			}
			return fmt.Errorf("telegram send: %w", channels.ErrTemporary)
		}
	}

	return nil
}

func (c *TelegramChannel) handleMessage(ctx context.Context, message *telego.Message) error {
	if message == nil || message.From == nil {
		return nil
	}

	user := message.From
	senderID := fmt.Sprintf("%d", user.ID)

	if !c.IsAllowed(senderID) {
		logger.L().Debug().Str("user_id", senderID).Msg("Telegram: message rejected by allowlist")
		return nil
	}

	content := ""
	if message.Text != "" {
		content = message.Text
	}
	if message.Caption != "" {
		if content != "" {
			content += "\n"
		}
		content += message.Caption
	}

	if content == "" {
		content = "[empty message]"
	}

	chatID := message.Chat.ID
	chatIDStr := fmt.Sprintf("%d", chatID)

	isMentioned := false
	if message.Chat.Type != "private" {
		isMentioned = c.isBotMentioned(message)
		if isMentioned {
			content = c.stripBotMention(content)
		}

		respond, cleaned := c.shouldRespondInGroup(isMentioned, content)
		if !respond {
			return nil
		}
		content = cleaned
	}

	metadata := map[string]string{
		"message_id": fmt.Sprintf("%d", message.MessageID),
		"chat_type":  message.Chat.Type,
		"username":   user.Username,
		"first_name": user.FirstName,
		"is_group":   fmt.Sprintf("%t", message.Chat.Type != "private"),
	}
	if message.ReplyToMessage != nil {
		metadata["reply_to_message_id"] = fmt.Sprintf("%d", message.ReplyToMessage.MessageID)
	}

	inboundMsg := &bus.InBoundMessage{
		InChannel: "telegram",
		SenderID:  senderID,
		ChatID:    chatIDStr,
		Content:   content,
		TimeStamp: time.Now(),
		Metadata:  metadata,
	}

	return c.base.PublishInBoundMessage(ctx, inboundMsg)
}

func (c *TelegramChannel) shouldRespondInGroup(isMentioned bool, content string) (bool, string) {
	gt := c.config.GroupTrigger

	if gt.MentionOnly && !isMentioned {
		return false, ""
	}

	if len(gt.Prefixes) > 0 {
		for _, prefix := range gt.Prefixes {
			if strings.HasPrefix(content, prefix) {
				return true, strings.TrimSpace(strings.TrimPrefix(content, prefix))
			}
		}
		if !isMentioned {
			return false, ""
		}
	}

	return true, content
}

func (c *TelegramChannel) isBotMentioned(message *telego.Message) bool {
	text, entities := "", []telego.MessageEntity{}
	if message.Text != "" {
		text = message.Text
		entities = message.Entities
	} else if message.Caption != "" {
		text = message.Caption
		entities = message.CaptionEntities
	}

	if text == "" || len(entities) == 0 {
		return false
	}

	botUsername := ""
	if c.bot != nil {
		botUsername = c.bot.Username()
	}
	runes := []rune(text)

	for _, entity := range entities {
		entityText, ok := telegramEntityText(runes, entity)
		if !ok {
			continue
		}

		switch entity.Type {
		case telego.EntityTypeMention:
			if botUsername != "" && strings.EqualFold(entityText, "@"+botUsername) {
				return true
			}
		case telego.EntityTypeTextMention:
			if botUsername != "" && entity.User != nil && strings.EqualFold(entity.User.Username, botUsername) {
				return true
			}
		case telego.EntityTypeBotCommand:
			if c.isBotCommandForUs(entityText) {
				return true
			}
		}
	}
	return false
}

func (c *TelegramChannel) isBotCommandForUs(entityText string) bool {
	if !strings.HasPrefix(entityText, "/") {
		return false
	}
	command := strings.TrimPrefix(entityText, "/")
	if command == "" {
		return false
	}

	at := strings.IndexRune(command, '@')
	if at == -1 {
		return true
	}

	mentionUsername := command[at+1:]
	if mentionUsername == "" || c.botUsername == "" {
		return false
	}
	return strings.EqualFold(mentionUsername, c.botUsername)
}

func (c *TelegramChannel) stripBotMention(content string) string {
	botUsername := c.botUsername
	if botUsername == "" {
		return content
	}
	re := regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(botUsername))
	content = re.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

func telegramEntityText(runes []rune, entity telego.MessageEntity) (string, bool) {
	if entity.Offset < 0 || entity.Length <= 0 {
		return "", false
	}
	end := entity.Offset + entity.Length
	if entity.Offset >= len(runes) || end > len(runes) {
		return "", false
	}
	return string(runes[entity.Offset:end]), true
}

func splitTelegramMessage(content string, maxLen int) []string {
	runes := []rune(content)
	if len(runes) <= maxLen {
		return []string{content}
	}

	var chunks []string
	for len(runes) > 0 {
		cut := maxLen
		if cut > len(runes) {
			cut = len(runes)
		}
		if cut < len(runes) {
			lastNL := strings.LastIndex(string(runes[:cut]), "\n")
			if lastNL > maxLen/2 {
				cut = lastNL + 1
			}
		}
		chunks = append(chunks, string(runes[:cut]))
		runes = runes[cut:]
	}
	return chunks
}
