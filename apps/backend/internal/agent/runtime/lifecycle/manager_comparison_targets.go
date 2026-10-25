package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

type comparisonTargetSetter interface {
	SetComparisonTargets(ctx context.Context, targets map[string]models.ComparisonTarget) error
}

// ComparisonTargetProvider hydrates the durable comparison-target projection
// for one task. Keys use the same workspace subpath convention as base
// branches: the empty key addresses a single-repository/root tracker.
type ComparisonTargetProvider func(ctx context.Context, taskID string) (map[string]models.ComparisonTarget, error)

func (m *Manager) SetComparisonTargetProvider(fn ComparisonTargetProvider) {
	m.comparisonTargetProvider = fn
}

// pushTaskComparisonTargets refreshes one agentctl instance from durable task
// state. An empty successful map is intentionally sent: it clears a stale
// provider target after manual comparison selection or detach.
func (m *Manager) pushTaskComparisonTargets(ctx context.Context, taskID, executionID string, setter comparisonTargetSetter) {
	if taskID == "" || setter == nil || m.comparisonTargetProvider == nil {
		return
	}
	targets, err := m.comparisonTargetProvider(ctx, taskID)
	if err != nil {
		m.logger.Warn("failed to hydrate comparison targets for workspace",
			zap.String("task_id", taskID),
			zap.String("execution_id", executionID),
			zap.Error(err))
		return
	}
	if err := setter.SetComparisonTargets(ctx, targets); err != nil {
		m.logger.Warn("failed to seed comparison targets on agentctl",
			zap.String("task_id", taskID),
			zap.String("execution_id", executionID),
			zap.Error(err))
	}
}

// PushComparisonTargetsForTask implements the task-service live pusher. The
// durable change may be made from a cancelled HTTP request, so fan-out uses a
// bounded detached context and one execution failure never blocks siblings.
func (m *Manager) PushComparisonTargetsForTask(ctx context.Context, taskID string, targets map[string]models.ComparisonTarget) {
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
		pushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		err := client.SetComparisonTargets(pushCtx, targets)
		cancel()
		releaseClient()
		if err != nil {
			m.logger.Warn("failed to push comparison targets to agentctl",
				zap.String("task_id", taskID),
				zap.String("execution_id", exec.ID),
				zap.String("session_id", exec.SessionID),
				zap.Error(err))
		}
	}
}
