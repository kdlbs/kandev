package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// permissiveTestCapabilities keeps test executors working without the
// production lifecycle adapter while still satisfying the required runtime
// admission seam.
type permissiveTestCapabilities struct{}

func (permissiveTestCapabilities) RequiresCloneURL(string) bool          { return false }
func (permissiveTestCapabilities) ShouldApplyPreferredShell(string) bool { return false }
func (permissiveTestCapabilities) ValidateExecutorRuntime(string) error  { return nil }

func (permissiveTestCapabilities) ValidateExecutionRuntime(context.Context, string, string, string) error {
	return nil
}

func withTestCapabilities(e *executor.Executor) *executor.Executor {
	e.SetCapabilities(permissiveTestCapabilities{})
	return e
}
