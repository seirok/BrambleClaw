# Hook 快速参考卡片

## 目录
- [内部 Hook 速查](#内部-hook-速查)
- [外部 Hook 速查](#外部-hook-速查)
- [配置速查](#配置速查)
- [常见场景](#常见场景)

---

## 内部 Hook 速查

### 最小模板

```go
package hooks

import (
    "context"
    "neoclaw/internal/hook"
)

func MyHook(ctx context.Context, input any) (any, error) {
    // 你的逻辑
    return input, nil
}

func init() {
    hook.Register("my.hook", MyHook)
}
```

### 优先级

```go
hook.RegisterWithPriority("my.hook", hook.PriorityHigh, MyHook)
// PriorityHigh = 10
// PriorityNormal = 50 (默认)
// PriorityLow = 100
```

### 触发

```go
// 基本触发
result, err := hook.Emit(ctx, "my.hook", data)

// 带策略
result, errs := hook.EmitWithStrategy(ctx, "my.hook", data, 
    hook.ErrorStrategyContinue)

// 必须成功
result := hook.MustEmit(ctx, "my.hook", data)
```

### 错误策略

| 策略 | 行为 |
|------|------|
| `ErrorStrategyStop` | 出错即停止（默认）|
| `ErrorStrategyContinue` | 继续执行后续 Hook |
| `ErrorStrategyIgnore` | 忽略错误继续 |

---

## 外部 Hook 速查

### Python 最小模板

```python
#!/usr/bin/env python3
import json
import sys

def main():
    request = json.loads(sys.stdin.read())
    data = request.get("data", {})
    
    response = {
        "decision": "allow",
        "message": "Hello!"
    }
    
    print(json.dumps(response))
    sys.exit(0)

if __name__ == "__main__":
    main()
```

### 使用工具库

```python
from hook_utils import (
    read_request,
    write_response,
    hook_handler,
    HookResponseBuilder,
    HookValidator
)

@hook_handler
def handle(request):
    data = request.get("data", {})
    
    ok, msg = HookValidator.required(data, ["name"])
    if not ok:
        return HookResponseBuilder.deny(msg)
    
    return HookResponseBuilder.allow("Success!")

def main():
    request = read_request()
    response = handle(request)
    write_response(response)
```

### 决策类型

```json
// 1. Allow - 允许
{"decision": "allow", "message": "Ok"}

// 2. Deny - 拒绝
{"decision": "deny", "message": "Rejected"}

// 3. Modify - 允许并修改
{"decision": "modify", "message": "Changed", "modified_data": {...}}
```

### Bash 最小模板

```bash
#!/usr/bin/env bash
set -euo pipefail

INPUT=$(cat)

cat <<EOF
{"decision":"allow","message":"Hello from Bash"}
EOF
```

---

## 配置速查

### YAML 配置结构

```yaml
version: "1.0"

defaults:
  timeout_ms: 5000
  max_output_size: 1048576
  working_dir: "./scripts"
  env:
    - "KEY=VALUE"

definitions:
  - point: "my.hook"
    type: "external"
    enabled: true
    priority: 50
    config:
      command: "python3"
      script_path: "./script.py"
      args: ["--arg"]
      timeout_ms: 3000
      env: ["EXTRA=value"]
```

### Go 代码配置

```go
config := structs.HookConfig{
    Version: "1.0",
    Defaults: structs.HookDefaults{
        TimeoutMs:  5000,
        WorkingDir: "./scripts",
    },
    Definitions: []structs.HookDefinition{
        {
            Point:   "my.hook",
            Type:    structs.HookTypeExternal,
            Enabled: true,
            Config: structs.ExternalConfig{
                Command:    "python3",
                ScriptPath: "./script.py",
            },
        },
    },
}

engine.LoadConfig(config)
```

---

## 常见场景

### 场景 1: 数据验证

```go
func ValidateHook(ctx context.Context, input any) (any, error) {
    data := input.(map[string]any)
    if data["name"] == "" {
        return nil, fmt.Errorf("name required")
    }
    return input, nil
}
```

### 场景 2: 数据转换

```go
func TransformHook(ctx context.Context, input any) (any, error) {
    data := input.(map[string]any)
    data["new_field"] = "value"
    return data, nil
}
```

### 场景 3: Python 条件拒绝

```python
data = request.get("data", {})
if data.get("amount", 0) > 10000:
    return {"decision": "deny", "message": "Too big!"}
return {"decision": "allow"}
```

### 场景 4: Python 修改数据

```python
data = request.get("data", {}).copy()
data["amount"] *= 0.95  # 5% 折扣
return {
    "decision": "modify",
    "message": "Discount applied",
    "modified_data": data
}
```

### 场景 5: 链式处理

```go
// 注册多个 Hook，按优先级执行
hook.RegisterWithPriority("chain", hook.PriorityHigh, CleanHook)
hook.RegisterWithPriority("chain", hook.PriorityNormal, ValidateHook)
hook.RegisterWithPriority("chain", hook.PriorityLow, EnhanceHook)
```

### 场景 6: 调试

```go
// 启用调试
engine.SetDebugEnabled(true)

// 查看指标
metrics := hook.Metrics("my.hook")
fmt.Printf("Executions: %d\n", metrics.ExecutionCount)

// 列出 Hook 点
points := hook.List()
```

---

## 调试清单

如果 Hook 不工作，检查：

- [ ] Hook 是否已正确注册？
- [ ] Hook 点名称是否正确？
- [ ] Hook 是否启用（`enabled: true`）？
- [ ] 脚本路径是否正确？
- [ ] 脚本是否有执行权限？
- [ ] 启用调试模式查看日志？
- [ ] 脚本是否输出有效 JSON？
- [ ] 响应是否包含 `decision` 字段？

---

## 快速复制代码块

### 内部 Hook 模板

```go
package hooks

import (
    "context"
    "neoclaw/internal/hook"
)

func NAMEHook(ctx context.Context, input any) (any, error) {
    // TODO: 你的代码
    return input, nil
}

func init() {
    hook.RegisterWithPriority("POINT", hook.PriorityNormal, NAMEHook)
}
```

### Python Hook 模板

```python
#!/usr/bin/env python3
import json
import sys
sys.path.insert(0, './scripts')

from hook_utils import (
    read_request,
    write_response,
    hook_handler,
    HookResponseBuilder
)

@hook_handler
def handle(request):
    data = request.get("data", {})
    return HookResponseBuilder.allow("Success!")

if __name__ == "__main__":
    write_response(handle(read_request()))
```

### Bash Hook 模板

```bash
#!/usr/bin/env bash
set -euo pipefail

INPUT=$(cat)

cat <<EOF
{"decision":"allow","message":"Hello!"}
EOF
```

---

## 更多资源

- 完整文档: `docs/hooks/hook-customization-guide.md`
- 使用文档: `docs/hooks/hook-usage.md`
- Hook 点列表: `docs/hooks/hook-points.md`
- 示例代码: `examples/`
