# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Brambleclaw is a Go-based AI Agent framework featuring a modular architecture with Gateway routing, message bus communication, and pluggable tools.

## Architecture

### Core Components

1. **Gateway** (`gateway/`): Message router that directs inbound messages to appropriate Agents based on routing rules
2. **Agent** (`agent/`): Core AI agent implementation with session management, LLM client, and orchestrator
3. **Message Bus** (`bus/`): Pub/sub message system for internal communication between components
4. **Channels** (`channel/`): Interface implementations for different input sources (CLI, etc.)
5. **Tools** (`tools/`): Pluggable tool system including file system, shell, code sandbox, web search
6. **Config** (`config/`): Configuration loading with multi-path search and validation

### Data Flow

```
Inbound Message → Channel → MessageBus → Gateway → Router → Agent → LLM → Response → OutBound
```

### Key Files

- `cli/cli.go`: Main CLI entry with cobra commands (`agent`, `init`, `debug`)
- `gateway/gateway.go`: Gateway lifecycle and message processing loops
- `agent/agent.go`: Agent initialization and message processing
- `agent/orchestrator.go`: LLM interaction and tool execution flow
- `bus/queue.go`: Message bus implementation with pub/sub

## Common Commands

### Build and Run

```bash
# Build the project
go build -o brambleclaw .

# Run the CLI
go run . --help
go run . agent              # Start interactive agent
go run . agent -m "hello"   # Single message mode
go run . init               # Configuration wizard
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests for specific package
go test ./agent/...
go test ./gateway/...

# Run a specific test
go test ./agent -run TestOrchestrator
go test -v ./config -run TestLoad

# Run with coverage
go test -cover ./...
```

### Linting

```bash
# Format code
go fmt ./...

# Vet code
go vet ./...

# Run golangci-lint (if installed)
golangci-lint run
```

## Configuration

Configuration is loaded from multiple paths in priority order:

1. Explicit path via `BRAMBLECLAW_CONFIG` env var
2. `./config/config.json` (current directory)
3. User config dir: `~/.config/brambleclaw/config.json`
4. Home directory: `~/.brambleclaw/config.json`

Key configuration sections:

- `llm`: API key, base URL, model name
- `gateway`: Routing rules, retry policy, health checks
- `agents`: Agent-specific configurations
- `tools`: Enable/disable tools (web_search, mcp, etc.)
- `log`: Log path, level, console output

## Development Notes

### Adding a New Tool

1. Create tool implementation in `tools/` directory
2. Implement `Tool` interface: `Name()`, `Description()`, `Execute()`
3. Register in `cli/cli.go` where other tools are registered

### Adding a New Channel

1. Create channel implementation in `channel/` directory
2. Embed `BaseChannel` and implement required interfaces
3. Register in Gateway initialization in `cli/cli.go`

### Session Management

Sessions are identified by `channel::chatID` format. The `SessionManager` handles persistence and history management. Session data is stored in the workspace directory.

### Gateway Routing

Routes are configured in `gateway.routes` with priority-based matching. The Router selects the highest priority matching route for each inbound message.

## Dependencies

Key external dependencies:

- `github.com/spf13/cobra`: CLI framework
- `github.com/rs/zerolog`: Structured logging
- `github.com/google/uuid`: UUID generation
- `github.com/gomarkdown/markdown`: Markdown processing
- `gopkg.in/natefinch/lumberjack.v2`: Log rotation


## Principles
- Please read my current project code and add or modify it based on mine. Utilize existing modules and components as much as possible; if necessary, you can make careful changes, but you must explain the reasons in detail. You can also add new components/modules/structures to assist in the implementation. Avoid over-engineering.

- The code style should be concise and clear, with good comments.

- For error handling, follow the single-wrapping principle, wrapping only at the source (i.e., the lowest level calling the standard library or third-party library). Low-level responsibility: Use `fmt.Errorf("Operation description (%s): %w", params, err)`. It must include key context (such as ID, path, etc.) and use `%w` to maintain the error chain.

Middle-level responsibility: Duplicate wrapping is strictly prohibited. Only use `if err != nil { return err }` to directly pass through error messages, ensuring that error messages are not stacked or redundant.

- Regarding logging, the logger module, building upon error handling, calls `logger.L().Error().Str().Msg()` at the top level of the error chain to print error logs. Generally, the top level doesn't return an error, because if it did, it means the error is still in an intermediate layer.

- `logger.L().Debug.Str().Msg()` should be added to every critical implementation to print debug logs, facilitating development and debugging without worrying about overhead.

- The architecture design needs to ensure decoupling, scalability, and ease of adding new features later. The architecture design should be reasonable, preventing circular imports of packages.

- Provide rich and complete unit tests and ensure they are passed.