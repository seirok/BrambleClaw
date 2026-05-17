package hook

import (
	"context"
	"neoclaw/internal/config/structs"
	"neoclaw/internal/events"
	"time"
)

// RegisterObservers 为配置的 hook points 注册观察者
func RegisterObservers(eventBus *events.EventBus, cfg structs.ThinkingVisibilityConfig) {
	if !cfg.Enabled || eventBus == nil {
		return
	}

	// 创建 point 到配置的映射
	pointConfig := make(map[string]structs.ThinkingPointConfig)
	for _, pc := range cfg.Points {
		pointConfig[pc.Point] = pc
	}

	// 遍历所有 hook points，为启用的注册观察者
	for point, pc := range pointConfig {
		if !pc.Enabled {
			continue
		}

		// 闭包捕获 point, eventBus
		hookPoint := point
		observerHook := func(ctx context.Context, input any) (any, error) {
			// 创建事件（原始数据，格式化在 TUI 端进行）
			event := events.ThinkingEvent{
				Point:     hookPoint,
				Timestamp: time.Now(),
				Data:      input,
			}
			// 发布事件（非阻塞）
			eventBus.Publish(event)
			// 原样返回数据，纯观察
			return input, nil
		}

		// 注册为低优先级，在所有业务 hook 之后执行
		RegisterWithPriority(point, PriorityLow, observerHook)
	}
}
