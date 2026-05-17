package hooks

import (
	"context"
	"fmt"
	"neoclaw/internal/hook"
)

// =====================================================================
// 示例 1: 简单的 Hello World Hook
// =====================================================================

// HelloHook 是一个简单的问候 Hook
func HelloHook(ctx context.Context, input any) (any, error) {
	fmt.Printf("👋 Hello from Hook! Input: %v\n", input)

	// 返回原始数据（不修改）
	return input, nil
}

// =====================================================================
// 示例 2: 数据验证 Hook
// =====================================================================

// ValidationHook 验证输入数据
func ValidationHook(ctx context.Context, input any) (any, error) {
	data, ok := input.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("input must be a map")
	}

	// 检查必需字段
	requiredFields := []string{"name", "email"}
	for _, field := range requiredFields {
		if _, exists := data[field]; !exists {
			return nil, fmt.Errorf("missing required field: %s", field)
		}
	}

	// 验证通过
	fmt.Printf("✅ Validation passed for: %s\n", data["name"])
	return input, nil
}

// =====================================================================
// 示例 3: 数据修改 Hook（流水线模式）
// =====================================================================

// TransformHook 修改输入数据
func TransformHook(ctx context.Context, input any) (any, error) {
	data, ok := input.(map[string]any)
	if !ok {
		return input, nil
	}

	// 添加或修改字段
	data["processed"] = true
	data["processed_at"] = "2026-05-17"

	// 如果有 name 字段，转为大写
	if name, exists := data["name"].(string); exists {
		data["name_uppercase"] = toUpper(name)
	}

	fmt.Printf("🔄 Transformed data: %v\n", data)
	return data, nil
}

// toUpper 将字符串转为大写（简化实现）
func toUpper(s string) string {
	result := make([]rune, len(s))
	for i, c := range s {
		if c >= 'a' && c <= 'z' {
			result[i] = c - 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// =====================================================================
// 示例 4: 优先级链式处理
// =====================================================================

// Step1CleanHook 步骤 1: 清理数据
func Step1CleanHook(ctx context.Context, input any) (any, error) {
	fmt.Println("📍 Step 1: Cleaning data")
	return input, nil
}

// Step2ValidateHook 步骤 2: 验证数据
func Step2ValidateHook(ctx context.Context, input any) (any, error) {
	fmt.Println("📍 Step 2: Validating data")
	return input, nil
}

// Step3EnhanceHook 步骤 3: 增强数据
func Step3EnhanceHook(ctx context.Context, input any) (any, error) {
	fmt.Println("📍 Step 3: Enhancing data")
	return input, nil
}

// =====================================================================
// 初始化：注册所有 Hook
// =====================================================================

func init() {
	// 注册简单 Hook
	hook.Register("example.hello", HelloHook)

	// 注册验证 Hook（高优先级）
	hook.RegisterWithPriority("example.data.process", hook.PriorityHigh, ValidationHook)

	// 注册转换 Hook（中等优先级）
	hook.RegisterWithPriority("example.data.process", hook.PriorityNormal, TransformHook)

	// 注册链式处理示例
	hook.RegisterWithPriority("example.chain", hook.PriorityHigh, Step1CleanHook)
	hook.RegisterWithPriority("example.chain", hook.PriorityNormal, Step2ValidateHook)
	hook.RegisterWithPriority("example.chain", hook.PriorityLow, Step3EnhanceHook)
}
