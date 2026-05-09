package cli

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/agent"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/channel"
	"brambleclaw/internal/config"
	"brambleclaw/internal/config/structs"
	"brambleclaw/internal/events"
	"brambleclaw/internal/gateway"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/runtime"
	"brambleclaw/internal/teamtool"
	"brambleclaw/internal/tools"
	"context"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "brambleclaw",
	Short: "Go-based AI Agent framework",
	Long:  `brambleclaw is a Go language implementation of an AI Agent framework.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run:   runVersion,
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "启动 AI 代理服务",
	RunE:  runAgent,
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "输出并格式化最近的日志和分析 session",
	RunE:  runDebug,
}

// 选项
var (
	agentMessage string
	agentSession string
	debugLines   int
	debugSession bool
)

func init() {
	rootCmd.AddCommand(versionCmd)

	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "非交互式执行：发送一条消息后退出")
	agentCmd.Flags().StringVarP(&agentSession, "session", "s", "default", "指定 Session Key，保留上下文对话")

	rootCmd.AddCommand(agentCmd)

	debugCmd.Flags().IntVarP(&debugLines, "lines", "n", 100, "输出最近的日志行数")
	debugCmd.Flags().BoolVarP(&debugSession, "session", "s", false, "分析 session")
	rootCmd.AddCommand(debugCmd)
}

func Execute() error {
	// 初始化配置单例
	config.Init()
	logger.L().Debug().Msg("配置加载成功")

	err := rootCmd.Execute()
	if err != nil {
		logger.L().Error().Err(err).Msg("命令执行失败")
	}
	return err
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Println("brambleclaw 1.0.0")
	fmt.Println("License: MIT")
}

func runDebug(cmd *cobra.Command, args []string) error {
	// 如果指定了 -s 或 --session，运行 session 分析器
	if debugSession {
		return runDebugSessions()
	}

	logPath := util.GetLogPath()
	logger.L().Debug().Str("log_path", logPath).Int("lines", debugLines).Msg("开始格式化输出日志")

	err := logger.AnalyzeLogs(logPath, debugLines)
	if err != nil {
		return err
	}
	return nil
}

// ========== Bubble Tea TUI Model ==========

// chatMessage 一条聊天消息
type chatMessage struct {
	Content   string
	IsUser    bool
	Timestamp time.Time
}

// agentResponseMsg 用于传递 Agent 回复
type agentResponseMsg struct {
	Content string
}

// thinkingEventMsg 用于传递思考事件
type thinkingEventMsg struct {
	Event events.ThinkingEvent
}

// sidebarTickMsg 用于触发侧边栏数据刷新
type sidebarTickMsg struct{}

// sidebarStats 侧边栏统计数据
type sidebarStats struct {
	// 实时累加（来自 thinkingEventMsg）
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	HookCounts       map[string]int64 // category -> count
	HookErrors       map[string]int64 // category -> error count

	// 低频刷新（来自 sidebarTickMsg）
	HookAvgMs      map[string]float64 // category -> avg duration ms
	ModelName      string
	AgentName      string
	IsPaused       bool
	FileOps        int64
	CmdExecs       int64
	BlockedOps     int64
	MessageCount   int
	SessionAge     time.Duration
	Summarized     int
	MCPClientCount int
}

// appModel TUI 状态
type appModel struct {
	textInput       textinput.Model
	viewport        viewport.Model
	eventViewport   viewport.Model
	sidebarViewport viewport.Model
	spinner         spinner.Model
	help            help.Model
	keys            keyMap
	messages        []chatMessage
	eventLog        []events.ThinkingEvent
	waiting         bool
	showBanner      bool
	width           int
	height          int
	msgBus          *bus.MessageBus
	agentSession    string
	quitting        bool
	err             error
	eventFocused    bool // true=event 面板有焦点
	sidebarEnabled  bool
	sidebarWidth    int
	sidebarStats    sidebarStats
	sidebarSections []structs.SidebarSection
	focus           focusRegion // 替换原来的 eventFocused bool
}

type keyMap struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Quit  key.Binding
}

var keys = keyMap{
	Up:    key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "滚动")),
	Down:  key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "滚动")),
	Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "发送")),
	Quit:  key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "退出")),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Quit}
}

// 3. 实现 FullHelp 方法 (通常用于扩展视图，这里可以直接返回一样的内容)
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},    // 第一行
		{k.Enter, k.Quit}, // 第二行
	}
}

// 样式定义
var (
	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("135")) // 紫色

	agentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")) // 青色

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")) // 粉色

	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	inputPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86"))

	inputTextStyle = lipgloss.NewStyle()
)

type focusRegion int

const (
	focusInput focusRegion = iota // 0: 输入区
	focusChat                     // 1: 主内容区
	focusEvent                    // 2: 思考区
)

func newAppModel(msgBus *bus.MessageBus, session string) appModel {
	cfg := config.Get()
	sidebarCfg := cfg.Sidebar

	ti := textinput.New()
	ti.Placeholder = "输入消息..."
	ti.Focus()
	ti.Prompt = "> "
	ti.PromptStyle = inputPromptStyle
	ti.TextStyle = inputTextStyle
	ti.CharLimit = 1000
	ti.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	vp := viewport.New(80, 20)
	eventVp := viewport.New(80, 10)
	eventVp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	sidebarVp := viewport.New(sidebarCfg.Width, 30)
	sidebarVp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86"))

	return appModel{
		textInput:       ti,
		viewport:        vp,
		eventViewport:   eventVp,
		sidebarViewport: sidebarVp,
		spinner:         s,
		messages:        []chatMessage{},
		eventLog:        []events.ThinkingEvent{},
		waiting:         false,
		showBanner:      true,
		msgBus:          msgBus,
		agentSession:    session,
		help:            help.New(),
		sidebarEnabled:  sidebarCfg.Enabled,
		sidebarWidth:    sidebarCfg.Width,
		sidebarSections: sidebarCfg.Sections,
		sidebarStats: sidebarStats{
			HookCounts: make(map[string]int64),
			HookErrors: make(map[string]int64),
			HookAvgMs:  make(map[string]float64),
		},
	}
}

func (m appModel) Init() tea.Cmd {
	if m.sidebarEnabled {
		return tea.Batch(
			textinput.Blink,
			m.spinner.Tick,
			tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return sidebarTickMsg{}
			}),
		)
	}
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
	)
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd  tea.Cmd
		vpCmd  tea.Cmd
		evpCmd tea.Cmd
		spCmd  tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyTab:
			// 循环切换：0 -> 1 -> 2 -> 0
			m.focus = (m.focus + 1) % 3

			// 只有在输入区时，textInput 才获取焦点
			if m.focus == focusInput {
				m.textInput.Focus()
			} else {
				m.textInput.Blur()
			}
			return m, nil

		case tea.KeyEnter:
			if m.waiting || m.eventFocused {
				return m, nil
			}
			input := m.textInput.Value()
			if input == "" {
				return m, nil
			}
			if input == "exit" || input == "quit" {
				m.quitting = true
				return m, tea.Quit
			}

			// 发送用户消息
			m.messages = append(m.messages, chatMessage{
				Content:   input,
				IsUser:    true,
				Timestamp: time.Now(),
			})
			m.showBanner = false
			m.textInput.SetValue("")

			// 发布到总线
			go func() {
				inboundMsg := &bus.InBoundMessage{
					InChannel: "cli",
					SenderID:  "cli",
					ChatID:    m.agentSession,
					Content:   input,
					TimeStamp: time.Now(),
				}
				ctx := context.Background()
				if err := m.msgBus.PublishInBoundMessage(ctx, inboundMsg); err != nil {
					logger.L().Error().Err(err).Msg("发送消息失败")
				}
			}()

			m.waiting = true
			return m, m.spinner.Tick

		case tea.KeyUp:
			if m.eventFocused {
				m.eventViewport.ScrollUp(1)
			} else {
				m.viewport.ScrollUp(1)
			}
		case tea.KeyDown:
			if m.eventFocused {
				m.eventViewport.ScrollDown(1)
			} else {
				m.viewport.ScrollDown(1)
			}
		}
		switch msg.String() {
		case "pgup":
			if m.eventFocused {
				m.eventViewport.PageUp()
			} else {
				m.viewport.PageUp()
			}
		case "pgdown":
			if m.eventFocused {
				m.eventViewport.PageDown()
			} else {
				m.viewport.PageDown()
			}
		}

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

		// --- 1. 横向宽度分配 ---
		sidebarWidth := 32 // 与 View 中的 sidebarWidth 保持一致
		mainContentWidth := m.width
		if m.sidebarEnabled {
			mainContentWidth = m.width - sidebarWidth
		}

		// 确保宽度不为负数（防止窗口缩得太小时崩溃）
		if mainContentWidth < 4 {
			mainContentWidth = 4
		}

		// --- 2. 纵向高度分配 ---
		reservedHeight := 10
		availableHeight := m.height - reservedHeight

		if availableHeight < 10 {
			availableHeight = 10
		} // 最小高度保证

		// 按比例分配：聊天区占 70%，思考区占 30%
		chatHeight := int(float64(availableHeight) * 0.7)
		eventHeight := availableHeight - chatHeight

		// --- 3. 更新各组件尺寸 ---

		// 主聊天视口：宽度需减去边框占用的 2 个字符
		m.viewport.Width = mainContentWidth - 4
		m.viewport.Height = chatHeight

		// 思考事件视口：宽度需减去边框占用的 2 个字符
		m.eventViewport.Width = mainContentWidth - 4
		m.eventViewport.Height = eventHeight

		// 侧边栏视口：
		// 注意：侧边栏的高度在 View 中是通过 lipgloss.Height(leftPanel) 动态计算的，
		// 这里设置 Width 即可，Height 可以设为一个安全值。
		if m.sidebarEnabled {
			m.sidebarViewport.Width = sidebarWidth - 4 // 减去边框和 Padding
		}

		// 输入框宽度：铺满全屏或预留少量边距
		m.textInput.Width = m.width - 4

		// 重新渲染内容以适配新宽度
		m.viewport.SetContent(m.renderMessages())
		m.eventViewport.SetContent(m.renderEvents())

		return m, nil

	case spinner.TickMsg:
		if m.waiting {
			m.spinner, spCmd = m.spinner.Update(msg)
		}

	case agentResponseMsg:
		// Agent 回复到达
		m.messages = append(m.messages, chatMessage{
			Content:   msg.Content,
			IsUser:    false,
			Timestamp: time.Now(),
		})
		m.waiting = false
		m.showBanner = false
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case thinkingEventMsg:
		// 思考事件到达
		m.eventLog = append(m.eventLog, msg.Event)
		// 保留最多 200 条
		if len(m.eventLog) > 200 {
			m.eventLog = m.eventLog[len(m.eventLog)-200:]
		}
		m.eventViewport.SetContent(m.renderEvents())

		if m.sidebarEnabled {
			event := msg.Event

			// 从事件点提取类别
			category := "UNKNOWN"
			switch {
			case strings.HasPrefix(event.Point, "hook.point.llm."):
				category = "LLM"
			case strings.HasPrefix(event.Point, "hook.point.tool."):
				category = "TOOL"
			case strings.HasPrefix(event.Point, "hook.point.agent."):
				category = "AGENT"
			case strings.HasPrefix(event.Point, "hook.point.message."):
				category = "MESSAGE"
			case strings.HasPrefix(event.Point, "hook.point.sandbox."):
				category = "SANDBOX"
			}
			// 增加该类别的计数
			m.sidebarStats.HookCounts[category]++

			// 检查是否是错误事件
			if strings.Contains(strings.ToLower(event.Point), "error") {
				m.sidebarStats.HookErrors[category]++
			}

			// 如果是LLM响应，提取token使用量
			if event.Point == "hook.point.llm.response" {
				resp := event.Data
				v := reflect.ValueOf(resp)
				if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
					v = v.Elem()
					if usageField := v.FieldByName("Usage"); usageField.IsValid() {
						if ptField := usageField.FieldByName("PromptTokens"); ptField.IsValid() {
							m.sidebarStats.PromptTokens += int(ptField.Int())
						}
						if ctField := usageField.FieldByName("CompletionTokens"); ctField.IsValid() {
							m.sidebarStats.CompletionTokens += int(ctField.Int())
						}
						m.sidebarStats.TotalTokens = m.sidebarStats.PromptTokens + m.sidebarStats.CompletionTokens
					}
				}
			}

			// 如果在底部，自动滚动到底部
			if m.eventViewport.AtBottom() {
				m.eventViewport.GotoBottom()
			}
		}
	case sidebarTickMsg:
		// 定时更新侧边栏低频数据
		if m.sidebarEnabled {
			cfg := config.Get()
			m.sidebarStats.ModelName = cfg.LLMConfig.Model
			m.sidebarStats.MessageCount = len(m.messages)
			m.sidebarViewport.SetContent(m.renderSidebar())
		}
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return sidebarTickMsg{}
		})

	case errMsg:
		m.err = msg.err
		return m, tea.Quit
	}

	m.textInput, tiCmd = m.textInput.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.eventViewport, evpCmd = m.eventViewport.Update(msg)
	m.sidebarViewport, _ = m.sidebarViewport.Update(msg)

	return m, tea.Batch(tiCmd, vpCmd, evpCmd, spCmd)
}

// renderMessages 渲染消息列表为字符串
func (m appModel) renderMessages() string {
	var messagesView string

	if m.showBanner && len(m.messages) == 0 {
		// Banner
		messagesView = renderClaudeBanner(m.viewport.Width) + "\n\n"
	}

	// 渲染消息列表
	for _, msg := range m.messages {
		var line string
		if msg.IsUser {
			line = userStyle.Render("You: "+msg.Content) + "\n"
		} else {
			line = agentStyle.Render("🐱: "+msg.Content) + "\n"
		}
		messagesView += line + "\n\n"
	}

	if m.waiting {
		messagesView += m.spinner.View() + " 🐱 正在思考...\n"
	}

	return messagesView
}

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

	// 渲染左侧面板 (Chat + Thinking)
	// 聊天区样式
	chatStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(mainWidth-2).
		Height(m.viewport.Height).
		Padding(0, 1) // 增加左右内边距

	if m.focus == focusChat {
		chatStyle = chatStyle.BorderForeground(activeColor)
	} else {
		chatStyle = chatStyle.BorderForeground(inactiveColor)
	}
	chatBox := chatStyle.Render(m.renderMessages())

	// 思考区样式
	eventStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(mainWidth-2).
		Height(m.eventViewport.Height).
		Padding(0, 1) // 增加左右内边距

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

	// 底部输入区 (带上下分割线且对齐)
	// 定义输入容器样式：上下边框，左边距 1 以对齐上方边框线
	inputContainerStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, true, false).
		BorderForeground(inactiveColor).
		Padding(0, 0, 0, 1). // 关键：左边补 1 格，对齐左侧边框
		Width(m.width)

	if m.focus == focusInput {
		inputContainerStyle = inputContainerStyle.BorderForeground(activeColor)
		m.textInput.PromptStyle = lipgloss.NewStyle().Foreground(activeColor)
	} else {
		m.textInput.PromptStyle = lipgloss.NewStyle().Foreground(inactiveColor)
	}

	// 渲染输入框包装
	inputBox := inputContainerStyle.Render(m.textInput.View())

	// 帮助信息（位于最底部）
	helpView := m.help.View(keys)

	// 组装最终布局
	// 拼接上方主区域
	mainLayout := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// 最终拼接顺序：主面板 -> 带线的输入区 -> 帮助提示
	return lipgloss.JoinVertical(lipgloss.Left,
		mainLayout,
		inputBox,
		helpView,
	)
}

// 添加 renderSidebar 函数：
func (m appModel) renderSidebar() string {
	var sb strings.Builder

	// 侧边栏标题
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true).
		MarginLeft(1)
	sb.WriteString(titleStyle.Render("📊 Statistics"))
	sb.WriteString("\n\n")
	sectionTitleStyle := lipgloss.NewStyle().Bold(true)
	contentStyle := lipgloss.NewStyle().
		PaddingLeft(3). // 2个Emoji宽度 + 1个空格
		Foreground(lipgloss.Color("245"))

	// 遍历启用的 sections 渲染
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

// 添加各个 render 辅助函数：
func (m appModel) renderTokenUsage(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("207")).
		Bold(true).
		Render("🗳️ Token Usage")
	sb.WriteString(sectionTitle + "\n")

	// 使用 %-11s 确保标签对齐，%5d 确保数字对齐且不至于隔太开
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
		Foreground(lipgloss.Color("69")). // 保留原色
		Render("🔗 Hook Stats")
	sb.WriteString(sectionTitle + "\n")

	categories := []string{"LLM", "TOOL", "AGENT", "MESSAGE", "SANDBOX"}
	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	for _, cat := range categories {
		count := m.sidebarStats.HookCounts[cat]
		errors := m.sidebarStats.HookErrors[cat]
		var line string
		if errors > 0 {
			// 使用 %-7s 确保类别名称宽度一致，%3d 确保数字对齐
			line = fmt.Sprintf("%-7s: %3d (%2d err)", cat, count, errors)
		} else {
			line = fmt.Sprintf("%-7s: %3d", cat, count)
		}
		sb.WriteString(statStyle.Render(line) + "\n")
	}
}

func (m appModel) renderModelInfo(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("78")). // 保留原色
		Render("🤖 Model Info")
	sb.WriteString(sectionTitle + "\n")

	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	if m.sidebarStats.ModelName == "" {
		// 这里的 config 调用请确保在作用域内可用
		// m.sidebarStats.ModelName = config.Get().LLMConfig.Model
	}

	sb.WriteString(statStyle.Render(fmt.Sprintf("Model: %s", m.sidebarStats.ModelName)) + "\n")
	sb.WriteString(statStyle.Render(fmt.Sprintf("Agent: %s", "main")) + "\n")
}

func (m appModel) renderSandboxStats(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("135")). // 保留原色
		Render("📁 Sandbox")
	sb.WriteString(sectionTitle + "\n")

	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	// 统一使用固定宽度对齐数字
	sb.WriteString(statStyle.Render(fmt.Sprintf("File Ops: %5d", m.sidebarStats.FileOps)) + "\n")
	sb.WriteString(statStyle.Render(fmt.Sprintf("Commands: %5d", m.sidebarStats.CmdExecs)) + "\n")
	sb.WriteString(statStyle.Render(fmt.Sprintf("Blocked:  %5d", m.sidebarStats.BlockedOps)) + "\n")
}

func (m appModel) renderSessionStats(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("214")). // 保留原色
		Render("💬 Session")
	sb.WriteString(sectionTitle + "\n")

	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	sb.WriteString(statStyle.Render(fmt.Sprintf("Messages: %3d", m.sidebarStats.MessageCount)) + "\n")
}

func (m appModel) renderMCPStats(sb *strings.Builder, titleStyle, contentStyle lipgloss.Style) {
	sectionTitle := titleStyle.
		Foreground(lipgloss.Color("141")). // 保留原色
		Render("🔌 MCP")
	sb.WriteString(sectionTitle + "\n")

	statStyle := contentStyle.Foreground(lipgloss.Color("240"))

	sb.WriteString(statStyle.Render(fmt.Sprintf("Clients: %2d", m.sidebarStats.MCPClientCount)) + "\n")
}

// renderEvents 渲染事件列表
func (m appModel) renderEvents() string {
	var sb strings.Builder
	for _, evt := range m.eventLog {
		// 获取事件的样式和格式化摘要
		style := getEventStyle(evt.Point)
		summary := formatEventSummary(evt)
		sb.WriteString(style.Render(summary))
		sb.WriteString("\n")
	}
	return sb.String()
}

var (
	claudeOrange = lipgloss.Color("#D97757") // Claude 标志性的橘粉色
	subtleGray   = lipgloss.Color("240")
)

func renderClaudeBanner(width int) string {
	// 1. 定义颜色：黑莓掌的琥珀色眼睛和深色毛发色
	amberEye := lipgloss.Color("#FFBF00") // 琥珀金
	furColor := lipgloss.Color("#8B4513") // 深棕色

	g := lipgloss.NewStyle().Foreground(lipgloss.Color("#7d7d7d")) // 灰毛
	e := lipgloss.NewStyle().Foreground(lipgloss.Color("#4fc3f7")) // 亮蓝眼
	n := lipgloss.NewStyle().Foreground(lipgloss.Color("#f06292")) // 粉鼻
	w := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")) // 白胡子
	s := "  "

	lines := []string{
		s + s + g.Render("█ ") + s + s + s + g.Render(" █") + s + s, // 尖耳朵
		s + g.Render("█████") + s + g.Render("█████") + s,           // 耳根
		g.Render("██████████████"),                                  // 额头
		// 眼睛：缩小为 1个双块，且两边留出灰边，眼神瞬间就亮了
		g.Render("██") + e.Render("██") + g.Render("██████") + e.Render("██") + g.Render("██"),
		g.Render("██████████████"),                                                                 // 脸颊
		s + g.Render("██") + w.Render("██") + n.Render("██") + w.Render("██") + g.Render("██") + s, // 鼻子
		s + s + w.Render("██████") + s + s,                                                         // 小嘴
		s + s + g.Render("██████") + s + s,                                                         // 下巴
	}
	pet := lipgloss.NewStyle().Foreground(furColor).Render(strings.Join(lines, "\n"))

	welcomeMsg := lipgloss.NewStyle().Bold(true).Render("Welcome to Brambleclaw.")

	// 左侧信息：加入“雷族”或“武士”身份标识
	clanInfo := lipgloss.NewStyle().Foreground(subtleGray).Faint(true).Render(
		"ThunderClan Edition • Warrior Code v1.0\n" +
			"~/GolandProjects/brambleclaw",
	)

	leftContent := lipgloss.JoinVertical(lipgloss.Center, welcomeMsg, "", pet)
	leftBox := lipgloss.JoinVertical(lipgloss.Left, leftContent, "", clanInfo)

	// 3. 中间分割线
	divider := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 2).
		Height(9).
		Render("")

	// 4. 右侧：What's new (保持 Claude 风格的极简列表)
	titleStyle := lipgloss.NewStyle().Foreground(amberEye).Bold(true)
	listStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))

	rightStyle := lipgloss.NewStyle().MarginLeft(3) // 增加 3 个空格的距离

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
		Width(width - 4). // 这里的 width 是 viewport 的宽度
		Render(bannerBody)
}

// getEventStyle 获取事件样式
func getEventStyle(point string) lipgloss.Style {
	switch {
	case strings.HasPrefix(point, "hook.point.llm."):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("69")) // blue
	case strings.HasPrefix(point, "hook.point.tool."):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // yellow
	case strings.HasPrefix(point, "hook.point.message."):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // gray
	case strings.HasPrefix(point, "hook.point.agent."):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("78")) // green
	case strings.HasPrefix(point, "hook.point.sandbox."):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("135")) // purple
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // gray
	}
}

// formatEventSummary 格式化事件摘要（通过类型断言处理 data）
func formatEventSummary(evt events.ThinkingEvent) string {
	switch {
	case strings.HasPrefix(evt.Point, "hook.point.llm."):
		return formatLLMSummary(evt)
	case strings.HasPrefix(evt.Point, "hook.point.tool."):
		return formatToolSummary(evt)
	case strings.HasPrefix(evt.Point, "hook.point.message."):
		return formatMessageSummary(evt)
	case strings.HasPrefix(evt.Point, "hook.point.agent."):
		return formatAgentSummary(evt)
	case strings.HasPrefix(evt.Point, "hook.point.sandbox."):
		return formatSandboxSummary(evt)
	default:
		return fmt.Sprintf("%s: %T", evt.Point, evt.Data)
	}
}

func formatLLMSummary(evt events.ThinkingEvent) string {
	switch evt.Point {
	case "hook.point.llm.request":
		req := evt.Data
		model := "unknown"
		msgCount := 0
		v := reflect.ValueOf(req)
		if v.Kind() == reflect.Struct {
			if modelField := v.FieldByName("Model"); modelField.IsValid() {
				model = modelField.String()
			}
			if msgsField := v.FieldByName("Messages"); msgsField.IsValid() {
				msgCount = msgsField.Len()
			}
		}
		return fmt.Sprintf("LLM → %s (%d messages)", model, msgCount)
	case "hook.point.llm.response":
		resp := evt.Data
		totalTokens := 0
		v := reflect.ValueOf(resp)
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
			v = v.Elem()
			if usageField := v.FieldByName("Usage"); usageField.IsValid() {
				if ptField := usageField.FieldByName("PromptTokens"); ptField.IsValid() {
					totalTokens += int(ptField.Int())
				}
				if ctField := usageField.FieldByName("CompletionTokens"); ctField.IsValid() {
					totalTokens += int(ctField.Int())
				}
			}
		}
		return fmt.Sprintf("LLM ← response (%d tokens)", totalTokens)
	case "hook.point.llm.error":
		if err, ok := evt.Data.(error); ok {
			return fmt.Sprintf("LLM ✗ %v", err)
		}
		return "LLM ✗ error"
	}
	return "LLM event"
}

func formatToolSummary(evt events.ThinkingEvent) string {
	switch evt.Point {
	case "hook.point.tool.pre-execute":
		if args, ok := evt.Data.(string); ok {
			return fmt.Sprintf("TOOL ▶ %s", truncate(args, 100))
		}
		return "TOOL ▶ executing"
	case "hook.point.tool.result":
		resultStr := fmt.Sprintf("%v", evt.Data)
		return fmt.Sprintf("TOOL ◀ %s", truncate(resultStr, 100))
	case "hook.point.tool.error":
		if err, ok := evt.Data.(error); ok {
			return fmt.Sprintf("TOOL ✗ %v", err)
		}
		return "TOOL ✗ error"
	}
	return "TOOL event"
}

func formatMessageSummary(evt events.ThinkingEvent) string {
	switch evt.Point {
	case "hook.point.message.pre-process":
		var content string
		v := reflect.ValueOf(evt.Data)
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
			v = v.Elem()
			if f := v.FieldByName("Content"); f.IsValid() {
				content = f.String()
			}
		}
		return fmt.Sprintf("MSG → processing: %s", truncate(content, 50))
	case "hook.point.message.pre-response":
		var content string
		v := reflect.ValueOf(evt.Data)
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
			v = v.Elem()
			if f := v.FieldByName("Content"); f.IsValid() {
				content = f.String()
			}
		}
		return fmt.Sprintf("MSG ← responding: %s", truncate(content, 50))
	case "hook.point.message.post-process":
		return "MSG ✔ processed"
	}
	return "MSG event"
}

func formatAgentSummary(evt events.ThinkingEvent) string {
	name := "unknown"
	v := reflect.ValueOf(evt.Data)
	if v.Kind() == reflect.Ptr {
		if nameMethod := v.MethodByName("Name"); nameMethod.IsValid() {
			results := nameMethod.Call(nil)
			if len(results) == 1 && results[0].Kind() == reflect.String {
				name = results[0].String()
			}
		}
	}
	switch evt.Point {
	case "hook.point.agent.create":
		return fmt.Sprintf("AGENT %s created", name)
	case "hook.point.agent.pre-start":
		return fmt.Sprintf("AGENT %s starting...", name)
	case "hook.point.agent.start":
		return fmt.Sprintf("AGENT %s started", name)
	case "hook.point.agent.pre-stop":
		return fmt.Sprintf("AGENT %s stopping...", name)
	case "hook.point.agent.stop":
		return fmt.Sprintf("AGENT %s stopped", name)
	}
	return "AGENT event"
}

func formatSandboxSummary(evt events.ThinkingEvent) string {
	return fmt.Sprintf("SANDBOX %s", strings.TrimPrefix(evt.Point, "hook.point.sandbox."))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	return s[:maxLen-3] + "..."
}

// ========== runAgent ==========

func runAgent(cmd *cobra.Command, args []string) error {
	logger.L().Debug().Msg("加载系统配置...")

	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("系统配置加载失败")
	}

	logger.Setup(cfg.Log.Level, cfg.Log.ConsoleEnabled)
	logger.L().Debug().Msg("日志配置已加载")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. 初始化消息总线
	logger.L().Debug().Msg("初始化消息总线...")
	msgBus := bus.NewMessageBus(cfg.BusBufSize)

	// 2. 初始化通道管理器
	logger.L().Debug().Msg("初始化通道管理器...")
	channelManager := channel.NewChannelManager(msgBus)
	logger.L().Debug().Msg("Initializing channel manager with config...")
	if err := channelManager.Initialize(ctx, cfg); err != nil {
		return fmt.Errorf("failed to initialize channel manager: %w", err)
	}

	// 5. 初始化 AgentManager
	logger.L().Debug().Msg("初始化 AgentManager...")
	agentManager := agent.NewAgentManager(msgBus, runtime.NewAgentRuntime())
	agentManager.SetToolFactory(func(a *agent.Agent) []tools.Tool {
		return []tools.Tool{teamtool.NewCreateTeamTool(a)}
	})

	// 6. 初始化 Gateway
	logger.L().Debug().Msg("初始化 Gateway...")
	gwCfg := config.Get().Gateway
	gw := gateway.NewGateway(
		gateway.WithRouter(gateway.NewRouter(gwCfg.Routes, agentManager)),
		gateway.WithAgentManager(agentManager),
		gateway.WithChannelManager(channelManager),
		gateway.WithMessageBus(msgBus),
	)

	// 7. 启动 Gateway 和 Channel
	logger.L().Debug().Msg("启动 Gateway 和 Channel...")
	if err := gw.Start(ctx); err != nil {
		return err
	}

	if err := channelManager.StartAll(ctx); err != nil {
		return err
	}

	// 等待 Gateway 准备好
	time.Sleep(100 * time.Millisecond)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 交互式与非交互式执行
	if agentMessage != "" {
		// 非交互式执行（保持不变）
		inboundMsg := &bus.InBoundMessage{
			InChannel: "cli",
			SenderID:  "cli",
			ChatID:    agentSession,
			Content:   agentMessage,
			TimeStamp: time.Now(),
		}

		// 订阅接收返回的消息
		sub := msgBus.Subscribe()
		defer msgBus.Unsubscribe(sub.ID)

		if err := msgBus.PublishInBoundMessage(ctx, inboundMsg); err != nil {
			return fmt.Errorf("发送消息失败: %w", err)
		}

		select {
		case outMsg := <-sub.Channel:
			if outMsg.ReplyTo == inboundMsg.ID {
				// Gateway 输出已通过 CLIChannel 的 Send 方法打印了，
				// 这里只需要等待执行完成即可退出
				time.Sleep(100 * time.Millisecond)
			}
		case <-time.After(30 * time.Second):
			return fmt.Errorf("执行超时")
		case <-ctx.Done():
		case <-sigChan:
			fmt.Println("\n收到退出信号...")
		}
	} else {
		// ========== 交互式执行 ==========

		// 创建 EventBus 并注册 observers
		tvCfg := cfg.Hooks.ThinkingVisibility
		var eventBus *events.EventBus
		if tvCfg.Enabled {
			eventBus = events.NewEventBus(tvCfg.MaxEvents)
			hook.RegisterObservers(eventBus, tvCfg)
		}

		model := newAppModel(msgBus, agentSession)
		p := tea.NewProgram(model, tea.WithAltScreen())

		// 获取 CLIChannel 并设置回调
		var responseChan chan string = make(chan string, 1)
		cliChan, err := channelManager.Get(ctx, "cli")
		if err == nil {
			if c, ok := cliChan.(*channel.CLIChannel); ok {
				c.SetOnResponse(func(content string) {
					responseChan <- content
				})
			}
		}

		// 响应通道 -> TUI
		go func() {
			for content := range responseChan {
				p.Send(agentResponseMsg{Content: content})
			}
		}()

		// EventBus -> TUI
		if eventBus != nil {
			go func() {
				for evt := range eventBus.Subscribe() {
					p.Send(thinkingEventMsg{Event: evt})
				}
			}()
			defer eventBus.Close()
		}

		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "TUI 错误: %v\n", err)
		}

		close(responseChan)
	}

	// 停止
	fmt.Println("Shutting down... Hope to see you next time!😊")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := gw.Stop(stopCtx); err != nil {
		logger.L().Error().Err(err).Msg("停止 Gateway 失败")
	}
	if err := channelManager.StopAll(ctx); err != nil {
		logger.L().Error().Err(err).Msg("停止 ChannelManager 失败")
	}
	if err := agentManager.StopAll(ctx); err != nil {
		logger.L().Error().Err(err).Msg("停止 AgentManager 失败")
	}
	os.Exit(0)
	return nil
}

// runDebugSessions 运行 session 分析器
func runDebugSessions() error {
	fmt.Println("============================================")
	fmt.Println("        Session 分析器")
	fmt.Println("============================================")
	fmt.Println()

	// 从单例获取配置
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("配置未初始化")
	}

	// 根据配置重新初始化日志
	logger.Setup(cfg.Log.Level, cfg.Log.ConsoleEnabled)

	// TODO:创建分析器

	return nil
}

// errMsg 用于错误传递
type errMsg struct {
	err error
}
