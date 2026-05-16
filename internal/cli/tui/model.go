package tui

import (
	"brambleclaw/internal/agent"
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
	"github.com/charmbracelet/bubbles/list"
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
	IsError   bool
	Timestamp time.Time
}

// AgentResponseMsg 用于传递 Agent 回复
type AgentResponseMsg struct {
	Content string
	MsgType string
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
	textInput         textinput.Model
	viewport          viewport.Model
	eventViewport     viewport.Model
	sidebarViewport   viewport.Model
	spinner           spinner.Model
	help              help.Model
	keys              keyMap
	messages          []chatMessage
	eventLog          []events.ThinkingEvent
	waiting           bool
	showBanner        bool
	width             int
	height            int
	msgBus            *bus.MessageBus
	//	currentChatID   string
	quitting          bool
	err               error
	eventFocused      bool // true=event 面板有焦点
	sidebarEnabled    bool
	sidebarWidth      int
	sidebarStats      sidebarStats
	sidebarSections   []structs.SidebarSection
	focus             focusRegion
	mode              appMode
	resumeList        list.Model
	pendingDeleteItem *sessionItem
	inputHistory      []string // ring buffer of submitted inputs, max 20
	historyIdx        int      // current navigation index (-1 = not browsing)
	historyDraft      string   // saves current draft when user starts browsing history
	// 	session         *session.Session
	// agentName       string
	agent *agent.Agent
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

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")) // 红色

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

type appMode int

const (
	modeInput appMode = iota
	modeWaiting
	modeResume
	modeDelete
	modeDeleteConfirm
)

// errMsg 用于错误传递
type errMsg struct {
	err error
}

// NewAppModel 创建 TUI 模型
func NewAppModel(msgBus *bus.MessageBus, agent *agent.Agent) appModel {
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
		help:            help.New(),
		sidebarEnabled:  sidebarCfg.Enabled,
		sidebarWidth:    sidebarCfg.Width,
		sidebarSections: sidebarCfg.Sections,
		sidebarStats: sidebarStats{
			HookCounts: make(map[string]int64),
			HookErrors: make(map[string]int64),
			HookAvgMs:  make(map[string]float64),
		},
		inputHistory: []string{},
		historyIdx:   -1,
		historyDraft: "",
		mode:         modeInput,
		agent:        agent,
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
	switch m.mode {
	case modeResume:
		return m.updateResume(msg)
	case modeDelete:
		return m.updateDelete(msg)
	case modeDeleteConfirm:
		return m.updateDeleteConfirm(msg)
	default: // modeInput and modeWaiting
		return m.updateNormal(msg)
	}
}

// updateNormal handles key messages in input and waiting modes
func (m appModel) updateNormal(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.eventFocused = (m.focus == focusEvent)
			// 只有在输入区时，textInput 才获取焦点
			if m.focus == focusInput {
				return m, m.textInput.Focus()
			} else {
				m.textInput.Blur()
				return m, nil
			}

		case tea.KeyEnter:
			if m.mode == modeWaiting || m.eventFocused {
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
			if input == "/resume" {
				m.saveCurrentSession()
				newModel, cmd, err := m.initSessionList("Resume Session - Select a conversation")
				if err != nil {
					logger.L().Error().Err(err).Msg("Failed to init resume list")
					return m, nil
				}
				newModel.mode = modeResume
				return newModel, cmd
			}

			if input == "/delete" {
				m.saveCurrentSession()
				newModel, cmd, err := m.initSessionList("Delete Session - Select a conversation to delete")
				if err != nil {
					logger.L().Error().Err(err).Msg("Failed to init delete list")
					return m, nil
				}
				// Filter out current session
				currentChatID := m.agent.GetSession().GetMetadata().ChatID
				var filteredItems []list.Item
				for _, item := range newModel.resumeList.Items() {
					if si, ok := item.(sessionItem); ok && si.chatID != currentChatID {
						filteredItems = append(filteredItems, item)
					}
				}
				newModel.resumeList.SetItems(filteredItems)
				newModel.mode = modeDelete
				return newModel, cmd
			}

			if input == "/clear" {
				// 创建新会话
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				sess := m.agent.GetSession()
				err := sess.Clear(ctx)
				if err != nil {
					return nil, nil
				}

				// 清空消息显示
				m.messages = []chatMessage{}
				m.textInput.SetValue("")
				m.viewport.SetContent(m.renderMessages())
				m.showBanner = true
				return m, nil
			}

			// 发送用户消息
			m.messages = append(m.messages, chatMessage{
				Content:   input,
				IsUser:    true,
				Timestamp: time.Now(),
			})
			m.showBanner = false
			m.textInput.SetValue("")

			// Save input to history
			m.inputHistory = append(m.inputHistory, input)
			if len(m.inputHistory) > 20 {
				m.inputHistory = m.inputHistory[len(m.inputHistory)-20:]
			}
			m.historyIdx = -1
			m.historyDraft = ""

			// 发布到总线
			go func() {
				inboundMsg := &bus.InBoundMessage{
					InChannel: "cli",
					SenderID:  "user",
					ChatID:    m.agent.GetSession().GetMetadata().ChatID,
					Content:   input,
					TimeStamp: time.Now(),
				}

				ctx := context.Background()
				if err := m.msgBus.PublishInBoundMessage(ctx, inboundMsg); err != nil {
					logger.L().Error().Err(err).Msg("Failed to send message")
				}
			}()

			m.mode = modeWaiting
			return m, m.spinner.Tick

		case tea.KeyUp:
			switch m.focus {
			case focusChat:
				m.viewport.ScrollUp(1)
			case focusEvent:
				m.eventViewport.ScrollUp(1)
			case focusInput:
				// Navigate to older history entry
				if len(m.inputHistory) > 0 {
					if m.historyIdx == -1 {
						// Start browsing history
						m.historyDraft = m.textInput.Value()
						m.historyIdx = len(m.inputHistory) - 1
					} else if m.historyIdx > 0 {
						// Move to older entry
						m.historyIdx--
					}
					// Update input with history entry
					m.textInput.SetValue(m.inputHistory[m.historyIdx])
					// Move cursor to end
					m.textInput, _ = m.textInput.Update(tea.KeyMsg{Type: tea.KeyEnd})
				}
			}

		case tea.KeyDown:
			switch m.focus {
			case focusChat:
				m.viewport.ScrollDown(1)
			case focusEvent:
				m.eventViewport.ScrollDown(1)
			case focusInput:
				// Navigate to newer history entry or back to draft
				if m.historyIdx >= 0 {
					m.historyIdx++
					if m.historyIdx >= len(m.inputHistory) {
						// Past newest entry, restore draft
						m.textInput.SetValue(m.historyDraft)
						m.historyIdx = -1
					} else {
						// Update input with newer history entry
						m.textInput.SetValue(m.inputHistory[m.historyIdx])
					}
					// Move cursor to end
					m.textInput, _ = m.textInput.Update(tea.KeyMsg{Type: tea.KeyEnd})
				}
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
		if m.mode == modeWaiting {
			m.spinner, spCmd = m.spinner.Update(msg)
		}

	case AgentResponseMsg:
		m.messages = append(m.messages, chatMessage{
			Content:   msg.Content,
			IsUser:    false,
			IsError:   msg.MsgType == "error",
			Timestamp: time.Now(),
		})
		m.mode = modeInput
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

	if m.focus == focusInput {
		m.textInput, tiCmd = m.textInput.Update(msg)
	}

	switch m.focus {
	case focusChat:
		// 只有焦点在对话区时，对话区才响应 Update（处理滚动等）
		m.viewport, vpCmd = m.viewport.Update(msg)
	case focusEvent:
		// 只有焦点在思考区时，思考区才响应 Update
		m.eventViewport, evpCmd = m.eventViewport.Update(msg)
	default:
		// 当焦点在输入框时，如果你希望此时按上下键也能滚动对话区，可以保留下面这行
		// 如果不希望滚动，则留空
		m.viewport, vpCmd = m.viewport.Update(msg)
	}
	m.sidebarViewport, _ = m.sidebarViewport.Update(msg)

	return m, tea.Batch(tiCmd, vpCmd, evpCmd, spCmd)
}

// saveCurrentSession saves the current session to disk if it's valid
func (m appModel) saveCurrentSession() {
	if m.agent == nil {
		logger.L().Fatal().Msg("main agent is nil")
	}
	sess := m.agent.GetSession()
	if sess != nil {
		if sess.IsValid() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := sess.Save(ctx)
			if err != nil {
				logger.L().Error().Msg("Failed to save current session")
			}
		}
	}
}
