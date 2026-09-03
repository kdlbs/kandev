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
	session, _, err := e.EnsureSessionForAgentWithCreation(
		ctx, task, agentInstanceID, agentProfileID, executorID, executorProfileID,
	)
	return session, err
}

// EnsureSessionForAgentWithCreation is the same office-session lookup as
// EnsureSessionForAgent, but also reports whether the returned row was
// inserted by this call. Reused rows already have a CREATED lifecycle event;
// callers must not publish another one for every office turn.
func (e *Executor) EnsureSessionForAgentWithCreation(
	ctx context.Context, task *v1.Task, agentInstanceID, agentProfileID, executorID, executorProfileID string,
) (*models.TaskSession, bool, error) {
	if task == nil || task.ID == "" {
		return nil, false, errors.New("EnsureSessionForAgent: task is required")
	}
	if agentInstanceID == "" {
		return nil, false, errors.New("EnsureSessionForAgent: agent_profile_id is required")
	}
	if agentProfileID == "" {
		return nil, false, ErrNoAgentProfileID
	}
	if err := e.PreflightManagedGitCredentials(
		ctx, task.WorkspaceID, task.ID, executorID, executorProfileID,
	); err != nil {
		return nil, false, err
	}

	existing, err := e.repo.GetTaskSessionByTaskAndAgent(ctx, task.ID, agentInstanceID)
	if err != nil {
		return nil, false, fmt.Errorf("lookup (task,agent) session: %w", err)
	}
	if existing != nil {
		if err := e.rebindOfficeSessionExecutionProfile(ctx, existing, agentProfileID); err != nil {
			return nil, false, err
		}
		reused, decision := e.tryReuseExistingSession(ctx, existing)
		if decision == reuseDecisionTerminal {
			// Fall through to create a new row below.
		} else {
			return reused, false, nil
		}
	}

	return e.createOfficeSessionWithBoundedRecovery(ctx, task, agentInstanceID, agentProfileID, executorID, executorProfileID)
}

// maxOfficeSessionCreateAttempts bounds EnsureSessionForAgentWithCreation's
// create-then-recover loop (AC-003.3): one initial attempt plus one retry
// after losing a create race and finding no live winner to reuse yet.
// Unbounded retry would spin forever if the guard and the recovery lookup
// disagree in a way that never resolves.
const maxOfficeSessionCreateAttempts = 2

// createOfficeSessionWithBoundedRecovery attempts to create a fresh office
// session, and on losing the race to a concurrent creator (
// taskrepo.ErrOfficeSessionRaceConflict) re-reads the pair and reuses the
// winner instead of surfacing the conflict. A non-conflict create failure,
// or a failure while re-reading after a conflict, is returned as itself —
// never laundered into the sentinel and never as a nil session with a nil
// error.
func (e *Executor) createOfficeSessionWithBoundedRecovery(
	ctx context.Context, task *v1.Task, agentInstanceID, agentProfileID, executorID, executorProfileID string,
) (*models.TaskSession, bool, error) {
	var lastErr error
	for attempt := 0; attempt < maxOfficeSessionCreateAttempts; attempt++ {
		created, err := e.createOfficeSession(ctx, task, agentInstanceID, agentProfileID, executorID, executorProfileID)
		if err == nil {
			return created, true, nil
		}
		lastErr = err
		if !errors.Is(err, taskrepo.ErrOfficeSessionRaceConflict) {
			return nil, false, err
		}

		raced, lookupErr := e.repo.GetTaskSessionByTaskAndAgent(ctx, task.ID, agentInstanceID)
		if lookupErr != nil {
			return nil, false, fmt.Errorf("lookup (task,agent) session after create race: %w", lookupErr)
		}
		if raced == nil {
			continue
		}
		if rebindErr := e.rebindOfficeSessionExecutionProfile(ctx, raced, agentProfileID); rebindErr != nil {
			return nil, false, rebindErr
		}
		reused, decision := e.tryReuseExistingSession(ctx, raced)
		if decision != reuseDecisionTerminal {
			return reused, false, nil
		}
		// The winner turned out terminal by the time we read it; retry the
		// create rather than reusing a row that can't be reused.
	}
	return nil, false, lastErr
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
			models.SessionMetaKeyContextWindow,
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
			models.SessionMetaKeyContextWindow,
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
		return e.tryFlipIdleSessionToRunning(ctx, session)
	case models.TaskSessionStateCreated, models.TaskSessionStateStarting,
		models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput:
		return session, reuseDecisionReused
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return nil, reuseDecisionTerminal
	default:
		return session, reuseDecisionReused
	}
}

