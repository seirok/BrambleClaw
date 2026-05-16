package command

import (
	"brambleclaw/internal/bus"
	"brambleclaw/internal/session"
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

const (
	barWidth = 40
	block    = "█"
)

// ContextCommand shows the context usage breakdown
type ContextCommand struct{}

// Name returns command name
func (c *ContextCommand) Name() string { return "context" }

// Description returns command description
func (c *ContextCommand) Description() string {
	return "Show current context usage breakdown"
}

// Usage returns usage info
func (c *ContextCommand) Usage() string { return "/context" }

// Execute runs the command
func (c *ContextCommand) Execute(ctx context.Context, agent any, msg any, args []string) error {
	type agentWithContext interface {
		GetSession() *session.Session
		Bus() *bus.MessageBus
		Name() string
	}

	a, ok := agent.(agentWithContext)
	if !ok {
		return fmt.Errorf("agent does not support GetSession")
	}

	m, ok := msg.(*bus.InBoundMessage)
	if !ok {
		return fmt.Errorf("invalid message type")
	}

	sess := a.GetSession()
	if sess == nil {
		return publishReply(ctx, agent, m, "No active session.")
	}

	messages := sess.Messages
	meta := sess.GetMetadata()

	// Count characters for each section
	var (
		systemPromptChrs     int
		compressedHistoryChrs int
		userHistoryChrs       int
		assistantHistoryChrs  int
		toolHistoryChrs       int
	)

	// Session summary from metadata
	sessionSummary := ""
	if meta != nil {
		sessionSummary = meta.SessionSummary
	}
	compressedHistoryChrs = utf8.RuneCountInString(sessionSummary)

	// System prompt (minus summary)
	if len(messages) > 0 {
		systemPromptText := messages[0].ToText()
		systemPromptChrs = max(0, utf8.RuneCountInString(systemPromptText)-compressedHistoryChrs)
	}

	// Active history from Summarized index onward
	type messageWithRole interface {
		GetRole() string
		ToText() string
	}

	summarized := max(0, min(sess.Summarized, len(messages)))

	for i := summarized; i < len(messages); i++ {
		msg := messages[i]
		am, ok := msg.(messageWithRole)
		if !ok {
			continue
		}
		role := am.GetRole()
		chrs := utf8.RuneCountInString(am.ToText())

		switch role {
		case "user":
			userHistoryChrs += chrs
		case "assistant":
			assistantHistoryChrs += chrs
		case "tool":
			toolHistoryChrs += chrs
		}
	}

	// Build the output
	reply := buildContextBar(
		systemPromptChrs,
		compressedHistoryChrs,
		userHistoryChrs,
		assistantHistoryChrs,
		toolHistoryChrs,
	)

	return publishReply(ctx, agent, m, reply)
}

func buildContextBar(system, compressed, user, assistant, tool int) string {
	type section struct {
		label string
		chars int
		color lipgloss.Color
	}

	sections := []section{
		{"System prompt", system, lipgloss.Color("86")},    // cyan
		{"History: compressed", compressed, lipgloss.Color("214")}, // orange
		{"History: user", user, lipgloss.Color("135")},    // purple
		{"History: assistant", assistant, lipgloss.Color("78")}, // green
		{"History: tool", tool, lipgloss.Color("220")},    // yellow
	}

	total := 0
	for _, s := range sections {
		total += s.chars
	}

	if total == 0 {
		return "No context usage yet."
	}

	var sb strings.Builder

	// Header
	fmt.Fprintf(&sb, "Context Usage: %s chars\n\n", formatNumber(total))

	// Calculate segment widths
	segmentWidths := make([]int, len(sections))
	usedWidth := 0
	for i, s := range sections {
		if s.chars > 0 {
			segmentWidths[i] = int(math.Round(float64(barWidth) * float64(s.chars) / float64(total)))
		} else {
			segmentWidths[i] = 0
		}
		usedWidth += segmentWidths[i]
	}

	// Adjust to exactly barWidth
	if usedWidth != barWidth && total > 0 {
		// Find the largest segment to add the difference to
		maxIdx := 0
		maxChars := sections[0].chars
		for i := 1; i < len(sections); i++ {
			if sections[i].chars > maxChars {
				maxIdx = i
				maxChars = sections[i].chars
			}
		}
		segmentWidths[maxIdx] += barWidth - usedWidth
	}

	// Build the bar
	sb.WriteString("[")
	for i, s := range sections {
		if segmentWidths[i] > 0 {
			style := lipgloss.NewStyle().Foreground(s.color)
			sb.WriteString(style.Render(strings.Repeat(block, segmentWidths[i])))
		}
	}
	// Fill remaining space with dim gray blocks if needed
	usedWidth = 0
	for _, w := range segmentWidths {
		usedWidth += w
	}
	if usedWidth < barWidth {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		sb.WriteString(style.Render(strings.Repeat(block, barWidth-usedWidth)))
	}
	sb.WriteString("]\n\n")

	// Build the legend
	// Find longest label for alignment
	maxLabelLen := 0
	for _, s := range sections {
		if len(s.label) > maxLabelLen {
			maxLabelLen = len(s.label)
		}
	}

	for _, s := range sections {
		percent := 0.0
		if total > 0 {
			percent = float64(s.chars) / float64(total) * 100
		}
		style := lipgloss.NewStyle().Foreground(s.color)
		swatch := style.Render(block)
		labelPad := maxLabelLen - len(s.label)
		fmt.Fprintf(&sb, "  %s %s%s  %7s  (%5.1f%%)\n",
			swatch,
			s.label,
			strings.Repeat(" ", labelPad),
			formatNumber(s.chars),
			percent,
		)
	}

	return sb.String()
}

func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	s := fmt.Sprintf("%d", n)
	var result strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(r)
	}
	return result.String()
}
