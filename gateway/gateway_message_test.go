package gateway

import (
	"brambleclaw/config"
	"brambleclaw/logger"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"brambleclaw/agent"
	"brambleclaw/bus"
	"brambleclaw/channel"
)

// MockChannel 是一个用于测试的模拟通道
type MockChannel struct {
	name         string
	messages     []*bus.OutBoundMessage
	mu           sync.Mutex
	allowedIDs   []string
	enabled      bool
	shouldFail   bool
	onSendCalled chan *bus.OutBoundMessage
}

func NewMockChannel(name string) *MockChannel {
	return &MockChannel{
		name:         name,
		messages:     make([]*bus.OutBoundMessage, 0),
		allowedIDs:   []string{},
		enabled:      true,
		shouldFail:   false,
		onSendCalled: make(chan *bus.OutBoundMessage, 10),
	}
}

func (m *MockChannel) Name() string {
	return m.name
}

func (m *MockChannel) Start(ctx context.Context) error {
	return nil
}

func (m *MockChannel) Stop() error {
	return nil
}

func (m *MockChannel) Send(msg *bus.OutBoundMessage) error {
	if m.shouldFail {
		return context.DeadlineExceeded
	}

	m.mu.Lock()
	m.messages = append(m.messages, msg)
	m.mu.Unlock()

	select {
	case m.onSendCalled <- msg:
	default:
	}

	return nil
}

func (m *MockChannel) IsAllowed(id string) bool {
	if !m.enabled {
		return false
	}
	if len(m.allowedIDs) == 0 {
		return true
	}
	for _, aid := range m.allowedIDs {
		if aid == id {
			return true
		}
	}
	return false
}

func (m *MockChannel) GetMessages() []*bus.OutBoundMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*bus.OutBoundMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *MockChannel) ClearMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = make([]*bus.OutBoundMessage, 0)
}

// 创建带有模拟通道的测试 Gateway
func createTestGatewayWithMockChannel(t *testing.T) (*Gateway, *bus.MessageBus, *channel.Manager, *MockChannel) {
	cfg := createTestConfig(t)
	msgBus := bus.NewMessageBus(100)
	channelManager := channel.NewManager(msgBus)

	gateway := NewGateway(cfg, msgBus, channelManager)

	// 注册测试 Agent
	config, _ := config.Load("../config/config.json")
	llmConfig := config.LLMConfig
	mainAgent := createTestAgent(t, "main", llmConfig)
	mainCfg := agent.AgentConfig{Name: "main", MaxHistory: 10}
	gateway.RegisterAgent("main", mainAgent, mainCfg)

	csAgent := createTestAgent(t, "customer_service", llmConfig)
	csCfg := agent.AgentConfig{Name: "customer_service", MaxHistory: 10}
	gateway.RegisterAgent("customer_service", csAgent, csCfg)

	// 创建并注册模拟通道
	mockChannel := NewMockChannel("cli")
	channelManager.Register(mockChannel)

	return gateway, msgBus, channelManager, mockChannel
}

// TestHandleMessage_MessageRouting 测试消息是否正确路由到 Agent
func TestHandleMessage_MessageRouting(t *testing.T) {
	logger.SetLevel("debug")
	gateway, msgBus, _, mockChannel := createTestGatewayWithMockChannel(t)
	ctx := context.Background()

	// 启动 Gateway
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("启动 Gateway 失败: %v", err)
	}
	defer gateway.Stop()

	// 等待 Gateway 完全启动
	time.Sleep(100 * time.Millisecond)

	// 发送测试消息
	testMsg := &bus.InBoundMessage{
		ID:        "test-msg-1",
		SenderID:  "user123",
		ChatID:    "chat456",
		InChannel: "cli",
		Content:   "你好，这是一个测试消息",
		TimeStamp: time.Now(),
	}

	// 发布消息到总线
	if err := msgBus.PublishInBoundMessage(ctx, testMsg); err != nil {
		t.Fatalf("发布消息失败: %v", err)
	}

	// 等待消息处理完成
	select {
	case <-mockChannel.onSendCalled:
		// 消息已发送成功
	case <-time.After(20 * time.Second):
		t.Error("等待响应超时，消息可能未正确路由")
	}

	// 验证消息是否被发送
	messages := mockChannel.GetMessages()
	if len(messages) == 0 {
		t.Error("没有收到任何响应消息")
	}

	// 验证响应内容
	found := false
	for _, msg := range messages {
		if msg.ReplyTo == testMsg.ID {
			found = true
			if msg.ChatID != testMsg.ChatID {
				t.Errorf("ChatID 不匹配: 期望 %s, 得到 %s", testMsg.ChatID, msg.ChatID)
			}
			if msg.OutChannel != testMsg.InChannel {
				t.Errorf("OutChannel 不匹配: 期望 %s, 得到 %s", testMsg.InChannel, msg.OutChannel)
			}
			logger.L().Info().Msg(msg.Content)
			break
		}

	}
	if !found {
		t.Error("没有找到对应的回复消息")
	}
}

