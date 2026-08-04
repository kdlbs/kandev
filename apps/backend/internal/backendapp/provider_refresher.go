package backendapp

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	mcpproviders "github.com/kandev/kandev/internal/mcp/providers"
	"github.com/kandev/kandev/internal/task/models"
)

type taskMCPProviderRefresherRepo interface {
	ListTaskRepositoryProviders(context.Context, string) ([]string, error)
	ListTaskSessions(context.Context, string) ([]*models.TaskSession, error)
}

type taskMCPProviderRefresherLifecycle interface {
	SetMcpProvidersForSession(context.Context, string, []string) error
}

type taskMCPProviderRefresher struct {
	repo      taskMCPProviderRefresherRepo
	lifecycle taskMCPProviderRefresherLifecycle
	logger    *logger.Logger
}

func newTaskMCPProviderRefresher(
	repo taskMCPProviderRefresherRepo,
	lifecycleMgr taskMCPProviderRefresherLifecycle,
	log *logger.Logger,
) *taskMCPProviderRefresher {
	return &taskMCPProviderRefresher{repo: repo, lifecycle: lifecycleMgr, logger: log}
}

// RefreshTaskMCPProviders derives the provider union from persisted task
// repository entities and replaces it on every active session's live
// agentctl. A session failure does not prevent other active sessions from
// being attempted; callers receive the aggregate error for logging while the
// committed source attachment remains authoritative.
func (r *taskMCPProviderRefresher) RefreshTaskMCPProviders(ctx context.Context, taskID string) error {
	if r == nil || r.repo == nil {
		return errors.New("task MCP provider refresher repository is unavailable")
	}
	providerValues, err := r.repo.ListTaskRepositoryProviders(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list task repository providers: %w", err)
	}
	providers := mcpproviders.Normalize(providerValues)

	sessions, err := r.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list task sessions: %w", err)
	}
	var refreshErr error
	for _, session := range sessions {
		if !sessionEligibleForMCPRefresh(session) {
			continue
		}
		if r.lifecycle == nil {
			refreshErr = errors.Join(refreshErr, errors.New("lifecycle manager is unavailable"))
			break
		}
		if err := r.lifecycle.SetMcpProvidersForSession(ctx, session.ID, providers); err != nil {
			if r.logger != nil {
				r.logger.Warn("live MCP provider refresh failed",
					zap.String("task_id", taskID),
					zap.String("session_id", session.ID),
					zap.Error(err))
			}
			refreshErr = errors.Join(refreshErr, fmt.Errorf("refresh session %s: %w", session.ID, err))
		}
	}
	return refreshErr
}

func sessionEligibleForMCPRefresh(session *models.TaskSession) bool {
	if session == nil {
		return false
	}
	switch session.State {
	case models.TaskSessionStateCreated,
		models.TaskSessionStateStarting,
		models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}

var _ interface {
	RefreshTaskMCPProviders(context.Context, string) error
} = (*taskMCPProviderRefresher)(nil)
