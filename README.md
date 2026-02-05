# Goa-AI Multi-Agent Quickstart with Revenium Metering

A demonstration project showing how to build a multi-agent AI system using the [Goa-AI](https://goa.design) framework with [Revenium](https://revenium.io) metering integration for usage tracking and monetization.

## Overview

This quickstart implements a travel planning assistant built from multiple specialized AI agents:

```
┌─────────────────────────────────────────────────────────────┐
│                      Assistant Agent                         │
│                    (demo.assistant)                          │
└─────────────────────┬───────────────────┬───────────────────┘
                      │                   │
                      ▼                   ▼
┌─────────────────────────────┐  ┌─────────────────────────────┐
│    Weather Forecaster       │  │     Travel Planner          │
│   (weather.forecaster)      │  │    (travel.planner)         │
└─────────────────────────────┘  └──────┬──────┬──────┬────────┘
                                        │      │      │
                      ┌─────────────────┘      │      └─────────────────┐
                      ▼                        ▼                        ▼
        ┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐
        │   Flight Finder     │  │   Hotel Advisor     │  │  Weather Forecaster │
        │  (flights.finder)   │  │  (hotels.advisor)   │  │ (weather.forecaster)│
        └─────────────────────┘  └─────────────────────┘  └─────────────────────┘
```

Each agent:
- Has its own internal toolset for domain-specific operations
- Exports a toolset interface for other agents to call it
- Is wrapped with Revenium metering for usage tracking

## Prerequisites

- Go 1.21+
- OpenAI API key
- Revenium API key (optional, for metering)

## Setup

1. Clone and navigate to the project:
   ```bash
   cd quickstart
   ```

2. Set environment variables:
   ```bash
   export OPENAI_API_KEY="your-openai-api-key"
   export REVENIUM_API_KEY="your-revenium-api-key" 
   ```

3. Install dependencies:
   ```bash
   go mod download
   ```

## Running

```bash
go run ./main.go
```

The demo runs several queries demonstrating agent collaboration:
- Weather queries (routed to the forecaster agent)
- Trip planning requests (coordinated through multiple agents)

## Project Structure

```
quickstart/
├── design/
│   └── design.go          # Goa DSL definitions for services and agents
├── gen/                   # Auto-generated code (do not edit)
│   ├── demo/              # Demo service with assistant agent
│   ├── weather/           # Weather service with forecaster agent
│   ├── flights/           # Flights service with finder agent
│   ├── hotels/            # Hotels service with advisor agent
│   └── travel/            # Travel service with planner agent
├── main.go                # Application entry point and runtime setup
├── go.mod
└── CLAUDE.md              # Development instructions
```

## Architecture

### Design-First Approach

The project uses Goa's design-first pattern. All agents, toolsets, and services are defined in `design/design.go`:

```go
var _ = Service("weather", func() {
    Agent("forecaster", "Weather specialist", func() {
        // Internal tools only this agent can use
        Use("weather_tools", func() {
            Tool("get_forecast", "Get forecast", func() { /* ... */ })
        })

        // Export makes this agent callable by other agents
        Export("ask_weather", func() {
            Tool("ask", "Ask weather specialist", func() { /* ... */ })
        })
    })
})
```

Run `goa gen quickstart/design` after modifying the design to regenerate code.

### Agent Composition

Agents can use other agents as tools via `UseAgentToolset`:

```go
Agent("planner", "Travel planning specialist", func() {
    UseAgentToolset("weather", "forecaster", "ask_weather")
    UseAgentToolset("flights", "finder", "ask_flights")
    UseAgentToolset("hotels", "advisor", "ask_hotels")
})
```

### Runtime Model

The Goa-AI runtime manages agent lifecycle through two interfaces:

- **Planner**: The agent's "brain" connecting to an LLM
  - `PlanStart()`: Initial decision based on user message
  - `PlanResume()`: Decision after tool execution results

- **Executor**: Implements tool execution with typed payloads

**Execution flow:**
```
User Message → PlanStart() → [Tool calls → PlanResume()]* → Final Response
```

## Revenium Integration

The project demonstrates Revenium metering for AI agent monetization:

### MeteringPlanner

Wraps planners to capture LLM usage:

```go
Planner: &revenium.MeteringPlanner{
    Inner:          yourPlanner,
    Meter:          meter,
    AgentID:        "weather.forecaster",
    Provider:       "OpenAI",
    ModelName:      "gpt-4o",
    CapturePrompts: true,  // Optional: capture prompts for debugging
}
```

### MeteringSink

Wraps stream sinks to capture tool execution events:

```go
rt := runtime.New(
    runtime.WithStream(&revenium.MeteringSink{
        Inner: &ConsoleSink{},
        Meter: meter,
    }),
)
```

### Meter Configuration

```go
meter, err := revenium.NewMeter(
    revenium.WithAPIKey(os.Getenv("REVENIUM_API_KEY")),
    revenium.WithEnvironment("development"),
    revenium.WithProductName("Free Trial"),
    revenium.WithSubscriber("user-123", "user@example.com"),
)
```

## Development

### Regenerate Code

After editing `design/design.go`:

```bash
goa gen quickstart/design
```

### Build

```bash
go build ./...
```

### Test

```bash
go test ./...
```

## Services and Agents

| Service | Agent | Description | Internal Tools | Exported Toolset |
|---------|-------|-------------|----------------|------------------|
| weather | forecaster | Weather specialist | `get_forecast` | `ask_weather` |
| flights | finder | Flight search | `search_flights` | `ask_flights` |
| hotels | advisor | Hotel search | `search_hotels` | `ask_hotels` |
| travel | planner | Trip coordinator | `estimate_budget` | `ask_travel` |
| demo | assistant | Main assistant | — | — |

## License

See LICENSE file for details.