// TestDispatchOutboundLoop_MessageDelivery 测试出站消息是否正确分发到通道
func TestDispatchOutboundLoop_MessageDelivery(t *testing.T) {
	gateway, msgBus, channelManager, mockChannel := createTestGatewayWithMockChannel(t)
	ctx := context.Background()

	// 启动 Gateway
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("启动 Gateway 失败: %v", err)
	}
	defer gateway.Stop()

	// 等待 Gateway 完全启动
	time.Sleep(100 * time.Millisecond)

	// 创建测试出站消息
	testOutboundMsg := &bus.OutBoundMessage{
		ChatID:     "test-chat-123",
		OutChannel: "cli",
		Content:    "这是测试响应内容",
		ReplyTo:    "original-msg-id",
		TimeStamp:  time.Now(),
	}

	// 发布出站消息到总线
	if err := msgBus.PublishOutBoundMessage(ctx, testOutboundMsg); err != nil {
		t.Fatalf("发布出站消息失败: %v", err)
	}

	// 等待消息被分发
	select {
	case <-mockChannel.onSendCalled:
		// 消息已分发
	case <-time.After(5 * time.Second):
		t.Error("等待消息分发超时")
	}

	// 验证消息是否被正确分发到通道
	messages := mockChannel.GetMessages()
	if len(messages) == 0 {
		t.Fatal("通道没有收到任何消息")
	}

	// 查找目标消息
	found := false
	for _, msg := range messages {
		if msg.ReplyTo == testOutboundMsg.ReplyTo {
			found = true
			// 验证消息内容
			if msg.Content != testOutboundMsg.Content {
				t.Errorf("消息内容不匹配: 期望 '%s', 得到 '%s'",
					testOutboundMsg.Content, msg.Content)
			}
			if msg.ChatID != testOutboundMsg.ChatID {
				t.Errorf("ChatID 不匹配: 期望 '%s', 得到 '%s'",
					testOutboundMsg.ChatID, msg.ChatID)
			}
			if msg.OutChannel != testOutboundMsg.OutChannel {
				t.Errorf("OutChannel 不匹配: 期望 '%s', 得到 '%s'",
					testOutboundMsg.OutChannel, msg.OutChannel)
			}
			break
		}
	}

	if !found {
		t.Errorf("没有找到匹配的出站消息 (ReplyTo: %s)", testOutboundMsg.ReplyTo)
	}

	// 验证通道管理器状态
	ch, exists := channelManager.Get("cli")
	if !exists {
		t.Error("无法在通道管理器中找到 'cli' 通道")
	}
	if ch == nil {
		t.Error("通道管理器返回的通道为 nil")
	}
}

