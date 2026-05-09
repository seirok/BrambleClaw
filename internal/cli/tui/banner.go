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

	g := lipgloss.NewStyle().Foreground(lipgloss.Color("#7d7d7d")) // 灰毛
	e := lipgloss.NewStyle().Foreground(lipgloss.Color("#4fc3f7")) // 亮蓝眼
	n := lipgloss.NewStyle().Foreground(lipgloss.Color("#f06292")) // 粉鼻
	w := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")) // 白胡子
	s := "  "

	lines := []string{
		s + s + g.Render("█ ") + s + s + s + g.Render(" █") + s + s, // 尖耳朵
		s + g.Render("█████") + s + g.Render("█████") + s,           // 耳根
		g.Render("██████████████"),                                  // 额头
		// 眼睛
		g.Render("██") + e.Render("██") + g.Render("██████") + e.Render("██") + g.Render("██"),
		g.Render("██████████████"),                                                                 // 脸颊
		s + g.Render("██") + w.Render("██") + n.Render("██") + w.Render("██") + g.Render("██") + s, // 鼻子
		s + s + w.Render("██████") + s + s,                                                         // 小嘴
		s + s + g.Render("██████") + s + s,                                                         // 下巴
	}
	pet := lipgloss.NewStyle().Foreground(furColor).Render(strings.Join(lines, "\n"))

	welcomeMsg := lipgloss.NewStyle().Bold(true).Render("Welcome to Brambleclaw.")

	clanInfo := lipgloss.NewStyle().Foreground(subtleGray).Faint(true).Render(
		"ThunderClan Edition • Warrior Code v1.0\n" +
			"~/GolandProjects/brambleclaw",
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
			titleStyle.Render("What's new in the Forest"),
			"",
			listStyle.Render("• Fixed viewport scrolling in Thinking Events"),
			listStyle.Render("• Integrated Claude-style minimalist interface"),
			listStyle.Render("• Enhanced Tab-navigation for three-pane layout"),
			listStyle.Render("• Warrior code optimization for faster inference"),
			"",
			lipgloss.NewStyle().Foreground(subtleGray).Italic(true).Render("May the StarClan light your path."),
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
