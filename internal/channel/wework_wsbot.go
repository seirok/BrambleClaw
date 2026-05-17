package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"neoclaw/internal/bus"
	"neoclaw/internal/config/structs"
	"neoclaw/internal/logger"
	"sync"
	"time"
)

// WeWorkWsBotChannel 企业微信 WebSocket AI Bot 通道
type WeWorkWsBotChannel struct {
	base              *BaseChannelImpl
	config            structs.WeWorkWsBotConfig
	conn              *websocket.Conn
	ctx               context.Context
	cancel            context.CancelFunc
	connMu            sync.Mutex
	waitResponseMsg   map[string]weworkWsBotMsgInfo
	waitResponseMsgMu sync.RWMutex
	connected         bool
	stopChan          chan struct{}
}

type weworkWsBotMsgInfo struct {
	reqID      string
	msgTime    int64
	fromUserID string
	chatID     string
	streamID   string
}

type weworkWsBotRequest[T any] struct {
	Cmd    string            `json:"cmd"`
	Header map[string]string `json:"headers"`
	Body   T                 `json:"body,omitempty"`
}

type weworkWsBotResponse struct {
	Cmd     string                  `json:"cmd"`
	Header  map[string]string       `json:"headers"`
	ErrCode int                     `json:"errcode"`
	ErrMsg  string                  `json:"errmsg"`
	Body    weworkWsBotResponseData `json:"body"`
}

type weworkWsBotResponseFrom struct {
	UserID string `json:"userid"`
}

type weworkWsBotResponseText struct {
	Content string `json:"content"`
}

type weworkWsBotResponseData struct {
	MsgID    string                  `json:"msgid"`
	AibotID  string                  `json:"aibotid"`
	ChatID   string                  `json:"chatid"`
	ChatType string                  `json:"chattype"`
	From     weworkWsBotResponseFrom `json:"from"`
	MsgType  string                  `json:"msgtype"`
	Text     weworkWsBotResponseText `json:"text"`
}

type weworkWsBotMsgResponse struct {
	MsgType string `json:"msgtype"`
	Stream  struct {
		ID      string `json:"id"`
		Finish  bool   `json:"finish"`
		Content string `json:"content"`
	} `json:"stream"`
}

type weworkWsBotMsgPushData struct {
	ChatID   string `json:"chatid"`
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Content string `json:"content"`
	} `json:"markdown"`
}

// NewWeWorkWsBotChannel 创建企业微信 WebSocket AI Bot 通道
func NewWeWorkWsBotChannel(cfg structs.WeWorkWsBotConfig, bus *bus.MessageBus) (*WeWorkWsBotChannel, error) {
	if cfg.BotID == "" || cfg.SecretID == "" {
		return nil, fmt.Errorf("wework_wsbot: bot_id and secret_id are required")
	}

	url := cfg.URL
	if url == "" {
		url = "wss://openws.work.weixin.qq.com"
	}

	reconnectDelay := cfg.ReconnectDelay
	if reconnectDelay == 0 {
		reconnectDelay = 3
	}

	heartbeat := cfg.Heartbeat
	if heartbeat == 0 {
		heartbeat = 30
	}

	baseCfg := &BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowFrom,
	}
	base := NewBaseChannelImpl("wework_wsbot", baseCfg, bus)

	return &WeWorkWsBotChannel{
		base: base,
		config: structs.WeWorkWsBotConfig{
			Enabled:        cfg.Enabled,
			BotID:          cfg.BotID,
			SecretID:       cfg.SecretID,
			URL:            url,
			Reconnect:      cfg.Reconnect,
			ReconnectDelay: reconnectDelay,
			Heartbeat:      heartbeat,
			AllowFrom:      cfg.AllowFrom,
		},
		stopChan:        make(chan struct{}),
		waitResponseMsg: make(map[string]weworkWsBotMsgInfo),
	}, nil
}

// Name 返回通道名称
func (c *WeWorkWsBotChannel) Name() string {
	return c.base.Name()
}

// IsAllowed 检查用户是否允许
func (c *WeWorkWsBotChannel) IsAllowed(id string) bool {
	return c.base.IsAllowed(id)
}

// Start 启动企业微信 WebSocket AI Bot 通道
func (c *WeWorkWsBotChannel) Start(ctx context.Context) error {
	logger.L().Info().Str("url", c.config.URL).Msg("WeWork WsBot channel starting")

	c.ctx, c.cancel = context.WithCancel(ctx)

	if err := c.doConnect(); err != nil {
		return fmt.Errorf("wework_wsbot: websocket connect failed: %w", err)
	}

	go c.handleMessages()

	return nil
}

func (c *WeWorkWsBotChannel) doConnect() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.connected = false
	}

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(c.ctx, c.config.URL, nil)
	if err != nil {
		return err
	}

	c.conn = conn
	c.connected = true

	logger.L().Info().Msg("WeWork WsBot: WebSocket connected")

	subscribe := weworkWsBotRequest[map[string]string]{
		Cmd: "aibot_subscribe",
		Header: map[string]string{
			"req_id": uuid.New().String(),
		},
		Body: map[string]string{
			"bot_id": c.config.BotID,
			"secret": c.config.SecretID,
		},
	}

	return c.conn.WriteJSON(subscribe)
}

