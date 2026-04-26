package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// isScriptMode returns true if the command starts with "e2e:" prefix
// (excluding the predefined "/e2e:" slash-command format).
func isScriptMode(cmd string) bool {
	return strings.HasPrefix(cmd, "e2e:")
}

// executeScript processes a multi-line script command.
func executeScript(e *emitter, fullPrompt, cmd string) {
	lines := strings.Split(cmd, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		executeCommand(e, fullPrompt, line)
	}
}

// executeCommand dispatches a single script command line.
func executeCommand(e *emitter, fullPrompt, line string) {
	switch {
	case strings.HasPrefix(line, "e2e:message("):
		text := extractStringArg(line, "e2e:message(")
		e.text(text)

	case strings.HasPrefix(line, "e2e:thinking("):
		text := extractStringArg(line, "e2e:thinking(")
		e.thought(text)

	case strings.HasPrefix(line, "e2e:delay("):
		ms := extractIntArg(line, "e2e:delay(")
		fixedDelay(ms)

	case strings.HasPrefix(line, "e2e:mcp:"):
		executeMCPCommand(e, fullPrompt, line)

	case strings.HasPrefix(line, "e2e:plan("):
		executePlanCommand(e, line)

	case strings.HasPrefix(line, "e2e:tool_use("):
		executeSimulatedToolUse(e, line)

	case strings.HasPrefix(line, "e2e:tool_result("):
		executeSimulatedToolResult(e, line)

	case strings.HasPrefix(line, "e2e:monitor_start("):
		executeMonitorStart(e, line)

	case strings.HasPrefix(line, "e2e:monitor_event("):
		executeMonitorEvent(e, line)

	case strings.HasPrefix(line, "e2e:monitor_end("):
		executeMonitorEnd(e, line)
	}
}

// executeMonitorStart emits a Monitor registration sequence reproducing the
// claude-agent-acp wire format. Format: e2e:monitor_start("taskId", "command")
//
// The taskId is what subsequent monitor_event / monitor_end directives use
// to look the Monitor back up. Command is recorded in the rawInput so the
// frontend can label the card (e.g. "gh pr checks --watch").
func executeMonitorStart(e *emitter, line string) {
	inner := extractParenContent(line, "e2e:monitor_start(")
	taskID, rest := extractFirstStringArg(inner)
	if taskID == "" {
		e.text("Script error: monitor_start requires taskId")
		return
	}
	rest = strings.TrimSpace(rest)
	rest = strings.TrimPrefix(rest, ",")
	command, _ := extractFirstStringArg(rest)
	toolID := nextToolID()
	state.monitorTools[taskID] = toolID
	e.startMonitorTool(toolID, taskID, command)
}

// executeMonitorEvent emits a `<task-notification>` envelope as if the
// model echoed an injected user turn. Format: e2e:monitor_event("taskId", "event body")
func executeMonitorEvent(e *emitter, line string) {
	inner := extractParenContent(line, "e2e:monitor_event(")
	taskID, rest := extractFirstStringArg(inner)
	if taskID == "" {
		e.text("Script error: monitor_event requires taskId")
		return
	}
	rest = strings.TrimSpace(rest)
	rest = strings.TrimPrefix(rest, ",")
	body, _ := extractFirstStringArg(rest)
	e.emitMonitorEvent(taskID, body)
}

// executeMonitorEnd emits the terminal tool_call_update for a previously
// started Monitor. Format: e2e:monitor_end("taskId")
func executeMonitorEnd(e *emitter, line string) {
	taskID := extractStringArg(line, "e2e:monitor_end(")
	tcID, ok := state.monitorTools[taskID]
	if !ok {
		e.text("Script error: monitor_end for unknown taskId " + taskID)
		return
	}
	delete(state.monitorTools, taskID)
	e.endMonitorTool(tcID)
}

// executeMCPCommand parses and executes: e2e:mcp:<server>:<tool>(<json_args>)
func executeMCPCommand(e *emitter, fullPrompt, line string) {
	rest := strings.TrimPrefix(line, "e2e:mcp:")

	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		e.text("Script error: invalid MCP command: " + line)
		return
	}
	server := rest[:colonIdx]
	remainder := rest[colonIdx+1:]

	parenIdx := strings.Index(remainder, "(")
	if parenIdx < 0 {
		e.text("Script error: missing args in MCP command: " + line)
		return
	}
	toolName := remainder[:parenIdx]
	closeIdx := strings.LastIndex(remainder, ")")
	if closeIdx <= parenIdx {
		e.text("Script error: missing closing paren in MCP command: " + line)
		return
	}
	argsStr := remainder[parenIdx+1 : closeIdx]

	var args map[string]any
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		e.text(fmt.Sprintf("Script error: bad MCP args JSON: %v", err))
		return
	}

	substituteContextPlaceholders(args, fullPrompt)

	toolID := nextToolID()
	e.startTool(toolID, toolName, acp.ToolKindOther, args)

	result, err := callMCPTool(server, toolName, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mock-agent: MCP call %s/%s failed: %v\n", server, toolName, err)
		e.completeTool(toolID, map[string]any{"error": "MCP error: " + err.Error()})
	} else {
		e.completeTool(toolID, map[string]any{"result": result})
	}
}

