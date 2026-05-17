package channel

import (
	"context"
	"fmt"
	"neoclaw/internal/bus"
	"neoclaw/internal/config/structs"
	"neoclaw/internal/logger"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

const discordMaxMessageLength = 2000

var _ BaseChannel = (*DiscordChannel)(nil)

type DiscordChannel struct {
	base    *BaseChannelImpl
	config  structs.DiscordConfig
	session *discordgo.Session
	running bool
	mu      sync.Mutex
	cancel  context.CancelFunc
}

func (c *DiscordChannel) Name() string {
	return c.base.Name()
}

func (c *DiscordChannel) Start(ctx context.Context) error {
	if c.config.BotToken == "" {
		return fmt.Errorf("discord bot_token is empty")
	}

	var err error
	c.session, err = discordgo.New("Bot " + c.config.BotToken)
	if err != nil {
		return fmt.Errorf("discordgo new: %w", err)
	}

	c.session.AddHandler(c.handleMessageCreate)
	c.session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent

	runCtx, runCancel := context.WithCancel(ctx)
	c.cancel = runCancel
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	go func() {
		if err := c.session.Open(); err != nil {
			logger.L().Error().Err(err).Msg("failed to start discord session")
		}
	}()

	go func() {
		<-runCtx.Done()
		_ = c.Stop(context.Background())
	}()

	return nil
}

func (c *DiscordChannel) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return nil
	}
	c.running = false
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if c.session != nil {
		_ = c.session.Close()
		c.session = nil
	}
	return nil
}

func (c *DiscordChannel) Send(ctx context.Context, message *bus.OutBoundMessage) error {
	if message.ChatID == "" {
		return fmt.Errorf("chat ID is empty")
	}
	if c.session == nil {
		return fmt.Errorf("discord session not initialized")
	}

	content := message.Content
	if content == "" {
		return nil
	}

	chunks := splitMessage(content, discordMaxMessageLength)
	for i, chunk := range chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_, err := c.session.ChannelMessageSend(message.ChatID, chunk)
		if err != nil {
			logger.L().Error().Err(err).Int("chunk", i).Str("chat_id", message.ChatID).Msg("failed to send discord message chunk")
			return fmt.Errorf("discord send message: %w", err)
		}
		if i < len(chunks)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
}

func (c *DiscordChannel) IsAllowed(id string) bool {
	return c.base.IsAllowed(id)
}

func NewDiscordChannel(cfg structs.DiscordConfig, bus *bus.MessageBus) (*DiscordChannel, error) {
	baseCfg := &BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowFrom,
	}
	base := NewBaseChannelImpl("discord", baseCfg, bus)
	ch := &DiscordChannel{
		base:   base,
		config: cfg,
	}
	return ch, nil
}

func (c *DiscordChannel) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.ID == "" || m.Author.Bot {
		return
	}

	content := strings.TrimSpace(m.Content)
	if content == "" {
		return
	}

	senderID := m.Author.ID
	chatID := m.ChannelID

	isDM := m.GuildID == ""
	if !isDM {
		if !c.shouldProcessGuildMessage(s, m.Message) {
			return
		}
		content = stripBotMention(content, s.State.User.ID)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	if !c.IsAllowed(senderID) {
		logger.L().Debug().Str("sender_id", senderID).Msg("discord: sender not allowed, ignoring message")
		return
	}

	inboundMsg := &bus.InBoundMessage{
		InChannel: "discord",
		SenderID:  senderID,
		ChatID:    chatID,
		Content:   content,
		TimeStamp: time.Now(),
		Metadata: map[string]string{
			"message_id": m.ID,
		},
	}
	if m.GuildID != "" {
		inboundMsg.Metadata["guild_id"] = m.GuildID
	}

	if err := c.base.PublishInBoundMessage(context.Background(), inboundMsg); err != nil {
		logger.L().Error().Err(err).Msg("discord: failed to publish inbound message")
	}
}

func (c *DiscordChannel) shouldProcessGuildMessage(s *discordgo.Session, m *discordgo.Message) bool {
	if c.config.GroupTrigger.MentionOnly || len(c.config.GroupTrigger.Prefixes) == 0 {
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				return true
			}
		}
		return false
	}

	for _, mention := range m.Mentions {
		if mention.ID == s.State.User.ID {
			return true
		}
	}

	for _, prefix := range c.config.GroupTrigger.Prefixes {
		if strings.HasPrefix(m.Content, prefix) {
			return true
		}
	}

	return false
}

func stripBotMention(content, botID string) string {
	mentionPattern1 := "<@" + botID + ">"
	mentionPattern2 := "<@!" + botID + ">"
	if strings.HasPrefix(content, mentionPattern1) {
		return strings.TrimPrefix(content, mentionPattern1)
	}
	if strings.HasPrefix(content, mentionPattern2) {
		return strings.TrimPrefix(content, mentionPattern2)
	}
	return content
}

func splitMessage(content string, maxLen int) []string {
	if len(content) <= maxLen {
		return []string{content}
	}

	var chunks []string
	lines := strings.Split(content, "\n")
	current := ""
	for _, line := range lines {
		if current == "" {
			if len(line) <= maxLen {
				current = line
			} else {
				for i := 0; i < len(line); i += maxLen {
					end := i + maxLen
					if end > len(line) {
						end = len(line)
					}
					chunks = append(chunks, line[i:end])
				}
			}
			continue
		}

		candidate := current + "\n" + line
		if len(candidate) <= maxLen {
			current = candidate
		} else {
			chunks = append(chunks, current)
			if len(line) <= maxLen {
				current = line
			} else {
				for i := 0; i < len(line); i += maxLen {
					end := i + maxLen
					if end > len(line) {
						end = len(line)
					}
					chunks = append(chunks, line[i:end])
				}
				current = ""
			}
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}
	return chunks
}