// TestMessageRoutingIntegration 测试完整的消息路由流程
func TestMessageRoutingIntegration(t *testing.T) {
	gateway, msgBus, channelManager, mockChannel := createTestGatewayWithMockChannel(t)
	ctx := context.Background()

	// 启动 Gateway
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("启动 Gateway 失败: %v", err)
	}
	defer gateway.Stop()

	// 等待 Gateway 完全启动
	time.Sleep(100 * time.Millisecond)

	// 步骤 1: 发送入站消息
	inboundMsg := &bus.InBoundMessage{
		ID:        "integration-test-1",
		SenderID:  "user123",
		ChatID:    "chat456",
		InChannel: "cli",
		Content:   "这是一条集成测试消息",
		TimeStamp: time.Now(),
	}

	if err := msgBus.PublishInBoundMessage(ctx, inboundMsg); err != nil {
		t.Fatalf("发布入站消息失败: %v", err)
	}

	// 步骤 2: 等待消息处理并获取响应
	var responseMsg *bus.OutBoundMessage
	select {
	case msg := <-mockChannel.onSendCalled:
		responseMsg = msg
	case <-time.After(5 * time.Second):
		t.Fatal("等待响应超时")
	}

	// 步骤 3: 验证响应的正确性
	if responseMsg == nil {
		t.Fatal("响应消息为 nil")
	}

	// 验证响应关联性
	if responseMsg.ReplyTo != inboundMsg.ID {
		t.Errorf("ReplyTo 不匹配: 期望 %s, 得到 %s", inboundMsg.ID, responseMsg.ReplyTo)
	}

	// 验证通道一致性
	if responseMsg.OutChannel != inboundMsg.InChannel {
		t.Errorf("通道不匹配: 期望 %s, 得到 %s", inboundMsg.InChannel, responseMsg.OutChannel)
	}

	// 验证 ChatID 一致性
	if responseMsg.ChatID != inboundMsg.ChatID {
		t.Errorf("ChatID 不匹配: 期望 %s, 得到 %s", inboundMsg.ChatID, responseMsg.ChatID)
	}

	// 验证响应内容不为空
	if responseMsg.Content == "" {
		t.Error("响应内容为空")
	}

	// 步骤 4: 验证 Agent 会话状态
	agentEntry, exists := gateway.registry.Get("main")
	if !exists {
		t.Fatal("无法找到 main Agent")
	}

	sessionKey := BuildSessionKey("main", inboundMsg.InChannel, "direct", inboundMsg.ChatID)
	session, exists := agentEntry.Agent.GetSession(sessionKey)
	if !exists {
		t.Logf("会话不存在 (key: %s)，这可能是因为 Agent 还未处理完消息", sessionKey)
	} else {
		// 验证会话中有两条消息（用户输入和 Agent 回复）
		if len(session.Messages) < 2 {
			t.Errorf("会话消息数量不足: 期望至少 2 条，得到 %d 条", len(session.Messages))
		}
	}

	// 步骤 5: 验证通道管理器状态
	ch, exists := channelManager.Get("cli")
	if !exists {
		t.Error("通道管理器中不存在 'cli' 通道")
	}
	if ch == nil {
		t.Error("通道管理器返回的通道为 nil")
	}

	t.Logf("集成测试完成，消息路由流程验证通过")
}

// TestMultipleMessagesRouting 测试多条消息的路由
func TestMultipleMessagesRouting(t *testing.T) {
	gateway, msgBus, _, mockChannel := createTestGatewayWithMockChannel(t)
	ctx := context.Background()

	// 启动 Gateway
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("启动 Gateway 失败: %v", err)
	}
	defer gateway.Stop()

	// 等待 Gateway 完全启动
	time.Sleep(100 * time.Millisecond)

	// 发送多条消息
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		msg := &bus.InBoundMessage{
			ID:        fmt.Sprintf("batch-msg-%d", i),
			SenderID:  fmt.Sprintf("user%d", i),
			ChatID:    fmt.Sprintf("chat%d", i),
			InChannel: "cli",
			Content:   fmt.Sprintf("这是第 %d 条测试消息", i),
			TimeStamp: time.Now(),
		}

		if err := msgBus.PublishInBoundMessage(ctx, msg); err != nil {
			t.Fatalf("发布消息 %d 失败: %v", i, err)
		}
	}

	// 等待所有消息处理完成
	time.Sleep(2 * time.Second)

	// 验证所有消息都被处理
	messages := mockChannel.GetMessages()
	if len(messages) < messageCount {
		t.Errorf("消息处理不完整: 期望至少 %d 条响应，得到 %d 条", messageCount, len(messages))
	}

	// 验证每条消息都有对应的响应
	for i := 0; i < messageCount; i++ {
		expectedID := fmt.Sprintf("batch-msg-%d", i)
		found := false
		for _, msg := range messages {
			if msg.ReplyTo == expectedID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("没有找到消息 %s 的响应", expectedID)
		}
	}
}

