package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// delayRange returns min/max delay in milliseconds based on model name.
func delayRange(model string) (int, int) {
	switch model {
	case "mock-fast":
		return 10, 50
	case "mock-slow":
		return 500, 3000
	default:
		return 100, 500
	}
}

// randomDelay sleeps for a random duration within the model's delay range.
func randomDelay(model string) {
	lo, hi := delayRange(model)
	ms := lo + rand.Intn(hi-lo+1)
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// fixedDelay sleeps for a fixed duration (for e2e scenarios).
func fixedDelay(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// handleUserPrompt routes a user prompt to the appropriate sequence generator.
func handleUserPrompt(enc *json.Encoder, scanner *bufio.Scanner, prompt, model string) {
	prompt = strings.TrimSpace(prompt)

	// Emit system message at the start of every turn
	emitSystemMessage(enc)

	// Some commands emit their own result; track whether we need the default one.
	customResult := false

	switch {
	case strings.EqualFold(prompt, "all"):
		emitAllTypes(enc, scanner, model)
	case strings.EqualFold(prompt, "/error"):
		emitError(enc, model)
		customResult = true
	case strings.EqualFold(prompt, "/slow") || strings.HasPrefix(strings.ToLower(prompt), "/slow "):
		emitSlowResponse(enc, scanner, prompt, model)
	case strings.EqualFold(prompt, "/thinking"):
		emitThinkingSequence(enc, model)
	case strings.HasPrefix(prompt, "/tool:"):
		toolName := strings.TrimPrefix(prompt, "/tool:")
		emitSpecificTool(enc, scanner, strings.TrimSpace(toolName), model)
	case strings.HasPrefix(prompt, "/subagent"):
		emitSubagentSequence(enc, scanner, model)
	case strings.HasPrefix(prompt, "/e2e:"):
		scenarioName := strings.TrimPrefix(prompt, "/e2e:")
		emitPredefinedScenario(enc, scanner, strings.TrimSpace(scenarioName))
		// e2e:error emits its own result
		if strings.TrimSpace(scenarioName) == "error" {
			customResult = true
		}
	case strings.HasPrefix(prompt, "/todo"):
		emitTodoSequence(enc, model)
	default:
		emitRandomResponse(enc, scanner, prompt, model)
	}

	if !customResult {
		emitResult(enc, false, "")
	}
}

// emitSystemMessage writes the system message to stdout.
func emitSystemMessage(enc *json.Encoder) {
	_ = enc.Encode(SystemMsg{
		Type:          TypeSystem,
		SessionID:     sessionID,
		SessionStatus: "active",
	})
}

// emitResult writes the final result message to stdout.
func emitResult(enc *json.Encoder, isError bool, errText string) {
	var resultJSON json.RawMessage
	if isError {
		resultJSON, _ = json.Marshal(errText)
	} else {
		resultJSON, _ = json.Marshal(ResultData{
			Text:      "Mock agent completed successfully.",
			SessionID: sessionID,
		})
	}

	_ = enc.Encode(ResultMsg{
		Type:              TypeResult,
		Result:            resultJSON,
		CostUSD:           0.0042,
		DurationMS:        1500,
		DurationAPIMS:     1200,
		IsError:           isError,
		NumTurns:          1,
		TotalInputTokens:  1500,
		TotalOutputTokens: 500,
		ModelUsage: map[string]ModelUsageStats{
			"mock-default": {ContextWindow: 200000},
		},
	})
}

// emitError emits an error result.
func emitError(enc *json.Encoder, model string) {
	randomDelay(model)
	emitTextBlock(enc, "Simulating an error condition...", "")
	randomDelay(model)
	emitResult(enc, true, "Mock error: something went wrong during processing")
	// Return without emitting the normal result (handler skips the final emitResult
	// because we already emitted one). We handle this by making the caller check.
}

// emitSlowResponse generates a response with configurable total duration.
// Accepts "/slow" (defaults to 5s) or "/slow <duration>" (e.g. "/slow 60s", "/slow 2m").
func emitSlowResponse(enc *json.Encoder, _ *bufio.Scanner, prompt, model string) {
	totalDuration := 5 * time.Second
	parts := strings.Fields(prompt)
	if len(parts) >= 2 {
		if d, err := time.ParseDuration(parts[1]); err == nil && d > 0 {
			totalDuration = d
		}
	}

	// Divide total duration into steps
	steps := 5
	stepDelay := totalDuration / time.Duration(steps)

	emitThinking(enc, model)
	time.Sleep(stepDelay)

	emitTextBlock(enc, fmt.Sprintf("Running slow response (%s total)...", totalDuration), "")
	time.Sleep(stepDelay)

	emitReadFile(enc, model)
	time.Sleep(stepDelay)

	emitCodeSearch(enc, model)
	time.Sleep(stepDelay)

	emitTextBlock(enc, fmt.Sprintf("Slow response complete after %s.", totalDuration), "")
	time.Sleep(stepDelay)
}

// emitRandomResponse generates a random mix of 2-5 events.
func emitRandomResponse(enc *json.Encoder, _ *bufio.Scanner, prompt, model string) {
	generators := []func(){
		func() { emitThinking(enc, model) },
		func() { emitTextBlock(enc, "I'll help you with that. Let me look into it.", "") },
		func() { emitReadFile(enc, model) },
		func() { emitCodeSearch(enc, model) },
		func() { emitWebFetch(enc, model) },
	}

	// Always start with thinking
	emitThinking(enc, model)
	randomDelay(model)

	// Pick 1-4 more random events
	count := 1 + rand.Intn(4)
	for i := 0; i < count; i++ {
		idx := rand.Intn(len(generators))
		generators[idx]()
		randomDelay(model)
	}

	// End with a text summary
	emitTextBlock(enc, "I've completed the analysis of your request: \""+prompt+"\". Everything looks good!", "")
}

// emitAllTypes emits one of every message type.
func emitAllTypes(enc *json.Encoder, scanner *bufio.Scanner, model string) {
	emitThinking(enc, model)
	randomDelay(model)
	emitTextBlock(enc, "Starting comprehensive demonstration of all message types...", "")
	randomDelay(model)
	emitReadFile(enc, model)
	randomDelay(model)
	emitEditFile(enc, scanner, model)
	randomDelay(model)
	emitShellExec(enc, scanner, model)
	randomDelay(model)
	emitCodeSearch(enc, model)
	randomDelay(model)
	emitSubagent(enc, scanner, model)
	randomDelay(model)
	emitTodo(enc, model)
	randomDelay(model)
	emitWebFetch(enc, model)
	randomDelay(model)
	emitTextBlock(enc, "All message types demonstrated successfully!", "")
}

// emitThinkingSequence emits extended thinking/reasoning blocks.
func emitThinkingSequence(enc *json.Encoder, model string) {
	thoughts := []string{
		"Let me analyze this problem step by step...",
		"First, I need to consider the architecture and how the components interact.",
		"The key insight is that we need to handle both synchronous and asynchronous flows.",
		"I should also consider edge cases: what happens when the input is empty? What about concurrent access?",
		"After careful analysis, I believe the best approach is to use a channel-based pattern with proper synchronization.",
	}

	for _, thought := range thoughts {
		randomDelay(model)
		emitThinkingBlock(enc, thought, "")
	}

	randomDelay(model)
	emitTextBlock(enc, "After careful reasoning, here is my analysis:\n\n1. The architecture is sound\n2. Error handling covers edge cases\n3. The implementation follows Go best practices", "")
}

// emitSpecificTool emits a single specific tool call.
func emitSpecificTool(enc *json.Encoder, scanner *bufio.Scanner, toolName, model string) {
	switch strings.ToLower(toolName) {
	case "read":
		emitReadFile(enc, model)
	case "edit":
		emitEditFile(enc, scanner, model)
	case "exec", "bash":
		emitShellExec(enc, scanner, model)
	case "search", "grep":
		emitCodeSearch(enc, model)
	case "webfetch", "web":
		emitWebFetch(enc, model)
	default:
		emitTextBlock(enc, "Unknown tool: "+toolName+". Available: read, edit, exec, search, webfetch", "")
	}
}

// emitTodoSequence emits a todo management sequence.
func emitTodoSequence(enc *json.Encoder, model string) {
	emitThinking(enc, model)
	randomDelay(model)
	emitTextBlock(enc, "I'll create a task list for this work.", "")
	randomDelay(model)
	emitTodo(enc, model)
	randomDelay(model)
	emitTextBlock(enc, "Task list has been updated.", "")
}

// emitSubagentSequence emits a subagent Task sequence.
func emitSubagentSequence(enc *json.Encoder, scanner *bufio.Scanner, model string) {
	emitThinking(enc, model)
	randomDelay(model)
	emitTextBlock(enc, "I'll delegate this to a subagent for parallel processing.", "")
	randomDelay(model)
	emitSubagent(enc, scanner, model)
	randomDelay(model)
	emitTextBlock(enc, "Subagent task completed successfully.", "")
}
