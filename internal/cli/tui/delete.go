package tui

import (
	"context"
	util "neoclaw/internal"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/logger"
	"neoclaw/internal/store"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// updateDelete handles key messages in delete mode
func (m appModel) updateDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			selectedItem := m.resumeList.SelectedItem()
			if selectedItem != nil {
				if item, ok := selectedItem.(sessionItem); ok {
					m.pendingDeleteItem = &item
					m.mode = modeDeleteConfirm
					return m, nil
				}
			}
			return m, nil
		case tea.KeyEsc:
			m.mode = modeInput
			m = m.refreshSuggestions()
			return m, nil
		}
	case tea.WindowSizeMsg:
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

	m.resumeList, cmd = m.resumeList.Update(msg)
	return m, cmd
}

// updateDeleteConfirm handles key messages in delete confirm mode
func (m appModel) updateDeleteConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			m.pendingDeleteItem = nil
			m.mode = modeDelete
			return m, nil
		default:
			switch msg.String() {
			case "y", "Y":
				if m.pendingDeleteItem != nil {
					err := m.deleteSession(m.pendingDeleteItem.chatID)
					if err != nil {
						m.messages = append(m.messages, chatMessage{
							Content:   "Failed to delete session: " + err.Error(),
							IsUser:    false,
							IsError:   true,
							Timestamp: time.Now(),
						})
					} else {
						m.messages = append(m.messages, chatMessage{
							Content:   "Session deleted: " + m.pendingDeleteItem.firstUserMsg,
							IsUser:    false,
							IsError:   false,
							Timestamp: time.Now(),
						})
					}
					m.pendingDeleteItem = nil
				}
				m.mode = modeInput
				m = m.refreshSuggestions()
				m.viewport.SetContent(m.renderMessages())
				m.viewport.GotoBottom()
				return m, nil
			case "n", "N":
				m.pendingDeleteItem = nil
				m.mode = modeDelete
				return m, nil
			}
		}
	}
	return m, nil
}

// deleteSession deletes the session files from disk
func (m appModel) deleteSession(chatID string) error {
	ctx := context.Background()
	sessionKey := util.BuildSessionKey(m.agent.Name(), interfaces.CliChannelName, chatID)
	fileKey := util.SessionKeyToFile(sessionKey)

	// Delete .jsonl file
	sessionStore := store.NewFileStorage[struct{}](filepath.Join(m.agent.Workspace(), "memory"))
	sessionFile := util.GetSessionFile(fileKey)
	if err := sessionStore.Delete(ctx, sessionFile); err != nil {
		logger.L().Error().Err(err).Str("file", sessionFile).Msg("Failed to delete session file")
		return err
	}

	// Delete .meta.json file
	metaStore := store.NewFileStorage[struct{}](filepath.Join(m.agent.Workspace(), "memory", "meta_data"))
	metaFile := util.GetSessionMetaFile(fileKey)
	if err := metaStore.Delete(ctx, metaFile); err != nil {
		logger.L().Error().Err(err).Str("file", metaFile).Msg("Failed to delete session meta file")
		return err
	}

	logger.L().Info().Str("chat_id", chatID).Msg("Session deleted successfully")
	return nil
}

// viewDelete renders the delete session list
func (m appModel) viewDelete() string {
	activeColor := lipgloss.Color("196") // Red
	inactiveColor := lipgloss.Color("248")
	sidebarWidth := 32
	mainWidth := m.width
	if m.sidebarEnabled {
		mainWidth = m.width - sidebarWidth
	}

	deleteStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(activeColor).
		Padding(0, 1)

	availableHeight := m.viewport.Height + m.eventViewport.Height + 1
	deleteStyle.Height(availableHeight)

	deleteBox := deleteStyle.Width(mainWidth - 2).Render(m.resumeList.View())

	var rightPanel string
	if m.sidebarEnabled {
		leftHeight := lipgloss.Height(deleteBox)
		sidebarStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(inactiveColor).
			Padding(0, 1).
			Width(sidebarWidth - 2).
			Height(leftHeight - 2)
		rightPanel = sidebarStyle.Render(m.renderSidebar())
	}

	hintStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, true, false).
		BorderForeground(inactiveColor).
		Padding(0, 0, 0, 1).
		Width(m.width).
		Foreground(lipgloss.Color("240"))
	hintBox := hintStyle.Render("Navigate: Up/Down | Select: Enter | Cancel: Esc")

	mainLayout := lipgloss.JoinHorizontal(lipgloss.Top, deleteBox, rightPanel)

	return lipgloss.JoinVertical(lipgloss.Left, mainLayout, hintBox)
}

// viewDeleteConfirm renders the delete confirmation prompt
func (m appModel) viewDeleteConfirm() string {
	activeColor := lipgloss.Color("196") // Red
	inactiveColor := lipgloss.Color("248")
	sidebarWidth := 32
	mainWidth := m.width
	if m.sidebarEnabled {
		mainWidth = m.width - sidebarWidth
	}

	title := "Confirm Deletion"
	if m.pendingDeleteItem != nil {
		title = "Delete \"" + m.pendingDeleteItem.firstUserMsg + "\"?"
	}

	confirmStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(activeColor).
		Padding(1, 2).
		Width(mainWidth - 4)

	availableHeight := m.viewport.Height + m.eventViewport.Height + 1
	confirmStyle.Height(availableHeight)

	confirmBox := confirmStyle.Render(
		lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(title) +
			"\n\nThis action cannot be undone.\n\n" +
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Confirm: y | Cancel: n / Esc"),
	)

	var rightPanel string
	if m.sidebarEnabled {
		leftHeight := lipgloss.Height(confirmBox)
		sidebarStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(inactiveColor).
			Padding(0, 1).
			Width(sidebarWidth - 2).
			Height(leftHeight - 2)
		rightPanel = sidebarStyle.Render(m.renderSidebar())
	}

	hintStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, true, false).
		BorderForeground(inactiveColor).
		Padding(0, 0, 0, 1).
		Width(m.width).
		Foreground(lipgloss.Color("240"))
	hintBox := hintStyle.Render("Confirm: y | Cancel: n / Esc")

	mainLayout := lipgloss.JoinHorizontal(lipgloss.Top, confirmBox, rightPanel)

	return lipgloss.JoinVertical(lipgloss.Left, mainLayout, hintBox)
}
