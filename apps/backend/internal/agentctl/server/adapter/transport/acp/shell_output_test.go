package acp

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestNormalizeShellToolResultProviderShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		result      any
		wantStdout  string
		wantStderr  string
		wantExit    float64
		wantHasExit bool
	}{
		{
			name: "codex formatted output and exit",
			result: map[string]any{
				"formatted_output": "codex output\n",
				"exit_code":        float64(4),
			},
			wantStdout:  "codex output\n",
			wantExit:    4,
			wantHasExit: true,
		},
		{
			name: "opencode output and nested exit",
			result: map[string]any{
				"output": "opencode output\n",
				"metadata": map[string]any{
					"exit": float64(7),
				},
			},
			wantStdout:  "opencode output\n",
			wantExit:    7,
			wantHasExit: true,
		},
		{
			name: "auggie embedded streams and return code",
			result: map[string]any{
				"output": "<return-code>1</return-code><output>auggie out</output><stderr>auggie err</stderr>",
			},
			wantStdout:  "auggie out",
			wantStderr:  "auggie err",
			wantExit:    1,
			wantHasExit: true,
		},
		{
			name:        "claude plain output keeps exit unknown",
			result:      "claude output\n",
			wantStdout:  "claude output\n",
			wantHasExit: false,
		},
		{
			name: "explicit streams and top-level exit take precedence",
			result: map[string]any{
				"stdout":           "explicit stdout",
				"stderr":           "explicit stderr",
				"formatted_output": "formatted fallback",
				"output":           "output fallback",
				"exit_code":        float64(5),
				"metadata":         map[string]any{"exit": float64(9)},
			},
			wantStdout:  "explicit stdout",
			wantStderr:  "explicit stderr",
			wantExit:    5,
			wantHasExit: true,
		},
		{
			name: "malformed exits stay unknown",
			result: map[string]any{
				"output":    "output with malformed exit",
				"exit_code": "not-an-exit",
				"metadata":  map[string]any{"exit": "still-not-an-exit"},
			},
			wantStdout:  "output with malformed exit",
			wantHasExit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			normalizer := NewNormalizer("")
			payload := normalizer.NormalizeToolCall("execute", map[string]any{
				"kind":      "execute",
				"raw_input": map[string]any{"command": "test-command"},
			})

			normalizer.NormalizeToolResult(payload, tt.result)

			output := shellOutputJSON(t, payload.ShellExec().Output)
			require.Equal(t, tt.wantStdout, output["stdout"])
			require.Equal(t, tt.wantStderr, stringValue(output["stderr"]))
			exitCode, hasExit := output["exit_code"]
			require.Equal(t, tt.wantHasExit, hasExit)
			if tt.wantHasExit {
				require.Equal(t, tt.wantExit, exitCode)
			}
		})
	}
}

func TestNormalizeShellToolResultBoundsOutputAndPreservesUTF8(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "long-command"},
	})
	input := strings.Repeat("a", 256*1024) + "three bytes: \u2603"

	normalizer.NormalizeToolResult(payload, input)

	output := shellOutputJSON(t, payload.ShellExec().Output)
	stdout, ok := output["stdout"].(string)
	require.True(t, ok)
	require.LessOrEqual(t, len(stdout), 256*1024)
	require.True(t, utf8.ValidString(stdout))
	require.Contains(t, stdout, "three bytes: \u2603")
	require.Equal(t, true, output["truncated"])
	require.NotContains(t, output, "exit_code")
}

func TestNormalizeShellToolResultProviderMapPreservesTruncation(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"formatted_output", "output"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			normalizer := NewNormalizer("")
			payload := normalizer.NormalizeToolCall("execute", map[string]any{
				"kind":      "execute",
				"raw_input": map[string]any{"command": "long-command"},
			})

			normalizer.NormalizeToolResult(payload, map[string]any{
				field: strings.Repeat("x", maxShellOutputBytes+1),
			})

			require.Len(t, payload.ShellExec().Output.Stdout, maxShellOutputBytes)
			require.True(t, payload.ShellExec().Output.Truncated)
		})
	}
}

