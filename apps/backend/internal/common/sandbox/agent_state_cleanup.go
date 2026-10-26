package sandbox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// RemoveCapturedAgentState removes only the allocation bound to a captured
// scope. The lifecycle owner must first prove all attempts using it have stopped.
func RemoveCapturedAgentState(base string, snapshot *AgentStateSnapshot) error {
	if snapshot == nil || !validRuntimeDirectory(base) || !validAgentStateScope(snapshot.Scope) {
		return ErrInvalidPolicy
	}
	raw, err := json.Marshal(snapshot.Scope)
	if err != nil {
		return ErrInvalidPolicy
	}
	root := filepath.Join(base, "agent-state-"+runtimeDigest(raw))
	if snapshot.Home != filepath.Join(root, "home") {
		return ErrInvalidPolicy
	}
	before, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !validRuntimeDirectory(root) {
		return ErrInvalidPolicy
	}
	state, err := OpenAgentState(base, snapshot.Scope)
	if err != nil {
		return err
	}
	defer func() { _ = state.Close() }()
	if !os.SameFile(before, state.identity) {
		return ErrInvalidPolicy
	}
	return state.Remove()
}
