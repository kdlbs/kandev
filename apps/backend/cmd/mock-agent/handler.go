package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// overloaded529Message is the exact transient 529 message the real Anthropic
// API returns and the claude-agent-acp adapter surfaces over ACP. The
// orchestrator's routingerr classifier matches the "529 ... Overloaded"
// signature in this string and routes it to the retry-with-backoff path.
const overloaded529Message = "Internal error: API Error: 529 Overloaded. This is a server-side issue, " +
	"usually temporary — try again in a moment. If it persists, check https://status.claude.com."

// overloadedCmdRe matches `/overloaded` or `/e2e:overloaded`, optionally
// followed by `:N` — the number of consecutive prompts to fail with a 529
// before recovering (default 1, so the turn self-heals on the first retry).
// Use a large N (e.g. `/overloaded:9`) to exhaust the retry budget and fall
// through to the red recovery banner.
var overloadedCmdRe = regexp.MustCompile(`(?i)^/(?:e2e:)?overloaded(?::(\d+))?$`)

const changesWalkthroughPromptMarker = "Please create an agent-authored walkthrough of the current changes"

func isChangesWalkthroughRequest(prompt string) bool {
	cmd := stripKandevSystem(strings.TrimSpace(prompt))
	legacyPrompt := strings.Contains(cmd, changesWalkthroughPromptMarker) &&
		strings.Contains(cmd, "show_walkthrough_kandev") &&
		strings.Contains(cmd, "Available changed files:")
	promptReference := strings.Contains(cmd, "@changes-walkthrough") &&
		strings.Contains(cmd, "Changed files:")
	return legacyPrompt || promptReference
}

// parseOverloadedCmd reports whether the prompt is the /overloaded command and,
// if so, how many consecutive prompts it should fail before recovering
// (default 1). Returns ok=false for any other prompt.
func parseOverloadedCmd(prompt string) (failTimes int, ok bool) {
	cmd := stripKandevSystem(strings.TrimSpace(prompt))
	m := overloadedCmdRe.FindStringSubmatch(cmd)
	if m == nil {
		return 0, false
	}
	failTimes = 1
	if m[1] != "" {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 0 {
			failTimes = n
		}
	}
	return failTimes, true
}

// handleOverloaded implements the /overloaded scenario: it returns a real
// prompt-time ACP error carrying the production 529 string for the first N
// prompts of a session, then recovers with a normal text response. Returns
// handled=false when the prompt is not the overloaded command.
func (a *mockAgent) handleOverloaded(ctx context.Context, sid acp.SessionId, prompt string) (acp.PromptResponse, error, bool) {
	failTimes, ok := parseOverloadedCmd(prompt)
	if !ok {
		return acp.PromptResponse{}, nil, false
	}

	// The orchestrator's backoff retry tears the agent process down and
	// relaunches it via --resume, so an in-memory counter would reset every
	// attempt and never recover. Persist the attempt count in a file keyed by
	// the (resume-stable) session id so fail-then-recover survives relaunch.
	attempt := nextOverloadedAttempt(sid)

	if attempt <= failTimes {
		_, _ = fmt.Fprintf(logOutput, "mock-agent[%d]: emitting 529 Overloaded for session %s (failure %d/%d)\n",
			os.Getpid(), sid, attempt, failTimes)
		return acp.PromptResponse{}, &acp.RequestError{
			Code:    -32603,
			Message: overloaded529Message,
			Data:    map[string]any{"errorKind": "server_error"},
		}, true
	}

	// Recovered: clear the counter and emit a normal success response so the
	// turn completes (and the orchestrator clears the retry budget).
	_ = os.Remove(overloadedCounterPath(sid))
	e := &emitter{ctx: ctx, conn: a.conn, sid: sid}
	e.text(fmt.Sprintf("Provider recovered after %d transient failure(s) — here is your response.", failTimes))
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
}

