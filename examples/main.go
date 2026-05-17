package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"

	_ "neoclaw/examples/hooks" // 导入内部 Hook
	"neoclaw/internal/config/structs"
	"neoclaw/internal/hook"
)

func main() {
	fmt.Println("🚀 Neoclaw Hook 定制示例")
	fmt.Println("=========================")

	ctx := context.Background()
	engine := hook.GetEngine()

	// 1. 启用调试模式
	engine.SetDebugEnabled(true)

	// 2. 从配置文件加载外部 Hook
	fmt.Println("\n📦 加载外部 Hook 配置...")
	if err := loadHooksFromConfig(engine, "./examples/config/hooks.yaml"); err != nil {
		log.Printf("警告: 无法加载配置: %v", err)
	}

	// 3. 列出所有注册的 Hook 点
	fmt.Println("\n📋 已注册的 Hook 点:")
	for _, point := range engine.List() {
		fmt.Printf("  - %s (%d 个 Hook)\n", point, engine.Count(point))
	}

	// 4. 演示内部 Hook
	fmt.Println("\n🔧 示例 1: 内部 Hook")
	demonstrateInternalHook(ctx)

	// 5. 演示外部 Hook
	fmt.Println("\n🐍 示例 2: 外部 Hook (Python)")
	demonstrateExternalHook(ctx, engine)

	// 6. 演示链处理
	fmt.Println("\n⛓️  示例 3: 链式 Hook")
	demonstrateChainHook(ctx)

	fmt.Println("\n✅ 示例完成！")
}

// loadHooksFromConfig 从 YAML 配置文件加载 Hook
func loadHooksFromConfig(engine hook.HookEngine, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var config structs.HookConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	// 验证配置
	config.Validate()

	return engine.LoadConfig(config)
}

// demonstrateInternalHook 演示内部 Hook 使用
func demonstrateInternalHook(ctx context.Context) {
	testData := map[string]any{
		"name":  "Alice",
		"email": "alice@example.com",
	}

	fmt.Println("  输入数据:", testData)

	result, err := hook.Emit(ctx, "example.data.process", testData)
	if err != nil {
		fmt.Printf("  ❌ 错误: %v\n", err)
		return
	}

	fmt.Printf("  ✅ 结果: %v\n", result)
}

// demonstrateExternalHook 演示外部 Hook 使用
func demonstrateExternalHook(ctx context.Context, engine hook.HookEngine) {
	// 测试 1: 小额订单（允许）
	fmt.Println("\n  🧪 测试 1: 小额订单 (允许)")
	smallOrder := map[string]any{
		"order_id": "ORD-001",
		"amount":   1500.0,
		"user_id":  "user-123",
	}

	result, err := engine.Emit(ctx, "example.order_audit", smallOrder)
	if err != nil {
		fmt.Printf("  ❌ 拒绝: %v\n", err)
	} else {
		fmt.Printf("  ✅ 通过: %v\n", result)
	}

	// 测试 2: 大额订单（修改）
	fmt.Println("\n  🧪 测试 2: 大额订单 (自动折扣)")
	largeOrder := map[string]any{
		"order_id": "ORD-002",
		"amount":   6000.0,
		"user_id":  "user-456",
	}

	result, err = engine.Emit(ctx, "example.order_audit", largeOrder)
	if err != nil {
		fmt.Printf("  ❌ 拒绝: %v\n", err)
	} else {
		fmt.Printf("  ✅ 修改: %v\n", result)
	}

	// 测试 3: 超限额订单（拒绝）
	fmt.Println("\n  🧪 测试 3: 超限额订单 (拒绝)")
	hugeOrder := map[string]any{
		"order_id": "ORD-003",
		"amount":   15000.0,
		"user_id":  "user-789",
	}

	result, err = engine.Emit(ctx, "example.order_audit", hugeOrder)
	if err != nil {
		fmt.Printf("  ❌ 拒绝: %v\n", err)
	} else {
		fmt.Printf("  ✅ 通过: %v\n", result)
	}
}

// demonstrateChainHook 演示链式 Hook
func demonstrateChainHook(ctx context.Context) {
	fmt.Println("  执行链式处理...")

	result, err := hook.EmitWithStrategy(
		ctx,
		"example.chain",
		map[string]string{"data": "test"},
		hook.ErrorStrategyContinue,
	)

	if err != nil {
		fmt.Printf("  ❌ 错误: %v\n", err)
	} else {
		fmt.Printf("  ✅ 完成: %v\n", result)
	}
}
