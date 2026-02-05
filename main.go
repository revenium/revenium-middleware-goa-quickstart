package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	revenium "github.com/revenium/revenium-middleware-goa"

	assistant "quickstart/gen/demo/agents/assistant"
	assistantspecs "quickstart/gen/demo/agents/assistant/specs"
	finder "quickstart/gen/flights/agents/finder"
	finderagenttools "quickstart/gen/flights/agents/finder/agenttools/ask_flights"
	finderspecs "quickstart/gen/flights/agents/finder/specs"
	advisor "quickstart/gen/hotels/agents/advisor"
	advisoragenttools "quickstart/gen/hotels/agents/advisor/agenttools/ask_hotels"
	advisorspecs "quickstart/gen/hotels/agents/advisor/specs"
	travelplanner "quickstart/gen/travel/agents/planner"
	travelplanneragenttools "quickstart/gen/travel/agents/planner/agenttools/ask_travel"
	travelplannerspecs "quickstart/gen/travel/agents/planner/specs"
	forecaster "quickstart/gen/weather/agents/forecaster"
	forecasteragenttools "quickstart/gen/weather/agents/forecaster/agenttools/ask_weather"
	forecasterspecs "quickstart/gen/weather/agents/forecaster/specs"

	"goa.design/goa-ai/features/model/openai"
	"goa.design/goa-ai/runtime/agent/model"
	"goa.design/goa-ai/runtime/agent/planner"
	"goa.design/goa-ai/runtime/agent/runtime"
	"goa.design/goa-ai/runtime/agent/stream"
	"goa.design/goa-ai/runtime/agent/tools"
)

// openAIToolName converts a dotted goa-ai tool name (e.g. "ask_weather.ask")
// to an OpenAI-compatible name (e.g. "ask_weather-ask") since OpenAI requires
// names matching ^[a-zA-Z0-9_-]+$.
func openAIToolName(name string) string { return strings.ReplaceAll(name, ".", "-") }

// goaToolName converts an OpenAI tool name back to the dotted goa-ai form.
func goaToolName(name string) tools.Ident { return tools.Ident(strings.Replace(name, "-", ".", 1)) }

// buildToolDefs converts tool specs to OpenAI-compatible model.ToolDefinition.
func buildToolDefs(specs []tools.ToolSpec) []*model.ToolDefinition {
	var defs []*model.ToolDefinition
	for _, spec := range specs {
		var schema any
		if spec.Payload.Schema != nil {
			_ = json.Unmarshal(spec.Payload.Schema, &schema)
		}
		defs = append(defs, &model.ToolDefinition{
			Name:        openAIToolName(string(spec.Name)),
			Description: spec.Description,
			InputSchema: schema,
		})
	}
	return defs
}

// flattenMessages converts goa-ai messages to text-only messages compatible with
// the OpenAI adapter, which only handles TextPart.
func flattenMessages(msgs []*model.Message) []*model.Message {
	var out []*model.Message
	for _, m := range msgs {
		var parts []model.Part
		for _, p := range m.Parts {
			switch v := p.(type) {
			case model.TextPart:
				parts = append(parts, v)
			case model.ToolUsePart:
				payload, _ := json.Marshal(v.Input)
				parts = append(parts, model.TextPart{
					Text: fmt.Sprintf("[Called tool %s with: %s]", v.Name, payload),
				})
			case model.ToolResultPart:
				content, _ := json.Marshal(v.Content)
				parts = append(parts, model.TextPart{
					Text: fmt.Sprintf("[Tool result: %s]", content),
				})
			default:
				parts = append(parts, p)
			}
		}
		if len(parts) == 0 {
			parts = []model.Part{model.TextPart{Text: " "}}
		}
		out = append(out, &model.Message{Role: m.Role, Parts: parts})
	}
	return out
}