// overloadedCounterPath returns the temp-file path tracking how many times the
// /overloaded scenario has fired for a session. Keyed by the resume-stable ACP
// session id so the count survives process relaunch across backoff retries.
func overloadedCounterPath(sid acp.SessionId) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(string(sid))
	return filepath.Join(os.TempDir(), "kandev-mock-overloaded-"+safe+".count")
}

// nextOverloadedAttempt increments and returns the persisted attempt count.
func nextOverloadedAttempt(sid acp.SessionId) int {
	path := overloadedCounterPath(sid)
	prev := 0
	if raw, err := os.ReadFile(path); err == nil {
		if n, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil {
			prev = n
		}
	}
	next := prev + 1
	_ = os.WriteFile(path, []byte(strconv.Itoa(next)), 0o600)
	return next
}

// bulkCmdRe matches `/bulk` or `/e2e:bulk`, optionally followed by `:N` or ` N`
// — the number of agent messages to emit (default 120, capped at 1000). Used to
// populate a long chat history for manually testing scrollback and the "Load
// older messages" pagination in dev/preview.
var bulkCmdRe = regexp.MustCompile(`(?i)^/(?:e2e:)?bulk(?:[: ]+(\d+))?$`)

const (
	bulkDefaultCount = 120
	bulkMaxCount     = 1000
	// Tool-input/output map keys, shared to avoid repeating the literals.
	toolKeyFilePath = "file_path"
	toolKeyContent  = "content"
)

// parseBulkCmd reports whether the prompt is the /bulk command and, if so, how
// many messages to emit (default 120, capped at 1000). Returns ok=false for any
// other prompt.
func parseBulkCmd(prompt string) (count int, ok bool) {
	cmd := stripKandevSystem(strings.TrimSpace(prompt))
	m := bulkCmdRe.FindStringSubmatch(cmd)
	if m == nil {
		return 0, false
	}
	count = bulkDefaultCount
	if m[1] != "" {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			count = n
		}
	}
	if count > bulkMaxCount {
		count = bulkMaxCount
	}
	return count, true
}

// emitBulk emits `count` short agent messages, each followed by a lightweight
// completed tool call. The tool-call boundary flushes the buffered text above
// into its own message row — consecutive agent text chunks otherwise coalesce
// into a single message — so the result is a long, paginated conversation. Handy
// for manually exercising chat scrollback and the "Load older messages" button
// in dev/preview without a real agent. Usage: `/bulk`, `/bulk:300`, `/bulk 300`.
func emitBulk(e *emitter, count int) {
	e.text(fmt.Sprintf("Generating %d messages to populate the chat history…", count))
	for i := 1; i <= count; i++ {
		e.text(fmt.Sprintf(
			"Bulk message %d of %d — filler content for testing chat pagination and the load-older button.",
			i, count))
		// The tool-call start flushes the text above into its own message row.
		toolID := nextToolID()
		e.startTool(toolID, fmt.Sprintf("Read file-%d.txt", i), acp.ToolKindRead,
			map[string]any{toolKeyFilePath: fmt.Sprintf("/tmp/bulk/file-%d.txt", i)})
		e.completeTool(toolID, map[string]any{toolKeyContent: fmt.Sprintf("contents of file %d", i)})
		fixedDelay(8)
	}
	e.text(fmt.Sprintf(
		"Done — emitted %d messages. Scroll up and use “Load older messages” to page through them.",
		count))
}

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

// stripKandevSystem removes all <kandev-system>...</kandev-system> blocks from the
// prompt. Tags can be prepended (backend system context injection) or appended
// (frontend plan/document context), so we strip all occurrences.
func stripKandevSystem(prompt string) string {
	result := kandevSystemRegex.ReplaceAllString(prompt, "")
	return strings.TrimSpace(result)
}

