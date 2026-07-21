package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

type officeSessionMetadataUpdater interface {
	UpdateTaskSessionIfCurrentStateRemovingMetadataKeys(
		ctx context.Context,
		session *models.TaskSession,
		expected models.TaskSessionState,
		keys []string,
	) (bool, error)
}

// EnsureSessionForAgent returns a persistent task session for the (task,
// agent) pair, creating one when no row exists. This is the office run
// entry point — every run for a participant agent ends up here. Distinct
// from PrepareSession's per-launch model: an office session is keyed on
// (task_id, agent_profile_id) and reused across turns. The state is flipped
// back to RUNNING when an IDLE row is reused; terminal rows are left in place
// and a fresh row is created instead.
//
// Caller hands the returned session to LaunchPreparedSession to bring up the
// executor and run the ACP handshake, exactly like the kanban path.
func (e *Executor) EnsureSessionForAgent(
	ctx context.Context, task *v1.Task, agentInstanceID, agentProfileID, executorID, executorProfileID string,
) (*models.TaskSession, error) {
	if task == nil || task.ID == "" {
		return nil, errors.New("EnsureSessionForAgent: task is required")
	}
	if agentInstanceID == "" {
		return nil, errors.New("EnsureSessionForAgent: agent_profile_id is required")
	}
	if agentProfileID == "" {
		return nil, ErrNoAgentProfileID
	}

	existing, err := e.repo.GetTaskSessionByTaskAndAgent(ctx, task.ID, agentInstanceID)
	if err != nil {
		return nil, fmt.Errorf("lookup (task,agent) session: %w", err)
	}
	if existing != nil {
		if err := e.rebindOfficeSessionExecutionProfile(ctx, existing, agentProfileID); err != nil {
			return nil, err
		}
		reused, decision := e.tryReuseExistingSession(ctx, existing)
		if decision == reuseDecisionTerminal {
			// Fall through to create a new row below.
		} else {
			return reused, nil
		}
	}

	created, err := e.createOfficeSession(ctx, task, agentInstanceID, agentProfileID, executorID, executorProfileID)
	if err != nil && errors.Is(err, taskrepo.ErrOfficeSessionRaceConflict) {
		// Lost the race against a concurrent caller. Re-read and reuse.
		raced, lookupErr := e.repo.GetTaskSessionByTaskAndAgent(ctx, task.ID, agentInstanceID)
		if lookupErr == nil && raced != nil {
			if rebindErr := e.rebindOfficeSessionExecutionProfile(ctx, raced, agentProfileID); rebindErr != nil {
				return nil, rebindErr
			}
			reused, _ := e.tryReuseExistingSession(ctx, raced)
			if reused != nil {
				return reused, nil
			}
		}
	}
	return created, err
}

func (e *Executor) rebindOfficeSessionExecutionProfile(
	ctx context.Context, session *models.TaskSession, executionProfileID string,
) error {
	if session == nil || executionProfileID == "" || session.ExecutionProfileID == executionProfileID {
		return nil
	}
	snapshot, isPassthrough := e.resolveAgentProfileSnapshot(ctx, executionProfileID)
	updater, ok := e.repo.(officeSessionMetadataUpdater)
	if !ok {
		return errors.New("office session rebind requires guarded metadata updates")
	}
	for {
		if isStopTerminalSessionState(session.State) {
			return nil
		}

		expectedState := session.State
		updated := *session
		updated.ExecutionProfileID = executionProfileID
		updated.AgentProfileSnapshot = snapshot
		updated.IsPassthrough = isPassthrough
		// Provider-native state must not override the newly selected profile.
		updated.Metadata = cloneMetadata(session.Metadata)
		for _, key := range []string{
			"acp_session_id",
			models.SessionMetaKeySessionMode,
			models.SessionMetaKeyRuntimeConfig,
			models.SessionMetaKeyRuntimeConfigOverrides,
			models.SessionMetaKeyACPConfigBaseline,
			models.SessionMetaKeyACPModelState,
			"context_window",
			models.SessionMetaKeyLastAgentError,
		} {
			delete(updated.Metadata, key)
		}
		changed, err := updater.UpdateTaskSessionIfCurrentStateRemovingMetadataKeys(ctx, &updated, expectedState, []string{
			"acp_session_id",
			models.SessionMetaKeySessionMode,
			models.SessionMetaKeyRuntimeConfig,
			models.SessionMetaKeyRuntimeConfigOverrides,
			models.SessionMetaKeyACPConfigBaseline,
			models.SessionMetaKeyACPModelState,
			"context_window",
			models.SessionMetaKeyLastAgentError,
		})
		if err != nil {
			return fmt.Errorf("update office execution profile: %w", err)
		}
		if changed {
			*session = updated
			return nil
		}

		fresh, err := e.repo.GetTaskSession(ctx, session.ID)
		if err != nil {
			return fmt.Errorf("reload office session after execution profile race: %w", err)
		}
		if fresh == nil {
			return fmt.Errorf("reload office session after execution profile race: %w", models.ErrTaskSessionNotFound)
		}
		*session = *fresh
		if session.ExecutionProfileID == executionProfileID || isStopTerminalSessionState(session.State) {
			return nil
		}
	}
}

