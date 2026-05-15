package tui

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/agent"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/store"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"brambleclaw/internal/session"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

const maxReserveLen = 40

func truncateWithEllipsis(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// sessionItem represents a session in the list
type sessionItem struct {
	chatID         string
	firstUserMsg   string
	lastActiveTime time.Time
}

func (i sessionItem) Title() string {
	if len(i.firstUserMsg) > 80 {
		return i.firstUserMsg[:80] + "..."
	}
	return i.firstUserMsg
}

func (i sessionItem) Description() string {
	return i.lastActiveTime.Format("2006-01-02 15:04")
}

func (i sessionItem) FilterValue() string {
	return i.firstUserMsg
}

// initResumeList initializes the resume list with session metadata
func (m appModel) initSessionList(title string) (appModel, tea.Cmd, error) {
	// 加载所有session meta 信息
	ctx := context.Background()
	metaPath := filepath.Join(m.agent.Workspace(), "memory", "meta_data")
	metaStore := store.NewFileStorage[session.SessionMetadata](metaPath)
	metas := session.NewSessionMetadataRegistry()
	entries, err := os.ReadDir(metaPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return m, nil, fmt.Errorf("failed to read directory: %w", err)
		}
		// Directory doesn't exist yet — treat as empty, still initialize the list
		entries = nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			meta, err := metaStore.Load(ctx, entry.Name())
			if err != nil {
				logger.L().Error().Err(err).Str("file", entry.Name()).Msg("failed to load session metadata")
				continue
			}

			err = metas.Register(ctx, entry.Name(), meta)
			if err != nil {
				logger.L().Err(err).Str("file", entry.Name()).Msg("failed to register session metadata")
				continue
			}

		}
	}

	logger.L().Debug().Int("total_meta_count", metas.Len()).Str("agent_name", m.agent.Name()).Msg("Loading session metadata")

	// resume list item
	var items []list.Item
	for _, meta := range metas.List(ctx) {
		title := meta.FirstUserMessage
		if title == "" {
			title = "[Empty Session]"
		}

		title = truncateWithEllipsis(title, maxReserveLen)
		items = append(items, sessionItem{
			chatID:         meta.ChatID,
			firstUserMsg:   title,
			lastActiveTime: meta.UpdatedAt,
		})
	}

	logger.L().Debug().Int("item_count", len(items)).Msg("Session resume list initialized")

	// Calculate dimensions
	width := m.viewport.Width + 2 // Account for border
	height := m.viewport.Height + m.eventViewport.Height + 1

	if width < 20 {
		width = 20
	}
	if height < 10 {
		height = 10
	}

	listDelegate := list.NewDefaultDelegate()
	newList := list.New(items, &listDelegate, width, height) // 传递指针而不是值
	newList.Title = title
	newList.SetShowStatusBar(true)
	newList.SetFilteringEnabled(true)

	m.resumeList = newList
	var cmd tea.Cmd
	// 跳过 Update(nil) 调用，防止 delegate 接口变为 nil
	_ = cmd

	return m, cmd, nil
}

// updateResume handles key messages in resume mode
func (m appModel) updateResume(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			// Select session - handle before list.Update consumes it
			selectedItem := m.resumeList.SelectedItem()
			if selectedItem != nil {
				if item, ok := selectedItem.(sessionItem); ok {
					logger.L().Debug().Str("chat_id", item.chatID).Msg("Selected session, switching...")
					result := m.switchSession(item.chatID)
					if am, ok := result.(appModel); ok && am.err != nil {
						m.err = am.err
						return m, nil
					}
					return result, nil
				}
				logger.L().Debug().Msg("Selected item type assertion failed")
			} else {
				logger.L().Debug().Msg("No item selected or list is empty")
			}
			return m, nil
		case tea.KeyEsc:
			// Cancel and return
			m.mode = modeInput
			return m, nil
		}
	case tea.WindowSizeMsg:
		// Update list dimensions
		width := m.viewport.Width + 2
		height := m.viewport.Height + m.eventViewport.Height + 1

		if width < 20 {
			width = 20
		}
		if height < 10 {
			height = 10
		}
		m.resumeList.SetSize(width, height)
	}

	// Let list handle other keys (up/down navigation)
	m.resumeList, cmd = m.resumeList.Update(msg)
	return m, cmd
}

// switchSession loads a session and updates the UI
func (m appModel) switchSession(chatID string) tea.Model {
	ctx := context.Background()
	// Build session key
	sessionKey := util.BuildSessionKey(m.agent.Name(), interfaces.CliChannelName, chatID)

	// Load session
	sess, err := m.agent.LoadSession(ctx, sessionKey)
	if err != nil {
		logger.L().Error().Err(err).Str("session key", sessionKey).Msg("failed to load session")
		m.err = err
		return m
	}
	m.agent.SetSession(sess)

	// Convert session messages to chatMessage format
	var chatMsgs []chatMessage
	for _, msg := range sess.Messages {
		isUser := false
		// Check if it's an AgentMessage and look at Role field
		if agentMsg, ok := msg.(*agent.AgentMessage); ok {
			isUser = agentMsg.Role == agent.RoleUser
		} else {
			// Fallback for other message types
			isUser = msg.GetSource() == "user"
		}

		// Skip system messages
		if agentMsg, ok := msg.(*agent.AgentMessage); ok && agentMsg.Role == agent.RoleSystem {
			continue
		}

		chatMsg := chatMessage{
			Content:   msg.ToText(),
			IsUser:    isUser,
			Timestamp: msg.GetCreatedAt(),
		}
		chatMsgs = append(chatMsgs, chatMsg)
	}
	logger.L().Debug().Str("session", sess.Name()).Msg("history session has been reloaded.")

	// Restore sidebar stats from session metadata
	meta := sess.GetMetadata()
	if meta != nil {
		m.sidebarStats.TotalTokens = meta.TokenCount
		m.sidebarStats.MessageCount = meta.MessageCount
		m.sidebarStats.AgentName = meta.AgentName
		m.sidebarStats.SessionAge = time.Since(meta.CreatedAt)
		m.sidebarStats.Summarized = sess.Summarized
	}

	// Update model
	m.messages = chatMsgs
	m.mode = modeInput
	m.showBanner = false

	// Update viewport - reset Y position first to avoid border issues
	m.viewport.YOffset = 0
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	// Refresh sidebar immediately with restored stats
	if m.sidebarEnabled {
		m.sidebarViewport.SetContent(m.renderSidebar())
	}

	return m
}
