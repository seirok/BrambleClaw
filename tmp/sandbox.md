# Sandbox 沙箱模块

## 概述

Sandbox 模块为 brambleclaw 提供了安全的执行环境，实现路径隔离、权限控制和审计功能。

## 核心功能

### 1. 路径隔离
- 文件操作限制在指定的工作目录内
- 可配置是否允许读取工作目录外的文件
- 支持额外的允许写入路径（如 `/tmp`）

### 2. 权限控制
- 文件系统操作：读、写、删除、列表
- 命令执行：只允许白名单中的命令
- 文件大小限制和总容量限制

### 3. 安全执行
- 命令执行超时控制
- 最大输出大小限制
- 工作目录隔离

### 4. 审计日志
- 完整记录所有文件操作
- 记录命令执行历史
- 支持审计事件查询和统计
- 日志轮转和压缩

## 模块结构

```
sandbox/
├── config.go          # 配置定义和加载
├── audit.go            # 审计日志功能
├── sandbox.go          # 核心沙箱功能
├── filesystem.go       # 带沙箱的文件系统工具
├── shell.go            # 带沙箱的命令执行工具
└── README.md           # 使用文档
```

## 配置示例

```yaml
# sandbox.yaml
enabled: true
workspace: "./workspace"
allow_read_outside_workspace: false

filesystem:
  allow_write_paths:
    - "^/tmp/"
    - "^/var/tmp/"
  max_file_size: 104857600      # 100MB
  max_total_size: 1073741824    # 1GB

execution:
  allowed_commands:
    - ls
    - cat
    - grep
    - python3
    - node
  timeout: 30s
  max_output_size: 1048576      # 1MB

audit:
  enabled: true
  log_path: "./logs/sandbox_audit.log"
  max_size: 100                 # MB
  max_backups: 10
```

## 使用方式

### 1. 初始化沙箱

```go
package main

import (
    "log"
    "brambleclaw/sandbox"
)

func main() {
    // 加载配置
    config, err := sandbox.LoadConfig("sandbox.yaml")
    if err != nil {
        log.Fatal(err)
    }

    // 创建审计日志记录器
    auditLogger, err := sandbox.NewAuditLogger(config.Audit)
    if err != nil {
        log.Fatal(err)
    }
    defer auditLogger.Close()

    // 创建沙箱
    sb, err := sandbox.NewSandbox(config, auditLogger)
    if err != nil {
        log.Fatal(err)
    }

    // 使用沙箱...
}
```

### 2. 文件系统操作

```go
// 创建带沙箱的文件系统工具
fsTool := sandbox.NewFileSystemTool(sb)

// 读取文件（会自动验证路径）
content, err := fsTool.Execute(ctx, `{
    "command": "read",
    "path": "test.txt"
}`)

// 写入文件（会自动验证路径和权限）
result, err := fsTool.Execute(ctx, `{
    "command": "write",
    "path": "output.txt",
    "content": "Hello, World!"
}`)

// 列出目录
files, err := fsTool.Execute(ctx, `{
    "command": "list",
    "path": "."
}`)

// 删除文件
result, err := fsTool.Execute(ctx, `{
    "command": "delete",
    "path": "old.txt"
}`)
```

### 3. 命令执行

```go
// 创建带沙箱的 Shell 工具
shellTool := sandbox.NewShellTool(sb)

// 执行白名单中的命令
result, err := shellTool.Execute(ctx, `{
    "command": "ls -la"
}`)

// 执行 Python 脚本
result, err := shellTool.Execute(ctx, `{
    "command": "python3 -c 'print(\"Hello\")'"
}`)

// 尝试执行不在白名单中的命令会被拒绝
result, err := shellTool.Execute(ctx, `{
    "command": "rm -rf /"
}`)
// 返回错误: 命令 'rm' 不在允许的白名单中
```

### 4. 直接操作沙箱

