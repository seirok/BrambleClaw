package channel

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"neoclaw/internal/bus"
	"neoclaw/internal/config/structs"
	"neoclaw/internal/logger"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// WeWorkChannel 企业微信 HTTP 回调通道
type WeWorkChannel struct {
	base           *BaseChannelImpl
	corpID         string
	agentID        string
	secret         string
	token          string
	encodingAESKey string
	webhookPort    int
	accessToken    string
	tokenExpiresAt int64
	mu             sync.Mutex
	httpClient     *http.Client
	cancel         context.CancelFunc
}

// NewWeWorkChannel 创建企业微信 HTTP 回调通道
func NewWeWorkChannel(cfg structs.WeWorkConfig, bus *bus.MessageBus) (*WeWorkChannel, error) {
	if cfg.CorpID == "" || cfg.Secret == "" || cfg.AgentID == "" {
		return nil, fmt.Errorf("wework: corp_id, secret and agent_id are required")
	}

	baseCfg := &BaseChannelConfig{
		Enabled:    cfg.Enabled,
		AllowedIDs: cfg.AllowFrom,
	}
	base := NewBaseChannelImpl("wework", baseCfg, bus)

	port := cfg.WebhookPort
	if port == 0 {
		port = 8766
	}

	return &WeWorkChannel{
		base:           base,
		corpID:         cfg.CorpID,
		agentID:        cfg.AgentID,
		secret:         cfg.Secret,
		token:          cfg.Token,
		encodingAESKey: cfg.EncodingAESKey,
		webhookPort:    port,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Name 返回通道名称
func (c *WeWorkChannel) Name() string {
	return c.base.Name()
}

// IsAllowed 检查用户是否允许
func (c *WeWorkChannel) IsAllowed(id string) bool {
	return c.base.IsAllowed(id)
}

// Start 启动企业微信通道
func (c *WeWorkChannel) Start(ctx context.Context) error {
	logger.L().Info().Int("port", c.webhookPort).Msg("WeWork channel starting")

	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	mux := http.NewServeMux()
	mux.HandleFunc("/wework/event", c.handleWebhook)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", c.webhookPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logger.L().Info().Int("port", c.webhookPort).Msg("WeWork webhook server started")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.L().Error().Err(err).Msg("WeWork webhook server error")
		}
	}()

	go func() {
		<-runCtx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
		logger.L().Info().Msg("WeWork webhook server stopped")
	}()

	return nil
}

// Stop 停止企业微信通道
func (c *WeWorkChannel) Stop(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	logger.L().Info().Msg("WeWork channel stopped")
	return nil
}

