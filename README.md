# NeoClaw：你的一站式AI助手

![License](https://img.shields.io/badge/license-MIT-blue) ![Go Version](https://img.shields.io/badge/go-1.25.9-00ADD8?logo=go&logoColor=white) ![Status](https://img.shields.io/badge/Matrix-Enabled-00FF41?logo=matrix&logoColor=00FF41)

Neo 总能运用他的能力，帮你搞定一切--

开个玩笑：）NeoClaw 是一款基于Go 语言的轻量级、可扩展的一站式AI Agent 框架，支持多 agent 协作、多渠道接入、技能扩展和 Hook 系统。提供飞书，钉钉等渠道接入。

## ✨ 特性

- 🤖 **多 Agent 协作** - 采用类似 AutoGen 的团队协作模式，Agent 可创建多agent 团队来分步解决复杂任务
- 📡 **多渠道接入** - 原生支持 QQ、飞书、钉钉、CLI 等渠道
- 🛠️ **技能系统** - YAML + Markdown 定义，支持自动发现与热加载
- 🔌 **Hook 系统** - 外部脚本扩展，支持用户定制自己的 Bash/Python 脚本，实现运行时控制
- 🧰 **内置工具** - Web 搜索、URL 解析、文件读写、系统操作，团队创建等
- ⏰ **定时机制** - 支持设置定时任务，到时自动执行
- 🧠 **记忆管理** - 采取层级摘要压缩策略，将对话历史递归抽象为多层 Summary Node，使旧记忆高度概括而近期记忆保持细节
- 🎨 **TUI 界面** - 基于 Bubble Tea 的终端交互，支持丰富的对话内命令操作
- 📝 **结构化日志** - 基于zerolog + lumerjack 的高性能日志记录

## 🚀 快速开始

### 安装

#### Windows

```powershell
git clone https://github.com/seirok/neoclaw.git
cd neoclaw
go build -o brambleclaw.exe ./cmd/brambleclaw
```

#### macOS / Linux

```bash
git clone https://github.com/seirok/neoclaw.git
cd neoclawhttps://github.com/seirok/neoclaw.git
go build -o neoclaw ./cmd/neoclaw
```

### 配置

首次运行会自动创建配置文件：

- **Windows**: `%USERPROFILE%\.neoclaw\settings.json`
- **macOS / Linux**: `~/.neoclaw/settings.json`
- 启动前，请先设置好环境变量

#### Windows 环境变量

```powershell
$env:NEO_KEY="your-api-key"
$env:NEO_URL="https://api.example.com"
$env:NEO_MODEL="your model"
```

#### macOS / Linux 环境变量

```bash
export NEO_KEY="your-api-key"
export NEO_URL="https://api.example.com"
export NEO_MODEL="your model"
```

### 运行

#### Windows

```powershell
# 启动agent
.\brambleclaw.exe agent

# Debug 模式
.\brambleclaw.exe debug --lines 100
```

#### macOS / Linux

```bash
# 启动agent
./neoclaw agent

# Debug 模式
./neoclaw debug --lines 100
```

### 对话内命令

| 命令 | 说明 |
|------|------|
| `/help` | 列出所有可用命令和技能 |
| `/context` | 查看当前上下文使用情况（含可视化进度条） |
| `/compact` | 手动触发上下文压缩 |
| `/model [名称]` | 查看当前模型，或切换到指定模型 |
| `/skills` | 列出所有可用技能及详情 |
| `/undo` | 撤销上一轮对话 |
| `/clear` | 创建新会话（保留旧会话记录） |
| `/reset` | 重置当前会话（清空消息，保留会话标识） |
| `/resume` | 恢复历史会话 |
| `/delete` | 删除历史会话 |

## 🏗️ 架构设计

### 核心模块


| 模块       | 说明                          |
| ---------- | ----------------------------- |
| `agent`    | ChatAgent 接口和 LLM 集成     |
| `runtime`  | Topic 发布订阅，Agent 运行时  |
| `messages` | 消息类型系统                  |
| `team`     | 多 Agent 团队协作             |
| `channel`  | 外部渠道集成（QQ/飞书/钉钉）  |
| `skill`    | 技能系统（热加载）            |
| `hook`     | Hook 扩展系统                 |
| `tools`    | 工具系统（Web Search/MCP 等） |
| `session`  | 会话管理和持久化              |
| `config`   | 配置管理                      |

## 🛠️ 技术栈

- **Go 1.25.9**
- **Cobra** - CLI 框架
- **Bubble Tea** - TUI 界面
- **zerolog** - 结构化日志

## 📄 许可证

MIT License
