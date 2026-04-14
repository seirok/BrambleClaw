package gateway

import (
	"testing"

	"brambleclaw/bus"
)

func TestBuildSessionKey(t *testing.T) {
	tests := []struct {
		agentName string
		channel   string
		chatType  string
		chatID    string
		expected  string
	}{
		{
			agentName: "main",
			channel:   "weixin",
			chatType:  "direct",
			chatID:    "wxid_abc123",
			expected:  "agent:main:weixin:direct:wxid_abc123",
		},
		{
			agentName: "customer_service",
			channel:   "cli",
			chatType:  "group",
			chatID:    "group_123",
			expected:  "agent:customer_service:cli:group:group_123",
		},
	}

	for _, tt := range tests {
		result := BuildSessionKey(tt.agentName, tt.channel, tt.chatType, tt.chatID)
		if result != tt.expected {
			t.Errorf("BuildSessionKey(%s, %s, %s, %s) = %s, 期望 %s",
				tt.agentName, tt.channel, tt.chatType, tt.chatID, result, tt.expected)
		}
	}
}

func TestParseSessionKey(t *testing.T) {
	tests := []struct {
		sessionKey  string
		expectError bool
		agentName   string
		channel     string
		chatType    string
		chatID      string
	}{
		{
			sessionKey:  "agent:main:weixin:direct:wxid_abc123",
			expectError: false,
			agentName:   "main",
			channel:     "weixin",
			chatType:    "direct",
			chatID:      "wxid_abc123",
		},
		{
			sessionKey:  "invalid_session_key",
			expectError: true,
		},
		{
			sessionKey:  "wrong:main:weixin:direct:wxid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		agentName, channel, chatType, chatID, err := ParseSessionKey(tt.sessionKey)

		if tt.expectError {
			if err == nil {
				t.Errorf("ParseSessionKey(%s) 期望返回错误，但得到 nil", tt.sessionKey)
			}
			continue
		}

		if err != nil {
			t.Errorf("ParseSessionKey(%s) 返回意外错误: %v", tt.sessionKey, err)
			continue
		}

		if agentName != tt.agentName {
			t.Errorf("ParseSessionKey(%s) agentName = %s, 期望 %s", tt.sessionKey, agentName, tt.agentName)
		}
		if channel != tt.channel {
			t.Errorf("ParseSessionKey(%s) channel = %s, 期望 %s", tt.sessionKey, channel, tt.channel)
		}
		if chatType != tt.chatType {
			t.Errorf("ParseSessionKey(%s) chatType = %s, 期望 %s", tt.sessionKey, chatType, tt.chatType)
		}
		if chatID != tt.chatID {
			t.Errorf("ParseSessionKey(%s) chatID = %s, 期望 %s", tt.sessionKey, chatID, tt.chatID)
		}
	}
}

func TestMatchRouteCondition(t *testing.T) {
	tests := []struct {
		name       string
		msg        *bus.InBoundMessage
		conditions map[string]string
		expected   bool
	}{
		{
			name: "匹配用户ID",
			msg: &bus.InBoundMessage{
				SenderID:  "user123",
				InChannel: "cli",
				ChatID:    "chat456",
			},
			conditions: map[string]string{
				"user_id": "user123",
			},
			expected: true,
		},
		{
			name: "不匹配用户ID",
			msg: &bus.InBoundMessage{
				SenderID:  "user123",
				InChannel: "cli",
				ChatID:    "chat456",
			},
			conditions: map[string]string{
				"user_id": "user999",
			},
			expected: false,
		},
		{
			name: "匹配通道",
			msg: &bus.InBoundMessage{
				SenderID:  "user123",
				InChannel: "weixin",
				ChatID:    "chat456",
			},
			conditions: map[string]string{
				"channel": "weixin",
			},
			expected: true,
		},
		{
			name: "多条件匹配",
			msg: &bus.InBoundMessage{
				SenderID:  "user123",
				InChannel: "cli",
				ChatID:    "chat456",
			},
			conditions: map[string]string{
				"user_id": "user123",
				"channel": "cli",
			},
			expected: true,
		},
		{
			name: "多条件部分匹配",
			msg: &bus.InBoundMessage{
				SenderID:  "user123",
				InChannel: "cli",
				ChatID:    "chat456",
			},
			conditions: map[string]string{
				"user_id": "user123",
				"channel": "weixin", // 不匹配
			},
			expected: false,
		},
		{
			name: "未知条件",
			msg: &bus.InBoundMessage{
				SenderID:  "user123",
				InChannel: "cli",
				ChatID:    "chat456",
			},
			conditions: map[string]string{
				"unknown_key": "some_value",
			},
			expected: false,
		},
		{
			name: "空条件",
			msg: &bus.InBoundMessage{
				SenderID:  "user123",
				InChannel: "cli",
				ChatID:    "chat456",
			},
			conditions: map[string]string{},
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchRouteCondition(tt.msg, tt.conditions)
			if result != tt.expected {
				t.Errorf("MatchRouteCondition() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}

func TestMatchRouteCondition_NilMessage(t *testing.T) {
	conditions := map[string]string{
		"user_id": "user123",
	}

	result := MatchRouteCondition(nil, conditions)
	if result {
		t.Error("对于 nil 消息应该返回 false")
	}
}

func TestMatchRouteCondition_NilConditions(t *testing.T) {
	msg := &bus.InBoundMessage{
		SenderID:  "user123",
		InChannel: "cli",
		ChatID:    "chat456",
	}

	result := MatchRouteCondition(msg, nil)
	if !result {
		t.Error("对于 nil 条件应该返回 true")
	}
}
