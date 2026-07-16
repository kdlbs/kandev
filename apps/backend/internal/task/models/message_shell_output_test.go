package models

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestMessageToAPIProjectsTypedShellOutputWithoutMutation(t *testing.T) {
	exitCode := 7
	normalized := streams.NewShellExec("go test ./...", "/workspace", "", 0, false)
	normalized.ShellExec().Output = &streams.ShellExecOutput{
		ExitCode:  &exitCode,
		Stdout:    "pass ✓\n",
		Stderr:    "warning\n",
		Truncated: true,
	}
	message := &Message{Metadata: map[string]any{
		"status":     "completed",
		"normalized": normalized,
	}}

	projected := jsonMetadata(t, message.ToAPI().Metadata)
	output := projected["normalized"].(map[string]any)["shell_exec"].(map[string]any)["output"].(map[string]any)

	require.NotContains(t, output, "stdout")
	require.NotContains(t, output, "stderr")
	require.Equal(t, true, output["has_output"])
	require.Equal(t, float64(len("pass ✓\n")), output["stdout_bytes"])
	require.Equal(t, float64(len("warning\n")), output["stderr_bytes"])
	require.Equal(t, float64(exitCode), output["exit_code"])
	require.Equal(t, true, output["truncated"])
	require.Equal(t, "pass ✓\n", normalized.ShellExec().Output.Stdout)
	require.Equal(t, "warning\n", normalized.ShellExec().Output.Stderr)
}

func TestMessageToAPIProjectsPersistedShellOutputWithoutMutation(t *testing.T) {
	output := map[string]any{
		"stdout":    "persisted stdout",
		"stderr":    "",
		"truncated": false,
	}
	message := &Message{Metadata: map[string]any{
		"status": "running",
		"normalized": map[string]any{
			"kind": "shell_exec",
			"shell_exec": map[string]any{
				"command": "make test",
				"output":  output,
			},
		},
	}}

	projected := jsonMetadata(t, message.ToAPI().Metadata)
	projectedOutput := projected["normalized"].(map[string]any)["shell_exec"].(map[string]any)["output"].(map[string]any)

	require.NotContains(t, projectedOutput, "stdout")
	require.NotContains(t, projectedOutput, "stderr")
	require.Equal(t, true, projectedOutput["has_output"])
	require.Equal(t, float64(len("persisted stdout")), projectedOutput["stdout_bytes"])
	require.Equal(t, float64(0), projectedOutput["stderr_bytes"])
	require.Equal(t, false, projectedOutput["truncated"])
	require.Equal(t, "persisted stdout", output["stdout"])
	originalShell := message.Metadata["normalized"].(map[string]any)["shell_exec"].(map[string]any)
	require.Equal(t, output, originalShell["output"])
}

func TestProjectMessageMetadataMatchesTypedAndPersistedShellOutput(t *testing.T) {
	exitCode := 3
	typed := streams.NewShellExec("printf ✓", "/workspace", "print", 30, true)
	typed.ShellExec().Output = &streams.ShellExecOutput{ExitCode: &exitCode, Stdout: "✓", Truncated: true}
	persisted := map[string]any{
		"kind": "shell_exec",
		"shell_exec": map[string]any{
			"command": "printf ✓", "work_dir": "/workspace", "description": "print",
			"timeout": float64(30), "background": true,
			"output": map[string]any{"exit_code": float64(3), "stdout": "✓", "truncated": true},
		},
	}

	typedJSON, err := json.Marshal(ProjectMessageMetadata(map[string]any{"normalized": typed})["normalized"])
	require.NoError(t, err)
	persistedJSON, err := json.Marshal(ProjectMessageMetadata(map[string]any{"normalized": persisted})["normalized"])
	require.NoError(t, err)
	require.JSONEq(t, string(typedJSON), string(persistedJSON))
}

func jsonMetadata(t *testing.T, metadata map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return decoded
}
