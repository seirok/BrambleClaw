package agent

import (
	"context"
	"fmt"
	"neoclaw/internal/interfaces"
	"strings"
)

// SubContextBuilder 构建二级 Agent 的简化上下文，实现 interfaces.Builder
type SubContextBuilder struct {
	name         string
	description  string
	systemPrompt string
	subTools     interfaces.Registry[interfaces.Tool]
}

func NewSubContextBuilder(
	name, description, systemPrompt string,
	subTools interfaces.Registry[interfaces.Tool],
) *SubContextBuilder {
	return &SubContextBuilder{
		name:         name,
		description:  description,
		systemPrompt: systemPrompt,
		subTools:     subTools,
	}
}

// Build 实现 interfaces.Builder
func (b *SubContextBuilder) Build() (string, error) {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# %s\n\n", b.name)
	fmt.Fprintf(&sb, "## Description\n%s\n\n", b.description)

	if b.systemPrompt != "" {
		fmt.Fprintf(&sb, "## Instructions\n%s\n\n", b.systemPrompt)
	}

	if b.subTools != nil {
		toolList := b.subTools.List(context.Background())
		if len(toolList) > 0 {
			sb.WriteString("## Available Tools\n")
			for _, tool := range toolList {
				fmt.Fprintf(&sb, "- **%s**: %s\n", tool.Name(), tool.Description())
			}
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}
