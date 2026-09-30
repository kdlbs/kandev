package lifecycle

import (
	"context"
	"maps"
	"time"

	"go.uber.org/zap"
)

// baseBranchSetter is the agentctl surface used to replace a workspace's
// per-repo base-branch map. *agentctl.Client satisfies it; tests supply a fake.
type baseBranchSetter interface {
	SetBaseBranches(ctx context.Context, branches map[string]string) error
}

// BaseBranchProvider hydrates the stored per-repo {RepositoryName → base_branch}
// map for a task. Wired to the task service, which reads task_repositories.
type BaseBranchProvider func(ctx context.Context, taskID string) (map[string]string, error)

// SetBaseBranchProvider wires the DB-backed hydrator used before instance
// creation and before publishing agentctl readiness.
func (m *Manager) SetBaseBranchProvider(fn BaseBranchProvider) {
	m.baseBranchProvider = fn
}

// seedExecutionBaseBranches gives the create request its own base-branch map
// before agentctl starts trackers and computes their initial Git state.
func (m *Manager) seedExecutionBaseBranches(ctx context.Context, taskID, executionID string, metadata map[string]interface{}) {
	branches := getMetadataStringMap(metadata, MetadataKeyBaseBranches)
	if len(branches) == 0 && taskID != "" && m.baseBranchProvider != nil {
		hydrationCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var err error
		branches, err = m.baseBranchProvider(hydrationCtx, taskID)
		if err != nil {
			m.logger.Warn("failed to hydrate base branches before workspace creation",
				zap.String("task_id", taskID),
				zap.String("execution_id", executionID),
				zap.Error(err))
			return
		}
	}
	if len(branches) > 0 {
		metadata[MetadataKeyBaseBranches] = maps.Clone(branches)
	}
}

// pushTaskBaseBranches refreshes taskID's stored base-branch map at readiness.
// It applies changes since creation-time seeding and hydrates already-running
// workspaces recovered by Manager.Start without a new create request.
//
// Best-effort throughout: a missing provider, a hydration failure, or a push
// failure is logged and never blocks the workspace. The persisted
// task_repositories.base_branch remains the source of truth.
func (m *Manager) pushTaskBaseBranches(ctx context.Context, taskID, executionID string, setter baseBranchSetter) {
	if taskID == "" || setter == nil || m.baseBranchProvider == nil {
		return
	}
	branches, err := m.baseBranchProvider(ctx, taskID)
	if err != nil {
		m.logger.Warn("failed to hydrate base branches for workspace",
			zap.String("task_id", taskID),
			zap.String("execution_id", executionID),
			zap.Error(err))
		return
	}
	// SetBaseBranches *replaces* the stored map, so an empty hydration must not
	// be pushed — on the full-launch path it would wipe the map LaunchRequest
	// metadata already seeded.
	if len(branches) == 0 {
		return
	}
	if err := setter.SetBaseBranches(ctx, branches); err != nil {
		m.logger.Warn("failed to seed base branches on agentctl",
			zap.String("task_id", taskID),
			zap.String("execution_id", executionID),
			zap.Error(err))
	}
}

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
		client, releaseClient := exec.AcquireAgentCtlClient()
		if client == nil {
			continue
		}
		err := client.SetBaseBranches(ctx, branches)
		releaseClient()
		if err != nil {
			m.logger.Warn("failed to push base branches to agentctl",
				zap.String("task_id", taskID),
				zap.String("execution_id", exec.ID),
				zap.String("session_id", exec.SessionID),
				zap.Error(err))
		}
	}
}
