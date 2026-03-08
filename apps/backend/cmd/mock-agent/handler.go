package main

import (
	"fmt"
	"math/rand"
	"os"
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

// stripKandevSystem removes <kandev-system>...</kandev-system> blocks injected by
// injectKandevContext so that /e2e: routing works regardless of system prompt wrapping.
func stripKandevSystem(prompt string) string {
	const endTag = "</kandev-system>"
	idx := strings.LastIndex(prompt, endTag)
	if idx < 0 {
		return prompt
	}
	return strings.TrimSpace(prompt[idx+len(endTag):])
}

// handlePrompt routes a user prompt to the appropriate sequence generator.
func handlePrompt(e *emitter, prompt, model string) {
	prompt = strings.TrimSpace(prompt)

	// Extract the user-facing content for command routing.
	cmd := stripKandevSystem(prompt)

	// Script mode: each line is a command (e2e:message, e2e:mcp:*, etc.)
	if isScriptMode(cmd) {
		executeScript(e, prompt, cmd)
		return
	}

	switch {
	case strings.EqualFold(cmd, "all") || strings.EqualFold(cmd, "/all"):
		emitAllTypes(e, model)
	case strings.EqualFold(cmd, "/error"):
		emitError(e, model)
	case strings.EqualFold(cmd, "/slow") || strings.HasPrefix(strings.ToLower(cmd), "/slow "):
		emitSlowResponse(e, cmd, model)
	case strings.EqualFold(cmd, "/thinking"):
		emitThinkingSequence(e, model)
	case strings.HasPrefix(cmd, "/tool:"):
		toolName := strings.TrimPrefix(cmd, "/tool:")
		emitSpecificTool(e, strings.TrimSpace(toolName), model)
	case strings.HasPrefix(cmd, "/subagent"):
		emitSubagentSequence(e, model)
	case strings.HasPrefix(cmd, "/e2e:"):
		rest := strings.TrimPrefix(cmd, "/e2e:")
		scenarioName, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
		emitPredefinedScenario(e, scenarioName)
	case strings.EqualFold(cmd, "/crash"):
		emitCrash(e, model)
	case strings.HasPrefix(cmd, "/todo"):
		emitTodoSequence(e, model)
	case strings.EqualFold(cmd, "/mermaid"):
		emitMermaidSequence(e, model)
	default:
		emitRandomResponse(e, cmd, model)
	}
}

// emitError emits an error message.
func emitError(e *emitter, model string) {
	randomDelay(model)
	e.text("Simulating an error condition...")
	randomDelay(model)
	e.text("Mock error: something went wrong during processing")
}

// emitCrash simulates an agent crash by exiting with code 1 after emitting
// some output. Useful for testing recovery flows.
func emitCrash(e *emitter, model string) {
	randomDelay(model)
	e.text("Processing your request...")
	randomDelay(model)
	fmt.Fprintln(os.Stderr, "mock-agent: simulating crash (exit 1)")
	os.Exit(1)
}

// emitSlowResponse generates a response with configurable total duration.
func emitSlowResponse(e *emitter, prompt, model string) {
	totalDuration := 5 * time.Second
	parts := strings.Fields(prompt)
	if len(parts) >= 2 {
		if d, err := time.ParseDuration(parts[1]); err == nil && d > 0 {
			totalDuration = d
		}
	}

	steps := 5
	stepDelay := totalDuration / time.Duration(steps)

	emitThinking(e, model)
	time.Sleep(stepDelay)

	e.text(fmt.Sprintf("Running slow response (%s total)...", totalDuration))
	time.Sleep(stepDelay)

	emitReadFile(e, model)
	time.Sleep(stepDelay)

	emitCodeSearch(e, model)
	time.Sleep(stepDelay)

	e.text(fmt.Sprintf("Slow response complete after %s.", totalDuration))
	time.Sleep(stepDelay)
}

// emitRandomResponse generates a random mix of 2-5 events.
func emitRandomResponse(e *emitter, prompt, model string) {
	generators := []func(){
		func() { emitThinking(e, model) },
		func() { e.text("I'll help you with that. Let me look into it.") },
		func() { emitReadFile(e, model) },
		func() { emitCodeSearch(e, model) },
		func() { emitWebFetch(e, model) },
	}

	// Always start with thinking
	emitThinking(e, model)
	randomDelay(model)

	// Pick 1-4 more random events
	count := 1 + rand.Intn(4)
	for i := 0; i < count; i++ {
		idx := rand.Intn(len(generators))
		generators[idx]()
		randomDelay(model)
	}

	// End with a text summary
	e.text("I've completed the analysis of your request: \"" + prompt + "\". Everything looks good!")
}

// emitAllTypes emits one of every message type.
func emitAllTypes(e *emitter, model string) {
	emitThinking(e, model)
	randomDelay(model)
	e.text("Starting comprehensive demonstration of all message types...")
	randomDelay(model)
	emitReadFile(e, model)
	randomDelay(model)
	emitEditFile(e, model)
	randomDelay(model)
	emitShellExec(e, model)
	randomDelay(model)
	emitCodeSearch(e, model)
	randomDelay(model)
	emitSubagent(e, model)
	randomDelay(model)
	emitTodo(e, model)
	randomDelay(model)
	emitWebFetch(e, model)
	randomDelay(model)
	e.text("All message types demonstrated successfully!")
}

// emitThinkingSequence emits extended thinking/reasoning blocks.
func emitThinkingSequence(e *emitter, model string) {
	thoughts := []string{
		"Let me analyze this problem step by step...",
		"First, I need to consider the architecture and how the components interact.",
		"The key insight is that we need to handle both synchronous and asynchronous flows.",
		"I should also consider edge cases: what happens when the input is empty? What about concurrent access?",
		"After careful analysis, I believe the best approach is to use a channel-based pattern with proper synchronization.",
	}

	for _, thought := range thoughts {
		randomDelay(model)
		e.thought(thought)
	}

	randomDelay(model)
	e.text("After careful reasoning, here is my analysis:\n\n1. The architecture is sound\n2. Error handling covers edge cases\n3. The implementation follows Go best practices")
}

// emitSpecificTool emits a single specific tool call.
func emitSpecificTool(e *emitter, toolName, model string) {
	switch strings.ToLower(toolName) {
	case "read":
		emitReadFile(e, model)
	case "edit":
		emitEditFile(e, model)
	case "exec", "bash":
		emitShellExec(e, model)
	case "search", "grep":
		emitCodeSearch(e, model)
	case "webfetch", "web":
		emitWebFetch(e, model)
	default:
		e.text("Unknown tool: " + toolName + ". Available: read, edit, exec, search, webfetch")
	}
}

// emitTodoSequence emits a todo management sequence.
func emitTodoSequence(e *emitter, model string) {
	emitThinking(e, model)
	randomDelay(model)
	e.text("I'll create a task list for this work.")
	randomDelay(model)
	emitTodo(e, model)
	randomDelay(model)
	e.text("Task list has been updated.")
}

// emitSubagentSequence emits a subagent Task sequence.
func emitSubagentSequence(e *emitter, model string) {
	emitThinking(e, model)
	randomDelay(model)
	e.text("I'll delegate this to a subagent for parallel processing.")
	randomDelay(model)
	emitSubagent(e, model)
	randomDelay(model)
	e.text("Subagent task completed successfully.")
}
