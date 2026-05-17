# Hook 定制开发指南

本文档详细指导用户如何在 neoclaw 框架中定制自己的 Hook，包括内部 Go Hook 和外部脚本 Hook 的完整开发流程。

---

## 目录

1. [快速入门](#快速入门)
2. [内部 Go Hook 开发](#内部-go-hook-开发)
3. [外部脚本 Hook 开发](#外部脚本-hook-开发)
4. [配置文件管理](#配置文件管理)
5. [最佳实践](#最佳实践)
6. [调试与测试](#调试与测试)
7. [常见问题](#常见问题)

---

## 快速入门

### 5分钟快速体验

#### 1. 创建你的第一个内部 Hook

```go
// examples/hooks/hello_hook.go
package hooks

import (
    "context"
    "fmt"
    "neoclaw/internal/hook"
)

func init() {
    // 注册一个简单的 Hook
    hook.Register("example.hello", func(ctx context.Context, input any) (any, error) {
        fmt.Printf("👋 Hello Hook! Input: %v\n", input)
        return input, nil
    })
}
```

#### 2. 触发你的 Hook

```go
// examples/trigger_example.go
package main

import (
    "context"
    "neoclaw/internal/hook"
)

func main() {
    ctx := context.Background()
    
    // 触发 Hook
    result, err := hook.Emit(ctx, "example.hello", map[string]string{
        "message": "Hello from Hook!",
    })
    
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("✅ Result: %v\n", result)
}
```

#### 3. 创建你的第一个外部 Hook（Python）

创建 `scripts/hello_hook.py`：

```python
#!/usr/bin/env python3
import json
import sys

def main():
    # 读取输入
    request = json.loads(sys.stdin.read())
    
    # 处理业务逻辑
    data = request.get("data", {})
    name = data.get("name", "World")
    
    # 输出响应
    response = {
        "decision": "allow",
        "message": f"Hello {name} from Python!",
        "extensions": {
            "processed_by": "python_hook",
            "timestamp": request.get("timestamp")
        }
    }
    
    print(json.dumps(response))
    sys.exit(0)

if __name__ == "__main__":
    main()
```

在配置中添加：

```yaml
# config/hooks.yaml
version: "1.0"
defaults:
  timeout_ms: 5000
  working_dir: "./scripts"

definitions:
  - point: "example.hello_external"
    type: "external"
    enabled: true
    config:
      command: "python3"
      script_path: "./hello_hook.py"
```

---

## 内部 Go Hook 开发

### 基础概念

内部 Hook 是在进程内执行的 Go 函数，具有以下特点：

- ✅ **高性能**: 零 IPC 开销
- ✅ **类型安全**: 完整的 Go 类型系统支持
- ✅ **访问上下文**: 可直接访问应用内部状态
- ⚠️ **部署限制**: 需要重新编译才能修改

### Hook 函数签名

```go
type HookFunc func(ctx context.Context, input any) (any, error)
```

- **ctx**: 上下文，包含超时、取消、请求 ID 等信息
- **input**: 传入的数据，可以是任意类型
- **返回值 (any)**: 修改后的数据（使用流水线模式）
- **返回值 (error)**: 如果返回错误，执行会根据策略停止

### 注册方式

#### 方式 1: 默认优先级注册

```go
package hooks

import (
    "context"
    "neoclaw/internal/hook"
)

func MyHook(ctx context.Context, input any) (any, error) {
    // 处理逻辑
    return input, nil
}

func init() {
    hook.Register("myapp.before_action", MyHook)
}
```

#### 方式 2: 指定优先级注册

```go
hook.RegisterWithPriority("myapp.before_action", hook.PriorityHigh, MyHook)
```

优先级常量：
- `PriorityHigh (10)`: 最先执行
- `PriorityNormal (50)`: 默认
- `PriorityLow (100)`: 最后执行

#### 方式 3: 使用 HookEngine 直接注册

```go
engine := hook.GetEngine()
engine.Register("myapp.before_action", MyHook)
```

### 实际开发示例

#### 示例 1: 数据验证 Hook

```go
package hooks

import (
    "context"
    "fmt"
    "neoclaw/internal/hook"
)

type User struct {
    Name  string
    Email string
    Age   int
}

func ValidateUserHook(ctx context.Context, input any) (any, error) {
    user, ok := input.(User)
    if !ok {
        // 也可以处理 map 类型
        m, ok := input.(map[string]any)
        if !ok {
            return nil, fmt.Errorf("invalid input type")
        }
        user = User{
            Name:  m["name"].(string),
            Email: m["email"].(string),
            Age:   m["age"].(int),
        }
    }

    // 验证规则
    if user.Name == "" {
        return nil, fmt.Errorf("name is required")
    }
    if user.Age < 0 || user.Age > 150 {
        return nil, fmt.Errorf("age must be between 0 and 150")
    }

    return user, nil
}

func init() {
    hook.Register("myapp.user.validate", ValidateUserHook)
}
```

#### 示例 2: 数据转换/增强 Hook

```go
package hooks

import (
    "context"
    "time"
    "neoclaw/internal/hook"
)

type Order struct {
    ID         string
    Amount     float64
    Status     string
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

func EnhanceOrderHook(ctx context.Context, input any) (any, error) {
    order, ok := input.(Order)
    if !ok {
        return input, nil
    }

    // 增强：设置默认状态
    if order.Status == "" {
        order.Status = "pending"
    }

    // 增强：设置时间戳
    now := time.Now()
    if order.CreatedAt.IsZero() {
        order.CreatedAt = now
    }
    order.UpdatedAt = now

    return order, nil
}

func init() {
    hook.RegisterWithPriority("myapp.order.enhance", hook.PriorityNormal, EnhanceOrderHook)
}
```

#### 示例 3: 访问控制 Hook

```go
package hooks

import (
    "context"
    "fmt"
    "neoclaw/internal/hook"
)

type AuthContext struct {
    UserID  string
    Roles   []string
    IsAdmin bool
}

func AccessControlHook(ctx context.Context, input any) (any, error) {
    // 从上下文中获取认证信息（需要应用层支持）
    auth, ok := ctx.Value("auth").(AuthContext)
    if !ok {
        return nil, fmt.Errorf("authentication required")
    }

    // 检查权限
    if !auth.IsAdmin {
        return nil, fmt.Errorf("admin privileges required")
    }

    // 允许执行
    return input, nil
}

func init() {
    hook.Register("myapp.admin.only", AccessControlHook)
}
```

#### 示例 4: 日志/监控 Hook

```go
package hooks

import (
    "context"
    "log"
    "neoclaw/internal/hook"
)

func LoggingHook(ctx context.Context, input any) (any, error) {
    // 记录输入
    log.Printf("[Hook] Executing with input: %v", input)

    // 记录指标（伪代码）
    // metrics.Increment("hook.calls", 1)

    return input, nil
}

func init() {
    // 使用低优先级，确保业务逻辑先执行
    hook.RegisterWithPriority("myapp.monitor", hook.PriorityLow, LoggingHook)
}
```

#### 示例 5: 链式处理 Hook（流水线模式）

```go
package hooks

import (
    "context"
    "strings"
    "neoclaw/internal/hook"
)

// Hook 1: 清理数据
func CleanDataHook(ctx context.Context, input any) (any, error) {
    if str, ok := input.(string); ok {
        return strings.TrimSpace(str), nil
    }
    if m, ok := input.(map[string]any); ok {
        for k, v := range m {
            if s, ok := v.(string); ok {
                m[k] = strings.TrimSpace(s)
            }
        }
    }
    return input, nil
}

// Hook 2: 验证数据
func ValidateDataHook(ctx context.Context, input any) (any, error) {
    // ... 验证逻辑
    return input, nil
}

// Hook 3: 转换数据
func TransformDataHook(ctx context.Context, input any) (any, error) {
    // ... 转换逻辑
    return input, nil
}

func init() {
    hook.RegisterWithPriority("myapp.data.process", hook.PriorityHigh, CleanDataHook)
    hook.RegisterWithPriority("myapp.data.process", hook.PriorityNormal, ValidateDataHook)
    hook.RegisterWithPriority("myapp.data.process", hook.PriorityLow, TransformDataHook)
}
```

### 注销 Hook

```go
// 注销特定 Hook
err := hook.Unregister("myapp.hook", MyHookFunc)
if err != nil {
    // 处理错误
}

// 清除整个 Hook 点的所有 Hook
hook.Clear("myapp.hook")
```

---

## 外部脚本 Hook 开发

### 基础概念

外部 Hook 通过子进程执行脚本，具有以下特点：

- ✅ **灵活**: 支持任意语言（Python、Bash、Node.js 等）
- ✅ **热更新**: 无需重新编译主程序
- ✅ **隔离**: 脚本崩溃不影响主进程
- ⚠️ **开销**: IPC 和进程启动开销

### 通信协议

#### 请求格式（Go → 脚本）

```json
{
  "hook_point": "example.hook_point",
  "timestamp": 1714284000123,
  "request_id": "req_abc123xyz",
  "data": {
    "key": "value",
    "nested": { ... }
  },
  "metadata": {
    "optional": "info"
  }
}
```

#### 响应格式（脚本 → Go）

```json
{
  "decision": "allow",
  "message": "处理完成",
  "modified_data": { ... },
  "extensions": { ... }
}
```

**决策类型**：
- `allow`: 允许，数据不变
- `deny`: 拒绝，返回错误
- `modify`: 允许但修改数据

### Python Hook 开发

#### 方式 1: 基础实现

```python
#!/usr/bin/env python3
import json
import sys

def main():
    try:
        # 读取请求
        request = json.loads(sys.stdin.read())
        
        # 提取数据
        data = request.get("data", {})
        hook_point = request.get("hook_point")
        
        # 处理逻辑
        result = process_data(data, hook_point)
        
        # 输出响应
        print(json.dumps(result))
        sys.exit(0)
        
    except Exception as e:
        error_response = {
            "decision": "deny",
            "message": f"Hook error: {str(e)}"
        }
        print(json.dumps(error_response))
        sys.exit(1)

def process_data(data, hook_point):
    # 你的业务逻辑
    return {
        "decision": "allow",
        "message": "Success"
    }

if __name__ == "__main__":
    main()
```

#### 方式 2: 使用工具库（推荐）

创建 `scripts/hook_utils.py`：

```python
#!/usr/bin/env python3
import json
import sys
import logging
from typing import Dict, Any, Callable, Optional
from functools import wraps

# 配置日志（输出到 stderr）
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    stream=sys.stderr
)
logger = logging.getLogger(__name__)

class HookResponseBuilder:
    """响应构建器"""
    
    @staticmethod
    def allow(message: str = "允许执行", 
              extensions: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        response = {"decision": "allow", "message": message}
        if extensions:
            response["extensions"] = extensions
        return response
    
    @staticmethod
    def deny(message: str, 
             extensions: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        response = {"decision": "deny", "message": message}
        if extensions:
            response["extensions"] = extensions
        return response
    
    @staticmethod
    def modify(data: Dict[str, Any], 
               message: str = "数据已修改",
               extensions: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        response = {
            "decision": "modify",
            "message": message,
            "modified_data": data
        }
        if extensions:
            response["extensions"] = extensions
        return response

class HookValidator:
    """数据验证器"""
    
    @staticmethod
    def required(data: Dict[str, Any], fields: list) -> tuple[bool, str]:
        """检查必需字段"""
        for field in fields:
            if field not in data or data[field] is None:
                return False, f"缺少必需字段: {field}"
        return True, ""
    
    @staticmethod
    def range_check(value: float, min_val: float, max_val: float) -> tuple[bool, str]:
        """范围检查"""
        if value < min_val or value > max_val:
            return False, f"值 {value} 不在范围 [{min_val}, {max_val}] 内"
        return True, ""

def read_request() -> Dict[str, Any]:
    """从 stdin 读取并解析请求"""
    input_data = sys.stdin.read()
    return json.loads(input_data)

def write_response(response: Dict[str, Any]):
    """输出响应到 stdout"""
    print(json.dumps(response, ensure_ascii=False))

def hook_handler(func: Callable) -> Callable:
    """装饰器: 统一处理异常"""
    @wraps(func)
    def wrapper(*args, **kwargs):
        try:
            result = func(*args, **kwargs)
            if "decision" not in result:
                result["decision"] = "allow"
            return result
        except Exception as e:
            logger.exception("Hook execution failed")
            return HookResponseBuilder.deny(f"处理异常: {str(e)}")
    return wrapper
```

使用工具库的完整示例：

```python
#!/usr/bin/env python3
import sys
import logging

sys.path.insert(0, './scripts')

from hook_utils import (
    read_request,
    write_response,
    hook_handler,
    HookResponseBuilder,
    HookValidator
)

logger = logging.getLogger(__name__)

@hook_handler
def handle_order_audit(request):
    data = request.get("data", {})
    
    # 验证必需字段
    ok, msg = HookValidator.required(data, ["order_id", "amount", "user_id"])
    if not ok:
        return HookResponseBuilder.deny(msg)
    
    amount = data.get("amount", 0)
    
    # 金额超限检查
    if amount > 10000:
        return HookResponseBuilder.deny(f"金额 {amount} 超过限额 10000")
    
    # 大额订单折扣
    if amount > 5000:
        modified_data = data.copy()
        discount = amount * 0.05
        modified_data["amount"] = round(amount - discount, 2)
        modified_data["discount"] = round(discount, 2)
        
        return HookResponseBuilder.modify(
            modified_data,
            f"已应用 5% 折扣",
            {"discount_amount": discount}
        )
    
    # 正常通过
    return HookResponseBuilder.allow(
        "订单审核通过",
        {"order_id": data.get("order_id")}
    )

@hook_handler
def handle_greeting(request):
    data = request.get("data", {})
    name = data.get("name", "World")
    return HookResponseBuilder.allow(f"Hello, {name}!")

def main():
    request = read_request()
    hook_point = request.get("hook_point")
    
    # 路由到不同处理器
    handlers = {
        "example.order_audit": handle_order_audit,
        "example.greeting": handle_greeting,
    }
    
    handler = handlers.get(hook_point)
    if handler:
        response = handler(request)
    else:
        response = HookResponseBuilder.allow(f"Unknown hook point: {hook_point}")
    
    write_response(response)
    sys.exit(0)

if __name__ == "__main__":
    main()
```

### Bash Hook 开发

#### 使用 jq（推荐，需要安装）

```bash
#!/usr/bin/env bash
set -euo pipefail

# 读取输入
INPUT=$(cat)

# 解析字段
HOOK_POINT=$(echo "$INPUT" | jq -r '.hook_point // empty')
DATA=$(echo "$INPUT" | jq -c '.data // {}')
AMOUNT=$(echo "$DATA" | jq -r '.amount // 0')
USER_ID=$(echo "$DATA" | jq -r '.user_id // "unknown"')

# 日志（输出到 stderr）
echo "[INFO] Hook: $HOOK_POINT, User: $USER_ID" >&2

# 业务逻辑
if (( $(echo "$AMOUNT > 10000" | bc -l) )); then
    jq -n \
        --arg msg "Amount $AMOUNT exceeds limit" \
        '{
            "decision": "deny",
            "message": $msg
        }'
    exit 0
fi

# 允许
jq -n \
    --arg uid "$USER_ID" \
    '{
        "decision": "allow",
        "message": "Check passed",
        "extensions": { "user_id": $uid }
    }'
```

#### 纯 Bash 实现（无依赖）

```bash
#!/usr/bin/env bash
set -euo pipefail

INPUT=$(cat)

# 简单 JSON 解析（非完整解析，仅适用于简单场景）
extract_string() {
    local json="$1"
    local key="$2"
    echo "$json" | grep -o "\"$key\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" |
        sed "s/\"$key\"[[:space:]]*:[[:space:]]*\"//;s/\"$//" |
        head -1
}

extract_number() {
    local json="$1"
    local key="$2"
    echo "$json" | grep -o "\"$key\"[[:space:]]*:[[:space:]]*[0-9]*\.?[0-9]*" |
        sed "s/\"$key\"[[:space:]]*:[[:space:]]*//" |
        head -1
}

NAME=$(extract_string "$INPUT" "name")
AMOUNT=$(extract_number "$INPUT" "amount")

# 默认值
NAME=${NAME:-"World"}
AMOUNT=${AMOUNT:-0}

# 业务逻辑
if [ "$AMOUNT" != "0" ] && (( $(echo "$AMOUNT > 10000" | bc -l 2>/dev/null || echo 0) )); then
    cat <<EOF
{"decision":"deny","message":"Amount $AMOUNT exceeds limit"}
EOF
    exit 0
fi

# 允许
cat <<EOF
{"decision":"allow","message":"Hello $NAME","extensions":{"processed":true}}
EOF
```

### Node.js Hook 开发

```javascript
#!/usr/bin/env node

const fs = require('fs');

function main() {
    try {
        // 读取输入
        const input = fs.readFileSync(0, 'utf-8');
        const request = JSON.parse(input);
        
        const data = request.data || {};
        const hookPoint = request.hook_point;
        
        // 处理逻辑
        const response = handleRequest(hookPoint, data);
        
        // 输出响应
        console.log(JSON.stringify(response));
        process.exit(0);
        
    } catch (error) {
        const response = {
            decision: "deny",
            message: `Hook error: ${error.message}`
        };
        console.log(JSON.stringify(response));
        process.exit(1);
    }
}

function handleRequest(hookPoint, data) {
    switch (hookPoint) {
        case "example.greeting":
            const name = data.name || "World";
            return {
                decision: "allow",
                message: `Hello, ${name} from Node.js!`,
                extensions: { runtime: "nodejs" }
            };
        
        case "example.validate":
            const amount = data.amount || 0;
            if (amount > 10000) {
                return {
                    decision: "deny",
                    message: `Amount ${amount} exceeds limit`
                };
            }
            return { decision: "allow", message: "Validated" };
        
        default:
            return { decision: "allow", message: "Unknown hook" };
    }
}

main();
```

### 注册外部 Hook

#### 通过配置文件注册（推荐）

```yaml
version: "1.0"

defaults:
  timeout_ms: 5000
  max_output_size: 1048576
  working_dir: "./scripts"
  shell: "/bin/bash"
  env:
    - "HOOK_ENV=production"
    - "LOG_LEVEL=info"

definitions:
  # Python Hook
  - point: "example.order_audit"
    type: "external"
    enabled: true
    priority: 50
    config:
      command: "python3"
      script_path: "./order_audit.py"
      args:
        - "--strict"
      timeout_ms: 3000
      working_dir: "./scripts"
      env:
        - "AUDIT_LEVEL=high"
  
  # Bash Hook
  - point: "example.greeting"
    type: "external"
    enabled: true
    config:
      command: "bash"
      script_path: "./greeting.sh"
  
  # Node.js Hook
  - point: "example.node"
    type: "external"
    enabled: true
    config:
      command: "node"
      script_path: "./node_hook.js"
```

#### 通过代码注册

```go
engine := hook.GetEngine()

err := engine.RegisterExternal("example.hook", structs.ExternalConfig{
    Command:    "python3",
    ScriptPath: "./scripts/my_hook.py",
    Args:       []string{"--verbose"},
    TimeoutMs:  3000,
    WorkingDir: "./scripts",
    Env:        []string{"CUSTOM_ENV=value"},
})

if err != nil {
    log.Fatal(err)
}
```

---

## 配置文件管理

### 配置结构详解

完整的配置文件结构：

```yaml
version: "1.0"

defaults:
  timeout_ms: 5000                    # 默认超时（毫秒）
  max_output_size: 1048576            # 最大输出大小（字节，默认 1MB）
  working_dir: "./scripts"            # 默认工作目录
  shell: "/bin/bash"                  # 默认 Shell
  env:                                 # 默认环境变量
    - "HOOK_ENV=production"
    - "LOG_LEVEL=info"

definitions:
  - point: "hook.point.name"           # Hook 点名称
    type: "external"                   # 类型: internal / external
    enabled: true                      # 是否启用
    priority: 50                       # 优先级 (1-100)
    config:                            # 外部 Hook 配置（仅 type=external）
      command: "python3"               # 执行命令
      script_path: "./script.py"       # 脚本路径
      args: []                         # 额外参数
      timeout_ms: 3000                 # 超时（覆盖默认）
      working_dir: "./scripts"         # 工作目录（覆盖默认）
      max_output_size: 524288          # 最大输出（覆盖默认）
      env: []                          # 额外环境变量

thinking_visibility:                   # 思考过程可视化配置
  enabled: true
  max_events: 200
  points:
    - point: "hook.point.llm.request"
      enabled: true
      verbosity: "summary"
    - point: "hook.point.llm.response"
      enabled: true
      verbosity: "summary"
```

### 配置加载方式

#### 方式 1: 通过代码加载

```go
package main

import (
    "context"
    "log"
    
    "neoclaw/internal/config/structs"
    "neoclaw/internal/hook"
)

func loadHooks() {
    config := structs.HookConfig{
        Version: "1.0",
        Defaults: structs.HookDefaults{
            TimeoutMs:     5000,
            MaxOutputSize: 1024 * 1024,
            WorkingDir:    "./scripts",
            Shell:         "/bin/bash",
            Env:           []string{"LOG_LEVEL=info"},
        },
        Definitions: []structs.HookDefinition{
            {
                Point:   "example.hook",
                Type:    structs.HookTypeExternal,
                Enabled: true,
                Config: structs.ExternalConfig{
                    Command:    "python3",
                    ScriptPath: "./scripts/hook.py",
                },
            },
        },
    }
    
    engine := hook.GetEngine()
    if err := engine.LoadConfig(config); err != nil {
        log.Fatal(err)
    }
}
```

#### 方式 2: 从 YAML 文件加载

```go
package main

import (
    "os"
    
    "gopkg.in/yaml.v3"
    "neoclaw/internal/config/structs"
    "neoclaw/internal/hook"
)

func loadConfigFromYAML(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return err
    }
    
    var config structs.HookConfig
    if err := yaml.Unmarshal(data, &config); err != nil {
        return err
    }
    
    engine := hook.GetEngine()
    return engine.LoadConfig(config)
}
```

### 多环境配置

```yaml
# config/hooks.dev.yaml
version: "1.0"
defaults:
  timeout_ms: 10000  # 开发环境超时更长
  working_dir: "./scripts"
  env:
    - "HOOK_ENV=development"

definitions:
  - point: "example.debug"
    type: "external"
    enabled: true
    config:
      command: "python3"
      script_path: "./debug_hook.py"
---
# config/hooks.prod.yaml
version: "1.0"
defaults:
  timeout_ms: 3000   # 生产环境更严格
  working_dir: "/opt/app/scripts"
  env:
    - "HOOK_ENV=production"

definitions:
  - point: "example.audit"
    type: "external"
    enabled: true
    config:
      command: "python3"
      script_path: "/opt/app/scripts/audit.py"
```

---

## 最佳实践

### 1. 错误处理策略

#### 内部 Hook 错误处理

```go
// 使用特定错误类型
type HookError struct {
    Point   string
    Reason  string
    Details map[string]any
}

func (e *HookError) Error() string {
    return fmt.Sprintf("[%s] %s", e.Point, e.Reason)
}

// 在 Hook 中使用
func MyHook(ctx context.Context, input any) (any, error) {
    if somethingWrong {
        return nil, &HookError{
            Point:   "my.hook",
            Reason:  "validation failed",
            Details: map[string]any{"field": "name"},
        }
    }
    return input, nil
}

// 触发时选择策略
result, errs := hook.EmitWithStrategy(
    ctx, 
    "my.hook", 
    data,
    hook.ErrorStrategyContinue,  // 继续执行后续 Hook
)
```

#### 外部 Hook 错误处理

```python
# 在脚本中总是输出有效 JSON
try:
    # 业务逻辑
    result = do_something()
    print(json.dumps(result))
    sys.exit(0)
except Exception as e:
    # 即使出错也要输出有效 JSON
    error_response = {
        "decision": "deny",
        "message": str(e),
        "extensions": {
            "error_type": type(e).__name__
        }
    }
    print(json.dumps(error_response))
    sys.exit(1)
```

### 2. 性能优化

#### 内部 Hook

```go
// 避免在 Hook 中做耗时操作
func FastHook(ctx context.Context, input any) (any, error) {
    // ✅ 好：快速计算
    result := transform(input)
    return result, nil
}

func SlowHook(ctx context.Context, input any) (any, error) {
    // ❌ 坏：耗时操作
    time.Sleep(1 * time.Second)
    return input, nil
}
```

#### 外部 Hook

```python
# 脚本启动速度很重要
#!/usr/bin/env python3

# ✅ 好：只导入必要的库
import sys
import json

# ❌ 坏：导入大量不必要的库
# import numpy
# import pandas
# import tensorflow

def main():
    # 快速处理
    pass
```

### 3. 安全实践

#### 验证所有输入

```go
func SafeHook(ctx context.Context, input any) (any, error) {
    data, ok := input.(map[string]any)
    if !ok {
        return nil, fmt.Errorf("invalid input")
    }
    
    // 验证每个字段
    userID, ok := data["user_id"].(string)
    if !ok {
        return nil, fmt.Errorf("user_id must be string")
    }
    
    // 不要信任输入数据
    if len(userID) > 100 {
        return nil, fmt.Errorf("user_id too long")
    }
    
    return input, nil
}
```

#### 外部脚本安全

```python
# 不要执行从输入中获取的命令
# ❌ 危险
cmd = data.get("command")
os.system(cmd)

# ✅ 安全
allowed_commands = {"ls", "pwd"}
cmd = data.get("command")
if cmd in allowed_commands:
    os.system(cmd)
```

### 4. 日志与监控

```go
func MonitoredHook(ctx context.Context, input any) (any, error) {
    start := time.Now()
    defer func() {
        duration := time.Since(start)
        log.Printf("[Hook] my.hook took %s", duration)
        // metrics.RecordHookDuration("my.hook", duration)
    }()
    
    // 业务逻辑
    return input, nil
}
```

```python
# 在 Python 中使用 stderr 记录日志
import sys
import logging

logging.basicConfig(stream=sys.stderr, level=logging.INFO)
logger = logging.getLogger(__name__)

logger.info("Processing hook request")
```

### 5. 组织 Hook 代码

#### 推荐的项目结构

```
your-project/
├── internal/
│   └── hooks/              # 内部 Go Hook
│       ├── order_hooks.go
│       ├── user_hooks.go
│       ├── audit_hooks.go
│       └── init.go         # 注册入口
├── scripts/                # 外部脚本 Hook
│   ├── python/
│   │   ├── audit_order.py
│   │   ├── validate_user.py
│   │   └── hook_utils.py   # 工具库
│   ├── bash/
│   │   └── notify.sh
│   └── node/
│       └── webhook.js
├── config/
│   └── hooks/
│       ├── base.yaml
│       ├── dev.yaml
│       └── prod.yaml
└── docs/
    └── hooks/
        └── HOOKS.md        # Hook 文档
```

#### init.go 示例

```go
package hooks

import "neoclaw/internal/hook"

func init() {
    // 注册所有内部 Hook
    registerOrderHooks()
    registerUserHooks()
    registerAuditHooks()
}
```

---

## 调试与测试

### 启用调试模式

```go
engine := hook.GetEngine()
engine.SetDebugEnabled(true)
```

启用后会输出：
- Hook 注册信息
- 执行顺序
- 输入/输出数据
- 错误详情
- 执行耗时

### 测试 Hook

#### 内部 Hook 单元测试

```go
package hooks_test

import (
    "context"
    "testing"
    
    "your-project/internal/hooks"
    "neoclaw/internal/hook"
)

func TestMyHook(t *testing.T) {
    ctx := context.Background()
    
    // 测试允许的情况
    result, err := hook.Emit(ctx, "my.hook", validInput)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }
    
    // 测试拒绝的情况
    _, err = hook.Emit(ctx, "my.hook", invalidInput)
    if err == nil {
        t.Fatal("Expected error, got none")
    }
}
```

#### 外部 Hook 测试脚本

创建 `scripts/test_hook.py`：

```python
#!/usr/bin/env python3
import sys
import json

def test_hook():
    # 模拟请求
    test_request = {
        "hook_point": "example.test",
        "timestamp": 1234567890,
        "request_id": "test_req_123",
        "data": {
            "test_field": "test_value"
        }
    }
    
    # 将请求输入到 Hook 脚本
    import subprocess
    process = subprocess.Popen(
        ["python3", "./your_hook.py"],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE
    )
    
    stdout, stderr = process.communicate(
        json.dumps(test_request).encode()
    )
    
    print(f"Exit code: {process.returncode}")
    print(f"Stdout: {stdout.decode()}")
    print(f"Stderr: {stderr.decode()}")
    
    try:
        response = json.loads(stdout.decode())
        print(f"Response: {json.dumps(response, indent=2)}")
    except:
        print("Invalid JSON response")

if __name__ == "__main__":
    test_hook()
```

### 获取执行指标

```go
// 获取特定 Hook 点的指标
metrics := hook.Metrics("my.hook")
if metrics != nil {
    fmt.Printf("Execution count: %d\n", metrics.ExecutionCount)
    fmt.Printf("Total duration: %s\n", metrics.TotalDuration)
    fmt.Printf("Error count: %d\n", metrics.ErrorCount)
}

// 列出所有 Hook 点
points := hook.List()
for _, point := range points {
    fmt.Printf("Hook point: %s\n", point)
}

// 重置指标
hook.ResetMetrics()
```

---

## 常见问题

### Q: 内部 Hook 和外部 Hook 如何选择？

**A**: 使用以下决策树：
- 简单逻辑、性能敏感 → 内部 Hook
- 需要频繁更新、多语言支持 → 外部 Hook
- 数据验证、快速计算 → 内部 Hook
- 调用外部 API、复杂业务 → 外部 Hook

### Q: 如何调试 Hook 不执行？

**A**: 检查以下几点：
1. Hook 是否已正确注册？
2. Hook 点名称是否正确？
3. Hook 是否启用？
4. 使用 `engine.List()` 查看注册的 Hook 点
5. 启用调试模式查看日志

### Q: 外部 Hook 超时怎么办？

**A**: 
1. 增加 `timeout_ms` 配置
2. 优化脚本性能
3. 将复杂操作异步化
4. 检查是否有资源竞争

### Q: 如何让 Hook 修改数据？

**A**:
- 内部 Hook: 返回修改后的数据
- 外部 Hook: 返回 `decision: "modify"` 和 `modified_data`

### Q: Hook 执行顺序如何控制？

**A**:
- 使用 `RegisterWithPriority` 设置优先级
- 优先级数值越小越先执行
- 相同优先级按注册顺序执行

### Q: 如何临时禁用 Hook？

**A**:
- 配置文件: 设置 `enabled: false`
- 代码: 使用 `Unregister` 注销
- 全局: 不触发该 Hook 点即可

---

## 附录

### A. 完整示例项目结构

参见 `examples/hooks/` 目录下的完整示例代码。

### B. 可用的 Hook 点列表

参见 `docs/hooks/hook-points.md`。

### C. 更多文档

- [Hook 使用指南](./hook-usage.md)
- [Hook 点列表](./hook-points.md)
- [Hook 设计文档](./hook-upgrade-design.md)

---

**文档版本**: 1.0
**最后更新**: 2026-05-17