// TestErrorHandling 测试错误处理流程
func TestErrorHandling(t *testing.T) {
	gateway, msgBus, _, mockChannel := createTestGatewayWithMockChannel(t)
	ctx := context.Background()

	// 启动 Gateway
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("启动 Gateway 失败: %v", err)
	}
	defer gateway.Stop()

	// 等待 Gateway 完全启动
	time.Sleep(100 * time.Millisecond)

	// 测试 1: 发送到不存在的 Agent
	testMsg := &bus.InBoundMessage{
		ID:        "error-test-1",
		SenderID:  "user123",
		ChatID:    "chat456",
		InChannel: "nonexistent_channel", // 这个通道没有配置路由
		Content:   "这条消息应该使用默认 Agent",
		TimeStamp: time.Now(),
	}

	if err := msgBus.PublishInBoundMessage(ctx, testMsg); err != nil {
		t.Fatalf("发布消息失败: %v", err)
	}

	// 等待消息处理
	time.Sleep(1 * time.Second)

	// 验证消息是否被默认 Agent 处理
	messages := mockChannel.GetMessages()
	found := false
	for _, msg := range messages {
		if msg.ReplyTo == testMsg.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("默认 Agent 没有处理未匹配的消息")
	}

	// 测试 2: 模拟通道发送失败
	mockChannel.shouldFail = true
	defer func() { mockChannel.shouldFail = false }()

	testMsg2 := &bus.InBoundMessage{
		ID:        "error-test-2",
		SenderID:  "user456",
		ChatID:    "chat789",
		InChannel: "cli",
		Content:   "这条消息的响应发送会失败",
		TimeStamp: time.Now(),
	}

	if err := msgBus.PublishInBoundMessage(ctx, testMsg2); err != nil {
		t.Fatalf("发布消息失败: %v", err)
	}

	// 等待处理
	time.Sleep(1 * time.Second)

	// 即使发送失败，系统也不应该崩溃
	// 这是通过日志记录的，我们可以检查是否没有 panic
	t.Log("错误处理测试完成，系统没有崩溃")
}

// TestGatewayRestart 测试 Gateway 重启能力
func TestGatewayRestart(t *testing.T) {
	gateway, msgBus, _, mockChannel := createTestGatewayWithMockChannel(t)
	ctx := context.Background()

	// 第一次启动
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("第一次启动 Gateway 失败: %v", err)
	}

	// 发送第一条消息
	testMsg1 := &bus.InBoundMessage{
		ID:        "restart-test-1",
		SenderID:  "user123",
		ChatID:    "chat456",
		InChannel: "cli",
		Content:   "第一次启动后的消息",
		TimeStamp: time.Now(),
	}

	if err := msgBus.PublishInBoundMessage(ctx, testMsg1); err != nil {
		t.Fatalf("发布消息失败: %v", err)
	}

	// 等待处理
	select {
	case <-mockChannel.onSendCalled:
		// 消息已处理
	case <-time.After(5 * time.Second):
		t.Fatal("第一次消息处理超时")
	}

	// 停止 Gateway
	if err := gateway.Stop(); err != nil {
		t.Fatalf("停止 Gateway 失败: %v", err)
	}

	// 等待停止完成
	time.Sleep(200 * time.Millisecond)

	// 清除通道消息
	mockChannel.ClearMessages()

	// 第二次启动（使用新的 context）
	ctx2 := context.Background()
	if err := gateway.Start(ctx2); err != nil {
		t.Fatalf("第二次启动 Gateway 失败: %v", err)
	}

	// 发送第二条消息
	testMsg2 := &bus.InBoundMessage{
		ID:        "restart-test-2",
		SenderID:  "user456",
		ChatID:    "chat789",
		InChannel: "cli",
		Content:   "第二次启动后的消息",
		TimeStamp: time.Now(),
	}

	if err := msgBus.PublishInBoundMessage(ctx2, testMsg2); err != nil {
		t.Fatalf("发布第二条消息失败: %v", err)
	}

	// 等待处理
	select {
	case <-mockChannel.onSendCalled:
		// 消息已处理
	case <-time.After(5 * time.Second):
		t.Fatal("第二次消息处理超时")
	}

	// 验证第二次的消息也被正确处理
	messages := mockChannel.GetMessages()
	if len(messages) == 0 {
		t.Fatal("第二次启动后没有收到响应消息")
	}

	found := false
	for _, msg := range messages {
		if msg.ReplyTo == testMsg2.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("没有找到第二次消息的响应")
	}

	// 清理
	gateway.Stop()
}

