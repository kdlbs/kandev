package lifecycle

import (
	"context"

	"go.uber.org/zap"
)

// PushBaseBranchesForTask sends an updated per-repo base-branch map to every
// running execution of taskID. Implements
// service.AgentBaseBranchPusher — called from
// service.UpdateRepositoryBaseBranch after the DB write so the
// changes-panel diff stats refresh without a session restart.
//
// Best-effort: per-execution failures are logged at warn but do not abort
// the loop, and there is no return value. The persisted
// task_repositories.base_branch is the source of truth; the next session
// launch rebuilds trackers from it.
func (m *Manager) PushBaseBranchesForTask(ctx context.Context, taskID string, branches map[string]string) {
	if taskID == "" {
		return
	}
	for _, exec := range m.executionStore.List() {
		if exec.TaskID != taskID {
			continue
		}
		client := exec.GetAgentCtlClient()
		if client == nil {
			continue
		}
		if err := client.SetBaseBranches(ctx, branches); err != nil {
			m.logger.Warn("failed to push base branches to agentctl",
				zap.String("task_id", taskID),
				zap.String("execution_id", exec.ID),
				zap.String("session_id", exec.SessionID),
				zap.Error(err))
		}
	}
}