// Send 发送消息
func (c *WeWorkChannel) Send(ctx context.Context, msg *bus.OutBoundMessage) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s", token)

	payload := map[string]interface{}{
		"touser":  msg.ChatID,
		"msgtype": "text",
		"agentid": c.agentID,
		"text": map[string]string{
			"content": msg.Content,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("wework: json marshal failed: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("wework: http post failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("wework: json decode failed: %w", err)
	}

	if result.ErrCode != 0 {
		return fmt.Errorf("wework: send failed: %s", result.ErrMsg)
	}

	logger.L().Debug().Str("chat_id", msg.ChatID).Int("content_length", len(msg.Content)).Msg("WeWork message sent")
	return nil
}

func (c *WeWorkChannel) handleWebhook(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	signature := query.Get("msg_signature")
	timestamp := query.Get("timestamp")
	nonce := query.Get("nonce")
	echostr := query.Get("echostr")

	if r.Method == http.MethodGet {
		if !c.verifySignature(c.token, timestamp, nonce, echostr, signature) {
			logger.L().Warn().Msg("WeWork: invalid signature for GET")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if c.encodingAESKey != "" && echostr != "" {
			decrypted, err := c.decryptMsg(echostr)
			if err != nil {
				logger.L().Error().Err(err).Msg("WeWork: failed to decrypt echostr")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write(decrypted)
		} else {
			_, _ = w.Write([]byte(echostr))
		}
		return
	}

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logger.L().Error().Err(err).Msg("WeWork: failed to read request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var encryptedMsg struct {
		XMLName    xml.Name `xml:"xml"`
		ToUserName string   `xml:"ToUserName"`
		Encrypt    string   `xml:"Encrypt"`
		AgentID    string   `xml:"AgentID"`
	}

	var plainTextBytes []byte
	if err := xml.Unmarshal(bodyBytes, &encryptedMsg); err == nil && encryptedMsg.Encrypt != "" {
		if !c.verifySignature(c.token, timestamp, nonce, encryptedMsg.Encrypt, signature) {
			logger.L().Warn().Msg("WeWork: invalid signature for POST")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if c.encodingAESKey != "" {
			decrypted, err := c.decryptMsg(encryptedMsg.Encrypt)
			if err != nil {
				logger.L().Error().Err(err).Msg("WeWork: failed to decrypt message")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			plainTextBytes = decrypted
		} else {
			plainTextBytes = bodyBytes
		}
	} else {
		plainTextBytes = bodyBytes
	}

	var msg struct {
		XMLName      xml.Name `xml:"xml"`
		ToUserName   string   `xml:"ToUserName"`
		FromUserName string   `xml:"FromUserName"`
		CreateTime   int64    `xml:"CreateTime"`
		MsgType      string   `xml:"MsgType"`
		Content      string   `xml:"Content"`
		MsgID        string   `xml:"MsgId"`
		AgentID      string   `xml:"AgentID"`
	}

	if err := xml.Unmarshal(plainTextBytes, &msg); err != nil {
		logger.L().Error().Err(err).Str("body", string(plainTextBytes)).Msg("WeWork: failed to unmarshal XML")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !c.IsAllowed(msg.FromUserName) {
		logger.L().Debug().Str("sender_id", msg.FromUserName).Msg("WeWork: sender not allowed")
		w.WriteHeader(http.StatusOK)
		return
	}

	if msg.MsgType == "text" {
		inMsg := &bus.InBoundMessage{
			InChannel: "wework",
			SenderID:  msg.FromUserName,
			ChatID:    msg.FromUserName,
			Content:   strings.TrimSpace(msg.Content),
			TimeStamp: time.Unix(msg.CreateTime, 0),
			Metadata: map[string]string{
				"msg_id":   msg.MsgID,
				"agent_id": msg.AgentID,
			},
		}

		if inMsg.Content == "" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if err := c.base.PublishInBoundMessage(r.Context(), inMsg); err != nil {
			logger.L().Error().Err(err).Msg("WeWork: failed to publish inbound message")
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (c *WeWorkChannel) decryptMsg(encrypted string) ([]byte, error) {
	if c.encodingAESKey == "" {
		return nil, fmt.Errorf("wework: encoding_aes_key not configured")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("wework: base64 decode failed: %w", err)
	}

	key := c.encodingAESKey + "="
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("wework: key decode failed: %w", err)
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("wework: aes cipher failed: %w", err)
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("wework: ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("wework: ciphertext not a multiple of block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)

	padding := int(ciphertext[len(ciphertext)-1])
	if padding < 1 || padding > aes.BlockSize {
		return nil, fmt.Errorf("wework: invalid padding")
	}
	ciphertext = ciphertext[:len(ciphertext)-padding]

	if len(ciphertext) < 16 {
		return nil, fmt.Errorf("wework: decrypted text too short")
	}
	content := ciphertext[16:]

	if len(content) < 4 {
		return nil, fmt.Errorf("wework: content too short for length header")
	}

	msgLen := int(content[0])<<24 | int(content[1])<<16 | int(content[2])<<8 | int(content[3])
	if len(content) < 4+msgLen {
		return nil, fmt.Errorf("wework: content too short for message")
	}

	message := content[4 : 4+msgLen]

	if len(content) < 4+msgLen+len(c.corpID) {
		return nil, fmt.Errorf("wework: content too short for corp_id")
	}
	receivedCorpID := string(content[4+msgLen : 4+msgLen+len(c.corpID)])
	if receivedCorpID != c.corpID {
		return nil, fmt.Errorf("wework: corp_id mismatch: expected %s, got %s", c.corpID, receivedCorpID)
	}

	return message, nil
}

func (c *WeWorkChannel) computeSignature(token, timestamp, nonce, data string) string {
	strs := []string{token, timestamp, nonce, data}
	sort.Strings(strs)
	joined := strings.Join(strs, "")
	h := sha1.New()
	h.Write([]byte(joined))
	return hex.EncodeToString(h.Sum(nil))
}

func (c *WeWorkChannel) verifySignature(token, timestamp, nonce, data, signature string) bool {
	expected := c.computeSignature(token, timestamp, nonce, data)
	if expected != signature {
		logger.L().Debug().Str("expected", expected).Str("received", signature).Msg("WeWork: signature mismatch")
		return false
	}
	return true
}

func (c *WeWorkChannel) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Unix() < c.tokenExpiresAt {
		return c.accessToken, nil
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s", c.corpID, c.secret)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("wework: http request create failed: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("wework: http get failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("wework: json decode failed: %w", err)
	}

	if result.ErrCode != 0 {
		return "", fmt.Errorf("wework: get token failed: %s", result.ErrMsg)
	}

	c.accessToken = result.AccessToken
	c.tokenExpiresAt = time.Now().Unix() + result.ExpiresIn - 200
	return c.accessToken, nil
}
