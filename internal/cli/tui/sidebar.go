package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderSidebar 渲染侧边栏
func (m appModel) renderSidebar() string {
	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true).
		MarginLeft(1)
	sb.WriteString(titleStyle.Render("📊 Statistics"))
	sb.WriteString("\n\n")
	sectionTitleStyle := lipgloss.NewStyle().Bold(true)
	contentStyle := lipgloss.NewStyle().
		PaddingLeft(3).
		Foreground(lipgloss.Color("245"))

	for _, section := range m.sidebarSections {
		if !section.Enabled {
			continue
		}

		switch section.Name {
		case "token_usage":
			m.renderTokenUsage(&sb, sectionTitleStyle, contentStyle)
		case "hook_stats":
			m.renderHookStats(&sb, sectionTitleStyle, contentStyle)
		case "model_info":
			m.renderModelInfo(&sb, sectionTitleStyle, contentStyle)
		case "sandbox":
			m.renderSandboxStats(&sb, sectionTitleStyle, contentStyle)
		case "session":
			m.renderSessionStats(&sb, sectionTitleStyle, contentStyle)
		case "mcp":
			m.renderMCPStats(&sb, sectionTitleStyle, contentStyle)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m appModel) renderTokenUsage(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("207")).
		Bold(true).
		Render("🗳️ Token Usage")
	sb.WriteString(sectionTitle + "\n")

	promptStr := fmt.Sprintf("%-11s %5d", "Prompt:", m.sidebarStats.PromptTokens)
	completionStr := fmt.Sprintf("%-11s %5d", "Completion:", m.sidebarStats.CompletionTokens)
	totalStr := fmt.Sprintf("%-11s %5d", "Total:", m.sidebarStats.TotalTokens)

	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	sb.WriteString(statStyle.Render(promptStr) + "\n")
	sb.WriteString(statStyle.Render(completionStr) + "\n")
	sb.WriteString(statStyle.Render(totalStr) + "\n")
}

func (m appModel) renderHookStats(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("69")).
		Render("🔗 Hook Stats")
	sb.WriteString(sectionTitle + "\n")

	categories := []string{"LLM", "TOOL", "AGENT", "MESSAGE", "SANDBOX"}
	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	for _, cat := range categories {
		count := m.sidebarStats.HookCounts[cat]
		errors := m.sidebarStats.HookErrors[cat]
		var line string
		if errors > 0 {
			line = fmt.Sprintf("%-7s: %3d (%2d err)", cat, count, errors)
		} else {
			line = fmt.Sprintf("%-7s: %3d", cat, count)
		}
		sb.WriteString(statStyle.Render(line) + "\n")
	}
}

func (m appModel) renderModelInfo(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("78")).
		Render("🤖 Model Info")
	sb.WriteString(sectionTitle + "\n")

	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	sb.WriteString(statStyle.Render(fmt.Sprintf("Model: %s", m.sidebarStats.ModelName)) + "\n")
	sb.WriteString(statStyle.Render(fmt.Sprintf("Agent: %s", "main")) + "\n")
}

func (m appModel) renderSandboxStats(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("135")).
		Render("📁 Sandbox")
	sb.WriteString(sectionTitle + "\n")

	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	sb.WriteString(statStyle.Render(fmt.Sprintf("File Ops: %5d", m.sidebarStats.FileOps)) + "\n")
	sb.WriteString(statStyle.Render(fmt.Sprintf("Commands: %5d", m.sidebarStats.CmdExecs)) + "\n")
	sb.WriteString(statStyle.Render(fmt.Sprintf("Blocked:  %5d", m.sidebarStats.BlockedOps)) + "\n")
}

func (m appModel) renderSessionStats(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("214")).
		Render("💬 Session")
	sb.WriteString(sectionTitle + "\n")

	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	sb.WriteString(statStyle.Render(fmt.Sprintf("Messages: %3d", m.sidebarStats.MessageCount)) + "\n")
}

func (m appModel) renderMCPStats(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("141")).
		Render("🔌 MCP")
	sb.WriteString(sectionTitle + "\n")

	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	sb.WriteString(statStyle.Render(fmt.Sprintf("Clients: %2d", m.sidebarStats.MCPClientCount)) + "\n")
}
