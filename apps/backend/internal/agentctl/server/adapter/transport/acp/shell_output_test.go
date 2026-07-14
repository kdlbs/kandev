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
