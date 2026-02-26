package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// isScriptMode returns true if the command starts with "e2e:" prefix
// (excluding the predefined "/e2e:" slash-command format).
func isScriptMode(cmd string) bool {
	return strings.HasPrefix(cmd, "e2e:")
}

// executeScript processes a multi-line script command.
func executeScript(enc *json.Encoder, fullPrompt, cmd string) {
	lines := strings.Split(cmd, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		executeCommand(enc, fullPrompt, line)
	}
}

// executeCommand dispatches a single script command line.
func executeCommand(enc *json.Encoder, fullPrompt, line string) {
	switch {
	case strings.HasPrefix(line, "e2e:message("):
		text := extractStringArg(line, "e2e:message(")
		emitTextBlock(enc, text, "")

	case strings.HasPrefix(line, "e2e:thinking("):
		text := extractStringArg(line, "e2e:thinking(")
		emitThinkingBlock(enc, text, "")

	case strings.HasPrefix(line, "e2e:delay("):
		ms := extractIntArg(line, "e2e:delay(")
		fixedDelay(ms)

	case strings.HasPrefix(line, "e2e:mcp:"):
		executeMCPCommand(enc, fullPrompt, line)

	case strings.HasPrefix(line, "e2e:tool_use("):
		executeSimulatedToolUse(enc, line)

	case strings.HasPrefix(line, "e2e:tool_result("):
		executeSimulatedToolResult(enc, line)
	}
}

// executeMCPCommand parses and executes: e2e:mcp:<server>:<tool>(<json_args>)
func executeMCPCommand(enc *json.Encoder, fullPrompt, line string) {
	rest := strings.TrimPrefix(line, "e2e:mcp:")

	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		emitTextBlock(enc, "Script error: invalid MCP command: "+line, "")
		return
	}
	server := rest[:colonIdx]
	remainder := rest[colonIdx+1:]

	parenIdx := strings.Index(remainder, "(")
	if parenIdx < 0 {
		emitTextBlock(enc, "Script error: missing args in MCP command: "+line, "")
		return
	}
	toolName := remainder[:parenIdx]
	closeIdx := strings.LastIndex(remainder, ")")
	if closeIdx <= parenIdx {
		emitTextBlock(enc, "Script error: missing closing paren in MCP command: "+line, "")
		return
	}
	argsStr := remainder[parenIdx+1 : closeIdx]

	var args map[string]any
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		emitTextBlock(enc, fmt.Sprintf("Script error: bad MCP args JSON: %v", err), "")
		return
	}

	substituteContextPlaceholders(args, fullPrompt)

	toolID := nextToolID()
	state.lastToolID = toolID
	emitToolUseBlock(enc, toolID, toolName, args)

	result, err := callMCPTool(server, toolName, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mock-agent: MCP call %s/%s failed: %v\n", server, toolName, err)
		emitToolResultBlock(enc, toolID, "MCP error: "+err.Error(), true)
	} else {
		emitToolResultBlock(enc, toolID, result, false)
	}
}

// executeSimulatedToolUse emits a simulated tool_use block (no real execution).
// Format: e2e:tool_use("name", {"key":"value"})
func executeSimulatedToolUse(enc *json.Encoder, line string) {
	inner := extractParenContent(line, "e2e:tool_use(")
	name, rest := extractFirstStringArg(inner)

	var input map[string]any
	rest = strings.TrimSpace(rest)
	if rest != "" {
		rest = strings.TrimPrefix(rest, ",")
		rest = strings.TrimSpace(rest)
		if err := json.Unmarshal([]byte(rest), &input); err != nil {
			emitTextBlock(enc, fmt.Sprintf("Script error: bad tool_use input JSON: %v", err), "")
			return
		}
	}

	toolID := nextToolID()
	state.lastToolID = toolID
	emitToolUseBlock(enc, toolID, name, input)
}

// executeSimulatedToolResult emits a tool_result for the last tool_use.
// Format: e2e:tool_result("content")
func executeSimulatedToolResult(enc *json.Encoder, line string) {
	content := extractStringArg(line, "e2e:tool_result(")
	emitToolResultBlock(enc, state.lastToolID, content, false)
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
	// Find closing quote (handling escaped quotes)
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
