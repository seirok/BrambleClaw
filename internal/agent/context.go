package agent

import (
	"brambleclaw/internal/config"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/session"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

type DynamicInfo struct {
	channel  string
	chatID   string
	senderID string
	usage    int
}

type ContextBuilder struct {
	agent             *Agent
	compact           *config.CompactConfig
	summaryCompressor *SummaryCompressor
}

func NewContextBuilder(compactCfg *config.CompactConfig) (*ContextBuilder, error) {
	contextBuilder := &ContextBuilder{
		compact: compactCfg,
	}
	return contextBuilder, nil
}

func (cb *ContextBuilder) Agent() *Agent { return cb.agent }

func (cb *ContextBuilder) SetAgent(agent *Agent) { cb.agent = agent }

func formatCurrentSenderLine(id string) string {
	if id == "" {
		logger.L().Error().Msg("sender ID is empty")
		return ""
	}
	return fmt.Sprintf("Sender ID: %s", id)
}

func (cb *ContextBuilder) BuildDynamicCtx(info *DynamicInfo) string {
	// Time
	now := time.Now().Format("2006-01-02 15:04 (Monday)")

	// Runtime
	rt := fmt.Sprintf("%s %s, Go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	// Sender Info
	senderLine := formatCurrentSenderLine(info.senderID)

	// Compose
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Current Time\n%s\n\n", now)
	fmt.Fprintf(&sb, "## Current Runtime\n%s\n\n", rt)
	fmt.Fprintf(&sb, "## Current Session\nChannel: %s\nChat ID: %s\n\n", info.channel, info.chatID)
	fmt.Fprintf(&sb, "## Current Sender\n%s", senderLine)
	return sb.String()
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

func (cb *ContextBuilder) ListSkills() []SkillInfo {
	var skills []SkillInfo
	skillsDir := filepath.Join(cb.Agent().Workspace(), "skills")

	// 1. 检查目录是否存在
	info, err := os.Stat(skillsDir)
	if os.IsNotExist(err) {
		logger.L().Warn().Str("path", skillsDir).Msg("skills directory does not exist")
		return nil
	}
	if !info.IsDir() {
		logger.L().Warn().Str("path", skillsDir).Msg("skills path is not a directory")
		return nil
	}

	// 2. 遍历目录
	err = filepath.Walk(skillsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// 这里返回 err 会停止整个 Walk 过程
			return err
		}

		if !info.IsDir() && filepath.Base(path) == "SKILL.md" {
			content, err := os.ReadFile(path)
			if err != nil {
				// 记录单个文件读取错误，但不一定要中断整个遍历，可以选 Warn
				logger.L().Warn().Err(err).Str("file", path).Msg("could not read skill file")
				return nil
			}

			title, description := extractMarkdownMetadata(string(content))
			if title == "" || description == "" {
				logger.L().Warn().Str("file", path).Msg("invalid metadata in SKILL.md")
				return nil
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

	// 3. 处理 Walk 过程中发生的意外错误（如权限问题等）
	if err != nil {
		logger.L().Error().Err(err).Msg("error occurred during walking skills directory")
		return nil
	}

	// 4. 检查是否找到了 Skill
	if len(skills) == 0 {
		logger.L().Warn().Str("path", skillsDir).Msg("no valid skills found in directory")
	}

	return skills
}

func (cb *ContextBuilder) BuildSkillsSummary() string {
	// Load skill
	allSkills := cb.ListSkills()

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

	return strings.Join(lines, "\n")
}

func (cb *ContextBuilder) BuildStaticCtx() string {
	//
	var lines []string

	// 1. AI 身份与基本规则
	identity := cb.getIdentity()
	lines = append(lines, identity)

	// 2. 加载Bootstrap（助手身份设定）
	bootstrap := cb.LoadBootstrapFiles()
	lines = append(lines, bootstrap)

	// 3. Skill summary
	summary := cb.BuildSkillsSummary()
	lines = append(lines, summary)

	// 4. Memory
	memory := cb.GetMemoryContext()
	lines = append(lines, memory)

	//
	staticCtx := strings.Join(lines, "\n\n-------\n\n")
	return staticCtx
}

func (cb *ContextBuilder) LoadBootstrapFiles() string {
	var sb strings.Builder
	files := []string{"AGENT.md", "SOUL.md", "USER.md"}
	for _, file := range files {
		filePath := filepath.Join(cb.Agent().Workspace(), file)

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			if err := cb.createDefaultBootstrapFile(file, filePath); err != nil {
				logger.L().Warn().Err(err).Str("file", file).Msg("创建默认 bootstrap 文件失败")
				continue
			}
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				logger.L().Debug().Str("file", file).Msg("Bootstrap file not found, skipping")
				continue
			}
			logger.L().Error().Err(err).Str("file", file).Msg("failed to read file")
			return ""
		}
		fmt.Fprintf(&sb, "## %s\n\n", strings.TrimSpace(string(data)))
	}

	return sb.String()
}

func (cb *ContextBuilder) createDefaultBootstrapFile(filename, filePath string) error {
	var content string

	switch filename {
	case "AGENT.md":
		content = "---\n" +
			"name: brambleclaw\n" +
			"description: >\n" +
			"  The default general-purpose assistant for everyday conversation, problem\n" +
			"  solving, and workspace help.\n" +
			"---\n" +
			"You are my default assistant for this workspace.\n" +
			"Your name is brambleclaw 🦞.\n\n" +
			"## Role\n\n" +
			"You are an ultra-lightweight personal AI assistant written in Go, designed to\n" +
			"be practical, accurate, and efficient.\n\n" +
			"## Mission\n\n" +
			"- Help with general requests, questions, and problem solving\n" +
			"- Use available tools when action is required\n" +
			"- Stay useful even on constrained hardware and minimal environments\n\n" +
			"## Capabilities\n\n" +
			"- Web search and content fetching\n" +
			"- File system operations\n" +
			"- Shell command execution\n" +
			"- Skill-based extension\n" +
			"- Memory and context management\n" +
			"- Multi-channel messaging integrations when configured\n\n" +
			"## Working Principles\n\n" +
			"- Be clear, direct, and accurate\n" +
			"- Prefer simplicity over unnecessary complexity\n" +
			"- Be transparent about actions and limits\n" +
			"- Respect user control, privacy, and safety\n" +
			"- Aim for fast, efficient help without sacrificing quality\n\n" +
			"## Goals\n\n" +
			"- Provide fast and lightweight AI assistance\n" +
			"- Support customization through skills and workspace files\n" +
			"- Remain effective on constrained hardware\n" +
			"- Improve through feedback and continued iteration\n\n" +
			"Read `SOUL.md` as part of your identity and communication style."

	case "SOUL.md":
		content = "# Soul\n\n" +
			"I am BrambleClaw: calm, helpful, and practical.\n\n" +
			"## Personality\n\n" +
			"- Helpful and friendly\n" +
			"- Concise and to the point\n" +
			"- Curious and eager to learn\n" +
			"- Honest and transparent\n" +
			"- Calm under uncertainty\n\n" +
			"## Values\n\n" +
			"- Accuracy over speed\n" +
			"- User privacy and safety\n" +
			"- Transparency in actions\n" +
			"- Continuous improvement\n" +
			"- Simplicity over unnecessary complexity"

	case "USER.md":
		content = "# User\n\n" +
			"Information about the user goes here.\n\n" +
			"## Preferences\n\n" +
			"- Communication style: (casual/formal)\n" +
			"- Timezone: (your timezone)\n" +
			"- Language: (your preferred language)\n\n" +
			"## Personal Information\n\n" +
			"- Name: (optional)\n" +
			"- Location: (optional)\n" +
			"- Occupation: (optional)\n\n" +
			"## Learning Goals\n\n" +
			"- What the user wants to learn from AI\n" +
			"- Preferred interaction style\n" +
			"- Areas of interest"

	default:
		return fmt.Errorf("unknown bootstrap file: %s", filename)
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write default %s: %w", filename, err)
	}

	logger.L().Info().Str("file", filename).Msg("已创建默认 bootstrap 文件")
	return nil
}

func (cb *ContextBuilder) getDiscoveryRule() string {
	return ""
}

func (cb *ContextBuilder) GetMemoryContext() string {
	var sb strings.Builder

	memory := cb.GetLongTerm()
	dailyNotes, _ := cb.GetRecentlyDailyNotes(3)

	fmt.Fprintf(&sb, "## Memory Context\n%s\n\n", memory)
	fmt.Fprintf(&sb, "## Recently Daily Notes\n%s\n", dailyNotes)
	return sb.String()
}

func (cb *ContextBuilder) GetLongTerm() string {
	longTermMemoryFile := filepath.Join(cb.Agent().Workspace(), "memory", "MEMORY.md")
	data, err := os.ReadFile(longTermMemoryFile)
	if err != nil {
		logger.L().Warn().Str("file", longTermMemoryFile).Err(err).Msg("Failed to read memory file")
		return ""
	}
	return string(data)
}

func (cb *ContextBuilder) Compact(ctx context.Context, sess *session.Session, info *DynamicInfo) error {
	// 检查是否需要压缩
	msgs := sess.Messages
	if info.usage <= cb.compact.CompactThreshold || len(msgs)%cb.compact.CompactRounds != 0 {
		return nil
	}
	// 边界检查
	if sess.Summarized+(cb.compact.CompactRounds/4) > len(msgs) {
		logger.L().Debug().Int("summarized", sess.Summarized).
			Int("rounds", cb.compact.CompactRounds).
			Int("msgsLen", len(msgs)).
			Msg("compact boundary check failed, skipping")
		return nil
	}
	needToCompact := msgs[sess.Summarized:(sess.Summarized + (cb.compact.CompactRounds)/4)]
	summarizeMsg := NewAgentMessage(cb.Agent().Name(), RoleUser, "Provide a concise summary of this conversation by far, preserving core context and key points.\n")
	needToCompact = append(needToCompact, summarizeMsg)

	resp, err := cb.Agent().orche.Run(ctx, needToCompact)
	if err != nil {
		return fmt.Errorf("compact: generate summary failed: %w", err)
	}

	// 提取摘要内容
	summaryContent := ""
	if len(resp.Choices) > 0 {
		summaryContent = resp.Choices[0].Message.Content
	}

	// 确保 SummaryCompressor 已初始化
	if cb.summaryCompressor == nil {
		return fmt.Errorf("compact: summaryCompressor not initialized")
	}

	// 添加摘要到层级压缩器（AddSummary 会自动处理层级压缩）
	_, err = cb.summaryCompressor.AddSummary(summaryContent, time.Now())
	if err != nil {
		logger.L().Error().Err(err).Str("sessionKey", sess.Key).
			Msg("compact: failed to add summary to compressor, but continuing")
	}

	// 更新 SessionSummary 到 metadata
	if summaryContent != "" {
		if err := cb.Agent().sessionManager.UpdateSessionSummary(sess.Key, summaryContent); err != nil {
			logger.L().Error().Err(err).Str("sessionKey", sess.Key).
				Msg("compact: failed to update session summary, but continuing")
		} else {
			logger.L().Debug().Str("sessionKey", sess.Key).
				Str("summary", summaryContent[:min(100, len(summaryContent))]).
				Msg("compact: session summary updated successfully")
		}

		// 重新构建 System Prompt 并替换 Messages[0]
		newSystemPrompt, err := cb.Build(info)
		if err != nil {
			logger.L().Error().Err(err).Str("sessionKey", sess.Key).
				Msg("compact: failed to rebuild system prompt, but continuing")
		} else {
			// 替换 Messages[0]
			if len(sess.Messages) > 0 {
				sess.Messages[0] = NewAgentMessage(cb.Agent().Name(), RoleSystem, newSystemPrompt)
				sess.Modified = true
				logger.L().Debug().Str("sessionKey", sess.Key).
					Msg("compact: system prompt updated with new session summary")
			}
		}
	}

	// 更新边界指针
	sess.Summarized += cb.compact.CompactRounds / 4

	// 更新 token 使用情况
	currentTokenUsed := resp.Usage.CompletionTokens + resp.Usage.PromptTokens
	cb.Agent().sessionManager.Update(sess, currentTokenUsed)

	return nil
}

func (cb *ContextBuilder) GetSessionSummary() string {
	if cb.summaryCompressor == nil {
		return ""
	}
	return cb.summaryCompressor.BuildSessionSummary()
}

func (cb *ContextBuilder) GetRecentlyDailyNotes(days int) (string, error) {
	var sb strings.Builder

	//
	for i := range days {
		//
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102")
		dailyNote := filepath.Join(cb.Agent().Workspace(), "memory", dateStr[:6], dateStr+".md")
		data, err := os.ReadFile(dailyNote)
		if err != nil {
			logger.L().Debug().Str("file", dailyNote).Msg("failed to read daily note file, maybe not exist")
			return "", nil
		}

		//
		fmt.Fprintf(&sb, "### %s\n", dateStr)
		sb.Write(data)
	}
	return sb.String(), nil
}

func (cb *ContextBuilder) getIdentity() string {
	workspacePath, _ := filepath.Abs(filepath.Join(cb.Agent().Workspace()))
	toolDiscovery := cb.getDiscoveryRule()

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
		workspacePath, workspacePath, workspacePath, workspacePath, workspacePath, workspacePath, toolDiscovery)
}

func (cb *ContextBuilder) Build(info *DynamicInfo) (string, error) {
	var systemPrompt []string
	staticCtx := cb.BuildStaticCtx()
	systemPrompt = append(systemPrompt, staticCtx)

	dynamicCtx := cb.BuildDynamicCtx(info)
	systemPrompt = append(systemPrompt, dynamicCtx)

	sessionSummary := cb.GetSessionSummary()
	systemPrompt = append(systemPrompt, sessionSummary)

	return strings.Join(systemPrompt, "\n\n"), nil

}

func (cb *ContextBuilder) BuildMessages(history []*ChatMsg, userMsg ChatMsg) error {
	return nil
}
