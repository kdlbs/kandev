package acp

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const maxShellOutputBytes = 256 * 1024

type normalizedShellResult struct {
	exitCode  *int
	stdout    string
	stderr    string
	hasOutput bool
	truncated bool
}

func applyFinalShellResult(payload *streams.ShellExecPayload, result any) {
	normalized := normalizeFinalShellResult(result)
	if !normalized.hasOutput && normalized.exitCode == nil {
		return
	}
	output := ensureShellOutput(payload)
	if normalized.hasOutput {
		output.Stdout = normalized.stdout
		output.Stderr = normalized.stderr
		output.Truncated = normalized.truncated
	}
	if normalized.exitCode != nil {
		output.ExitCode = normalized.exitCode
	}
}

// NormalizeShellToolUpdate merges live and final ACP shell result fields into
// the active normalized payload. It reports whether the update contained a
// recognized shell output field so statusless updates can be persisted.
func (n *Normalizer) NormalizeShellToolUpdate(
	payload *streams.NormalizedPayload,
	meta map[string]any,
	contents []streams.ToolCallContentItem,
	rawOutput any,
) bool {
	if payload == nil || payload.ShellExec() == nil {
		return false
	}
	shell := payload.ShellExec()
	recognized := false

	if data, ok := terminalOutputData(meta, "terminal_output_delta"); ok {
		appendShellStdout(shell, data)
		recognized = true
	}
	if content, ok := cumulativeShellContent(contents); ok {
		replaceShellStdout(shell, content)
		recognized = true
	}
	if rawOutput != nil {
		applyFinalShellResult(shell, rawOutput)
		recognized = true
	}
	if data, ok := terminalOutputData(meta, "terminal_output"); ok {
		if rawOutput != nil {
			replaceShellStdout(shell, data)
		} else {
			replaceOrAppendTerminalOutput(shell, data)
		}
		recognized = true
	}
	if exitCode, ok := terminalExitCode(meta); ok {
		ensureShellOutput(shell).ExitCode = intPtr(exitCode)
		recognized = true
	}

	return recognized
}

func ensureShellOutput(payload *streams.ShellExecPayload) *streams.ShellExecOutput {
	if payload.Output == nil {
		payload.Output = &streams.ShellExecOutput{}
	}
	return payload.Output
}

func appendShellStdout(payload *streams.ShellExecPayload, data string) {
	output := ensureShellOutput(payload)
	bounded, truncated := boundShellOutput(output.Stdout + data)
	output.Stdout = bounded
	output.Truncated = output.Truncated || truncated
}

func replaceShellStdout(payload *streams.ShellExecPayload, data string) {
	output := ensureShellOutput(payload)
	bounded, truncated := boundShellOutput(data)
	output.Stdout = bounded
	output.Truncated = output.Truncated || truncated
}

func replaceOrAppendTerminalOutput(payload *streams.ShellExecPayload, data string) {
	current := ensureShellOutput(payload).Stdout
	if current == "" || strings.HasPrefix(data, current) {
		replaceShellStdout(payload, data)
		return
	}
	// ACP does not identify cumulative terminal_output values. Treat values
	// without the current prefix as new chunks so mixed update streams remain intact.
	appendShellStdout(payload, data)
}

func terminalOutputData(meta map[string]any, key string) (string, bool) {
	if meta == nil {
		return "", false
	}
	value, ok := meta[key]
	if !ok {
		return "", false
	}
	if data, ok := value.(string); ok {
		return data, true
	}
	valueMap, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	data, ok := valueMap["data"].(string)
	return data, ok
}

func terminalExitCode(meta map[string]any) (int, bool) {
	if meta == nil {
		return 0, false
	}
	exit, ok := meta["terminal_exit"].(map[string]any)
	if !ok {
		return 0, false
	}
	if code, ok := parseExitCode(exit["exit_code"]); ok {
		return code, true
	}
	return parseExitCode(exit["code"])
}

func cumulativeShellContent(contents []streams.ToolCallContentItem) (string, bool) {
	var content strings.Builder
	found := false
	for _, item := range contents {
		if item.Type != toolContentType || item.Content == nil || item.Content.Type != contentTypeText {
			continue
		}
		content.WriteString(item.Content.Text)
		found = true
	}
	return content.String(), found
}