// reuseDecision describes what tryReuseExistingSession did with an existing
// row. terminal => caller must create a fresh session; reused => the row was
// kept (state may have been flipped from IDLE → RUNNING).
type reuseDecision int

const (
	reuseDecisionReused reuseDecision = iota
	reuseDecisionTerminal
)

// tryReuseExistingSession applies the spec's reuse rules to an existing
// (task, agent) row. IDLE flips back to RUNNING; non-terminal active states
// pass through unchanged; terminal rows tell the caller to create a fresh row.
func (e *Executor) tryReuseExistingSession(
	ctx context.Context, session *models.TaskSession,
) (*models.TaskSession, reuseDecision) {
	switch session.State {
	case models.TaskSessionStateIdle:
		if err := e.repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateRunning, ""); err != nil {
			e.logger.Warn("failed to flip office session IDLE→RUNNING; returning row anyway",
				zap.String("session_id", session.ID), zap.Error(err))
		} else {
			session.State = models.TaskSessionStateRunning
		}
		return session, reuseDecisionReused
	case models.TaskSessionStateCreated, models.TaskSessionStateStarting,
		models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput:
		return session, reuseDecisionReused
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return nil, reuseDecisionTerminal
	default:
		return session, reuseDecisionReused
	}
}

// createOfficeSession inserts a fresh task_sessions row for the given
// (task, agent) pair with state CREATED. Mirrors PrepareSession's repo lookups
// (primary repo, executor config, agent profile snapshot) but stores
// agent_profile_id so the row participates in the office-session unique index.
//
// is_primary is left false: office sessions don't use the primary mechanism;
// it stays for kanban / quick-chat advanced-mode resume.
func (e *Executor) createOfficeSession(
	ctx context.Context, task *v1.Task, agentInstanceID, agentProfileID, executorID, executorProfileID string,
) (*models.TaskSession, error) {
	metadata := cloneMetadata(task.Metadata)

	primaryTaskRepo, err := e.repo.GetPrimaryTaskRepository(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("get primary task repo: %w", err)
	}
	var repositoryID, baseBranch string
	if primaryTaskRepo != nil {
		repositoryID = primaryTaskRepo.RepositoryID
		baseBranch = primaryTaskRepo.BaseBranch
	}

	agentProfileSnapshot, isPassthrough := e.resolveAgentProfileSnapshot(ctx, agentProfileID)

	now := time.Now().UTC()
	// Office sessions are owned by the stable agent identity while their
	// concrete execution profile may change between runs.
	sessionAgentProfileID := agentInstanceID
	if sessionAgentProfileID == "" {
		sessionAgentProfileID = agentProfileID
	}
	session := &models.TaskSession{
		ID:                   uuid.New().String(),
		TaskID:               task.ID,
		AgentProfileID:       sessionAgentProfileID,
		ExecutionProfileID:   agentProfileID,
		RepositoryID:         repositoryID,
		BaseBranch:           baseBranch,
		State:                models.TaskSessionStateCreated,
		StartedAt:            now,
		UpdatedAt:            now,
		AgentProfileSnapshot: agentProfileSnapshot,
		IsPassthrough:        isPassthrough,
		Metadata:             metadata,
	}
	if executorProfileID != "" {
		session.ExecutorProfileID = executorProfileID
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["executor_profile_id"] = executorProfileID
	}

	execConfig := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if execConfig.ExecutorID != "" {
		session.ExecutorID = execConfig.ExecutorID
	}

	if err := e.repo.CreateTaskSession(ctx, session); err != nil {
		return nil, fmt.Errorf("persist office session: %w", err)
	}
	e.logger.Info("office session created",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("agent_profile_id", agentInstanceID))
	return session, nil
}