func TestNormalizeShellToolUpdateKeepsExplicitStderrTruncation(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "test-command"},
	})

	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output": map[string]any{"data": "structured stdout"}},
		nil,
		map[string]any{"stdout": "fallback", "stderr": strings.Repeat("e", 256*1024+1)},
	)

	require.Equal(t, "structured stdout", payload.ShellExec().Output.Stdout)
	require.Len(t, payload.ShellExec().Output.Stderr, 256*1024)
	require.True(t, payload.ShellExec().Output.Truncated)
}

func TestNormalizeShellToolUpdateClearsStdoutTruncationOnShortReplacement(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "test-command"},
	})

	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output": map[string]any{"data": "authoritative stdout"}},
		nil,
		strings.Repeat("x", maxShellOutputBytes+1),
	)

	require.Equal(t, "authoritative stdout", payload.ShellExec().Output.Stdout)
	require.False(t, payload.ShellExec().Output.Truncated)
}

func TestNormalizeShellToolUpdateAppendsNonCumulativeLiveOutput(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "test-command"},
	})

	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output_delta": map[string]any{"data": "hello\n"}},
		nil,
		nil,
	)
	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output": map[string]any{"data": "world\n"}},
		nil,
		nil,
	)

	require.Equal(t, "hello\nworld\n", payload.ShellExec().Output.Stdout)
}

func TestNormalizeShellToolUpdatePreservesStreamsMissingFromLaterResults(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "test-command"},
	})
	payload.ShellExec().Output = nil

	normalizer.NormalizeShellToolUpdate(
		payload,
		nil,
		nil,
		map[string]any{"stdout": "first stdout", "stderr": "retained stderr"},
	)
	normalizer.NormalizeShellToolUpdate(payload, nil, nil, map[string]any{"stdout": "final stdout"})
	require.Equal(t, "final stdout", payload.ShellExec().Output.Stdout)
	require.Equal(t, "retained stderr", payload.ShellExec().Output.Stderr)

	normalizer.NormalizeShellToolUpdate(payload, nil, nil, map[string]any{"stderr": "final stderr"})
	require.Equal(t, "final stdout", payload.ShellExec().Output.Stdout)
	require.Equal(t, "final stderr", payload.ShellExec().Output.Stderr)
}

func TestNormalizeShellToolResultReturnCodeOnlyDoesNotRenderMarkup(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "test-command"},
	})

	normalizer.NormalizeToolResult(payload, "<return-code>1</return-code>")

	require.Empty(t, payload.ShellExec().Output.Stdout)
	require.NotNil(t, payload.ShellExec().Output.ExitCode)
	require.Equal(t, 1, *payload.ShellExec().Output.ExitCode)
}

func TestNormalizeShellToolUpdateCumulativeOutputRemainsCorrectAfterTruncation(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "test-command"},
	})
	initial := strings.Repeat("a", maxShellOutputBytes) + "first tail"
	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output": map[string]any{"data": initial}},
		nil,
		nil,
	)
	cumulative := initial + " second tail"
	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output": map[string]any{"data": cumulative}},
		nil,
		nil,
	)

	want, _ := boundShellOutput(cumulative)
	require.Equal(t, want, payload.ShellExec().Output.Stdout)
	require.True(t, payload.ShellExec().Output.Truncated)
}