func normalizeFinalShellResult(result any) normalizedShellResult {
	result = unwrapShellRawOutput(result)
	switch value := result.(type) {
	case string:
		return normalizeShellText(value)
	case map[string]any:
		return normalizeShellResultMap(value)
	default:
		return normalizedShellResult{}
	}
}

func unwrapShellRawOutput(result any) any {
	resultMap, ok := result.(map[string]any)
	if !ok {
		return result
	}
	if rawOutput, ok := resultMap["rawOutput"]; ok {
		return rawOutput
	}
	return result
}

func normalizeShellResultMap(result map[string]any) normalizedShellResult {
	stdout, stdoutExplicit := result["stdout"].(string)
	stderr, stderrExplicit := result["stderr"].(string)
	formatted, hasFormatted := result["formatted_output"].(string)
	output, hasOutput := result["output"].(string)

	var normalized normalizedShellResult
	switch {
	case stdoutExplicit || stderrExplicit:
		normalized.stdout = stdout
		normalized.stderr = stderr
		normalized.hasOutput = true
	case hasFormatted:
		normalized = normalizeShellText(formatted)
	case hasOutput:
		normalized = normalizeShellText(output)
	}

	if code, ok := parseExitCode(result["exit_code"]); ok {
		normalized.exitCode = intPtr(code)
	} else if metadata, ok := result["metadata"].(map[string]any); ok {
		if code, ok := parseExitCode(metadata["exit"]); ok {
			normalized.exitCode = intPtr(code)
		}
	}

	normalized.stdout, normalized.truncated = boundShellOutput(normalized.stdout)
	var stderrTruncated bool
	normalized.stderr, stderrTruncated = boundShellOutput(normalized.stderr)
	normalized.truncated = normalized.truncated || stderrTruncated
	return normalized
}

func normalizeShellText(output string) normalizedShellResult {
	if !hasEmbeddedShellTags(output) {
		stdout, truncated := boundShellOutput(output)
		return normalizedShellResult{stdout: stdout, hasOutput: true, truncated: truncated}
	}

	stdout, hasStdout := embeddedShellField(output, "output")
	stderr, hasStderr := embeddedShellField(output, "stderr")
	codeText, hasExit := embeddedShellField(output, "return-code")
	result := normalizedShellResult{hasOutput: hasStdout || hasStderr}
	if hasStdout {
		result.stdout = strings.TrimSpace(stdout)
	}
	if hasStderr {
		result.stderr = strings.TrimSpace(stderr)
	}
	if code, ok := parseExitCode(codeText); hasExit && ok {
		result.exitCode = intPtr(code)
	}
	if !result.hasOutput {
		result.stdout = output
		result.hasOutput = true
	}

	result.stdout, result.truncated = boundShellOutput(result.stdout)
	var stderrTruncated bool
	result.stderr, stderrTruncated = boundShellOutput(result.stderr)
	result.truncated = result.truncated || stderrTruncated
	return result
}

func hasEmbeddedShellTags(output string) bool {
	return strings.Contains(output, "<return-code>") ||
		strings.Contains(output, "<output>") ||
		strings.Contains(output, "<stderr>")
}

func embeddedShellField(output, field string) (string, bool) {
	open := "<" + field + ">"
	close := "</" + field + ">"
	start := strings.Index(output, open)
	if start == -1 {
		return "", false
	}
	start += len(open)
	end := strings.Index(output[start:], close)
	if end == -1 {
		return "", false
	}
	return output[start : start+end], true
}

func parseExitCode(value any) (int, bool) {
	switch code := value.(type) {
	case int:
		return code, true
	case int32:
		return int(code), true
	case int64:
		return int(code), true
	case float64:
		if code != float64(int(code)) {
			return 0, false
		}
		return int(code), true
	case json.Number:
		parsed, err := strconv.Atoi(code.String())
		return parsed, err == nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(code))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func boundShellOutput(output string) (string, bool) {
	if len(output) <= maxShellOutputBytes {
		return output, false
	}
	start := len(output) - maxShellOutputBytes
	for start < len(output) && !utf8.RuneStart(output[start]) {
		start++
	}
	return output[start:], true
}

func intPtr(value int) *int {
	return &value
}
