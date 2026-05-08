package main

import (
	"context"
	"fmt"
	"os"

	"brambleclaw/internal/config/structs"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/logger"
)

func main() {
	ctx := context.Background()

	// 1. 加载 Hook 配置
	hookConfig := structs.HookConfig{
		Version: "1.0",
		Defaults: structs.HookDefaults{
			TimeoutMs:     5000,
			MaxOutputSize: 1024 * 1024,
			WorkingDir:    "./scripts",
		},
		Definitions: []structs.HookDefinition{
			{
				Point:   "order.before_save",
				Type:    structs.HookTypeExternal,
				Enabled: true,
				Config: structs.ExternalConfig{
					Command:    "python3",
					ScriptPath: "./scripts/audit_order.py",
					TimeoutMs:  2000,
				},
			},
			{
				Point:   "user.after_register",
				Type:    structs.HookTypeExternal,
				Enabled: true,
				Config: structs.ExternalConfig{
					Command:    "bash",
					ScriptPath: "./scripts/send_welcome.sh",
					TimeoutMs:  3000,
				},
			},
			{
				Point:   "api.before_request",
				Type:    structs.HookTypeExternal,
				Enabled: true,
				Config: structs.ExternalConfig{
					Command:    "bash",
					ScriptPath: "./scripts/rate_limit.sh",
					TimeoutMs:  1000,
				},
			},
		},
	}

	// 2. 获取全局引擎并加载配置
	engine := hook.GetEngine()
	if err := engine.LoadConfig(hookConfig); err != nil {
		logger.L().Fatal().Err(err).Msg("Failed to load hook config")
	}

	// 3. 注册内部 Hook（向后兼容）
	engine.Register("system.on_startup", func(ctx context.Context, input any) (any, error) {
		fmt.Println("[internal hook] system.on_startup triggered")
		return input, nil
	})

	// 4. 列出所有 Hook 点
	fmt.Println("=== Registered hook points ===")
	for _, point := range engine.List() {
		fmt.Printf("  - %s (%d hooks)\n", point, engine.Count(point))
	}

	// 5. 演示: 触发内部 Hook
	fmt.Println("\n=== Demo 1: Internal Hook ===")
	result, err := hook.Emit(ctx, "system.on_startup", map[string]string{"event": "demo"})
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else {
		fmt.Printf("  Result: %v\n", result)
	}

	// 6. 演示: 触发外部 Hook（需要 python3 和 audit_order.py 可用）
	fmt.Println("\n=== Demo 2: External Hook - small order ===")
	order := map[string]any{
		"order_id": "ORD-2024-001",
		"amount":   150.50,
		"user_id":  "user_99",
		"items": []map[string]any{
			{"name": "apple", "price": 50, "category": "fruit"},
		},
	}
	result, err = engine.Emit(ctx, "order.before_save", order)
	if err != nil {
		fmt.Printf("  Denied: %v\n", err)
	} else {
		fmt.Printf("  Allowed: %v\n", result)
	}

	// 7. 演示: 大额订单（触发 modify 决策）
	fmt.Println("\n=== Demo 3: External Hook - large order ===")
	largeOrder := map[string]any{
		"order_id": "ORD-2024-002",
		"amount":   6000.00,
		"user_id":  "user_100",
		"items": []map[string]any{
			{"name": "laptop", "price": 6000, "category": "electronics"},
		},
	}
	result, err = engine.Emit(ctx, "order.before_save", largeOrder)
	if err != nil {
		fmt.Printf("  Denied: %v\n", err)
	} else {
		fmt.Printf("  Modified: %v\n", result)
	}

	// 8. 演示: 超额订单（触发 deny 决策）
	fmt.Println("\n=== Demo 4: External Hook - oversized order ===")
	oversizedOrder := map[string]any{
		"order_id": "ORD-2024-003",
		"amount":   15000.00,
		"user_id":  "user_101",
		"items": []map[string]any{
			{"name": "car", "price": 15000, "category": "vehicle"},
		},
	}
	result, err = engine.Emit(ctx, "order.before_save", oversizedOrder)
	if err != nil {
		fmt.Printf("  Denied: %v\n", err)
	} else {
		fmt.Printf("  Allowed: %v\n", result)
	}

	// 9. 演示: Bash Hook - 用户注册后通知
	fmt.Println("\n=== Demo 5: Bash Hook - user after register ===")
	user := map[string]any{
		"user_id": "user_99",
		"email":   "user99@example.com",
	}
	result, err = engine.Emit(ctx, "user.after_register", user)
	if err != nil {
		fmt.Printf("  Denied: %v\n", err)
	} else {
		fmt.Printf("  Result: %v\n", result)
	}

	// 10. 查看 ProcessManager 状态
	fmt.Println("\n=== ProcessManager Status ===")
	pm := engine.ProcessManager()
	fmt.Printf("  Status: %d\n", pm.Status())
	for _, h := range pm.List(ctx) {
		fmt.Printf("  - %s (enabled=%v)\n", h.Name(), h.Enabled())
	}

	fmt.Println("\n=== All demos completed ===")
	os.Exit(0)
}
