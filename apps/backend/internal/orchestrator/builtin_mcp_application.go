package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type builtinApplicationRepository interface {
	SetBuiltinMCPApplicationPendingReason(context.Context, string, int64, string) error
	SupersedeBuiltinMCPApplication(context.Context, string, string, string, int64) (bool, error)
	ListBuiltinMCPApplications(context.Context) ([]*models.BuiltinMCPApplication, error)
	GetBuiltinMCPApplication(context.Context, string) (*models.BuiltinMCPApplication, error)
	ClaimBuiltinMCPApplication(context.Context, string, string, int64) (*models.BuiltinMCPApplication, error)
	AdvanceBuiltinMCPApplication(context.Context, *models.BuiltinMCPApplication, string, string) (bool, error)
	CommitBuiltinProviderBinding(context.Context, models.BuiltinMCPAcceptance) (bool, error)
}

func (s *Service) SetBuiltinApplicationAuthorizer(authorize mcpconfig.BuiltinPolicyAuthorizer) {
	s.builtinApplicationAuthorize = authorize
	if s.builtinApplicationWake == nil {
		s.builtinApplicationWake = make(chan struct{}, 1)
	}
}

// WakeBuiltinMCPApplications coalesces saves. The SQL intent remains authoritative.
func (s *Service) WakeBuiltinMCPApplications(_ string) {
	if s.builtinApplicationWake != nil {
		select {
		case s.builtinApplicationWake <- struct{}{}:
		default:
		}
	}
}

func (s *Service) startBuiltinMCPApplications(ctx context.Context) {
	if s.builtinApplicationAuthorize == nil || ctx == nil {
		return
	}
	s.builtinApplicationWorkers.Add(1)
	go func() {
		defer s.builtinApplicationWorkers.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			if err := s.ReconcileBuiltinMCPApplications(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("reconcile task MCP applications", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-s.builtinApplicationWake:
			}
		}
	}()
}

// ReconcileBuiltinMCPApplications does not start sessions or dispatch prompts.
func (s *Service) ReconcileBuiltinMCPApplications(ctx context.Context) error {
	repo, ok := s.repo.(builtinApplicationRepository)
	if !ok || s.builtinApplicationAuthorize == nil {
		return nil
	}
	runtime, ok := s.agentManager.(agentruntime.ProviderMCPReplacement)
	if !ok {
		return nil
	}
	rows, err := repo.ListBuiltinMCPApplications(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		operationCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
		err := s.reconcileBuiltinMCPApplication(operationCtx, repo, runtime, row)
		cancel()
		if err == nil {
			current, readErr := repo.GetBuiltinMCPApplication(ctx, row.SessionID)
			if readErr == nil && current != nil && current.Stage == models.BuiltinMCPApplicationApplied {
				s.tryFastPathDrainAfterEnqueue(ctx, row.TaskID, row.SessionID)
			}
		}
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("task MCP application deferred", zap.String("session_id", row.SessionID), zap.String("operation_id", row.OperationID), zap.Error(err))
		}
	}
	return nil
}

func (s *Service) reconcileBuiltinMCPApplication(ctx context.Context, repo builtinApplicationRepository, runtime agentruntime.ProviderMCPReplacement, row *models.BuiltinMCPApplication) error {
	release, ok := s.tryAcquireSessionLifecycleLock(row.SessionID)
	if !ok {
		return nil
	}
	defer release()
	return s.withBuiltinApplicationAdmission(ctx, row.SessionID, func(ctx context.Context) error {
		app, err := repo.GetBuiltinMCPApplication(ctx, row.SessionID)
		if err != nil || app == nil {
			return err
		}
		session, running, err := s.loadBuiltinApplicationSource(ctx, repo, app)
		if err != nil || session == nil || running == nil {
			return err
		}

		actorCtx := authn.WithIdentity(ctx, authn.Identity{UserID: app.RequestedBy, Role: authn.RoleMember})
		if err := s.builtinApplicationAuthorize(actorCtx, app.WorkspaceID, authz.ScopeTaskWrite); err != nil {
			return repo.SetBuiltinMCPApplicationPendingReason(ctx, app.SessionID, app.RowVersion, "access_changed")
		}
		return runtime.WithProviderMCPReplacement(ctx, app.SessionID, running.AgentExecutionID, func(control agentruntime.ProviderMCPControl) error {
			return s.applyBuiltinMCPApplication(actorCtx, repo, control, app, session, running)
		})
	})
}

func (s *Service) withBuiltinApplicationAdmission(ctx context.Context, sessionID string, fn func(context.Context) error) error {
	if s.messageQueue == nil {
		return fn(ctx)
	}
	return s.messageQueue.WithSessionAdmission(ctx, sessionID, fn)
}
func (s *Service) yieldBuiltinApplicationAdmission(ctx context.Context) {
	if s.messageQueue != nil {
		s.messageQueue.YieldSessionAdmission(ctx)
	}
}

func (s *Service) loadBuiltinApplicationSource(ctx context.Context, repo builtinApplicationRepository, app *models.BuiltinMCPApplication) (*models.TaskSession, *models.ExecutorRunning, error) {
	session, err := s.repo.GetTaskSession(ctx, app.SessionID)
	if err != nil {
		return nil, nil, err
	}
	if session == nil || session.State != models.TaskSessionStateWaitingForInput {
		return nil, nil, nil
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, app.SessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if running == nil || running.AgentExecutionID == "" || !running.Resumable || running.ResumeToken == "" {
		return nil, nil, nil
	}
	if app.Stage == models.BuiltinMCPApplicationPending {
		ready, err := s.builtinApplicationQuestionsSettled(ctx, repo, app)
		if err != nil || !ready {
			return nil, nil, err
		}
	}

	if app.Stage == models.BuiltinMCPApplicationPending && running.Status != models.ExecutorRunningStatusRunning {
		return nil, nil, nil
	}
	return session, running, nil
}

func (s *Service) builtinApplicationQuestionsSettled(ctx context.Context, repo builtinApplicationRepository, app *models.BuiltinMCPApplication) (bool, error) {
	reader, ok := s.repo.(interface {
		ListPendingInteractions(context.Context, models.PendingInteractionFilter) ([]*models.Message, error)
	})
	if !ok {
		return false, errors.New("pending input evidence is unavailable")
	}
	pending, err := reader.ListPendingInteractions(ctx, models.PendingInteractionFilter{SessionIDs: []string{app.SessionID}})
	if err != nil {
		return false, err
	}
	if len(pending) != 0 {
		return false, repo.SetBuiltinMCPApplicationPendingReason(ctx, app.SessionID, app.RowVersion, "waiting_for_safe_boundary")
	}
	return true, nil
}
