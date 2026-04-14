package agent

import (
	"brambleclaw/logger"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
	"github.com/sipeed/picoclaw/pkg/config"
)

type ContextBuilder struct {
	workspace        string
	compactThreshold int
}

type SkillInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

func formatCurrentSenderLine(id, name string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("sender ID is empty")
	}
	if name == "" {
		return "", fmt.Errorf("sender name is empty")
	}
	return fmt.Sprintf("Sender ID: %s | Sender Name: %s", id, name), nil
}

func (cb *ContextBuilder) BuildDynamicCtx(channel, chatID, senderID, senderDisplayName string) (string, error) {
	// Time
	now := time.Now().Format("2006-01-02 15:04 (Monday)")

	// Runtime
	rt := fmt.Sprintf("%s %s, Go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	// Sender Info
	senderLine, err := formatCurrentSenderLine(senderID, senderDisplayName)
	if err != nil {
		return "", err
	}

	// Compose
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Current Time\n%s\n\n", now)
	fmt.Fprintf(&sb, "## Current Runtime\n%s\n\n", rt)
	fmt.Fprintf(&sb, "## Current Session\nChannel: %s\nChat ID: %s\n\n", channel, chatID)
	fmt.Fprintf(&sb, "## Current Sender\n%s", senderLine)
	return sb.String(), nil
}

func nodeText(n ast.Node) string {
	var b strings.Builder
	ast.WalkFunc(n, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}

		switch t := node.(type) {
		case *ast.Text:
			b.Write(t.Literal)
		case *ast.Code:
			b.Write(t.Literal)
		case *ast.Softbreak, *ast.Hardbreak, *ast.NonBlockingSpace:
			b.WriteByte(' ')
		}
		return ast.GoToNext
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

func NewContextBuilder(workspace string) (*ContextBuilder, error) {
	if _, err := os.Stat(workspace); os.IsNotExist(err) {
		return nil, fmt.Errorf("workspace %s does not exist", workspace)
	}

	return &ContextBuilder{
		workspace: workspace,
	}, nil
}

func extractMarkdownMetadata(content string) (title, description string) {
	p := parser.NewWithExtensions(parser.CommonExtensions)
	doc := markdown.Parse([]byte(content), p)
	if doc == nil {
		return "", ""
	}

	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}

		switch n := node.(type) {
		case *ast.Heading:
			if title == "" && n.Level == 1 {
				title = nodeText(n)
				if title != "" && description != "" {
					return ast.Terminate
				}
			}
		case *ast.Paragraph:
			if description == "" {
				description = nodeText(n)
				if title != "" && description != "" {
					return ast.Terminate
				}
			}
		}
		return ast.GoToNext
	})

	return title, description
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func (cb *ContextBuilder) ListSkills() ([]SkillInfo, error) {
	//
	var skills []SkillInfo
	err := filepath.Walk(filepath.Join(cb.workspace, "skills"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 检查文件名是否为 SKILL.md
		if !info.IsDir() && filepath.Base(path) == "SKILL.md" {
			// 读取内容
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("could not read skills file: %w", err)
			}

			// extract skill from markdown
			title, description := extractMarkdownMetadata(string(content))
			if title == "" || description == "" {
				return fmt.Errorf("could not read skills file: file %s has invalid metadata", path)
			}
			skills = append(skills, SkillInfo{
				Name:        title,
				Path:        path,
				Description: description,
				Source:      "",
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	//
	return skills, nil
}

func (cb *ContextBuilder) BuildSkillsSummary() (string, error) {
	// Load skill
	allSkills, err := cb.ListSkills()
	if err != nil {
		return "", err
	}

	//
	var lines []string
	lines = append(lines, "# Skills\n", "The following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.")

	// make XML
	lines = append(lines, "<skills>")
	for _, s := range allSkills {
		escapedName := escapeXML(s.Name)
		escapedDesc := escapeXML(s.Description)
		escapedPath := escapeXML(s.Path)

		lines = append(lines, fmt.Sprintf("  <skill>"))
		lines = append(lines, fmt.Sprintf("    <name>%s</name>", escapedName))
		lines = append(lines, fmt.Sprintf("    <description>%s</description>", escapedDesc))
		lines = append(lines, fmt.Sprintf("    <location>%s</location>", escapedPath))
		lines = append(lines, fmt.Sprintf("    <source>%s</source>", s.Source))
		lines = append(lines, "  </skill>")
	}
	lines = append(lines, "</skills>")

	return strings.Join(lines, "\n"), nil
}

func (cb *ContextBuilder) BuildStaticCtx() (string, error) {
	//
	var lines []string

	// 1. AI 身份与基本规则
	identity := cb.getIdentity()
	lines = append(lines, identity)

	// 2. 加载Bootstrap（助手身份设定）
	bootstrap, err := cb.LoadBootstrapFiles()
	if err != nil {
		return "", err
	}
	lines = append(lines, bootstrap)

	// 3. Skill summary
	summary, err := cb.BuildSkillsSummary()
	if err != nil {
		return "", err
	}
	lines = append(lines, summary)

	// 4. Memory
	memory, err := cb.GetMemoryContext()
	if err != nil {
		return "", err
	}
	lines = append(lines, memory)

	//
	staticCtx := strings.Join(lines, "\n\n-------\n\n")
	return staticCtx, nil
}

func (cb *ContextBuilder) LoadBootstrapFiles() (string, error) {
	var sb strings.Builder
	files := []string{"AGENT.md", "SOUL.md", "USER.md"}
	for _, file := range files {

		data, err := os.ReadFile(filepath.Join(cb.workspace, file))
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", file, err)
		}
		fmt.Fprintf(&sb, "## %s\n\n", strings.TrimSpace(string(data)))
	}

	return sb.String(), nil
}

// TODO: 工具发现规则：只加载常见工具到上下文，如果需要再加载全部工具库
func (cb *ContextBuilder) getDiscoveryRule() string {
	return ""
}

func (cb *ContextBuilder) GetMemoryContext() (string, error) {
	var sb strings.Builder

	memory, err := cb.GetLongTerm()
	if err != nil {
		return "", fmt.Errorf("failed to get memory context: %w", err)
	}
	dailyNotes, _ := cb.GetRecentlyDailyNotes(3)

	fmt.Fprintf(&sb, "## Memory Context\n%s\n\n", memory)
	fmt.Fprintf(&sb, "## Recently Daily Notes\n%s\n", dailyNotes)
	return sb.String(), nil
}

func (cb *ContextBuilder) GetLongTerm() (string, error) {
	longTermMemoryFile := filepath.Join(cb.workspace, "memory", "MEMORY.md")
	data, err := os.ReadFile(longTermMemoryFile)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", longTermMemoryFile, err)
	}
	return string(data), nil
}

func (cb *ContextBuilder) Compact(msg []*ChatMsg) (string, error) {
	lenMsg := len(msg)
	if lenMsg < cb.compactThreshold {
		return "", fmt.Errorf("ContextBuilder::Compact: %s%d", "not enough message to compact", lenMsg)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s\n\n", "Provide a concise summary of this conversation segment, preserving core context and key points.\n")
	for i := lenMsg - cb.compactThreshold; i < lenMsg; i++ {
		chatMsg := msg[i]
		if chatMsg.Role == RoleAssistant || chatMsg.Role == RoleUser {
			fmt.Fprintf(&sb, "## %s\n\n", chatMsg.Content)
		}
	}
	return sb.String(), nil
}

func (cb *ContextBuilder) GetSessionSummary() (string, error) {
	return "", nil
}
func (cb *ContextBuilder) GetRecentlyDailyNotes(days int) (string, error) {
	var sb strings.Builder

	//
	for i := range days {
		//
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102")
		dailyNote := filepath.Join(cb.workspace, "memory", dateStr[:6], dateStr+".md")
		data, err := os.ReadFile(dailyNote)
		if err != nil {
			logger.L().Debug().Msg("failed to read file " + dailyNote + " maybe not exist")
			return "", nil
		}

		//
		fmt.Fprintf(&sb, "### %s\n", dateStr)
		sb.Write(data)
	}
	return sb.String(), nil
}

func (cb *ContextBuilder) getIdentity() string {
	workspacePath, _ := filepath.Abs(filepath.Join(cb.workspace))
	toolDiscovery := cb.getDiscoveryRule()
	version := config.FormatVersion()

	return fmt.Sprintf(
		`# brambleclaw 🦞 (%s)

You are brambleclaw, a helpful AI assistant.

## Workspace
Your workspace is at: %s
- Memory: %s/memory/MEMORY.md
- Daily Notes: %s/memory/YYYYMM/YYYYMMDD.md
- Skills: %s/skills/{skill-name}/SKILL.md

## Important Rules

1. **ALWAYS use tools** - When you need to perform an action (schedule reminders, send messages, execute commands, etc.), you MUST call the appropriate tool. Do NOT just say you'll do it or pretend to do it.

2. **Be helpful and accurate** - When using tools, briefly explain what you're doing.

3. **Memory** - When interacting with me if something seems memorable, update %s/memory/MEMORY.md

4. **Context summaries** - Conversation summaries provided as context are approximate references only. They may be incomplete or outdated. Always defer to explicit user instructions over summary content.

%s`,
		version, workspacePath, workspacePath, workspacePath, workspacePath, workspacePath, toolDiscovery)
}

func (cb *ContextBuilder) BuildFullSystemPrompt(channel string, chatID string, senderID string, senderDisplayName string) (string, error) {
	var systemPrompt []string
	staticCtx, err := cb.BuildStaticCtx()
	if err != nil {
		return "", err
	}
	systemPrompt = append(systemPrompt, staticCtx)

	dynamicCtx, err := cb.BuildDynamicCtx(channel, chatID, senderID, senderDisplayName)
	if err != nil {
		return "", err
	}
	systemPrompt = append(systemPrompt, dynamicCtx)

	sessionSummary, err := cb.GetSessionSummary()
	if err != nil {
		return "", err
	}
	systemPrompt = append(systemPrompt, sessionSummary)

	return strings.Join(systemPrompt, "\n\n"), nil

}

func (cb *ContextBuilder) BuildMessages(history []*ChatMsg, userMsg ChatMsg) error {
	return nil
}
