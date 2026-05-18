package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// uniqueIndexErrFragment matches the SQLite uniqueness violation surfaced
// when two concurrent EnsureSessionForAgent callers race past the SELECT and
// both INSERT a row for the same (task_id, agent_profile_id) pair.
const uniqueIndexErrFragment = "uniq_office_task_session"

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
		reused, decision := e.tryReuseExistingSession(ctx, existing)
		if decision == reuseDecisionTerminal {
			// Fall through to create a new row below.
		} else {
			return reused, nil
		}
	}

	created, err := e.createOfficeSession(ctx, task, agentInstanceID, agentProfileID, executorID, executorProfileID)
	if err != nil && strings.Contains(err.Error(), uniqueIndexErrFragment) {
		// Lost the race against a concurrent caller. Re-read and reuse.
		raced, lookupErr := e.repo.GetTaskSessionByTaskAndAgent(ctx, task.ID, agentInstanceID)
		if lookupErr == nil && raced != nil {
			reused, _ := e.tryReuseExistingSession(ctx, raced)
			if reused != nil {
				return reused, nil
			}
		}
	}
	return created, err
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
	// agentInstanceID and agentProfileID collapse to the same value after
	// ADR 0005 (the office agent IS the profile). Prefer the explicit
	// agent-id parameter when present; fall back to the profile id.
	sessionAgentProfileID := agentInstanceID
	if sessionAgentProfileID == "" {
		sessionAgentProfileID = agentProfileID
	}
	session := &models.TaskSession{
		ID:                   uuid.New().String(),
		TaskID:               task.ID,
		AgentProfileID:       sessionAgentProfileID,
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
