// Package engine_dispatcher provides the production implementation of
// office/shared.WorkflowEngineDispatcher. It bridges the office service's
// typed event subscribers to the workflow engine's HandleInput envelope
// by resolving the task's active session id and invoking
// engine.HandleTrigger.
//
// Constructed in cmd/kandev/main.go and passed to office service via
// SetWorkflowEngineDispatcher. The engine path is unconditional.
package engine_dispatcher

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/shared"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	"go.uber.org/zap"
)

// ErrNoSession is re-exported from office/shared so callers can compare
// via errors.Is without importing this package directly. Returned when a
// trigger arrives for a task with no active session — engine state is
// keyed on (taskID, sessionID), so the dispatcher cannot proceed without
// one.
var ErrNoSession = shared.ErrEngineNoSession

// SessionResolver looks up a task's session state for workflow triggers.
type SessionResolver interface {
	GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
	GetTaskSessionByTaskID(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
}

// EngineHandle is the engine surface the dispatcher needs. Defined as a
// minimal interface so tests can pass a fake.
type EngineHandle interface {
	HandleTrigger(ctx context.Context, in engine.HandleInput) (engine.HandleResult, error)
	// RecordParticipantDecision and EvaluateStepQuorum are the AC-57a/57d
	// engine decision entry points. Widening this dispatcher-local
	// interface (rather than shared.WorkflowEngineDispatcher) is the
	// documented AC-57d implementation note: callers reach the new
	// capability through Dispatcher.RecordDecision /
	// Dispatcher.EvaluateStepQuorum plus a type assertion against a
	// narrow caller-side interface, mirroring handledWorkflowEngineDispatcher.
	RecordParticipantDecision(ctx context.Context, sessionID string, in engine.DecisionInfo) (engine.RecordDecisionResult, error)
	EvaluateStepQuorum(ctx context.Context, taskID, sessionID string) (engine.QuorumSnapshot, error)
}

// RecordDecisionInput is what a transport must resolve before calling
// Dispatcher.RecordDecision: the task/step/participant identity and the
// verdict itself. Session resolution (AC-16/16a) happens inside
// RecordDecision, not here.
type RecordDecisionInput struct {
	TaskID        string
	StepID        string
	ParticipantID string
	Decision      string
	DeciderType   string
	DeciderID     string
	Role          string
	Comment       string
}

// RecordDecisionResult mirrors engine.RecordDecisionResult plus the
// validated StepID (AC-37) the decision was recorded against, which the
// AC-64 tool contract and AC-57b-i both need.
type RecordDecisionResult struct {
	StepID              string
	DecisionID          string
	DecidedAt           time.Time
	Transitioned        bool
	TransitionAbandoned bool
	FromStepID          string
	ToStepID            string
}

// Dispatcher resolves a task's active session and invokes the workflow
// engine. It implements shared.WorkflowEngineDispatcher.
type Dispatcher struct {
	engine   EngineHandle
	sessions SessionResolver
	logger   *logger.Logger
}

// New builds a Dispatcher. Both engine and sessions must be non-nil; the
// office service guards against accidentally wiring a nil dispatcher,
// but explicit construction here keeps the contract clear.
func New(eng EngineHandle, sessions SessionResolver, log *logger.Logger) *Dispatcher {
	return &Dispatcher{
		engine:   eng,
		sessions: sessions,
		logger:   log.WithFields(zap.String("component", "engine-dispatcher")),
	}
}

// HandleTrigger satisfies shared.WorkflowEngineDispatcher.
//
// Resolves the task's active session — or, for comment wakes, the latest
// reusable completed/idle session — then invokes engine.HandleTrigger. Errors from the
// engine (e.g. queue_run resolver failures) bubble up so the office event
// subscriber can log them.
func (d *Dispatcher) HandleTrigger(
	ctx context.Context,
	taskID string,
	trigger engine.Trigger,
	payload any,
	operationID string,
) error {
	_, err := d.HandleTriggerHandled(ctx, taskID, trigger, payload, operationID)
	return err
}

// HandleTriggerHandled reports whether the workflow engine found actions for
// the trigger. A no-action step is a successful no-op, but callers such as the
// dashboard still need to keep their legacy fallback wake path.
func (d *Dispatcher) HandleTriggerHandled(
	ctx context.Context,
	taskID string,
	trigger engine.Trigger,
	payload any,
	operationID string,
) (bool, error) {
	if taskID == "" {
		return false, fmt.Errorf("task_id is required")
	}
	session, err := d.resolveSession(ctx, taskID, trigger)
	if err != nil {
		return false, fmt.Errorf("resolve session: %w", err)
	}
	if session == nil {
		d.logger.Debug("engine trigger skipped: no active session",
			zap.String("task_id", taskID),
			zap.String("trigger", string(trigger)))
		return false, ErrNoSession
	}
	in := engine.HandleInput{
		TaskID:      taskID,
		SessionID:   session.ID,
		Trigger:     trigger,
		OperationID: operationID,
		Payload:     payload,
	}
	result, err := d.engine.HandleTrigger(ctx, in)
	if err != nil {
		return false, fmt.Errorf("engine handle %s: %w", trigger, err)
	}
	return result.Idempotent || result.ActionCount > 0, nil
}

func (d *Dispatcher) resolveSession(
	ctx context.Context, taskID string, trigger engine.Trigger,
) (*taskmodels.TaskSession, error) {
	session, err := d.sessions.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err == nil && session != nil {
		return session, nil
	}
	if err != nil && !errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return nil, fmt.Errorf("active session lookup: %w", err)
	}
	if trigger != engine.TriggerOnComment {
		return nil, nil
	}
	// Comment wakes are allowed after an office task's agent session has
	// completed or returned to reusable IDLE state. The workflow engine state is
	// keyed by (taskID, sessionID), so a post-completion comment intentionally
	// resumes the latest reusable session's persisted machine state instead of
	// starting a fresh state machine here.
	session, err = d.sessions.GetTaskSessionByTaskID(ctx, taskID)
	if err == nil && session != nil {
		if !isReusableCommentSession(session.State) {
			return nil, nil
		}
		return session, nil
	}
	if err != nil && !errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return nil, fmt.Errorf("latest session lookup: %w", err)
	}
	return nil, nil
}