func TestNormalizeShellToolResultStripsLeadingCommandEcho(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		command    string
		result     any
		wantStdout string
	}{
		{
			name:       "echoed command directly precedes output with no separator",
			command:    "cat file.txt",
			result:     "$ cat file.txt=== marker ===\n",
			wantStdout: "=== marker ===\n",
		},
		{
			name:       "echoed command with shell prompt and newline separator",
			command:    "ls -la",
			result:     "$ ls -la\ntotal 0\n",
			wantStdout: "total 0\n",
		},
		{
			name:       "multi-line command echoed verbatim before output",
			command:    "for i in 1 2; do\n  echo $i\ndone",
			result:     "$ for i in 1 2; do\n  echo $i\ndone\n1\n2\n",
			wantStdout: "1\n2\n",
		},
		{
			name:       "command text appearing later in output is preserved",
			command:    "echo hi",
			result:     "unrelated preamble\necho hi appears mid-output\n",
			wantStdout: "unrelated preamble\necho hi appears mid-output\n",
		},
		{
			name:       "output starting with a different command is preserved",
			command:    "echo hi",
			result:     "echo bye\n",
			wantStdout: "echo bye\n",
		},
		{
			// Regression for a review finding: without the "$ " prompt marker,
			// a bare command-text prefix is ambiguous with legitimate output
			// that happens to start with the same text (e.g. a file whose
			// content begins with the command string, or a command that
			// echoes its own argument). Only the evidenced "$ "-prefixed
			// terminal-echo shape is stripped.
			name:       "bare command-text prefix without a shell prompt is preserved",
			command:    "cat file.txt",
			result:     "cat file.txt",
			wantStdout: "cat file.txt",
		},
		{
			name:       "output that happens to start with the command text is preserved",
			command:    "echo hi",
			result:     "echo hi.txt contents follow\n",
			wantStdout: "echo hi.txt contents follow\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			normalizer := NewNormalizer("")
			payload := normalizer.NormalizeToolCall("execute", map[string]any{
				"kind":      "execute",
				"raw_input": map[string]any{"command": tt.command},
			})

			normalizer.NormalizeToolResult(payload, tt.result)

			require.Equal(t, tt.wantStdout, payload.ShellExec().Output.Stdout)
		})
	}
}

func TestNormalizeShellToolUpdateStripsLeadingCommandEchoFromLiveOutput(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "tail -f log.txt"},
	})

	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output_delta": map[string]any{"data": "$ tail -f log.txt"}},
		nil,
		nil,
	)
	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output_delta": map[string]any{"data": "\nline one\n"}},
		nil,
		nil,
	)

	require.Equal(t, "line one\n", payload.ShellExec().Output.Stdout)
}

// TestNormalizeShellToolUpdateCumulativeTerminalOutputDoesNotDuplicateAfterEchoStrip
// is a regression for a review finding: stripping the echo from the
// displayed Stdout after the first cumulative terminal_output frame must not
// corrupt the prefix comparison used to classify the next raw cumulative
// frame. Comparing the provider's next full (unstripped) snapshot against an
// already-stripped Stdout never finds a matching prefix, so the frame gets
// misclassified as a new delta chunk - duplicating the prior output and
// reintroducing the very echo this file exists to strip.
func TestNormalizeShellToolUpdateCumulativeTerminalOutputDoesNotDuplicateAfterEchoStrip(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "cmd"},
	})

	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output": map[string]any{"data": "$ cmd\nfirst\n"}},
		nil,
		nil,
	)
	require.Equal(t, "first\n", payload.ShellExec().Output.Stdout)

	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output": map[string]any{"data": "$ cmd\nfirst\nsecond\n"}},
		nil,
		nil,
	)
	require.Equal(t, "first\nsecond\n", payload.ShellExec().Output.Stdout)
}

