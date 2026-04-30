package hook

import (
	"encoding/json"
	"fmt"
	"time"
)

// DecisionType 表示 Hook 执行后的决策类型
type DecisionType string

const (
	// DecisionAllow 允许执行
	DecisionAllow DecisionType = "allow"
	// DecisionDeny 拒绝执行
	DecisionDeny DecisionType = "deny"
	// DecisionModify 允许但修改数据
	DecisionModify DecisionType = "modify"
)

// HookRequest 发送给外部脚本的请求
// 通过 stdin 以 JSON 格式传递
type HookRequest struct {
	// HookPoint 钩子点名称，如 "order.before_save"
	HookPoint string `json:"hook_point"`

	// Timestamp Unix 时间戳（毫秒）
	Timestamp int64 `json:"timestamp"`

	// RequestID 唯一请求 ID，用于追踪
	RequestID string `json:"request_id"`

	// Data 业务数据，JSON 任意类型
	Data json.RawMessage `json:"data"`

	// Metadata 元数据，可选
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewHookRequest 创建一个新的 HookRequest
func NewHookRequest(hookPoint string, data interface{}) (*HookRequest, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &HookRequest{
		HookPoint: hookPoint,
		Timestamp: time.Now().UnixMilli(),
		RequestID: generateRequestID(),
		Data:      dataBytes,
		Metadata:  make(map[string]interface{}),
	}, nil
}

// HookResponse 外部脚本返回的响应
// 通过 stdout 以 JSON 格式返回
type HookResponse struct {
	// Decision 核心决策: allow / deny / modify
	Decision DecisionType `json:"decision"`

	// Message 消息，deny 时必需，其他可选
	Message string `json:"message,omitempty"`

	// ModifiedData 修改后的数据，decision=modify 时必需
	ModifiedData json.RawMessage `json:"modified_data,omitempty"`

	// Extensions 扩展数据，可选，供业务使用
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// IsValid 检查决策类型是否有效
func (d DecisionType) IsValid() bool {
	switch d {
	case DecisionAllow, DecisionDeny, DecisionModify:
		return true
	}
	return false
}

// generateRequestID 生成唯一请求 ID
// 简化实现，实际可使用 UUID
func generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}
