# Hook 定制示例

本目录包含完整的 Hook 定制开发示例，展示了如何在 neoclaw 中创建和使用内部 Hook 和外部 Hook。

---

## 目录结构

```
examples/
├── hooks/                      # 内部 Go Hook
│   ├── doc.go                  # 包文档
│   └── internal_hooks.go       # 示例 Hook 实现
├── scripts/                    # 外部脚本 Hook
│   ├── hook_utils.py           # Python 工具库
│   ├── hello_hook.py           # 简单的 Hello Hook
│   ├── audit_order_hook.py     # 订单审计 Hook
│   └── bash_hook.sh            # Bash Hook 示例
├── config/
│   └── hooks.yaml              # Hook 配置示例
└── main.go                     # 运行示例的主程序
```

---

## 快速开始

### 前置要求

- Go 1.20+
- Python 3.8+ (用于外部 Hook 示例)
- Bash (用于 Bash Hook 示例)

### 运行示例

```bash
# 进入项目根目录
cd /path/to/neoclaw

# 运行示例
go run examples/main.go
```

---

## 示例说明

### 1. 内部 Hook (`examples/hooks/`)

#### HelloHook
简单的问候 Hook，展示基本用法。

```go
hook.Register("example.hello", HelloHook)
```

#### ValidationHook
数据验证 Hook，检查必需字段。

#### TransformHook
数据转换 Hook，修改输入数据。

#### 链式处理示例
展示如何使用优先级控制 Hook 执行顺序。

### 2. 外部 Hook (`examples/scripts/`)

#### hello_hook.py
简单的 Python Hook，返回问候语。

```yaml
- point: "example.hello"
  type: "external"
  config:
    command: "python3"
    script_path: "./hello_hook.py"
```

#### audit_order_hook.py
订单审计 Hook，具有以下功能：
- 验证必需字段
- 检查金额限额
- 大额订单自动打折
- 使用工具库简化开发

#### bash_hook.sh
纯 Bash 实现的 Hook，无外部依赖。

#### hook_utils.py
Python 工具库，提供：
- `HookResponseBuilder`: 构建标准响应
- `HookValidator`: 数据验证
- `@hook_handler`: 异常处理装饰器
- `read_request` / `write_response`: I/O 工具

### 3. 配置文件 (`examples/config/hooks.yaml`)

完整的 Hook 配置示例，展示：
- 全局默认配置
- 外部 Hook 定义
- 思考过程可视化配置

---

## 使用指南

### 创建你的第一个内部 Hook

1. 在 `examples/hooks/` 中创建新文件
2. 实现 Hook 函数
3. 在 `init()` 中注册
4. 运行示例测试

### 创建你的第一个外部 Hook

1. 在 `examples/scripts/` 中创建脚本
2. 使用 `hook_utils` 工具库
3. 在 `examples/config/hooks.yaml` 中配置
4. 运行示例测试

---

## API 参考

### 内部 Hook 函数签名

```go
type HookFunc func(ctx context.Context, input any) (any, error)
```

### 注册 Hook

```go
// 简单注册
hook.Register("point", MyHookFunc)

// 带优先级注册
hook.RegisterWithPriority("point", hook.PriorityHigh, MyHookFunc)

// 使用 HookEngine
engine := hook.GetEngine()
engine.Register("point", MyHookFunc)
```

### 触发 Hook

```go
// 基本触发
result, err := hook.Emit(ctx, "point", data)

// 带策略触发
result, errs := hook.EmitWithStrategy(
    ctx, "point", data,
    hook.ErrorStrategyContinue,
)
```

### 外部 Hook 响应格式

```json
{
  "decision": "allow",  // 或 "deny", "modify"
  "message": "描述信息",
  "modified_data": { ... },  // modify 时需要
  "extensions": { ... }      // 可选
}
```

---

## 更多文档

完整的 Hook 定制指南请参阅：
- `docs/hooks/hook-customization-guide.md` - Hook 定制开发指南
- `docs/hooks/hook-usage.md` - Hook 使用文档
- `docs/hooks/hook-points.md` - 可用 Hook 点列表

---

## 常见问题

### Q: 如何调试 Hook？

A: 启用调试模式：
```go
engine := hook.GetEngine()
engine.SetDebugEnabled(true)
```

### Q: 内部 Hook 和外部 Hook 如何选择？

A:
- 简单、性能敏感 → 内部 Hook
- 需要频繁更新、多语言 → 外部 Hook

### Q: 外部 Hook 可以使用其他语言吗？

A: 可以！任何可以读取 stdin 并写入 stdout 的语言都可以，包括：
- Python (示例中使用)
- Bash (示例中使用)
- Node.js
- Ruby
- Go
- 等等...

---

## 贡献

欢迎贡献更多 Hook 示例！

---

**最后更新**: 2026-05-17
