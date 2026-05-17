package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	claudeOrange = lipgloss.Color("#D97757") // Claude 标志性的橘粉色
	subtleGray   = lipgloss.Color("240")
)

func renderClaudeBanner(width int) string {
	furColor := lipgloss.Color("#8B4513")
	matrixGreen := lipgloss.Color("#00FF41") // 经典矩阵绿

	h := lipgloss.NewStyle().Foreground(lipgloss.Color("#1a1a1a")) // 头发/大墨镜/风衣（纯黑/深灰）
	//	s_skin := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffdbb5")) // 皮肤颜色
	m := lipgloss.NewStyle().Foreground(matrixGreen) // 墨镜反光/科技绿
	s := "  "                                        // 间距补白

	// 纯墨镜 Logo：14 字符宽，完美对齐
	lines := []string{
		s + s + h.Render("██████") + s + s,              // 1. 镜框上边缘连线
		h.Render("██████") + s + s + h.Render("██████"), // 2. 镜片上部（加宽）
		h.Render("██") + m.Render("██") + h.Render("██") + h.Render("██") + h.Render("██") + m.Render("██") + h.Render("██"), // 3. 核心：带绿色反光的镜片主体 + 鼻梁
		s + h.Render("████") + s + s + s + s + h.Render("████") + s,                                                          // 4. 镜片下沿收束
		s + s + h.Render("██") + s + s + s + s + h.Render("██") + s + s,                                                      // 5. 镜片最底部尖角
	}

	pet := lipgloss.NewStyle().Foreground(furColor).Render(strings.Join(lines, "\n"))

	welcomeMsg := lipgloss.NewStyle().Bold(true).Render("Welcome to Neoclaw.")

	clanInfo := lipgloss.NewStyle().Foreground(subtleGray).Faint(true).Render(
		"ThunderClan Edition • Warrior Code v1.0\n" +
			"~/GolandProjects/neoclaw",
	)

	leftContent := lipgloss.JoinVertical(lipgloss.Center, welcomeMsg, "", pet)
	leftBox := lipgloss.JoinVertical(lipgloss.Left, leftContent, "", clanInfo)

	// 中间分割线
	divider := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 2).
		Height(9).
		Render("")

	// 右侧：What's new
	titleStyle := lipgloss.NewStyle().Foreground(claudeOrange).Bold(true)
	listStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))

	rightStyle := lipgloss.NewStyle().MarginLeft(3)

	rightView := rightStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("What's new in the Matrix"),
			"",
			listStyle.Render("• Fixed viewport scrolling in Thinking Events"),
			listStyle.Render("• Integrated Claude-style minimalist interface"),
			listStyle.Render("• Enhanced Tab-navigation for three-pane layout"),
			listStyle.Render("• Warrior code optimization for faster inference"),
			"",
			lipgloss.NewStyle().Foreground(subtleGray).Italic(true).Render("Take the blue pill, or red?"),
		),
	)

	// 组合左右
	bannerBody := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(35).Align(lipgloss.Center).Render(leftBox),
		divider,
		rightView,
	)

	return lipgloss.NewStyle().
		Padding(1, 2).
		Width(width - 4).
		Render(bannerBody)
}