// TestNormalizeShellToolUpdateCumulativeOversizedFrameStaysCorrectAfterEchoStrip
// documents that branch selection in replaceOrAppendTerminalOutput cannot
// desync the final Stdout once a cumulative frame's own length reaches the
// output bound: boundShellOutput always keeps just the trailing N bytes, so
// bound(anything + frame) == bound(frame) whenever len(frame) >= N,
// regardless of whether that frame was classified as a replace or an append.
// The short-output regime (the actually-reported bug shape, and the sibling
// test above) is where a misclassified branch is observable; this test
// pins the oversized regime to a concrete, content-bearing assertion rather
// than relying on that invariant by inference.
func TestNormalizeShellToolUpdateCumulativeOversizedFrameStaysCorrectAfterEchoStrip(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "cmd"},
	})

	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output": map[string]any{"data": "$ cmd\nfirst\n"}},
		nil,
		nil,
	)
	require.Equal(t, "first\n", payload.ShellExec().Output.Stdout)

	oversized := "$ cmd\nfirst\n" + strings.Repeat("a", maxShellOutputBytes) + "DISTINGUISHABLE_TAIL"
	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output": map[string]any{"data": oversized}},
		nil,
		nil,
	)

	want, truncated := boundShellOutput(oversized)
	require.True(t, truncated)
	require.Equal(t, want, payload.ShellExec().Output.Stdout)
	require.Contains(t, payload.ShellExec().Output.Stdout, "DISTINGUISHABLE_TAIL")
	require.NotContains(t, payload.ShellExec().Output.Stdout, "first\nfirst\n")
}

// TestNormalizeShellToolUpdateFinalRawOutputSurvivesTerminalOutputClobber is a
// regression for a review finding: when a single update carries both a final
// rawOutput and a terminal_output field, the terminal_output branch replaces
// Stdout with the provider's raw (unstripped) snapshot after
// applyFinalShellResult already stripped it. The end-of-update pass must
// re-commit the strip - not defer it - whenever rawOutput is present, so an
// update whose final content is nothing but the echoed command still
// collapses to empty instead of surfacing the raw echo.
func TestNormalizeShellToolUpdateFinalRawOutputSurvivesTerminalOutputClobber(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "cmd"},
	})

	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{"terminal_output": map[string]any{"data": "$ cmd"}},
		nil,
		"$ cmd",
	)

	require.Empty(t, payload.ShellExec().Output.Stdout)
}

// TestNormalizeShellToolUpdateFinalTerminalExitCommitsExactMatchEcho is a
// regression for a review finding: a command can also be finalized purely by
// a terminal_exit exit-code frame with no rawOutput at all (the ACP terminal
// extension's own exit reporting). An exit-code-only completion is just as
// final as a rawOutput-bearing one, so an exact-match echo must collapse to
// empty here too - not stay deferred forever with no further update ever
// arriving to re-trigger the strict path.
func TestNormalizeShellToolUpdateFinalTerminalExitCommitsExactMatchEcho(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "cmd"},
	})

	normalizer.NormalizeShellToolUpdate(
		payload,
		map[string]any{
			"terminal_output": map[string]any{"data": "$ cmd"},
			"terminal_exit":   map[string]any{"exit_code": 0},
		},
		nil,
		nil,
	)

	require.Empty(t, payload.ShellExec().Output.Stdout)
	require.NotNil(t, payload.ShellExec().Output.ExitCode)
	require.Equal(t, 0, *payload.ShellExec().Output.ExitCode)
}

// TestNormalizeShellToolUpdateCommitsEchoStripExactlyOnceForRawOutputOnlyResult
// is a regression for a review finding: applyFinalShellResult already
// commits the strict strip when it runs (rawOutput != nil), so the
// end-of-update pass must not strip a second time on top of an
// already-normalized value - a raw-only result whose real output legitimately
// repeats the echoed command text (e.g. "$ cmd\n$ cmd\n") would otherwise lose
// the second, legitimate occurrence to a redundant re-strip.
func TestNormalizeShellToolUpdateCommitsEchoStripExactlyOnceForRawOutputOnlyResult(t *testing.T) {
	t.Parallel()

	normalizer := NewNormalizer("")
	payload := normalizer.NormalizeToolCall("execute", map[string]any{
		"kind":      "execute",
		"raw_input": map[string]any{"command": "cmd"},
	})

	normalizer.NormalizeShellToolUpdate(payload, nil, nil, "$ cmd\n$ cmd\n")

	require.Equal(t, "$ cmd\n", payload.ShellExec().Output.Stdout)
}

func shellOutputJSON(t *testing.T, output any) map[string]any {
	t.Helper()
	require.NotNil(t, output)

	data, err := json.Marshal(output)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	return decoded
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}