func interpretResponse(resp *model.Response) (*planner.PlanResult, error) {
	if len(resp.ToolCalls) > 0 {
		var reqs []planner.ToolRequest
		for _, tc := range resp.ToolCalls {
			reqs = append(reqs, planner.ToolRequest{
				Name:    goaToolName(string(tc.Name)),
				Payload: tc.Payload,
			})
		}
		return &planner.PlanResult{ToolCalls: reqs}, nil
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	msg := resp.Content[len(resp.Content)-1]

	var reqs []planner.ToolRequest
	for _, part := range msg.Parts {
		if p, ok := part.(model.ToolUsePart); ok {
			payload, _ := json.Marshal(p.Input)
			reqs = append(reqs, planner.ToolRequest{
				Name:    goaToolName(p.Name),
				Payload: payload,
			})
		}
	}
	if len(reqs) > 0 {
		return &planner.PlanResult{ToolCalls: reqs}, nil
	}

	return &planner.PlanResult{
		FinalResponse: &planner.FinalResponse{Message: &msg},
	}, nil
}

// LLMPlanner is a reusable planner that calls an LLM with the given tool specs.
type LLMPlanner struct {
	systemPrompt string
	modelID      string
	toolSpecs    []tools.ToolSpec
}

func (p *LLMPlanner) PlanStart(ctx context.Context, in *planner.PlanInput) (*planner.PlanResult, error) {
	client, ok := in.Agent.ModelClient(p.modelID)
	if !ok {
		return nil, fmt.Errorf("no model client %q", p.modelID)
	}

	msgs := append([]*model.Message{{
		Role:  model.ConversationRoleSystem,
		Parts: []model.Part{model.TextPart{Text: p.systemPrompt}},
	}}, in.Messages...)

	resp, err := client.Complete(ctx, &model.Request{
		Messages: flattenMessages(msgs),
		Tools:    buildToolDefs(p.toolSpecs),
	})
	if err != nil {
		return nil, err
	}
	return interpretResponse(resp)
}

func (p *LLMPlanner) PlanResume(ctx context.Context, in *planner.PlanResumeInput) (*planner.PlanResult, error) {
	client, ok := in.Agent.ModelClient(p.modelID)
	if !ok {
		return nil, fmt.Errorf("no model client %q", p.modelID)
	}

	msgs := append([]*model.Message{{
		Role:  model.ConversationRoleSystem,
		Parts: []model.Part{model.TextPart{Text: p.systemPrompt}},
	}}, in.Messages...)

	resp, err := client.Complete(ctx, &model.Request{
		Messages: flattenMessages(msgs),
		Tools:    buildToolDefs(p.toolSpecs),
	})
	if err != nil {
		return nil, err
	}
	return interpretResponse(resp)
}

// WeatherToolsExecutor handles the forecaster's internal weather_tools toolset.
type WeatherToolsExecutor struct{}

func (e *WeatherToolsExecutor) Execute(ctx context.Context, meta *runtime.ToolCallMeta, req *planner.ToolRequest) (*planner.ToolResult, error) {
	return &planner.ToolResult{
		Name:   req.Name,
		Result: map[string]any{"forecast": "22°C and sunny in the requested city"},
	}, nil
}

// FlightToolsExecutor handles the finder's internal flight_tools toolset.
type FlightToolsExecutor struct{}

func (e *FlightToolsExecutor) Execute(ctx context.Context, meta *runtime.ToolCallMeta, req *planner.ToolRequest) (*planner.ToolResult, error) {
	return &planner.ToolResult{
		Name:   req.Name,
		Result: map[string]any{"results": "Found 3 flights: AA101 $450, UA202 $380, DL303 $520"},
	}, nil
}

// HotelToolsExecutor handles the advisor's internal hotel_tools toolset.
type HotelToolsExecutor struct{}

func (e *HotelToolsExecutor) Execute(ctx context.Context, meta *runtime.ToolCallMeta, req *planner.ToolRequest) (*planner.ToolResult, error) {
	return &planner.ToolResult{
		Name:   req.Name,
		Result: map[string]any{"results": "Found 3 hotels: Grand Plaza $180/night, City Inn $95/night, Luxury Suites $320/night"},
	}, nil
}

// BudgetToolsExecutor handles the planner's internal budget_tools toolset.
type BudgetToolsExecutor struct{}

func (e *BudgetToolsExecutor) Execute(ctx context.Context, meta *runtime.ToolCallMeta, req *planner.ToolRequest) (*planner.ToolResult, error) {
	return &planner.ToolResult{
		Name:   req.Name,
		Result: map[string]any{"estimate": "Estimated total budget: $2,450"},
	}, nil
}

// ConsoleSink streams events to the console in real-time.
type ConsoleSink struct{}

func (s *ConsoleSink) Send(ctx context.Context, event stream.Event) error {
	switch e := event.(type) {
	case stream.ToolStart:
		fmt.Printf("Tool: %s\n", e.Data.ToolName)
	case stream.ToolEnd:
		fmt.Printf("Done: %s\n", e.Data.ToolName)
	case stream.AssistantReply:
		fmt.Print(e.Data.Text)
	case stream.Workflow:
		fmt.Printf("[%s]\n", e.Data.Phase)
	}
	return nil
}

func (s *ConsoleSink) Close(ctx context.Context) error { return nil }

func main() {
	ctx := context.Background()

	// Create Revenium meter
	meter, err := revenium.NewMeter(
		revenium.WithAPIKey(os.Getenv("REVENIUM_API_KEY")),
		revenium.WithEnvironment("development"),
		revenium.WithOrganizationName("goa"),
		revenium.WithDebug(true),
		revenium.WithSubscriptionID("sub-456"),
		revenium.WithProductName("Free Trial"),
		revenium.WithSubscriber("user-123", "user@example.com"),
		revenium.WithSubscriberCredential("Production Key", "pk-abc123"),
	)
	if err != nil {
		panic(err)
	}
	defer meter.Flush()

	modelClient, err := openai.NewFromAPIKey(os.Getenv("OPENAI_API_KEY"), "gpt-4o")
	if err != nil {
		panic(err)
	}

	// Wrap sink with MeteringSink
	rt := runtime.New(
		runtime.WithStream(&revenium.MeteringSink{
			Inner: &ConsoleSink{},
			Meter: meter,
		}),
	)

	if err := rt.RegisterModel("openai", modelClient); err != nil {
		panic(err)
	}

	// 1. Register the forecaster agent (weather service) with metering planner.
	err = forecaster.RegisterForecasterAgent(ctx, rt, forecaster.ForecasterAgentConfig{
		Planner: &revenium.MeteringPlanner{
			Inner: &LLMPlanner{
				systemPrompt: "You are a weather specialist. Use your weather tools to answer questions about the weather.",
				modelID:      "openai",
				toolSpecs:    forecasterspecs.Specs,
			},
			Meter:          meter,
			AgentID:        "weather.forecaster",
			Provider:       "OpenAI",
			ModelName:      "gpt-4o",
			CapturePrompts: true,
		},
	})
	if err != nil {
		panic(err)
	}

	// Register the forecaster's internal weather_tools executor
	if err := forecaster.RegisterUsedToolsets(ctx, rt,
		forecaster.WithWeatherToolsExecutor(&WeatherToolsExecutor{}),
	); err != nil {
		panic(err)
	}

	// Register the forecaster's exported ask_weather toolset so other agents can call it
	askWeatherReg := forecasteragenttools.NewForecasterToolsetRegistration(rt)
	if err := rt.RegisterToolset(askWeatherReg); err != nil {
		panic(err)
	}

	// 2. Register the finder agent (flights service) with metering planner.
	err = finder.RegisterFinderAgent(ctx, rt, finder.FinderAgentConfig{
		Planner: &revenium.MeteringPlanner{
			Inner: &LLMPlanner{
				systemPrompt: "You are a flight search specialist. Use your flight tools to search for flights when asked.",
				modelID:      "openai",
				toolSpecs:    finderspecs.Specs,
			},
			Meter:          meter,
			AgentID:        "flights.finder",
			Provider:       "OpenAI",
			ModelName:      "gpt-4o",
			CapturePrompts: true,
		},
	})
	if err != nil {
		panic(err)
	}

	// Register the finder's internal flight_tools executor
	if err := finder.RegisterUsedToolsets(ctx, rt,
		finder.WithFlightToolsExecutor(&FlightToolsExecutor{}),
	); err != nil {
		panic(err)
	}

	// Register the finder's exported ask_flights toolset
	askFlightsReg := finderagenttools.NewFinderToolsetRegistration(rt)
	if err := rt.RegisterToolset(askFlightsReg); err != nil {
		panic(err)
	}

	// 3. Register the advisor agent (hotels service) with metering planner.
	err = advisor.RegisterAdvisorAgent(ctx, rt, advisor.AdvisorAgentConfig{
		Planner: &revenium.MeteringPlanner{
			Inner: &LLMPlanner{
				systemPrompt: "You are a hotel search specialist. Use your hotel tools to search for hotels when asked.",
				modelID:      "openai",
				toolSpecs:    advisorspecs.Specs,
			},
			Meter:          meter,
			AgentID:        "hotels.advisor",
			Provider:       "OpenAI",
			ModelName:      "gpt-4o",
			CapturePrompts: true,
		},
	})
	if err != nil {
		panic(err)
	}

	// Register the advisor's internal hotel_tools executor
	if err := advisor.RegisterUsedToolsets(ctx, rt,
		advisor.WithHotelToolsExecutor(&HotelToolsExecutor{}),
	); err != nil {
		panic(err)
	}

	// Register the advisor's exported ask_hotels toolset
	askHotelsReg := advisoragenttools.NewAdvisorToolsetRegistration(rt)
	if err := rt.RegisterToolset(askHotelsReg); err != nil {
		panic(err)
	}

	// 4. Register the travel planner agent (travel service) with metering planner.
	err = travelplanner.RegisterPlannerAgent(ctx, rt, travelplanner.PlannerAgentConfig{
		Planner: &revenium.MeteringPlanner{
			Inner: &LLMPlanner{
				systemPrompt: "You are a travel planning specialist. Use your tools to plan trips: ask_weather for destination weather, ask_flights for flight options, ask_hotels for hotel options, and estimate_budget for budget estimates. Always gather weather, flights, and hotels information when planning a trip.",
				modelID:      "openai",
				toolSpecs:    travelplannerspecs.Specs,
			},
			Meter:          meter,
			AgentID:        "travel.planner",
			Provider:       "OpenAI",
			ModelName:      "gpt-4o",
			CapturePrompts: true,
		},
	})
	if err != nil {
		panic(err)
	}

	// Register the planner's internal budget_tools executor
	if err := travelplanner.RegisterUsedToolsets(ctx, rt,
		travelplanner.WithBudgetToolsExecutor(&BudgetToolsExecutor{}),
	); err != nil {
		panic(err)
	}

	// Register the planner's exported ask_travel toolset
	askTravelReg := travelplanneragenttools.NewPlannerToolsetRegistration(rt)
	if err := rt.RegisterToolset(askTravelReg); err != nil {
		panic(err)
	}

	// 5. Register the assistant agent (demo service) with metering planner.
	err = assistant.RegisterAssistantAgent(ctx, rt, assistant.AssistantAgentConfig{
		Planner: &revenium.MeteringPlanner{
			Inner: &LLMPlanner{
				systemPrompt: "You are a helpful assistant. Use the ask_weather tool for weather questions and the ask_travel tool for trip planning requests.",
				modelID:      "openai",
				toolSpecs:    assistantspecs.Specs,
			},
			Meter:     meter,
			AgentID:   "demo.assistant",
			Provider:  "OpenAI",
			ModelName: "gpt-4o",
		},
	})
	if err != nil {
		panic(err)
	}

	// Run demo queries
	client := assistant.NewClient(rt)

	queries := []string{
		"What's the weather like in Tokyo right now?",
		"Plan a 5-day trip to Paris from New York next month",
		"Will it rain in London this weekend?",
		"Plan a 3-day trip to Barcelona from San Francisco",
		"What's the forecast for Rome?",
	}

	for i, query := range queries {
		sessionID := fmt.Sprintf("session-%d", i+1)
		if _, err := rt.CreateSession(ctx, sessionID); err != nil {
			panic(err)
		}

		fmt.Printf("\n\n=== Query %d: %s ===\n", i+1, query)
		out, err := client.Run(ctx, sessionID, []*model.Message{{
			Role:  model.ConversationRoleUser,
			Parts: []model.Part{model.TextPart{Text: query}},
		}})
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		fmt.Printf("\nRunID: %s\n", out.RunID)
	}
}
