package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m appModel) View() string {
	if m.quitting {
		return "Bye! 👋\n"
	}

	// Show resume list if in resume mode
	if m.mode == modeResume {
		return m.viewResume()
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

	// 思考区样式 - 注意：不在这里设置 Height，让 viewport 管理内容高度
	eventStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(mainWidth-2).
		Padding(0, 1)

	if m.focus == focusEvent {
		eventStyle = eventStyle.BorderForeground(activeColor)
	} else {
		eventStyle = eventStyle.BorderForeground(inactiveColor)
	}

	// 思考区标题处理 - 设置到 viewport 内容中
	eventTitleStyle := lipgloss.NewStyle().Foreground(thinkingBlue)
	if m.focus == focusEvent {
		eventTitleStyle = eventTitleStyle.Bold(true)
	}
	eventTitle := eventTitleStyle.Render(" 🧠 [Thinking Events]")

	// 确保 viewport 有最新内容（包括标题）
	fullEventContent := lipgloss.JoinVertical(lipgloss.Left, eventTitle, m.renderEvents())
	m.eventViewport.SetContent(fullEventContent)

	// 使用 viewport.View() 来渲染，这样滚动才能工作
	eventBox := eventStyle.Height(m.eventViewport.Height + 2).Render(m.eventViewport.View())

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
	safeLeftPanel := lipgloss.Place(
		mainWidth,                  // 目标宽度
		lipgloss.Height(leftPanel), // 保持原有高度
		lipgloss.Left,              // 水平靠左
		lipgloss.Top,               // 垂直靠顶
		leftPanel,
	)
	var finalMainLayout string

	if m.sidebarEnabled {
		// 2. 约束右侧面板 (Sidebar)
		safeRightPanel := lipgloss.Place(
			sidebarWidth,
			lipgloss.Height(leftPanel), // 强制侧边栏与左侧对齐高度
			lipgloss.Left,
			lipgloss.Top,
			rightPanel,
		)

		// 3. 水平拼接两个经过“脱水处理”的安全面板
		finalMainLayout = lipgloss.JoinHorizontal(lipgloss.Top, safeLeftPanel, safeRightPanel)
	} else {
		finalMainLayout = safeLeftPanel
	}

	// mainLayout := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().MaxWidth(m.width).Render(finalMainLayout),
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

	// --- 3. 侧边栏对齐渲染 ---
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
