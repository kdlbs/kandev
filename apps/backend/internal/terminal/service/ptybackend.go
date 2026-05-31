package service

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/server/process"
)

// InteractiveRunnerBackend adapts *process.InteractiveRunner to the
// PTYBackend interface this service needs. Lives here (instead of in the
// agentctl package) because PTYBackend is owned by the consumer.
type InteractiveRunnerBackend struct {
	runner *process.InteractiveRunner
}

// NewInteractiveRunnerBackend wraps the supplied InteractiveRunner. The
// runner may be nil — in that case every method becomes a safe no-op /
// false. This matches the existing handler's defensive behaviour against a
// missing interactive runner (e.g. remote executor mode).
func NewInteractiveRunnerBackend(runner *process.InteractiveRunner) *InteractiveRunnerBackend {
	return &InteractiveRunnerBackend{runner: runner}
}

// Register pre-registers the user-shell entry on the runner so the next WS
// stream connect can lazily start the PTY against this terminalID. Uses
// RegisterScriptShell with empty initial command; the runner doesn't need
// to distinguish "plain shell" from "script with no command" at register
// time.
func (b *InteractiveRunnerBackend) Register(scopeID, terminalID string) {
	if b.runner == nil {
		return
	}
	b.runner.RegisterScriptShell(scopeID, terminalID, "", "")
}

// Stop tears down the PTY (if any) and removes the entry from the runner.
func (b *InteractiveRunnerBackend) Stop(ctx context.Context, scopeID, terminalID string) error {
	if b.runner == nil {
		return nil
	}
	return b.runner.StopUserShell(ctx, scopeID, terminalID)
}

// StopScope tears down every PTY registered under a task-environment scope.
func (b *InteractiveRunnerBackend) StopScope(ctx context.Context, scopeID string) (int, error) {
	if b.runner == nil {
		return 0, nil
	}
	return b.runner.StopUserShellsForScope(ctx, scopeID)
}

// IsAlive reports whether the PTY backing (scopeID, terminalID) is currently
// running.
func (b *InteractiveRunnerBackend) IsAlive(scopeID, terminalID string) bool {
	if b.runner == nil {
		return false
	}
	return b.runner.IsUserShellAlive(scopeID, terminalID)
}