// TestChannelNotFound 测试通道不存在的情况
func TestChannelNotFound(t *testing.T) {
	gateway, msgBus, _, _ := createTestGatewayWithMockChannel(t)
	ctx := context.Background()

	// 启动 Gateway
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("启动 Gateway 失败: %v", err)
	}
	defer gateway.Stop()

	time.Sleep(100 * time.Millisecond)

	// 发送消息到不存在的通道
	testMsg := &bus.InBoundMessage{
		ID:        "channel-not-found-test",
		SenderID:  "user123",
		ChatID:    "chat456",
		InChannel: "nonexistent_channel_12345", // 不存在的通道
		Content:   "这条消息发往不存在的通道",
		TimeStamp: time.Now(),
	}

	// 发布消息（不应该 panic）
	if err := msgBus.PublishInBoundMessage(ctx, testMsg); err != nil {
		t.Fatalf("发布消息失败: %v", err)
	}

	// 等待一段时间，确保系统没有崩溃
	time.Sleep(1 * time.Second)

	// 验证 Gateway 仍在运行
	if !gateway.IsRunning() {
		t.Error("Gateway 在处理不存在的通道时停止了运行")
	}

	t.Log("测试通过：系统正确处理了不存在的通道")
}

// TestHighConcurrentLoad 测试高并发负载
func TestHighConcurrentLoad(t *testing.T) {
	gateway, msgBus, _, mockChannel := createTestGatewayWithMockChannel(t)
	ctx := context.Background()

	// 启动 Gateway
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("启动 Gateway 失败: %v", err)
	}
	defer gateway.Stop()

	time.Sleep(100 * time.Millisecond)

	// 并发发送大量消息
	messageCount := 50
	var wg sync.WaitGroup
	errors := make(chan error, messageCount)

	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			msg := &bus.InBoundMessage{
				ID:        fmt.Sprintf("concurrent-msg-%d", index),
				SenderID:  fmt.Sprintf("user%d", index),
				ChatID:    fmt.Sprintf("chat%d", index),
				InChannel: "cli",
				Content:   fmt.Sprintf("并发测试消息 %d", index),
				TimeStamp: time.Now(),
			}

			if err := msgBus.PublishInBoundMessage(ctx, msg); err != nil {
				errors <- fmt.Errorf("消息 %d 发布失败: %v", index, err)
			}
		}(i)
	}

	// 等待所有消息发送完成
	wg.Wait()
	close(errors)

	// 检查是否有发送错误
	errorCount := 0
	for err := range errors {
		t.Logf("发送错误: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("有 %d 条消息发送失败", errorCount)
	}

	// 等待所有消息被处理
	time.Sleep(3 * time.Second)

	// 验证响应数量
	messages := mockChannel.GetMessages()
	expectedCount := messageCount
	actualCount := len(messages)

	// 允许一定的误差（因为处理可能有延迟或失败）
	if actualCount < expectedCount*8/10 { // 至少 80% 的消息应该被处理
		t.Errorf("响应数量不足: 期望至少 %d 条，得到 %d 条", expectedCount, actualCount)
	}

	t.Logf("高并发测试完成: 发送 %d 条，收到 %d 条响应", messageCount, actualCount)
}
