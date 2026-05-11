package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m appModel) View() string {
	if m.quitting {
		return "Bye! 👋\n"
	}

	// ---颜色与宽度分配---
	activeColor := lipgloss.Color("86")    // 亮青色
	inactiveColor := lipgloss.Color("248") // 调亮后的灰色边框
	thinkingBlue := lipgloss.Color("33")   // 蓝色思考区标题
	sidebarWidth := 32
	mainWidth := m.width
	if m.sidebarEnabled {
		mainWidth = m.width - sidebarWidth
	}

	// chat box
	chatStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(mainWidth-2).
		Height(m.viewport.Height).
		Padding(0, 1)

	if m.focus == focusChat {
		chatStyle = chatStyle.BorderForeground(activeColor)
	} else {
		chatStyle = chatStyle.BorderForeground(inactiveColor)
	}

	m.viewport.SetContent(m.renderMessages())
	chatBox := chatStyle.Render(m.viewport.View())
	// 	chatBox := chatStyle.Render(m.renderMessages())

	// 思考区样式
	eventStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(mainWidth-2).
		Height(m.eventViewport.Height).
		Padding(0, 1)

	if m.focus == focusEvent {
		eventStyle = eventStyle.BorderForeground(activeColor)
	} else {
		eventStyle = eventStyle.BorderForeground(inactiveColor)
	}

	// 思考区标题处理
	eventTitleStyle := lipgloss.NewStyle().Foreground(thinkingBlue)
	if m.focus == focusEvent {
		eventTitleStyle = eventTitleStyle.Bold(true)
	}
	eventTitle := eventTitleStyle.Render(" 🧠 [Thinking Events]")

	eventContent := lipgloss.JoinVertical(lipgloss.Left, eventTitle, m.renderEvents())
	eventBox := eventStyle.Render(eventContent)

	// 左侧纵向拼接
	leftPanel := lipgloss.JoinVertical(lipgloss.Left, chatBox, eventBox)

	// --- 3. 侧边栏对齐渲染 ---
	var rightPanel string
	if m.sidebarEnabled {
		leftHeight := lipgloss.Height(leftPanel)
		sidebarStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(inactiveColor).
			Padding(0, 1).
			Width(sidebarWidth - 2).
			Height(leftHeight - 2)

		rightPanel = sidebarStyle.Render(m.renderSidebar())
	}

	// 底部输入区
	inputContainerStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, true, false).
		BorderForeground(inactiveColor).
		Padding(0, 0, 0, 1).
		Width(m.width)

	if m.focus == focusInput {
		inputContainerStyle = inputContainerStyle.BorderForeground(activeColor)
		m.textInput.PromptStyle = lipgloss.NewStyle().Foreground(activeColor)
	} else {
		m.textInput.PromptStyle = lipgloss.NewStyle().Foreground(inactiveColor)
	}

	inputBox := inputContainerStyle.Render(m.textInput.View())

	helpView := m.help.View(keys)

	// 组装最终布局
	mainLayout := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	return lipgloss.JoinVertical(lipgloss.Left,
		mainLayout,
		inputBox,
		helpView,
	)
}

// renderMessages 渲染消息列表为字符串
func (m appModel) renderMessages() string {
	var messagesView string

	if m.showBanner && len(m.messages) == 0 {
		messagesView = renderClaudeBanner(m.viewport.Width) + "\n\n"
	}

	for _, msg := range m.messages {
		var line string
		if msg.IsUser {
			line = userStyle.Render("You: "+msg.Content) + "\n"
		} else if msg.IsError {
			line = errorStyle.Render("! "+msg.Content) + "\n"
		} else {
			line = agentStyle.Render("🐱: "+msg.Content) + "\n"
		}
		messagesView += line + "\n"
	}

	if m.waiting {
		messagesView += m.spinner.View() + " 🐱 正在思考...\n"
	}

	return messagesView
}

// renderEvents 渲染事件列表
func (m appModel) renderEvents() string {
	var sb strings.Builder
	for _, evt := range m.eventLog {
		style := getEventStyle(evt.Point)
		summary := formatEventSummary(evt)
		sb.WriteString(style.Render(summary))
		sb.WriteString("\n")
	}
	return sb.String()
}