func (c *WeWorkWsBotChannel) handleMessages() {
	defer func() {
		c.connMu.Lock()
		c.connected = false
		c.connMu.Unlock()
		logger.L().Warn().Msg("WeWork WsBot: WebSocket disconnected")
	}()

	heartbeatTicker := time.NewTicker(time.Duration(c.config.Heartbeat) * time.Second)
	defer heartbeatTicker.Stop()

	messageChan := make(chan []byte, 100)
	errorChan := make(chan error, 1)

	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			default:
			}

			c.connMu.Lock()
			conn := c.conn
			c.connMu.Unlock()

			if conn == nil {
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				errorChan <- err
				return
			}
			messageChan <- message
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-heartbeatTicker.C:
			c.sendHeartbeat()
		case message := <-messageChan:
			c.handleMessage(message)
		case err := <-errorChan:
			logger.L().Warn().Err(err).Msg("WeWork WsBot: WebSocket read error")
			if c.config.Reconnect && c.ctx.Err() == nil {
				c.handleReconnect(err)
			}
			return
		}
	}
}

func (c *WeWorkWsBotChannel) handleReconnect(lastErr error) {
	logger.L().Info().Err(lastErr).Msg("WeWork WsBot: attempting reconnect")

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-time.After(time.Second * time.Duration(c.config.ReconnectDelay)):
			if err := c.doConnect(); err != nil {
				logger.L().Warn().Err(err).Msg("WeWork WsBot: reconnect failed")
				continue
			}
			go c.handleMessages()
			logger.L().Info().Msg("WeWork WsBot: reconnected successfully")
			return
		}
	}
}

func (c *WeWorkWsBotChannel) sendHeartbeat() {
	c.connMu.Lock()
	conn := c.conn
	if conn != nil {
		ping := weworkWsBotRequest[any]{
			Cmd: "ping",
			Header: map[string]string{
				"req_id": uuid.New().String(),
			},
		}
		_ = conn.WriteJSON(ping)
	}
	c.connMu.Unlock()

	c.waitResponseMsgMu.Lock()
	var removeKeys []string
	tnow := time.Now().Unix()
	for k, v := range c.waitResponseMsg {
		if v.msgTime == 0 || tnow-v.msgTime > 24*60*60 {
			removeKeys = append(removeKeys, k)
		}
	}
	for _, k := range removeKeys {
		delete(c.waitResponseMsg, k)
	}
	c.waitResponseMsgMu.Unlock()
}

// Stop 停止企业微信 WebSocket AI Bot 通道
func (c *WeWorkWsBotChannel) Stop(ctx context.Context) error {
	close(c.stopChan)
	if c.cancel != nil {
		c.cancel()
	}

	c.connMu.Lock()
	conn := c.conn
	c.conn = nil
	c.connected = false
	c.connMu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}

	logger.L().Info().Msg("WeWork WsBot channel stopped")
	return nil
}

// Send 发送消息
func (c *WeWorkWsBotChannel) Send(ctx context.Context, msg *bus.OutBoundMessage) error {
	c.waitResponseMsgMu.RLock()
	replyInfo, hasReplyInfo := c.waitResponseMsg[msg.ReplyTo]
	c.waitResponseMsgMu.RUnlock()

	if hasReplyInfo && replyInfo.msgTime > 0 {
		var resp weworkWsBotRequest[weworkWsBotMsgResponse]
		resp.Cmd = "aibot_respond_msg"
		resp.Header = map[string]string{
			"req_id": replyInfo.reqID,
		}
		resp.Body.MsgType = "stream"
		resp.Body.Stream.ID = replyInfo.streamID
		resp.Body.Stream.Finish = true
		resp.Body.Stream.Content = msg.Content
		return c.sendMessage(resp)
	}

	var push weworkWsBotRequest[weworkWsBotMsgPushData]
	push.Cmd = "aibot_send_msg"
	push.Header = map[string]string{
		"req_id": uuid.New().String(),
	}
	push.Body.ChatID = msg.ChatID
	push.Body.MsgType = "markdown"
	push.Body.Markdown.Content = msg.Content
	return c.sendMessage(push)
}

func (c *WeWorkWsBotChannel) sendMessage(v any) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn == nil || !c.connected {
		return fmt.Errorf("wework_wsbot: not connected")
	}
	return c.conn.WriteJSON(v)
}

func (c *WeWorkWsBotChannel) handleMessage(data []byte) {
	logger.L().Debug().Str("data", string(data)).Msg("WeWork WsBot: received message")

	var resp weworkWsBotResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		logger.L().Warn().Err(err).Msg("WeWork WsBot: failed to unmarshal message")
		return
	}

	if resp.Cmd == "" {
		return
	}

	if resp.Cmd == "ping" {
		return
	}

	if resp.ErrCode != 0 {
		logger.L().Warn().Int("errcode", resp.ErrCode).Str("errmsg", resp.ErrMsg).Msg("WeWork WsBot: error response")
		return
	}

	msg := resp.Body
	reqID := resp.Header["req_id"]

	if msg.MsgID != "" {
		c.waitResponseMsgMu.Lock()
		c.waitResponseMsg[msg.MsgID] = weworkWsBotMsgInfo{
			reqID:      reqID,
			msgTime:    time.Now().Unix(),
			fromUserID: msg.From.UserID,
			chatID:     msg.ChatID,
			streamID:   uuid.New().String(),
		}
		c.waitResponseMsgMu.Unlock()
	}

	if !c.IsAllowed(msg.From.UserID) {
		logger.L().Debug().Str("sender_id", msg.From.UserID).Msg("WeWork WsBot: sender not allowed")
		return
	}

	if msg.MsgType == "text" {
		inMsg := &bus.InBoundMessage{
			InChannel: "wework_wsbot",
			SenderID:  msg.From.UserID,
			ChatID:    msg.MsgID,
			Content:   msg.Text.Content,
			TimeStamp: time.Now(),
			Metadata: map[string]string{
				"req_id":    reqID,
				"chat_id":   msg.ChatID,
				"chat_type": msg.ChatType,
			},
		}
		if err := c.base.PublishInBoundMessage(context.Background(), inMsg); err != nil {
			logger.L().Error().Err(err).Msg("WeWork WsBot: failed to publish inbound message")
		}
	}
}
