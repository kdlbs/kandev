package models

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/entityrefs"
)

// ShellExecOutputSnapshot is the full bounded output stored on a shell message.
type ShellExecOutputSnapshot struct {
	ExitCode  *int   `json:"exit_code,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// ProjectMessageMetadata removes shell output bodies from client message payloads.
// It copies the normalized shell path so repository-owned metadata is never mutated.
func ProjectMessageMetadata(metadata map[string]any) map[string]any {
	projected := metadata
	copied := false
	if rawReferences, exists := metadata["entity_references"]; exists {
		projected = copyMetadata(metadata)
		copied = true
		references := entityrefs.NormalizePersisted(rawReferences)
		if len(references) == 0 {
			delete(projected, "entity_references")
		} else {
			projected["entity_references"] = references
		}
	}
	output, ok := shellOutputFromMetadata(metadata)
	if !ok {
		return projected
	}
	normalized, ok := projectNormalizedShell(metadata["normalized"], shellOutputSummary(output))
	if !ok {
		return projected
	}
	if !copied {
		projected = copyMetadata(metadata)
	}
	projected["normalized"] = normalized
	return projected
}

// ExtractShellExecOutput returns the full stored snapshot for normalized shell metadata.
func ExtractShellExecOutput(metadata map[string]any) (ShellExecOutputSnapshot, bool) {
	return shellOutputFromMetadata(metadata)
}

func shellOutputFromMetadata(metadata map[string]any) (ShellExecOutputSnapshot, bool) {
	if metadata == nil {
		return ShellExecOutputSnapshot{}, false
	}
	raw, ok := metadata["normalized"]
	if !ok || raw == nil {
		return ShellExecOutputSnapshot{}, false
	}
	switch normalized := raw.(type) {
	case *streams.NormalizedPayload:
		if normalized == nil || normalized.ShellExec() == nil {
			return ShellExecOutputSnapshot{}, false
		}
		return shellOutputFromTyped(normalized.ShellExec().Output), true
	case map[string]any:
		shell, shellOK := normalized["shell_exec"].(map[string]any)
		if !shellOK {
			return ShellExecOutputSnapshot{}, false
		}
		return shellOutputFromMap(shell["output"]), true
	default:
		return ShellExecOutputSnapshot{}, false
	}
}

func shellOutputFromTyped(output *streams.ShellExecOutput) ShellExecOutputSnapshot {
	if output == nil {
		return ShellExecOutputSnapshot{}
	}
	return ShellExecOutputSnapshot{
		ExitCode:  output.ExitCode,
		Stdout:    output.Stdout,
		Stderr:    output.Stderr,
		Truncated: output.Truncated,
	}
}

func shellOutputFromMap(raw any) ShellExecOutputSnapshot {
	output, ok := raw.(map[string]any)
	if !ok {
		return ShellExecOutputSnapshot{}
	}
	result := ShellExecOutputSnapshot{
		Stdout:    StringFromAny(output["stdout"]),
		Stderr:    StringFromAny(output["stderr"]),
		Truncated: boolFromAny(output["truncated"]),
	}
	if exitCode, ok := intFromAny(output["exit_code"]); ok {
		result.ExitCode = &exitCode
	}
	return result
}

func shellOutputSummary(output ShellExecOutputSnapshot) map[string]any {
	summary := map[string]any{
		"has_output":   output.Stdout != "" || output.Stderr != "",
		"stdout_bytes": len(output.Stdout),
		"stderr_bytes": len(output.Stderr),
		"truncated":    output.Truncated,
	}
	if output.ExitCode != nil {
		summary["exit_code"] = *output.ExitCode
	}
	return summary
}

func projectNormalizedShell(raw any, summary map[string]any) (map[string]any, bool) {
	switch normalized := raw.(type) {
	case *streams.NormalizedPayload:
		if normalized == nil || normalized.ShellExec() == nil {
			return nil, false
		}
		return map[string]any{
			"kind":       string(streams.ToolKindShellExec),
			"shell_exec": projectTypedShell(normalized.ShellExec(), summary),
		}, true
	case map[string]any:
		shell, ok := normalized["shell_exec"].(map[string]any)
		if !ok {
			return nil, false
		}
		projectedShell := copyMetadata(shell)
		projectedShell["output"] = summary
		projected := copyMetadata(normalized)
		projected["shell_exec"] = projectedShell
		return projected, true
	default:
		return nil, false
	}
}

func projectTypedShell(shell *streams.ShellExecPayload, summary map[string]any) map[string]any {
	projected := map[string]any{
		"command": shell.Command,
		"output":  summary,
	}
	if shell.WorkDir != "" {
		projected["work_dir"] = shell.WorkDir
	}
	if shell.Description != "" {
		projected["description"] = shell.Description
	}
	if shell.Timeout != 0 {
		projected["timeout"] = shell.Timeout
	}
	if shell.Background {
		projected["background"] = true
	}
	return projected
}

func intFromAny(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case float64:
		return int(value), value == float64(int(value))
	default:
		return 0, false
	}
}

func boolFromAny(raw any) bool {
	value, _ := raw.(bool)
	return value
}
