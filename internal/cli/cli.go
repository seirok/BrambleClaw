package cli

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/agent"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/channel"
	"brambleclaw/internal/config"
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

// appModel TUI 状态
type appModel struct {
	textInput     textinput.Model
	viewport      viewport.Model
	eventViewport viewport.Model
	spinner       spinner.Model
	help          help.Model
	keys          keyMap
	messages      []chatMessage
	eventLog      []events.ThinkingEvent
	waiting       bool
	showBanner    bool
	width         int
	height        int
	msgBus        *bus.MessageBus
	agentSession  string
	quitting      bool
	err           error
	eventFocused  bool // true=event 面板有焦点
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

func newAppModel(msgBus *bus.MessageBus, session string) appModel {
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

	return appModel{
		textInput:     ti,
		viewport:      vp,
		eventViewport: eventVp,
		spinner:       s,
		messages:      []chatMessage{},
		eventLog:      []events.ThinkingEvent{},
		waiting:       false,
		showBanner:    true,
		msgBus:        msgBus,
		agentSession:  session,
		help:          help.New(),
	}
}

func (m appModel) Init() tea.Cmd {
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
			// 切换焦点
			m.eventFocused = !m.eventFocused
			if m.eventFocused {
				m.textInput.Blur()
			} else {
				m.textInput.Focus()
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
		totalHeight := msg.Height - 4 // 输入 + help
		chatHeight := totalHeight * 3 / 4
		eventHeight := totalHeight - chatHeight - 1 // -1 for divider
		if eventHeight < 3 {
			eventHeight = 3
			chatHeight = totalHeight - eventHeight - 1
		}
		m.viewport.Width = msg.Width
		m.viewport.Height = chatHeight
		m.eventViewport.Width = msg.Width
		m.eventViewport.Height = eventHeight
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
		// 如果在底部，自动滚动到底部
		if m.eventViewport.AtBottom() {
			m.eventViewport.GotoBottom()
		}

	case errMsg:
		m.err = msg.err
		return m, tea.Quit
	}

	m.textInput, tiCmd = m.textInput.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.eventViewport, evpCmd = m.eventViewport.Update(msg)

	return m, tea.Batch(tiCmd, vpCmd, evpCmd, spCmd)
}

// renderMessages 渲染消息列表为字符串
func (m appModel) renderMessages() string {
	var messagesView string

	if m.showBanner && len(m.messages) == 0 {
		// Banner
		color1 := lipgloss.NewStyle().Foreground(lipgloss.Color("8;2;138;43;226"))
		color2 := lipgloss.NewStyle().Foreground(lipgloss.Color("8;2;112;60;212"))
		color3 := lipgloss.NewStyle().Foreground(lipgloss.Color("8;2;87;77;199"))
		color4 := lipgloss.NewStyle().Foreground(lipgloss.Color("8;2;62;93;185"))

		banner :=
			color1.Render(`██████╗ ██████╗  █████╗ ███╗   ███╗██████╗ ██╗     ███████╗    ██████╗██╗       █████╗ ██╗    ██╗`) + "\n" +
				color2.Render(`██╔══██╗██╔══██╗██╔══██╗████╗ ████║██╔══██╗██║     ██╔════╝   ██╔════╝██║      ██╔══██╗██║    ██║`) + "\n" +
				color2.Render(`██████╔╝██████╔╝███████║██╔████╔██║██████╔╝██║     █████╗     ██║     ██║      ███████║██║ █╗ ██║`) + "\n" +
				color3.Render(`██╔══██╗██╔══██╗██╔══██║██║╚██╔╝██║██╔══██╗██║     ██╔══╝     ██║     ██║      ██╔══██║██║███╗██║`) + "\n" +
				color3.Render(`██████╔╝██║  ██║██║  ██║██║ ╚═╝ ██║██████╔╝███████╗███████╗    ╚██████╗███████╗██║  ██║╚███╔███╔╝`) + "\n" +
				color4.Render(`╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝     ╚═╝╚═════╝ ╚══════╝╚══════╝     ╚═════╝╚══════╝╚═╝  ╚═╝ ╚══╝╚══╝ `)

		messagesView = banner + "\n\n" +
			subtleStyle.Render("> 你好，请问有什么可以帮您？") + "\n\n"
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

	// 设置 viewport 内容
	m.viewport.SetContent(m.renderMessages())
	m.eventViewport.SetContent(m.renderEvents())

	// 分割线
	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(strings.Repeat("─", m.width))

	// 面板标题
	var titleStr string
	if m.eventFocused {
		titleStr = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true).
			Render("● [Thinking Events]")
	} else {
		titleStr = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("○ [Thinking Events]")
	}

	// 输入区域
	inputView := m.textInput.View()
	helpView := m.help.View(keys)
	helpStyle := lipgloss.NewStyle().MarginTop(1)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
		divider,
		titleStr,
		m.eventViewport.View(),
		"",
		inputView,
		helpStyle.Render(helpView),
	)
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