var kandevSystemRegex = regexp.MustCompile(`<kandev-system>[\s\S]*?</kandev-system>`)

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

	// Bulk mode: emit many messages to populate a long, paginated chat history.
	// Handled before the switch so `/e2e:bulk` doesn't fall into the generic
	// `/e2e:<scenario>` branch.
	if count, ok := parseBulkCmd(prompt); ok {
		emitBulk(e, count)
		return
	}
	if isChangesWalkthroughRequest(cmd) {
		scenarioWalkthroughRequested(e)
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
	case strings.EqualFold(cmd, "/subtask") || strings.HasPrefix(strings.ToLower(cmd), "/subtask "):
		emitCreateSubtask(e, cmd, model)
	case strings.HasPrefix(cmd, "/e2e:"):
		rest := strings.TrimPrefix(cmd, "/e2e:")
		scenarioName, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
		emitPredefinedScenario(e, scenarioName)
	// Friendly aliases for the two clarification e2e scenarios so the slash menu
	// exposes them without forcing users to type /e2e:clarification(-multi).
	case strings.EqualFold(cmd, "/ask-single"):
		emitPredefinedScenario(e, "clarification")
	case strings.EqualFold(cmd, "/ask-multiple"):
		emitPredefinedScenario(e, "clarification-multi")
	case strings.EqualFold(cmd, "/crash"):
		emitCrash(e, model)
	case strings.HasPrefix(cmd, "/todo"):
		emitTodoSequence(e, model)
	case strings.EqualFold(cmd, "/mermaid"):
		emitMermaidSequence(e, model)
	case strings.EqualFold(cmd, "/markdown"):
		emitMarkdownShowcase(e, model)
	case strings.EqualFold(cmd, "/sleep") || strings.HasPrefix(strings.ToLower(cmd), "/sleep "):
		emitSleep(e, cmd)
	default:
		emitRandomResponse(e, cmd, model)
	}
}

// emitSleep sleeps for the requested duration (default 10s) then responds.
// Useful for simulating a slow agent turn without any tool calls.
func emitSleep(e *emitter, cmd string) {
	d := 10 * time.Second
	parts := strings.Fields(cmd)
	if len(parts) >= 2 {
		if secs, err := time.ParseDuration(parts[1] + "s"); err == nil && secs > 0 {
			d = secs
		} else if parsed, err2 := time.ParseDuration(parts[1]); err2 == nil && parsed > 0 {
			d = parsed
		}
	}
	time.Sleep(d)
	e.text(fmt.Sprintf("Slept for %s.", d))
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

// emitCreateSubtask calls the kandev MCP `create_task_kandev` tool with
// parent_id="self" to create a subtask of the current task. Useful for
// manually exercising sidebar subtask UI in dev with KANDEV_MOCK_AGENT=true.
// Usage: `/subtask` or `/subtask My subtask title`.
func emitCreateSubtask(e *emitter, cmd, model string) {
	title := parseSubtaskTitle(cmd)

	args := map[string]any{
		"title":       title,
		"parent_id":   "self",
		"start_agent": false,
	}

	toolID := nextToolID()
	e.startTool(toolID, "create_task_kandev", acp.ToolKindOther, args)
	randomDelay(model)

	result, err := callMCPTool("kandev", "create_task_kandev", args)
	if err != nil {
		e.completeTool(toolID, map[string]any{"error": "MCP error: " + err.Error()})
		e.text(fmt.Sprintf("Failed to create subtask: %v", err))
		return
	}
	e.completeTool(toolID, map[string]any{"result": result})
	e.text(fmt.Sprintf("Created subtask %q under the current task.", title))
}

// parseSubtaskTitle extracts the title from a /subtask command. The dispatch
// matches case-insensitively, so we slice by prefix length rather than using
// strings.TrimPrefix (which would no-op on "/SubTask foo" and leak the prefix
// into the title). Returns an auto-generated title when no title is supplied.
func parseSubtaskTitle(cmd string) string {
	const prefix = "/subtask"
	title := ""
	if len(cmd) >= len(prefix) {
		title = strings.TrimSpace(cmd[len(prefix):])
	}
	if title == "" {
		title = fmt.Sprintf("Mock subtask %d", time.Now().Unix()%10000)
	}
	return title
}
