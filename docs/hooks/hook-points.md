# Hook 点完整列表

## 概述

本文档列出 brambleclaw 框架中所有可用的 Hook 点，包括现有 Hook 点和新增加的 Hook 点。每个 Hook 点包含名称、位置、类型和传递的数据结构。

## Hook 类型说明

- **Pipeline (blocking)**：调用方检查 `hook.Emit` 返回的错误和数据，错误会阻断流程，数据可被修改
- **Fire-and-forget**：调用方忽略 `hook.Emit` 的返回值，仅用于通知/观察

## Agent 相关 Hook

| Hook 点 | 文件位置 | 类型 | 传递数据 | 说明 |
|---------|----------|------|----------|------|
| `hook.point.agent.create` | `internal/agent/agent.go:43` | Fire-and-forget | `*Agent` | Agent 实例创建后触发 |
| `hook.point.agent.pre-start` | `internal/agent/agent.go:248` | Pipeline | `*Agent` | Agent 启动前触发，错误会阻止启动 |
| `hook.point.agent.start` | `internal/agent/agent.go:263` | Fire-and-forget | `*Agent` | Agent 启动后触发 |
| `hook.point.agent.pre-stop` | `internal/agent/agent.go:271` | Fire-and-forget | `*Agent` | Agent 停止前触发，错误仅记录 |
| `hook.point.agent.stop` | `internal/agent/agent.go:285` | Fire-and-forget | `*Agent` | Agent 停止后触发 |

## 消息处理 Hook

| Hook 点 | 文件位置 | 类型 | 传递数据 | 说明 |
|---------|----------|------|----------|------|
| `hook.point.message.pre-process` | `internal/agent/agent.go:121` | Pipeline | `*bus.InBoundMessage` | 处理入站消息前触发，可修改消息 |
| `hook.point.message.pre-response` | `internal/agent/agent.go:219` | Pipeline | `*bus.OutBoundMessage` | 发送响应前触发，可修改响应 |
| `hook.point.message.post-process` | `internal/agent/agent.go:233` | Fire-and-forget | `*bus.InBoundMessage` | 消息处理完成后触发 |
| `hook.point.message.route` | `internal/gateway/gateway.go:181` | Fire-and-forget | `RouteResult` | 消息路由解析后触发 |

## LLM 相关 Hook

| Hook 点 | 文件位置 | 类型 | 传递数据 | 说明 |
|---------|----------|------|----------|------|
| `hook.point.llm.request` | `internal/agent/orchestrator.go:105` | Pipeline | `ChatCompletionRequest` | 发送 LLM 请求前触发，可修改请求 |
| `hook.point.llm.error` | `internal/agent/orchestrator.go:116` | Fire-and-forget | `error` | LLM 调用失败时触发 |
| `hook.point.llm.response` | `internal/agent/orchestrator.go:121` | Pipeline | `*LLMResponse` | 收到 LLM 响应后触发，可修改响应 |

## 工具执行 Hook

| Hook 点 | 文件位置 | 类型 | 传递数据 | 说明 |
|---------|----------|------|----------|------|
| `hook.point.tool.pre-execute` | `internal/agent/orchestrator.go:192` | Pipeline | `args string` | 工具执行前触发，可修改参数 |
| `hook.point.tool.error` | `internal/agent/orchestrator.go:203` | Fire-and-forget | `error` | 工具执行失败时触发 |
| `hook.point.tool.result` | `internal/agent/orchestrator.go:208` | Pipeline | `result interface{}` | 工具执行完成后触发，可修改结果 |

## MCP 工具 Hook (新增)

| Hook 点 | 文件位置 | 类型 | 传递数据 | 说明 |
|---------|----------|------|----------|------|
| `hook.point.mcp.tool.pre-execute` | `internal/tools/mcp/manager.go:190` | Pipeline | `{"tool_name":string,"original_name":string,"args":string}` | MCP 工具执行前触发，可修改参数 |
| `hook.point.mcp.tool.result` | `internal/tools/mcp/manager.go:239` | Pipeline | `{"tool_name":string,"original_name":string,"result":string,"is_error":bool}` | MCP 工具执行后触发，可修改结果 |

## 沙箱 Hook (新增)

| Hook 点 | 文件位置 | 类型 | 传递数据 | 说明 |
|---------|----------|------|----------|------|
| `hook.point.sandbox.command.validate` | `internal/sandbox/sandbox.go:169` | Pipeline | `{"command":string,"cmd_name":string,"working_dir":string}` | 命令验证前触发，可修改或拒绝命令 |
| `hook.point.sandbox.path.validate` | `internal/sandbox/sandbox.go:85` | Pipeline | `{"path":string,"absolute_path":string,"for_write":bool,"workspace":string}` | 路径验证前触发，可修改或拒绝路径访问 |
| `hook.point.sandbox.command.execute` | `待实现` | Fire-and-forget | `{"command":string,"duration_ms":int64,"success":bool,"error":string?}` | 命令执行后触发，用于监控 |

## Gateway Hook (新增)

| Hook 点 | 文件位置 | 类型 | 传递数据 | 说明 |
|---------|----------|------|----------|------|
| `hook.point.gateway.pre-start` | `待实现` | Pipeline | `*Gateway` | Gateway 启动前触发，错误会阻止启动 |
| `hook.point.gateway.start` | `internal/gateway/gateway.go:113` | Fire-and-forget | `*Gateway` | Gateway 启动后触发 |
| `hook.point.gateway.stop` | `未添加` | Fire-and-forget | `*Gateway` | Gateway 停止后触发 (LOW 优先级) |

## 统计

- **总计 Hook 点**: 22 个
- **现有 Hook 点**: 15 个
- **新增 Hook 点**: 7 个 (5 个已实现，2 个待实现)
- **Pipeline 类型**: 12 个
- **Fire-and-forget 类型**: 10 个

## 待实现 Hook 点

1. `hook.point.sandbox.command.execute` - 沙箱命令执行监控
2. `hook.point.gateway.pre-start` - Gateway 启动前检查

## 使用示例

### 内部 Hook 注册
```go
hook.Register("hook.point.sandbox.command.validate", func(ctx context.Context, data any) (any, error) {
    m := data.(map[string]any)
    cmd := m["command"].(string)
    if strings.Contains(cmd, "rm -rf") {
        return nil, fmt.Errorf("危险命令被拒绝")
    }
    return data, nil // 允许
})
```

### 外部 Hook 配置 (YAML)
```yaml
hooks:
  definitions:
    - point: hook.point.sandbox.command.validate
      type: external
      enabled: true
      config:
        command: python
        scriptPath: ./scripts/validate_command.py
        timeoutMs: 2000
```

## 注意事项

1. Pipeline Hook 必须正确处理错误返回，否则会阻断业务流程
2. Fire-and-forget Hook 不检查错误，适合日志、监控等非关键操作
3. 外部 Hook 脚本必须遵循 JSON 输入输出协议（参考 `docs/hook-usage.md`）
4. 新增 Hook 点已通过基础测试，建议在生产前进行完整测试

---
*文档生成时间: 2026-04-29*
*Hook 系统版本: 2.0*