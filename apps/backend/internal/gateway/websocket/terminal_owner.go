package websocket

import (
	"context"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
)

func (h *TerminalHandler) resolveEnvironmentTerminalExecution(ctx context.Context, environmentID, terminalID string) (*agentruntime.AgentExecution, error) {
	if h.terminalSvc == nil || !terminalservice.IsManaged(terminalID) {
		return h.lifecycleMgr.GetOrEnsureExecutionForEnvironment(ctx, environmentID)
	}
	if err := h.lifecycleMgr.CheckEnvironmentExecAccess(ctx, environmentID); err != nil {
		return nil, err
	}
	taskID, err := h.terminalSvc.ShellOwnerTaskID(ctx, environmentID, terminalID)
	if err != nil {
		return nil, err
	}
	execution, err := h.lifecycleMgr.GetOrEnsureExecutionForTaskEnvironment(ctx, taskID, environmentID)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return nil, terminalservice.ErrTaskMismatch
	}
	return execution, err
}
