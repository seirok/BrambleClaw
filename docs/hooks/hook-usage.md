# Hook 框架使用文档

Hook 框架提供「注册-触发」机制，支持两种扩展方式：

- **内部 Hook**：Go 函数，进程内执行，零开销
- **外部 Hook**：Python/Bash/Shell 脚本，子进程执行，JSON 协议通信

---

## 目录

- [快速开始](#快速开始)
- [内部 Hook](#内部-hook)
- [外部 Hook](#外部-hook)
- [配置参考](#配置参考)
- [通信协议](#通信协议)
- [编写 Hook 脚本](#编写-hook-脚本)
  - [Python 脚本](#python-脚本)
  - [Bash 脚本（带 jq）](#bash-脚本带-jq)
  - [纯 Bash 脚本（无 jq）](#纯-bash-脚本无-jq)
- [错误处理策略](#错误处理策略)
- [进程管理](#进程管理)
- [完整示例](#完整示例)

---

## 快速开始

### 1. 最小可运行示例（内部 Hook）

```go
package main

import (
    "context"
    "fmt"
    "brambleclaw/internal/hook"
)

func main() {
    ctx := context.Background()

    // 注册
    hook.Register("user.before_create", func(ctx context.Context, input any) (any, error) {
        data := input.(map[string]any)
        fmt.Printf("即将创建用户: %v\n", data["name"])
        return input, nil
    })

    // 触发
    result, err := hook.Emit(ctx, "user.before_create", map[string]any{
        "name":  "Alice",
        "email": "alice@example.com",
    })
    if err != nil {
        fmt.Printf("被拒绝: %v\n", err)
    } else {
        fmt.Printf("结果: %v\n", result)
    }
}
```

### 2. 最小可运行示例（外部 Hook）

```go
package main

import (
    "context"
    "fmt"
    "log"

    "brambleclaw/internal/config/structs"
    "brambleclaw/internal/hook"
)

func main() {
    ctx := context.Background()
    engine := hook.GetEngine()

    // 加载配置
    config := structs.HookConfig{
        Version: "1.0",
        Defaults: structs.HookDefaults{
            TimeoutMs:  5000,
            WorkingDir: "./scripts",
        },
        Definitions: []structs.HookDefinition{
            {
                Point:   "order.before_save",
                Type:    structs.HookTypeExternal,
                Enabled: true,
                Config: structs.ExternalConfig{
                    Command:    "python3",
                    ScriptPath: "./scripts/audit_order.py",
                },
            },
        },
    }
    if err := engine.LoadConfig(config); err != nil {
        log.Fatal(err)
    }

    // 触发外部 Hook
    result, err := engine.Emit(ctx, "order.before_save", map[string]any{
        "order_id": "ORD-001",
        "amount":   150.0,
    })
    if err != nil {
        fmt.Printf("被拒绝: %v\n", err)
    } else {
        fmt.Printf("结果: %v\n", result)
    }
}
```

---

## 内部 Hook

内部 Hook 是普通的 Go 函数，在进程内同步执行，适用于轻量级逻辑。

### 注册

```go
// 基本注册（默认优先级）
hook.Register("order.before_save", func(ctx context.Context, input any) (any, error) {
    return input, nil
})

// 带优先级注册
hook.RegisterWithPriority("order.before_save", hook.PriorityHigh, func(ctx context.Context, input any) (any, error) {
    return input, nil
})
```

优先级常量：

| 常量 | 值 | 说明 |
|------|----|------|
| `PriorityHigh` | 10 | 最先执行 |
| `PriorityNormal` | 50 | 默认 |
| `PriorityLow` | 100 | 最后执行 |

同一个 Hook 点可以注册多个函数，按优先级从高到低依次执行。

### 触发

```go
// 基本触发（出错即停止）
result, err := hook.Emit(ctx, "order.before_save", orderData)

// 带策略触发（出错可继续）
result, errs := hook.EmitWithStrategy(ctx, "order.before_save", orderData, hook.ErrorStrategyContinue)

// 必须成功触发（出错 panic）
result := hook.MustEmit(ctx, "order.before_save", orderData)
```

### 注销

```go
err := hook.Unregister("order.before_save", myFunc)
```

> 注意：`Unregister` 通过函数指针比较，需要传入注册时的同一个函数变量。

### 查询

```go
// 列出所有 Hook 点
points := hook.List() // []string

// 查询某个点的 Hook 数量
count := hook.Count("order.before_save") // int
```

---

## 外部 Hook

外部 Hook 通过子进程执行脚本，Go 通过 stdin/stdout 与脚本交换 JSON 数据。

### 注册方式一：通过配置加载

```go
config := structs.HookConfig{
    Version: "1.0",
    Defaults: structs.HookDefaults{
        TimeoutMs:     5000,
        MaxOutputSize: 1048576, // 1MB
        WorkingDir:    "./scripts",
    },
    Definitions: []structs.HookDefinition{
        {
            Point:    "order.before_save",
            Type:     structs.HookTypeExternal,
            Enabled:  true,
            Priority: 50,
            Config: structs.ExternalConfig{
                Command:    "python3",
                ScriptPath: "./scripts/audit_order.py",
                TimeoutMs:  3000,  // 覆盖默认超时
            },
        },
    },
}

engine := hook.GetEngine()
err := engine.LoadConfig(config)
```

### 注册方式二：编程式注册

```go
engine := hook.GetEngine()
err := engine.RegisterExternal("order.before_save", structs.ExternalConfig{
    Command:    "python3",
    ScriptPath: "./scripts/audit_order.py",
    TimeoutMs:  3000,
})
```

### 注销外部 Hook

```go
err := engine.UnregisterExternal("order.before_save", "./scripts/audit_order.py")
```

### 执行流程

当调用 `engine.Emit(ctx, "order.before_save", data)` 时：

1. **内部 Hook 先执行** — 同一点上的 Go 函数按优先级依次运行
2. **外部 Hook 后执行** — 子进程依次启动，数据通过管道传递
3. **决策处理** — 根据脚本返回的 `decision` 字段决定后续行为

---

## 配置参考

### YAML 格式

```yaml
version: "1.0"

defaults:
  timeout_ms: 5000          # 全局默认超时（毫秒）
  max_output_size: 1048576  # 全局默认最大输出（字节）
  working_dir: "./scripts"  # 全局默认工作目录
  shell: "/bin/bash"        # 全局默认 Shell
  env:                      # 全局环境变量
    - LOG_LEVEL=info

definitions:
  - point: order.before_save
    type: external
    enabled: true
    priority: 50
    config:
      command: python3
      script_path: ./scripts/audit_order.py
      timeout_ms: 3000           # 覆盖默认超时
      working_dir: /app/hooks    # 覆盖默认工作目录
      max_output_size: 524288    # 覆盖默认最大输出
      args:                      # 传递给脚本的额外参数
        - --strict
      env:                       # 额外环境变量
        - DB_HOST=localhost

  - point: user.after_register
    type: external
    enabled: true
    config:
      command: bash
      script_path: ./scripts/send_welcome.sh

  - point: api.before_request
    type: external
    enabled: true
    config:
      command: bash
      script_path: ./scripts/rate_limit.sh
```

### 字段说明

#### HookConfig（根配置）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `Version` | string | 是 | 配置版本，当前为 `"1.0"` |
| `Defaults` | HookDefaults | 是 | 全局默认值 |
| `Definitions` | []HookDefinition | 是 | Hook 定义列表 |

#### HookDefaults（全局默认值）

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `TimeoutMs` | int | 5000 | 子进程超时（毫秒） |
| `MaxOutputSize` | int | 1048576 | 子进程 stdout 最大字节数 |
| `Shell` | string | `"/bin/bash"` | 默认 Shell |
| `WorkingDir` | string | `"./scripts"` | 默认工作目录 |
| `Env` | []string | `[]` | 全局环境变量（`KEY=VALUE` 格式） |

#### HookDefinition（单个 Hook 定义）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `Point` | string | 是 | Hook 点名称，如 `"order.before_save"` |
| `Type` | HookType | 是 | `"internal"` 或 `"external"` |
| `Enabled` | bool | 是 | 是否启用 |
| `Priority` | int | 否 | 优先级，默认 50 |
| `Config` | ExternalConfig | 否 | 外部 Hook 配置 |

#### ExternalConfig（外部脚本配置）

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `Command` | string | 是 | 执行命令，如 `"python3"`、`"bash"`、`"sh"` |
| `ScriptPath` | string | 是 | 脚本文件路径 |
| `Args` | []string | 否 | 额外命令行参数 |
| `TimeoutMs` | int | 否 | 超时覆盖，0 表示使用全局默认 |
| `WorkingDir` | int | 否 | 工作目录覆盖 |
| `MaxOutputSize` | int | 否 | 最大输出覆盖，0 表示使用全局默认 |
| `Env` | []string | 否 | 额外环境变量 |

---

## 通信协议

外部 Hook 通过 **stdin/stdout** 交换 JSON 数据，**stderr** 用于日志输出。

### 请求格式（Go → 脚本，通过 stdin）

```json
{
  "hook_point": "order.before_save",
  "timestamp": 1714368000,
  "request_id": "req_a1b2c3d4",
  "data": {
    "order_id": "ORD-001",
    "amount": 150.0,
    "user_id": "user_99"
  },
  "metadata": {}
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `hook_point` | string | Hook 点名称 |
| `timestamp` | int64 | Unix 时间戳（秒） |
| `request_id` | string | 请求唯一 ID，用于追踪 |
| `data` | object | 业务数据（即 `Emit` 传入的 data） |
| `metadata` | object | 可选元数据 |

### 响应格式（脚本 → Go，通过 stdout）

脚本必须输出一行合法 JSON 到 stdout：

```json
{
  "decision": "allow",
  "message": "检查通过",
  "extensions": {}
}
```

三种决策类型：

#### `allow` — 允许，数据不变

```json
{"decision": "allow", "message": "检查通过"}
```

#### `deny` — 拒绝，Emit 返回 error

```json
{"decision": "deny", "message": "金额超限"}
```

返回 `deny` 时，`Emit` 会返回 `error`，错误信息为 `message` 的内容。

#### `modify` — 允许，但修改数据

```json
{
  "decision": "modify",
  "message": "已应用折扣",
  "modified_data": {
    "order_id": "ORD-001",
    "amount": 5700.0,
    "discount": "5%"
  }
}
```

返回 `modify` 时，`Emit` 的返回值为 `modified_data`，后续 Hook 和调用方拿到的都是修改后的数据。

### 异常处理

| 场景 | 行为 |
|------|------|
| 脚本退出码 ≠ 0 | 视为执行失败，根据 ErrorStrategy 处理 |
| 脚本超时 | 进程被 kill，返回超时错误 |
| stdout 非法 JSON | 返回 JSON 解析错误 |
| stdout 超过 MaxOutputSize | 截断输出，返回错误 |
| `decision` 不是 allow/deny/modify | 返回无效决策错误 |

---

## 编写 Hook 脚本

### 核心规则

1. **从 stdin 读取** JSON 请求
2. **向 stdout 写入** JSON 响应（仅一行）
3. **日志写到 stderr**（不要写到 stdout，会破坏协议）
4. **退出码 0** 表示正常，非 0 表示故障
5. **响应必须包含** `decision` 字段（`allow`/`deny`/`modify`）

### Python 脚本

#### 使用工具库（推荐）

项目提供了 `scripts/hook_utils.py`，可直接导入：

```python
#!/usr/bin/env python3
import sys
sys.path.insert(0, './scripts')  # 确保能找到 hook_utils

from hook_utils import read_request, write_response, HookResponseBuilder, HookValidator

def main():
    req = read_request()
    data = req.get("data", {})

    # 字段校验
    ok, msg = HookValidator.required(data, ["order_id", "amount"])
    if not ok:
        write_response(HookResponseBuilder.deny(msg))
        return

    # 数值校验
    ok, msg = HookValidator.range_check(data["amount"], 0, 100000)
    if not ok:
        write_response(HookResponseBuilder.deny(msg))
        return

    # 允许通过
    write_response(HookResponseBuilder.allow("订单校验通过", {
        "validated": True,
        "order_id": data["order_id"]
    }))

if __name__ == "__main__":
    main()
```

#### 不使用工具库

```python
#!/usr/bin/env python3
import json
import sys

def main():
    try:
        req = json.loads(sys.stdin.read())
    except json.JSONDecodeError:
        print(json.dumps({"decision": "deny", "message": "无效的 JSON 输入"}))
        sys.exit(1)

    data = req.get("data", {})
    amount = data.get("amount", 0)

    if amount > 10000:
        print(json.dumps({"decision": "deny", "message": f"金额 {amount} 超过上限 10000"}))
    elif amount > 5000:
        data["amount"] = amount * 0.95
        data["discount"] = "5%"
        print(json.dumps({
            "decision": "modify",
            "message": "大额订单已打 95 折",
            "modified_data": data
        }))
    else:
        print(json.dumps({"decision": "allow", "message": "检查通过"}))

if __name__ == "__main__":
    main()
```

#### 使用装饰器

```python
#!/usr/bin/env python3
import sys
sys.path.insert(0, './scripts')

from hook_utils import read_request, write_response, safe_handler

@safe_handler  # 自动捕获异常并转为 deny 响应
def handle(req):
    data = req.get("data", {})
    if data.get("amount", 0) > 10000:
        return {"decision": "deny", "message": "金额超限"}
    return {"decision": "allow", "message": "通过"}

if __name__ == "__main__":
    req = read_request()
    result = handle(req)
    write_response(result)
```

### Bash 脚本（带 jq）

依赖 [jq](https://stedolan.github.io/jq/)，适合有 jq 的环境：

```bash
#!/usr/bin/env bash
set -euo pipefail

# 从 stdin 读取
INPUT=$(cat)

# 解析字段
HOOK_POINT=$(echo "$INPUT" | jq -r '.hook_point // empty')
DATA=$(echo "$INPUT" | jq -c '.data // {}')
USER_ID=$(echo "$DATA" | jq -r '.user_id // "unknown"')

# 日志到 stderr
echo "[INFO] hook_point=$HOOK_POINT user_id=$USER_ID" >&2

# 校验
if [ "$USER_ID" = "unknown" ]; then
    echo '{"decision":"deny","message":"缺少 user_id"}'
    exit 0
fi

# 返回 allow
jq -n \
    --arg uid "$USER_ID" \
    '{
        decision: "allow",
        message: "校验通过",
        extensions: { user_id: $uid }
    }'
```

### 纯 Bash 脚本（无 jq）

不依赖任何外部工具，适合最小化环境：

```bash
#!/usr/bin/env bash
set -euo pipefail

INPUT=$(cat)

# 简易 JSON 字段提取
extract_field() {
    local json="$1" key="$2"
    echo "$json" | grep -o "\"${key}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" \
        | sed "s/\"${key}\"[[:space:]]*:[[:space:]]*\"//;s/\"$//" \
        | head -1
}

extract_number() {
    local json="$1" key="$2"
    echo "$json" | grep -o "\"${key}\"[[:space:]]*:[[:space:]]*[0-9]*\.?[0-9]*" \
        | sed "s/\"${key}\"[[:space:]]*:[[:space:]]*//" \
        | head -1
}

AMOUNT=$(extract_number "$INPUT" "amount")

if [ -n "$AMOUNT" ] && [ "$(echo "$AMOUNT > 10000" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    cat <<EOF
{"decision":"deny","message":"金额 ${AMOUNT} 超限"}
EOF
    exit 0
fi

# 返回 allow
cat <<EOF
{"decision":"allow","message":"检查通过","extensions":{"amount":${AMOUNT:-0}}}
EOF
```

---

## 错误处理策略

通过 `EmitWithStrategy` 指定 Hook 执行出错时的行为：

```go
result, errs := hook.EmitWithStrategy(ctx, "order.before_save", data, strategy)
```

| 策略 | 常量 | 行为 |
|------|------|------|
| **Stop** | `ErrorStrategyStop` | 遇到第一个错误立即停止，返回错误 |
| **Continue** | `ErrorStrategyContinue` | 跳过出错的 Hook，继续执行后续 Hook，收集所有错误 |
| **Ignore** | `ErrorStrategyIgnore` | 忽略错误，像没出错一样继续 |

`Emit` 默认使用 `ErrorStrategyStop`。

### Continue 策略示例

```go
result, errs := hook.EmitWithStrategy(ctx, "order.before_save", data, hook.ErrorStrategyContinue)
if len(errs) > 0 {
    for _, e := range errs {
        log.Printf("Hook 出错: %v", e)
    }
}
// result 仍然可用，来自成功执行的 Hook
```

---

## 进程管理

### 查看状态

```go
pm := engine.ProcessManager()

// 获取状态码
status := pm.Status()
// interfaces.StatusIdle     = 0 (未初始化)
// interfaces.StatusRunning  = 1 (运行中)
// interfaces.StatusStopped  = 2 (已停止)
// interfaces.StatusError    = 3 (出错)

// 列出所有外部 Hook
for _, h := range pm.List(ctx) {
    fmt.Printf("  %s (enabled=%v)\n", h.Name(), h.Enabled())
}
```

### 启停控制

```go
pm := engine.ProcessManager()

// 启动所有外部 Hook
pm.StartAll(ctx)

// 停止所有外部 Hook
pm.StopAll(ctx)
```

### 单个 Hook 控制

```go
// 添加
err := pm.Add(ctx, "order.before_save:/scripts/check.py", externalHook)

// 移除
err := pm.Remove(ctx, "order.before_save:/scripts/check.py")

// 查询
h, err := pm.Get(ctx, "order.before_save:/scripts/check.py")
```

> **注意**：外部 Hook 的 registry key 格式为 `"hook点:脚本路径"`，例如 `"order.before_save:./scripts/audit_order.py"`。

### 调试模式

```go
engine.SetDebugEnabled(true)
// 启用后会输出详细的执行日志（子进程命令、耗时、stdout/stderr 等）
```

---

## 完整示例

以下是一个完整的可运行示例，展示内部 Hook + 外部 Hook 的混合使用：

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "brambleclaw/internal/config/structs"
    "brambleclaw/internal/hook"
)

func main() {
    ctx := context.Background()
    engine := hook.GetEngine()

    // ========== 1. 加载外部 Hook 配置 ==========
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
                },
            },
        },
    }
    if err := engine.LoadConfig(hookConfig); err != nil {
        log.Fatalf("加载配置失败: %v", err)
    }

    // ========== 2. 注册内部 Hook ==========
    engine.Register("system.on_startup", func(ctx context.Context, input any) (any, error) {
        fmt.Println("[内部 Hook] 系统启动")
        return input, nil
    })

    // ========== 3. 列出所有 Hook 点 ==========
    fmt.Println("=== 已注册的 Hook 点 ===")
    for _, point := range engine.List() {
        fmt.Printf("  - %s (%d 个 Hook)\n", point, engine.Count(point))
    }

    // ========== 4. 触发内部 Hook ==========
    fmt.Println("\n=== 内部 Hook 演示 ===")
    result, err := hook.Emit(ctx, "system.on_startup", map[string]string{"event": "demo"})
    if err != nil {
        fmt.Printf("  错误: %v\n", err)
    } else {
        fmt.Printf("  结果: %v\n", result)
    }

    // ========== 5. 触发外部 Hook - 小额订单（allow） ==========
    fmt.Println("\n=== 外部 Hook: 小额订单 ===")
    result, err = engine.Emit(ctx, "order.before_save", map[string]any{
        "order_id": "ORD-001",
        "amount":   150.0,
        "user_id":  "user_99",
        "items": []map[string]any{
            {"name": "apple", "price": 150, "category": "fruit"},
        },
    })
    if err != nil {
        fmt.Printf("  被拒绝: %v\n", err)
    } else {
        fmt.Printf("  通过: %v\n", result)
    }

    // ========== 6. 触发外部 Hook - 大额订单（modify） ==========
    fmt.Println("\n=== 外部 Hook: 大额订单 ===")
    result, err = engine.Emit(ctx, "order.before_save", map[string]any{
        "order_id": "ORD-002",
        "amount":   6000.0,
        "user_id":  "user_100",
        "items": []map[string]any{
            {"name": "laptop", "price": 6000, "category": "electronics"},
        },
    })
    if err != nil {
        fmt.Printf("  被拒绝: %v\n", err)
    } else {
        fmt.Printf("  已修改: %v\n", result)
    }

    // ========== 7. 触发外部 Hook - 超额订单（deny） ==========
    fmt.Println("\n=== 外部 Hook: 超额订单 ===")
    result, err = engine.Emit(ctx, "order.before_save", map[string]any{
        "order_id": "ORD-003",
        "amount":   15000.0,
        "user_id":  "user_101",
        "items": []map[string]any{
            {"name": "car", "price": 15000, "category": "vehicle"},
        },
    })
    if err != nil {
        fmt.Printf("  被拒绝: %v\n", err)
    } else {
        fmt.Printf("  通过: %v\n", result)
    }

    // ========== 8. 触发外部 Hook - Bash 脚本 ==========
    fmt.Println("\n=== 外部 Hook: Bash 脚本 ===")
    result, err = engine.Emit(ctx, "user.after_register", map[string]any{
        "user_id": "user_99",
        "email":   "user99@example.com",
    })
    if err != nil {
        fmt.Printf("  被拒绝: %v\n", err)
    } else {
        fmt.Printf("  结果: %v\n", result)
    }

    // ========== 9. 查看 ProcessManager 状态 ==========
    fmt.Println("\n=== ProcessManager 状态 ===")
    pm := engine.ProcessManager()
    fmt.Printf("  状态码: %d\n", pm.Status())
    for _, h := range pm.List(ctx) {
        fmt.Printf("  - %s (enabled=%v)\n", h.Name(), h.Enabled())
    }

    fmt.Println("\n=== 演示完成 ===")
    os.Exit(0)
}
```

### 运行

```bash
# 确保 scripts 目录下有对应脚本
ls scripts/
# audit_order.py  send_welcome.sh  rate_limit.sh  hook_utils.py

# 运行
go run cmd/example/main.go
```

### 预期输出

```
=== 已注册的 Hook 点 ===
  - system.on_startup (1 个 Hook)
  - order.before_save (1 个 Hook)
  - user.after_register (1 个 Hook)

=== 内部 Hook 演示 ===
[内部 Hook] 系统启动
  结果: map[event:demo]

=== 外部 Hook: 小额订单 ===
  通过: map[amount:150 order_id:ORD-001 user_id:user_99 ...]

=== 外部 Hook: 大额订单 ===
  已修改: map[amount:5700 discount:5% order_id:ORD-002 ...]

=== 外部 Hook: 超额订单 ===
  被拒绝: hook "order.before_save" denied: 单笔金额超限: 15000 > 10000

=== 外部 Hook: Bash 脚本 ===
  结果: map[decision:allow message:欢迎通知已发送 ...]

=== ProcessManager 状态 ===
  状态码: 1
  - order.before_save:./scripts/audit_order.py (enabled=true)
  - user.after_register:./scripts/send_welcome.sh (enabled=true)

=== 演示完成 ===
```

---

## 架构概览

```
┌─────────────────────────────────────────┐
│             应用层 (Your Code)           │
│   hook.Emit(ctx, "order.before_save")   │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│           HookEngine (统一入口)          │
│  ┌─────────────────────────────────────┐ │
│  │ 1. 查找内部 Hook (InternalRegistry) │ │
│  │ 2. 按优先级执行 Go 函数             │ │
│  │ 3. 查找外部 Hook (ProcessManager)   │ │
│  │ 4. 启动子进程，stdin/stdout 通信    │ │
│  │ 5. 处理决策 (allow/deny/modify)     │ │
│  └─────────────────────────────────────┘ │
└──────────────┬──────────────────────────┘
               │
    ┌──────────▼──────────┐
    │   子进程 (脚本)      │
    │  stdin  ← JSON 请求  │
    │  stdout → JSON 响应  │
    │  stderr → 日志输出   │
    └─────────────────────┘
```

---

## 接口契约

框架核心组件遵循 `internal/interfaces` 定义的泛型接口：

| 组件 | 实现接口 | 说明 |
|------|----------|------|
| `InternalHookRegistry` | `interfaces.Registry[*internalHookEntry]` | 内部 Hook 注册表 |
| `ExternalHookRegistry` | `interfaces.Registry[*ExternalHook]` | 外部 Hook 注册表 |
| `ProcessManager` | `interfaces.Manager[*ExternalHook]` | 外部 Hook 生命周期管理 |
| `ExternalHook` | `interfaces.Service` | 单个外部 Hook 实例 |

所有实现均包含编译期接口检查：

```go
var _ interfaces.Registry[*internalHookEntry] = (*InternalHookRegistry)(nil)
var _ interfaces.Registry[*ExternalHook]      = (*ExternalHookRegistry)(nil)
var _ interfaces.Manager[*ExternalHook]       = (*ProcessManager)(nil)
var _ interfaces.Service                      = (*ExternalHook)(nil)
```