// executePlanCommand emits an ACP Plan update.
// Format: e2e:plan([{"content":"task","status":"in_progress"},{"content":"task2","status":"pending"}])
func executePlanCommand(e *emitter, line string) {
	argsStr := extractParenContent(line, "e2e:plan(")
	var rawEntries []struct {
		Content  string `json:"content"`
		Status   string `json:"status"`
		Priority string `json:"priority,omitempty"`
	}
	if err := json.Unmarshal([]byte(argsStr), &rawEntries); err != nil {
		e.text(fmt.Sprintf("Script error: bad plan JSON: %v", err))
		return
	}
	entries := make([]acp.PlanEntry, len(rawEntries))
	for i, r := range rawEntries {
		entries[i] = acp.PlanEntry{
			Content:  r.Content,
			Status:   acp.PlanEntryStatus(r.Status),
			Priority: acp.PlanEntryPriority(r.Priority),
		}
	}
	e.plan(entries)
}

// executeSimulatedToolUse emits a simulated tool call start (no real execution).
// Format: e2e:tool_use("name", {"key":"value"})
func executeSimulatedToolUse(e *emitter, line string) {
	inner := extractParenContent(line, "e2e:tool_use(")
	name, rest := extractFirstStringArg(inner)

	var input map[string]any
	rest = strings.TrimSpace(rest)
	if rest != "" {
		rest = strings.TrimPrefix(rest, ",")
		rest = strings.TrimSpace(rest)
		if err := json.Unmarshal([]byte(rest), &input); err != nil {
			e.text(fmt.Sprintf("Script error: bad tool_use input JSON: %v", err))
			return
		}
	}

	toolID := nextToolID()
	e.startTool(toolID, name, acp.ToolKindOther, input)
}

// executeSimulatedToolResult emits a tool completion for the last tool_use.
// Format: e2e:tool_result("content")
func executeSimulatedToolResult(e *emitter, line string) {
	content := extractStringArg(line, "e2e:tool_result(")
	e.completeTool(acp.ToolCallId(state.lastToolID), map[string]any{"result": content})
}

// --- Argument parsing helpers ---

// extractStringArg extracts a quoted string argument from a command like prefix"text")
func extractStringArg(line, prefix string) string {
	rest := strings.TrimPrefix(line, prefix)
	rest = strings.TrimSuffix(rest, ")")
	rest = strings.TrimSpace(rest)
	if len(rest) >= 2 && rest[0] == '"' && rest[len(rest)-1] == '"' {
		rest = rest[1 : len(rest)-1]
	}
	return unescapeString(rest)
}

// extractIntArg extracts an integer argument from a command like prefix123)
func extractIntArg(line, prefix string) int {
	rest := strings.TrimPrefix(line, prefix)
	rest = strings.TrimSuffix(rest, ")")
	rest = strings.TrimSpace(rest)
	n, _ := strconv.Atoi(rest)
	return n
}

// extractParenContent extracts everything inside the outermost parentheses.
func extractParenContent(line, prefix string) string {
	rest := strings.TrimPrefix(line, prefix)
	if len(rest) > 0 && rest[len(rest)-1] == ')' {
		rest = rest[:len(rest)-1]
	}
	return rest
}

// extractFirstStringArg extracts the first quoted string and returns
// (value, remaining).
func extractFirstStringArg(s string) (string, string) {
	s = strings.TrimSpace(s)
	if len(s) == 0 || s[0] != '"' {
		return s, ""
	}
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++ // skip escaped char
			continue
		}
		if s[i] == '"' {
			return unescapeString(s[1:i]), s[i+1:]
		}
	}
	return s[1:], ""
}

// unescapeString handles basic escape sequences in script string args.
func unescapeString(s string) string {
	s = strings.ReplaceAll(s, `\\`, "\x00") // temp placeholder
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, "\x00", `\`)
	return s
}

// --- Context placeholder substitution ---

var (
	taskIDRegex    = regexp.MustCompile(`Kandev Task ID:\s*(\S+)`)
	sessionIDRegex = regexp.MustCompile(`Kandev Session ID:\s*(\S+)`)
)

// substituteContextPlaceholders replaces {task_id} and {session_id} placeholders
// in MCP tool args with values from the kandev-system context block.
func substituteContextPlaceholders(args map[string]any, fullPrompt string) {
	taskID := extractRegexMatch(taskIDRegex, fullPrompt)
	sessID := extractRegexMatch(sessionIDRegex, fullPrompt)

	for k, v := range args {
		if s, ok := v.(string); ok {
			s = strings.ReplaceAll(s, "{task_id}", taskID)
			s = strings.ReplaceAll(s, "{session_id}", sessID)
			args[k] = s
		}
	}
}

// extractRegexMatch returns the first capture group match, or empty string.
func extractRegexMatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}
