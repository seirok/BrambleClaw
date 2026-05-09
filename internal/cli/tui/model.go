package tui

import (
	"brambleclaw/internal/bus"
	"brambleclaw/internal/config"
	"brambleclaw/internal/config/structs"
	"brambleclaw/internal/events"
	"brambleclaw/internal/logger"
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// chatMessage 一条聊天消息
type chatMessage struct {
	Content   string
	IsUser    bool
	Timestamp time.Time
}

// AgentResponseMsg 用于传递 Agent 回复
type AgentResponseMsg struct {
	Content string
}

// ThinkingEventMsg 用于传递思考事件
type ThinkingEventMsg struct {
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
	focus           focusRegion
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

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Enter, k.Quit},
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

// errMsg 用于错误传递
type errMsg struct {
	err error
}

// NewAppModel 创建 TUI 模型
func NewAppModel(msgBus *bus.MessageBus, session string) appModel {
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

		sidebarWidth := 32
		mainContentWidth := m.width
		if m.sidebarEnabled {
			mainContentWidth = m.width - sidebarWidth
		}

		if mainContentWidth < 4 {
			mainContentWidth = 4
		}

		reservedHeight := 10
		availableHeight := m.height - reservedHeight

		if availableHeight < 10 {
			availableHeight = 10
		}

		chatHeight := int(float64(availableHeight) * 0.7)
		eventHeight := availableHeight - chatHeight

		m.viewport.Width = mainContentWidth - 4
		m.viewport.Height = chatHeight

		m.eventViewport.Width = mainContentWidth - 4
		m.eventViewport.Height = eventHeight

		if m.sidebarEnabled {
			m.sidebarViewport.Width = sidebarWidth - 4
		}

		m.textInput.Width = m.width - 4

		m.viewport.SetContent(m.renderMessages())
		m.eventViewport.SetContent(m.renderEvents())

		return m, nil

	case spinner.TickMsg:
		if m.waiting {
			m.spinner, spCmd = m.spinner.Update(msg)
		}

	case AgentResponseMsg:
		m.messages = append(m.messages, chatMessage{
			Content:   msg.Content,
			IsUser:    false,
			Timestamp: time.Now(),
		})
		m.waiting = false
		m.showBanner = false
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case ThinkingEventMsg:
		m.eventLog = append(m.eventLog, msg.Event)
		if len(m.eventLog) > 200 {
			m.eventLog = m.eventLog[len(m.eventLog)-200:]
		}
		m.eventViewport.SetContent(m.renderEvents())

		if m.sidebarEnabled {
			event := msg.Event

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
			m.sidebarStats.HookCounts[category]++

			if strings.Contains(strings.ToLower(event.Point), "error") {
				m.sidebarStats.HookErrors[category]++
			}

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

			if m.eventViewport.AtBottom() {
				m.eventViewport.GotoBottom()
			}
		}
	case sidebarTickMsg:
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
