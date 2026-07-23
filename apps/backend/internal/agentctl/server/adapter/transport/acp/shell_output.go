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
	exitCode        *int
	stdout          string
	stderr          string
	hasStdout       bool
	hasStderr       bool
	stdoutTruncated bool
	stderrTruncated bool
}

func applyFinalShellResult(payload *streams.ShellExecPayload, result any) {
	normalized := normalizeFinalShellResult(result)
	if !normalized.hasStdout && !normalized.hasStderr && normalized.exitCode == nil {
		return
	}
	output := ensureShellOutput(payload)
	if normalized.hasStdout {
		output.Stdout = stripLeadingCommandEcho(payload.Command, normalized.stdout)
		output.StdoutTruncated = normalized.stdoutTruncated
	}
	if normalized.hasStderr {
		output.Stderr = normalized.stderr
		output.StderrTruncated = normalized.stderrTruncated
	}
	if normalized.hasStdout || normalized.hasStderr {
		syncShellTruncation(output)
	}
	if normalized.exitCode != nil {
		output.ExitCode = normalized.exitCode
	}
}

// stripLeadingCommandEcho removes a leading terminal-prompt echo of the tool
// call's command from captured shell output. Some providers capture command
// output via a real terminal session whose input echo - rendered behind a
// "$ " shell prompt - is concatenated directly onto the actual output,
// duplicating the command the UI already renders above the output
// disclosure. Matching requires the literal "$ " prompt marker: a bare
// command-text prefix with no prompt is not stripped, because legitimate
// output can coincidentally start with text identical to the command (e.g.
// a file whose contents happen to begin with the command string).
//
// Used for final/terminal results, where the buffer is known to be complete:
// a stdout consisting of nothing but the echoed command commits to an empty
// result rather than leaving the redundant echo visible.
func stripLeadingCommandEcho(command, stdout string) string {
	return stripCommandEcho(command, stdout, true)
}

// stripPendingCommandEcho is the live-update counterpart of
// stripLeadingCommandEcho. Incremental terminal_output/terminal_output_delta
// chunks can split exactly between the echoed command and the separator that
// precedes real output (read() boundaries are arbitrary), so a buffer that
// currently equals nothing but the echo is left untouched - the separator or
// real output may still be in flight - instead of collapsing to empty and
// orphaning a later chunk's leading newline.
func stripPendingCommandEcho(command, stdout string) string {
	return stripCommandEcho(command, stdout, false)
}

func stripCommandEcho(command, stdout string, commitExactMatch bool) string {
	if command == "" || stdout == "" {
		return stdout
	}
	rest, ok := strings.CutPrefix(stdout, "$ "+command)
	if !ok {
		return stdout
	}
	if rest == "" && !commitExactMatch {
		return stdout
	}
	if trimmed, ok := strings.CutPrefix(rest, "\r\n"); ok {
		return trimmed
	}
	return strings.TrimPrefix(rest, "\n")
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

	if shell.Output != nil {
		if rawOutput != nil {
			// A final result arrived in this same update: commit the strip
			// even if the buffer is nothing but the echo, and re-apply it
			// last so a terminal_output field processed above (line ~118)
			// can't clobber applyFinalShellResult's already-stripped stdout
			// with the provider's raw, unstripped snapshot.
			shell.Output.Stdout = stripLeadingCommandEcho(shell.Command, shell.Output.Stdout)
		} else {
			shell.Output.Stdout = stripPendingCommandEcho(shell.Command, shell.Output.Stdout)
		}
	}

	return recognized
}

func ensureShellOutput(payload *streams.ShellExecPayload) *streams.ShellExecOutput {
	if payload.Output == nil {
		payload.Output = &streams.ShellExecOutput{}
	}
	if payload.Output.Truncated && !payload.Output.StdoutTruncated && !payload.Output.StderrTruncated {
		payload.Output.StdoutTruncated = payload.Output.Stdout != ""
		payload.Output.StderrTruncated = payload.Output.Stderr != ""
	}
	return payload.Output
}

func syncShellTruncation(output *streams.ShellExecOutput) {
	output.Truncated = output.StdoutTruncated || output.StderrTruncated
}

func appendShellStdout(payload *streams.ShellExecPayload, data string) {
	output := ensureShellOutput(payload)
	rawBounded, _ := boundShellOutput(output.RawTerminalOutput + data)
	output.RawTerminalOutput = rawBounded
	bounded, truncated := boundShellOutput(output.Stdout + data)
	output.Stdout = bounded
	output.StdoutTruncated = output.StdoutTruncated || truncated
	syncShellTruncation(output)
}

func replaceShellStdout(payload *streams.ShellExecPayload, data string) {
	output := ensureShellOutput(payload)
	bounded, truncated := boundShellOutput(data)
	output.RawTerminalOutput = bounded
	output.Stdout = bounded
	output.StdoutTruncated = truncated
	syncShellTruncation(output)
}

// replaceOrAppendTerminalOutput merges a new cumulative terminal_output
// snapshot into the stored output. The prefix comparison uses
// RawTerminalOutput - a raw parallel of Stdout that mirrors the same bounded
// append/replace bookkeeping but is never echo-stripped - rather than the
// displayed Stdout. Stdout can already have had a leading command echo
// stripped by the end of a prior update; comparing a fresh raw cumulative
// frame against that stripped value would never find a matching prefix,
// misclassifying every later frame as a new chunk and duplicating output
// (plus reintroducing the very echo this file strips).
func replaceOrAppendTerminalOutput(payload *streams.ShellExecPayload, data string) {
	baseline := ensureShellOutput(payload).RawTerminalOutput
	if baseline == "" || strings.HasPrefix(data, baseline) {
		replaceShellStdout(payload, data)
		return
	}
	// ACP does not identify cumulative terminal_output values. Treat values
	// without the baseline prefix as new chunks so mixed update streams remain intact.
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
		if stdoutExplicit {
			normalized.stdout = stdout
			normalized.hasStdout = true
		}
		if stderrExplicit {
			normalized.stderr = stderr
			normalized.hasStderr = true
		}
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

	stdout, stdoutTruncated := boundShellOutput(normalized.stdout)
	stderr, stderrTruncated := boundShellOutput(normalized.stderr)
	normalized.stdout = stdout
	normalized.stderr = stderr
	normalized.stdoutTruncated = normalized.stdoutTruncated || stdoutTruncated
	normalized.stderrTruncated = normalized.stderrTruncated || stderrTruncated
	return normalized
}

func normalizeShellText(output string) normalizedShellResult {
	if !hasEmbeddedShellTags(output) {
		stdout, truncated := boundShellOutput(output)
		return normalizedShellResult{stdout: stdout, hasStdout: true, stdoutTruncated: truncated}
	}

	stdout, hasStdout := embeddedShellField(output, "output")
	stderr, hasStderr := embeddedShellField(output, "stderr")
	codeText, hasExit := embeddedShellField(output, "return-code")
	result := normalizedShellResult{hasStdout: hasStdout, hasStderr: hasStderr}
	if hasStdout {
		result.stdout = strings.TrimSpace(stdout)
	}
	if hasStderr {
		result.stderr = strings.TrimSpace(stderr)
	}
	if code, ok := parseExitCode(codeText); hasExit && ok {
		result.exitCode = intPtr(code)
	}
	if !result.hasStdout && !result.hasStderr && !hasExit {
		result.stdout = output
		result.hasStdout = true
	}

	result.stdout, result.stdoutTruncated = boundShellOutput(result.stdout)
	result.stderr, result.stderrTruncated = boundShellOutput(result.stderr)
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
