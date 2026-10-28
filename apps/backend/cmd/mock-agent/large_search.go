package main

import (
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// largeSearchTextBytes is the observed incident result size. The ACP adapter
// normalizes this completion to a typed code_search payload with the rich text
// in ToolCallContents, so the fixture crosses the real preparation boundary.
const largeSearchTextBytes = 1_048_607

func scenarioLargeSearch(e *emitter) {
	e.startTool("large-search", "Search files", acp.ToolKindSearch, map[string]any{"query": "match line", "path": "."})
	fixedDelay(50)
	text := strings.Repeat("match line\n", (largeSearchTextBytes/11)+1)[:largeSearchTextBytes]
	e.completeToolWithContent("large-search", map[string]any{"output": "a.go\nb.go\nc.go"}, text)
	fixedDelay(100)
	e.text("SEARCH_DELIVERY_COMPLETE")
}

func scenarioLargeSearchOverflow(e *emitter) {
	e.startTool("large-search-overflow", "Search files", acp.ToolKindSearch, map[string]any{"query": "match line", "path": "."})
	fixedDelay(50)
	line := "internal/agent/runtime/lifecycle/delivery_health_monitor.go\n"
	output := strings.Repeat(line, (2<<20)/len(line)+1)
	e.completeTool("large-search-overflow", map[string]any{"output": output})
	fixedDelay(100)
	e.text("SEARCH_OVERFLOW_COMPLETE")
}

// oversizedOutputBytes exceeds the agentctl encoded event limit while staying
// below the producer queue bound, so the failure is a real permanent-size
// refusal rather than a dropped or blocked process write.
const oversizedOutputBytes = 2_200_000

func scenarioOversizedOutput(e *emitter) {
	e.text(strings.Repeat("x", oversizedOutputBytes))
	fixedDelay(100)
	e.text("OVERSIZED_DELIVERY_COMPLETE")
}

func scenarioOversizedOutputHold(e *emitter) {
	e.text(strings.Repeat("x", oversizedOutputBytes))
	// Ignore cancellation so the native prompt stays open without a terminal.
	// The backend must surface the retained failure through health observation.
	time.Sleep(60 * time.Second)
}
