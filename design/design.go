package design

import (
	. "goa.design/goa-ai/dsl"
	. "goa.design/goa/v3/dsl"
)

// Weather specialist agent—has its own tools and planner
var _ = Service("weather", func() {
	Agent("forecaster", "Weather specialist", func() {
		// Internal tools only this agent can use
		Use("weather_tools", func() {
			Tool("get_forecast", "Get forecast", func() {
				Args(func() {
					Attribute("city", String, "City")
					Required("city")
				})
				Return(func() {
					Attribute("forecast", String, "Forecast")
					Required("forecast")
				})
			})
		})

		// Export makes this agent callable as a tool by other agents.
		// The exported toolset defines the interface other agents see.
		Export("ask_weather", func() {
			Tool("ask", "Ask weather specialist", func() {
				Args(func() {
					Attribute("question", String, "Question")
					Required("question")
				})
				Return(func() {
					Attribute("answer", String, "Answer")
					Required("answer")
				})
			})
		})
	})
})

// Flights specialist agent—searches for flights
var _ = Service("flights", func() {
	Agent("finder", "Flight search specialist", func() {
		Use("flight_tools", func() {
			Tool("search_flights", "Search for flights", func() {
				Args(func() {
					Attribute("origin", String, "Origin city")
					Attribute("destination", String, "Destination city")
					Attribute("date", String, "Travel date")
					Required("origin", "destination", "date")
				})
				Return(func() {
					Attribute("results", String, "Flight search results")
					Required("results")
				})
			})
		})

		Export("ask_flights", func() {
			Tool("ask", "Ask flight search specialist", func() {
				Args(func() {
					Attribute("question", String, "Question")
					Required("question")
				})
				Return(func() {
					Attribute("answer", String, "Answer")
					Required("answer")
				})
			})
		})
	})
})

// Hotels specialist agent—searches for hotels
var _ = Service("hotels", func() {
	Agent("advisor", "Hotel search specialist", func() {
		Use("hotel_tools", func() {
			Tool("search_hotels", "Search for hotels", func() {
				Args(func() {
					Attribute("city", String, "City")
					Attribute("checkin", String, "Check-in date")
					Attribute("checkout", String, "Check-out date")
					Required("city", "checkin", "checkout")
				})
				Return(func() {
					Attribute("results", String, "Hotel search results")
					Required("results")
				})
			})
		})

		Export("ask_hotels", func() {
			Tool("ask", "Ask hotel search specialist", func() {
				Args(func() {
					Attribute("question", String, "Question")
					Required("question")
				})
				Return(func() {
					Attribute("answer", String, "Answer")
					Required("answer")
				})
			})
		})
	})
})

// Travel planner agent—coordinates weather, flights, and hotels
var _ = Service("travel", func() {
	Agent("planner", "Travel planning specialist", func() {
		Use("budget_tools", func() {
			Tool("estimate_budget", "Estimate travel budget", func() {
				Args(func() {
					Attribute("destination", String, "Destination city")
					Attribute("days", Int, "Number of days")
					Required("destination", "days")
				})
				Return(func() {
					Attribute("estimate", String, "Budget estimate")
					Required("estimate")
				})
			})
		})

		// Import other agents' exported toolsets
		UseAgentToolset("weather", "forecaster", "ask_weather")
		UseAgentToolset("flights", "finder", "ask_flights")
		UseAgentToolset("hotels", "advisor", "ask_hotels")

		Export("ask_travel", func() {
			Tool("ask", "Ask travel planning specialist", func() {
				Args(func() {
					Attribute("question", String, "Question")
					Required("question")
				})
				Return(func() {
					Attribute("answer", String, "Answer")
					Required("answer")
				})
			})
		})
	})
})

// Main assistant uses weather and travel agents as tools
var _ = Service("demo", func() {
	Agent("assistant", "A helpful assistant", func() {
		UseAgentToolset("weather", "forecaster", "ask_weather")
		UseAgentToolset("travel", "planner", "ask_travel")
	})
})