```go
// 验证路径
err := sb.ValidatePath("/etc/passwd", false) // 读取
err := sb.ValidatePath("/tmp/test.txt", true) // 写入

// 执行命令
output, err := sb.ExecuteCommand(ctx, "ls -la")

// 检查命令是否在白名单中
err := sb.ValidateCommand("python3")
```

## 审计日志

### 审计事件类型

```go
const (
    // 文件操作
    AuditEventFileRead    = "FILE_READ"
    AuditEventFileWrite   = "FILE_WRITE"
    AuditEventFileDelete  = "FILE_DELETE"
    AuditEventFileList    = "FILE_LIST"

    // 命令执行
    AuditEventCommandStart = "COMMAND_START"
    AuditEventCommandEnd   = "COMMAND_END"
    AuditEventCommandBlock = "COMMAND_BLOCK"

    // 安全事件
    AuditEventAccessDenied = "ACCESS_DENIED"
    AuditEventPathEscape   = "PATH_ESCAPE"
    AuditEventTimeout      = "TIMEOUT"
)
```

### 查看审计日志

审计日志以 JSON 格式存储，每行一个事件：

```json
{
  "timestamp": "2024-04-10T12:00:00Z",
  "event_type": "FILE_READ",
  "session_id": "sess_123",
  "operation": "read",
  "target": "/workspace/data.txt",
  "success": true,
  "workspace": "/workspace",
  "agent_name": "main",
  "duration": 1000000
}
```

### 审计日志轮转

- 单个日志文件最大 100MB
- 最多保留 10 个备份
- 自动压缩旧日志
- 保留最近 30 天的日志

## 安全特性

### 1. 路径隔离
- 所有文件操作限制在配置的工作目录内
- 防止路径遍历攻击（如 `../../../etc/passwd`）
- 可配置是否允许读取工作目录外的文件

### 2. 命令白名单
- 只允许执行预定义的安全命令
- 防止执行危险的系统命令
- 支持常用的开发工具（Python、Node.js、Git 等）

### 3. 资源限制
- 命令执行超时控制
- 最大输出大小限制
- 文件大小限制
- 存储容量限制

### 4. 审计追踪
- 所有操作都有完整的日志记录
- 支持安全事件分析和取证
- 可导出审计报告

## 最佳实践

1. **最小权限原则**：只授予必要的权限
2. **白名单机制**：明确允许的操作，默认拒绝其他所有操作
3. **审计先行**：在生产环境启用完整的审计日志
4. **定期审查**：定期审查审计日志，发现潜在的安全问题
5. **配置分离**：将沙箱配置与应用程序配置分离

## 故障排除

### 常见问题

1. **路径访问被拒绝**
   - 检查路径是否在工作目录内
   - 检查是否允许读取/写入该路径
   - 查看审计日志获取详细信息

2. **命令执行被拒绝**
   - 检查命令是否在白名单中
   - 确认命令名称正确（不含参数）
   - 查看审计日志获取详细信息

3. **审计日志未记录**
   - 确认审计功能已启用
   - 检查日志目录权限
   - 检查磁盘空间是否充足

## 扩展开发

### 添加自定义审计事件

```go
// 定义新的事件类型
const AuditEventCustom AuditEventType = "CUSTOM_EVENT"

// 记录自定义事件
func (s *Sandbox) LogCustomEvent(target string, success bool, details string) {
    s.logAuditEvent(AuditEventCustom, target, success, details)
}
```

### 集成到 Agent

```go
// 在 Agent 中启用沙箱
func (a *Agent) EnableSandbox(config *sandbox.SandboxConfig) error {
    auditLogger, err := sandbox.NewAuditLogger(config.Audit)
    if err != nil {
        return err
    }

    sb, err := sandbox.NewSandbox(config, auditLogger)
    if err != nil {
        return err
    }

    // 创建带沙箱的工具
    fsTool := sandbox.NewFileSystemTool(sb)
    shellTool := sandbox.NewShellTool(sb)

    // 注册工具到 Agent
    a.toolRegistry.Register(fsTool)
    a.toolRegistry.Register(shellTool)

    return nil
}
```

## 许可证

MIT License
