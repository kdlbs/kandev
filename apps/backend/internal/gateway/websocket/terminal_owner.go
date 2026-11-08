package websocket

import (
	"context"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	terminalrepository "github.com/kandev/kandev/internal/terminal/repository"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
)

// resolveEnvironmentTerminalExecution picks the execution that serves an
// environment user shell. A managed terminal runs in its persisted owner
// task's execution, never in whichever sibling task happens to serve the
// environment. Owner lookup uses OwnerForTerminal, the same read the agent
// terminal route uses.
func (h *TerminalHandler) resolveEnvironmentTerminalExecution(ctx context.Context, environmentID, terminalID string) (*agentruntime.AgentExecution, error) {
	if h.terminalSvc == nil || !terminalservice.IsManaged(terminalID) {
		return h.lifecycleMgr.GetOrEnsureExecutionForEnvironment(ctx, environmentID)
	}
	if err := h.lifecycleMgr.CheckEnvironmentExecAccess(ctx, environmentID); err != nil {
		return nil, err
	}
	taskID, err := h.terminalSvc.OwnerForTerminal(ctx, environmentID, terminalID)
	if errors.Is(err, terminalrepository.ErrNotFound) {
		return nil, terminalservice.ErrTaskMismatch
	}
	if err != nil {
		return nil, err
	}
	if err := h.lifecycleMgr.CheckTaskEnvironmentAccess(ctx, taskID, environmentID); err != nil {
		// The environment was already visible to this caller above, so a
		// removal after the owner lookup keeps its missing-target close.
		if errors.Is(err, repoerrors.ErrTaskEnvironmentNotFound) {
			return nil, err
		}
		return nil, terminalservice.ErrTaskMismatch
	}
	execution, err := h.lifecycleMgr.GetOrEnsureExecutionForTaskEnvironment(ctx, taskID, environmentID)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return nil, terminalservice.ErrTaskMismatch
	}
	return execution, err
}