func isReusableCommentSession(state taskmodels.TaskSessionState) bool {
	return state == taskmodels.TaskSessionStateCompleted || state == taskmodels.TaskSessionStateIdle
}

// RecordDecision is the AC-57a write-side engine decision entry point:
// it resolves the task's active session per AC-16/16a, then delegates the
// write-and-reevaluate to Engine.RecordParticipantDecision. AC-16a's
// "no session resolvable" case is not an error here — an empty sessionID
// tells the engine to record the decision and skip re-evaluation
// (reported under AC-23/F39 by the engine itself), matching the existing
// blank-session behavior RecordParticipantDecision already implements.
func (d *Dispatcher) RecordDecision(ctx context.Context, in RecordDecisionInput) (RecordDecisionResult, error) {
	if in.TaskID == "" {
		return RecordDecisionResult{}, fmt.Errorf("task_id is required")
	}
	sessionID, err := d.resolveActiveSessionID(ctx, in.TaskID)
	if err != nil {
		return RecordDecisionResult{}, fmt.Errorf("resolve active session: %w", err)
	}
	result, err := d.engine.RecordParticipantDecision(ctx, sessionID, engine.DecisionInfo{
		TaskID:        in.TaskID,
		StepID:        in.StepID,
		ParticipantID: in.ParticipantID,
		Decision:      in.Decision,
		DeciderType:   in.DeciderType,
		DeciderID:     in.DeciderID,
		Role:          in.Role,
		Comment:       in.Comment,
	})
	if err != nil {
		return RecordDecisionResult{}, fmt.Errorf("record participant decision: %w", err)
	}
	return RecordDecisionResult{
		StepID:              in.StepID,
		DecisionID:          result.DecisionID,
		DecidedAt:           result.DecidedAt,
		Transitioned:        result.Transitioned,
		TransitionAbandoned: result.TransitionAbandoned,
		FromStepID:          result.FromStepID,
		ToStepID:            result.ToStepID,
	}, nil
}

// EvaluateStepQuorum is the AC-57d read-only engine entry point. Per F38,
// the state read that satisfies TransitionStore.LoadState's signature uses
// ANY resolvable session for the task (GetTaskSessionByTaskID, latest,
// any state) since MachineState.CurrentStepID is derived from the task
// row, not the session; a task with no session at all yields a
// successful empty snapshot rather than an error. ReevaluationBlocked's
// second conjunct — "no active session" (AC-16's query, distinct from
// F38's any-session read) — is not something the engine can compute
// itself, so it is ANDed in here.
func (d *Dispatcher) EvaluateStepQuorum(ctx context.Context, taskID string) (engine.QuorumSnapshot, error) {
	sessionID, err := d.resolveLatestSessionID(ctx, taskID)
	if err != nil {
		return engine.QuorumSnapshot{}, fmt.Errorf("resolve latest session: %w", err)
	}
	snapshot, err := d.engine.EvaluateStepQuorum(ctx, taskID, sessionID)
	if err != nil {
		return engine.QuorumSnapshot{}, err
	}
	if snapshot.ReevaluationBlocked {
		noActiveSession, err := d.hasNoActiveSession(ctx, taskID)
		if err != nil {
			return engine.QuorumSnapshot{}, fmt.Errorf("resolve active session: %w", err)
		}
		snapshot.ReevaluationBlocked = noActiveSession
	}
	return snapshot, nil
}

// resolveActiveSessionID returns AC-16's active-session id, or "" when no
// such session is resolvable (AC-16a) — never an error for that case.
func (d *Dispatcher) resolveActiveSessionID(ctx context.Context, taskID string) (string, error) {
	session, err := d.sessions.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil {
		if errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
			return "", nil
		}
		return "", err
	}
	if session == nil {
		return "", nil
	}
	return session.ID, nil
}

// resolveLatestSessionID returns the F38 "any session" id (the task's
// most recent session regardless of state), or "" when the task has never
// had one.
func (d *Dispatcher) resolveLatestSessionID(ctx context.Context, taskID string) (string, error) {
	session, err := d.sessions.GetTaskSessionByTaskID(ctx, taskID)
	if err != nil {
		if errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
			return "", nil
		}
		return "", err
	}
	if session == nil {
		return "", nil
	}
	return session.ID, nil
}

// hasNoActiveSession reports whether AC-16's active-session query finds no
// row, the second conjunct of QuorumSnapshot.ReevaluationBlocked (AC-62).
func (d *Dispatcher) hasNoActiveSession(ctx context.Context, taskID string) (bool, error) {
	session, err := d.sessions.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil {
		if errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
			return true, nil
		}
		return false, err
	}
	return session == nil, nil
}