// tryFlipIdleSessionToRunning flips an IDLE row to RUNNING guarded by a
// CAS on the observed state, rather than an unconditional write. A blind
// write here could resurrect a row to RUNNING after a concurrent terminal
// transition (COMPLETED/FAILED/CANCELLED) landed between the read that
// classified this session as IDLE and this write — the same two-pool gap
// documented for creates, reachable via both the initial lookup and the
// bounded-recovery re-read. A CAS mismatch means exactly that race
// happened, so the fresh state — not the write's absence of an error — is
// what decides the outcome.
//
// The CAS itself uses the narrow, state-only UpdateTaskSessionStateIfCurrent
// rather than a full-row write: this session's in-memory copy was read
// before the flip, so a concurrent update to any other field (profile
// snapshot, routing, execution identifiers) between that read and this
// write must not be reverted. The row is always reloaded after the CAS,
// win or lose, so the caller observes the current stored values rather than
// a stale local copy.
func (e *Executor) tryFlipIdleSessionToRunning(
	ctx context.Context, session *models.TaskSession,
) (*models.TaskSession, reuseDecision) {
	_, _, err := e.repo.UpdateTaskSessionStateIfCurrent(
		ctx, session.ID, models.TaskSessionStateIdle, models.TaskSessionStateRunning, "",
	)
	if err != nil {
		e.logger.Warn("failed to flip office session IDLE→RUNNING; treating outcome as unknown",
			zap.String("session_id", session.ID), zap.Error(err))
		return nil, reuseDecisionTerminal
	}

	fresh, err := e.repo.GetTaskSession(ctx, session.ID)
	if err != nil || fresh == nil {
		e.logger.Warn("failed to reload office session after IDLE→RUNNING attempt; treating outcome as unknown",
			zap.String("session_id", session.ID), zap.Error(err))
		return nil, reuseDecisionTerminal
	}
	if isStopTerminalSessionState(fresh.State) {
		return nil, reuseDecisionTerminal
	}
	*session = *fresh
	return session, reuseDecisionReused
}

// createOfficeSession inserts a fresh task_sessions row for the given
// (task, agent) pair with state CREATED. Mirrors PrepareSession's repo lookups
// (primary repo, executor config, agent profile snapshot) but stores
// agent_profile_id so the row can participate in office-session uniqueness
// enforcement, which today is an in-transaction guard inside
// CreateOfficeTaskSession (not a database constraint) that returns
// taskrepo.ErrOfficeSessionRaceConflict on conflict — see that sentinel's
// doc comment.
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

	if err := e.persistOfficeSession(ctx, task.ID, session); err != nil {
		return nil, fmt.Errorf("persist office session: %w", err)
	}
	e.logger.Info("office session created",
		zap.String("task_id", task.ID),
		zap.String("session_id", session.ID),
		zap.String("agent_profile_id", agentInstanceID))
	return session, nil
}

func (e *Executor) persistOfficeSession(ctx context.Context, taskID string, session *models.TaskSession) error {
	if creator, ok := e.repo.(officeTaskSessionCreator); ok {
		return creator.CreateOfficeTaskSession(ctx, session)
	}
	creationLock := e.officeSessionLock(taskID)
	creationLock.Lock()
	defer creationLock.Unlock()
	return e.persistOfficeSessionFallback(ctx, taskID, session)
}

// persistOfficeSessionFallback is the in-process equivalent of
// CreateOfficeTaskSession for repositories that don't implement
// officeTaskSessionCreator. Callers hold e.officeSessionLock(taskID) for the
// duration, which is this fallback's only mutual-exclusion primitive — so
// the live-pair guard below must run inside that same critical section,
// restricted to the (task, agent) pair before classifying terminality,
// exactly like the repository-native guard.
func (e *Executor) persistOfficeSessionFallback(ctx context.Context, taskID string, session *models.TaskSession) error {
	existingSessions, err := e.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list task sessions before creating office session: %w", err)
	}
	if session.AgentProfileID != "" {
		for _, existing := range existingSessions {
			if existing.AgentProfileID != session.AgentProfileID {
				continue
			}
			if !isStopTerminalSessionState(existing.State) {
				return taskrepo.ErrOfficeSessionRaceConflict
			}
		}
	}
	if len(existingSessions) == 0 {
		if session.Metadata == nil {
			session.Metadata = make(map[string]interface{})
		}
		session.Metadata[models.SessionMetaKeyOrigin] = models.SessionOriginTaskInitial
	}
	return e.repo.CreateTaskSession(ctx, session)
}
