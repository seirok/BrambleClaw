package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m appModel) View() string {
	if m.quitting {
		return "Bye! 👋\n"
	}

	switch m.mode {
	case modeResume:
		return m.viewResume()
	case modeDelete:
		return m.viewDelete()
	case modeDeleteConfirm:
		return m.viewDeleteConfirm()
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

	// chat box - 注意：不在这里 SetContent，只使用 viewport.View()
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

	chatBox := chatStyle.Render(m.viewport.View())

	// 思考区样式 - 标题在 viewport 外部，不随内容滚动
	eventStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(mainWidth-2).
		Padding(0, 1)

	if m.focus == focusEvent {
		eventStyle = eventStyle.BorderForeground(activeColor)
	} else {
		eventStyle = eventStyle.BorderForeground(inactiveColor)
	}

	// 思考区标题处理 - 放在 viewport 外部，不随内容滚动
	eventTitleStyle := lipgloss.NewStyle().Foreground(thinkingBlue)
	if m.focus == focusEvent {
		eventTitleStyle = eventTitleStyle.Bold(true)
	}
	eventTitle := eventTitleStyle.Render(" 🧠 [Thinking Events]")

	// 渲染思考区内容（标题 + viewport 内容）
	eventContent := lipgloss.JoinVertical(
		lipgloss.Left,
		eventTitle,
		m.eventViewport.View(),
	)
	// 注意：不给 eventStyle 设置固定 Height，让内容自然流动
	eventBox := eventStyle.Render(eventContent)

	// 左侧纵向拼接
	leftPanel := lipgloss.JoinVertical(lipgloss.Left, chatBox, eventBox)

	// --- 3. 侧边栏渲染 ---
	var rightPanel string
	if m.sidebarEnabled {
		leftHeight := lipgloss.Height(leftPanel)
		sidebarStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(inactiveColor).
			Padding(0, 1).
			Width(sidebarWidth - 2)

		rightPanel = sidebarStyle.Height(leftHeight - 2).Render(m.renderSidebar())
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

	// 组装最终布局 - 不使用 lipgloss.Place()，避免额外的尺寸约束
	var finalMainLayout string
	if m.sidebarEnabled {
		finalMainLayout = lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
	} else {
		finalMainLayout = leftPanel
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		finalMainLayout,
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
	//	wrapWidth := 10
	//	msgWrapStyle := lipgloss.NewStyle().Width(wrapWidth).
	for _, msg := range m.messages {
		var line string
		if msg.IsUser {
			line = userStyle.Render("You: "+msg.Content) + "\n"
		} else if msg.IsError {
			line = errorStyle.Render("! "+msg.Content) + "\n"
		} else {
			iconStr := agentStyle.Render("🕶 : ")
			line = iconStr + agentStyle.Render(msg.Content) + "\n"
		}
		messagesView += line + "\n"
	}

	if m.mode == modeWaiting {
		iconStr := "🕶 "
		messagesView += m.spinner.View() + iconStr + " 正在思考...\n"
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

// viewResume renders the resume session list
func (m appModel) viewResume() string {
	// ---颜色与宽度分配---
	activeColor := lipgloss.Color("86")    // 亮青色
	inactiveColor := lipgloss.Color("248") // 调亮后的灰色边框
	sidebarWidth := 32
	mainWidth := m.width
	if m.sidebarEnabled {
		mainWidth = m.width - sidebarWidth
	}

	// 渲染左侧面板 (Resume List)
	resumeStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(activeColor).
		Padding(0, 1)

	// Get available height
	availableHeight := m.viewport.Height + m.eventViewport.Height + 1
	resumeStyle.Height(availableHeight)

	resumeBox := resumeStyle.Width(mainWidth - 2).Render(m.resumeList.View())

	// --- 3. 侧边栏渲染 ---
	var rightPanel string
	if m.sidebarEnabled {
		leftHeight := lipgloss.Height(resumeBox)
		sidebarStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(inactiveColor).
			Padding(0, 1).
			Width(sidebarWidth - 2).
			Height(leftHeight - 2)

		rightPanel = sidebarStyle.Render(m.renderSidebar())
	}

	// 底部提示区
	hintStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, true, false).
		BorderForeground(inactiveColor).
		Padding(0, 0, 0, 1).
		Width(m.width).
		Foreground(lipgloss.Color("240"))
	hintBox := hintStyle.Render("↑↓ 导航 | Enter 选择 | Esc 返回")

	// 组装最终布局
	mainLayout := lipgloss.JoinHorizontal(lipgloss.Top, resumeBox, rightPanel)

	return lipgloss.JoinVertical(lipgloss.Left,
		mainLayout,
		hintBox,
	)
}
