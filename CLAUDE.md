# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **Goa-AI agent framework quickstart** — a Go project demonstrating AI agent implementation using the Goa design-first code generation approach. It defines a `demo` service with an `assistant` agent that uses a `weather` toolset.

## Common Commands

```bash
# Build
go build ./...

# Run
go run ./main.go

# Generate code from design (after editing design/*.go)
goa gen quickstart/design

# Generate example scaffolding
goa example quickstart/design

# Test
go test ./...
```

## Architecture

### Design-First Code Generation

The project follows Goa's design-first pattern: **design files are the source of truth**, and the `gen/` directory is entirely auto-generated. Never edit files in `gen/` — they are overwritten on each `goa gen` run.

### Key Directories

- **`design/`** — DSL declarations defining services, agents, and toolsets. All structural changes start here.
- **`gen/`** — Auto-generated code (agents, toolsets, types, codecs, endpoints). Do not edit.
- **`main.go`** — Application entry point with runtime setup, planner/executor wiring.

### Runtime Model

The Goa-AI runtime manages the agent lifecycle through two core interfaces:

- **Planner** — The agent's "brain" connecting to an LLM. Implements `PlanStart()` (initial decision) and `PlanResume()` (decision after tool execution). Returns either tool call requests or a final response.
- **Executor** — Implements tool execution. Receives typed tool call requests and returns results.

**Execution flow:** User Message → `PlanStart()` → [Tool calls via Executor → `PlanResume()`]* → Final Response

### Agent Registration Pattern

```go
rt := runtime.New()
cfg := assistant.AssistantAgentConfig{Planner: myPlanner}
assistant.RegisterAssistantAgent(ctx, rt, cfg)
client := assistant.NewClient(rt)
```

Sessions must be explicitly created before runs: `rt.CreateSession(ctx, sessionID)`.

### Generated Code Structure (under `gen/demo/`)

- `agents/assistant/` — Agent registration (`registry.go`), config validation (`config.go`), workflow IDs (`agent.go`)
- `toolsets/weather/` — Tool specs with JSON schemas (`specs.go`), typed payload/result structs (`types.go`), codecs (`codecs.go`)

## Development Workflow

1. Edit `design/*.go` with DSL changes
2. Run `goa gen quickstart/design` to regenerate `gen/`
3. Implement or update Planner and Executor in `main.go` (or `internal/`)
4. Wire agents with the runtime and run
