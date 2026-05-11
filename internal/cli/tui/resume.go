package tui

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/agent"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

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
func (m appModel) initResumeList() (appModel, tea.Cmd, error) {
	if m.sessionManager == nil {
		return m, nil, fmt.Errorf("session manager not available")
	}

	ctx := context.Background()
	allMetas, err := m.sessionManager.LoadAllMetadata(ctx)
	if err != nil {
		return m, nil, err
	}

	logger.L().Debug().Int("total_meta_count", len(allMetas)).Str("agent_name", m.agentName).Msg("Loading session metadata")

	// Filter to current agent
	var items []list.Item
	for _, meta := range allMetas {
		if meta.AgentName == m.agentName {
			title := meta.FirstUserMessage
			if title == "" {
				// Try to load session to get first user message
				sessionKey := fmt.Sprintf("cli::%s::%s", m.agentName, meta.ChatID)
				sess, err := m.sessionManager.LoadSession(context.Background(), sessionKey)
				if err == nil && sess != nil {
					// Look for first user message in session
					for _, msg := range sess.Messages {
						if msg.GetSource() == "user" {
							title = truncateWithEllipsis(msg.ToText(), 80)
							break
						}
					}
				}
				if title == "" {
					title = "[No user messages yet]"
				}
			}
			items = append(items, sessionItem{
				chatID:         meta.ChatID,
				firstUserMsg:   title,
				lastActiveTime: meta.UpdatedAt,
			})
		}
	}

	logger.L().Debug().Int("item_count", len(items)).Msg("Session list initialized")

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
	newList := list.New(items, listDelegate, width, height)
	newList.Title = "Resume Session - Select a conversation"
	newList.SetShowStatusBar(true)
	newList.SetFilteringEnabled(true)

	m.resumeList = newList
	var cmd tea.Cmd
	m.resumeList, cmd = m.resumeList.Update(nil)

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
					return m.switchSession(item.chatID), nil
				}
			}
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
	if m.sessionManager == nil {
		return m
	}

	ctx := context.Background()

	// Build session key
	sessionKey := util.BuildSessionKey(m.agentName, interfaces.CliChannelName, chatID)

	// Load session
	sess, err := m.sessionManager.LoadSession(ctx, sessionKey)
	if err != nil {
		m.err = err
		return m
	}

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

	// Update model
	m.messages = chatMsgs
	m.currentChatID = chatID
	m.mode = modeInput
	m.showBanner = false

	// Update viewport - reset Y position first to avoid border issues
	m.viewport.YOffset = 0
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	return m
}
