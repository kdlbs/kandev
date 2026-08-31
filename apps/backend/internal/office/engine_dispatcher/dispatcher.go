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
	// GetTaskSession resolves a session by its own id, independent of
	// which task session is "active" or "latest". Used to target the
	// exact session TriggerOnAgentError's payload names (Review F1): an
	// office task can carry more than one session per task (one per
	// (task, agent) pair), so a task-scoped active/latest lookup can
	// return an unrelated sibling instead of the one that failed.
	GetTaskSession(ctx context.Context, id string) (*taskmodels.TaskSession, error)
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
	// ResolveParticipantRole is the AC-2/3/4/4a role-and-seat resolution
	// entry point the agent decision tool needs. Reached the same way as
	// the two methods above: via Dispatcher.ResolveParticipantRole plus a
	// narrow caller-side type assertion.
	ResolveParticipantRole(ctx context.Context, taskID, stepID, agentProfileID string) (role, participantID string, err error)
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
	// SessionID, when non-empty, names the decider's own calling session.
	// RecordDecision re-evaluates against this session instead of the
	// task's most-recently-started ("active") session, so a reviewer/
	// approver's decision is checked against the seats their own session
	// occupies rather than whichever session happens to be newest.
	// Callers only populate this behind features.officeSessionIdentity
	// (see office/dashboard.DashboardService.SetOfficeSessionIdentity);
	// leaving it empty preserves the existing resolveActiveSessionID path.
	SessionID string
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
	// Guards is the engine's pre-transition quorum snapshot.
	Guards []engine.QuorumGuardState
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
	session, err := d.resolveSession(ctx, taskID, trigger, payload)
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
	ctx context.Context, taskID string, trigger engine.Trigger, payload any,
) (*taskmodels.TaskSession, error) {
	// Review round-1 F1: an office task carries one session per (task,
	// agent) pair (executor_office.go's find-or-create), so
	// office-default.yml's multi-agent workflow routinely has more than
	// one session on a task at once. A task-scoped active/latest lookup
	// can therefore resolve an unrelated sibling session instead of the
	// one that actually failed. When the trigger's payload names the
	// exact session, resolve it directly and skip the heuristics below
	// entirely — this session is correct regardless of its current
	// state, since it was already identified as the one that failed.
	if trigger == engine.TriggerOnAgentError {
		if sessionID := agentErrorFailedSessionID(payload); sessionID != "" {
			return d.resolveSessionByID(ctx, taskID, sessionID)
		}
	}
	session, err := d.sessions.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err == nil && session != nil {
		return session, nil
	}
	if err != nil && !errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return nil, fmt.Errorf("active session lookup: %w", err)
	}
	if trigger != engine.TriggerOnComment && trigger != engine.TriggerOnAgentError {
		return nil, nil
	}
	// Comment wakes are allowed after an office task's agent session has
	// completed or returned to reusable IDLE state. The workflow engine state is
	// keyed by (taskID, sessionID), so a post-completion comment intentionally
	// resumes the latest reusable session's persisted machine state instead of
	// starting a fresh state machine here.
	//
	// Agent-error wakes reach this fallback only for legacy events with no
	// FailedSessionID (payload predates session_id — the direct-resolve
	// branch above handles every current event). It picks the latest
	// session only when that session is itself FAILED, which is at best
	// a guess at the failed session's identity, not a guarantee — kept
	// only so an old queued event does not silently no-op.
	session, err = d.sessions.GetTaskSessionByTaskID(ctx, taskID)
	if err == nil && session != nil {
		if !isReusableSessionForTrigger(trigger, session.State) {
			return nil, nil
		}
		return session, nil
	}
	if err != nil && !errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
		return nil, fmt.Errorf("latest session lookup: %w", err)
	}
	return nil, nil
}

// resolveSessionByID resolves a session by its own id — used for
// TriggerOnAgentError's FailedSessionID (Review F1) rather than a
// task-scoped active/latest lookup. It also verifies that the session belongs
// to the event task before returning it, so the engine never combines state
// from two task records. A not-found or mismatched id resolves to "no
// session" (nil, nil), consistent with every other branch of
// resolveSession, rather than an error: the session row may already be
// gone by the time the trigger dispatches, and that is a normal no-op,
// not a failure.
func (d *Dispatcher) resolveSessionByID(
	ctx context.Context, taskID, sessionID string,
) (*taskmodels.TaskSession, error) {
	session, err := d.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed session lookup: %w", err)
	}
	if session == nil || session.TaskID != taskID {
		return nil, nil
	}
	return session, nil
}

// agentErrorFailedSessionID extracts OnAgentErrorPayload.FailedSessionID
// when present. Empty for a payload that isn't OnAgentErrorPayload or
// predates session_id, in which case resolveSession falls back to the
// pre-F1 active/latest-FAILED heuristic.
func agentErrorFailedSessionID(payload any) string {
	if p, ok := payload.(engine.OnAgentErrorPayload); ok {
		return p.FailedSessionID
	}
	return ""
}

func isReusableSessionForTrigger(trigger engine.Trigger, state taskmodels.TaskSessionState) bool {
	if trigger == engine.TriggerOnAgentError {
		return state == taskmodels.TaskSessionStateFailed
	}
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
	var sessionID string
	var err error
	if in.SessionID != "" {
		sessionID, err = d.resolveDeciderSessionID(ctx, in.TaskID, in.SessionID)
	} else {
		sessionID, err = d.resolveActiveSessionID(ctx, in.TaskID)
	}
	if err != nil {
		return RecordDecisionResult{}, fmt.Errorf("resolve decider session: %w", err)
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
	// AC-15: the engine returns a populated result alongside a non-nil err
	// exactly when the write succeeded but the post-write re-evaluation
	// failed. That case must be reported as success — with the recorded
	// decision's identity kept — never collapsed into a write failure. An
	// empty DecisionID means the write itself never happened, which is a
	// genuine failure. This is the single funnel recordTaskDecision and
	// RecordAgentDecision both call through, so fixing it here covers both.
	if err != nil {
		if result.DecisionID == "" {
			return RecordDecisionResult{}, fmt.Errorf("record participant decision: %w", err)
		}
		d.logger.Warn("quorum re-evaluation failed after decision recorded",
			zap.String("task_id", in.TaskID),
			zap.String("step_id", in.StepID),
			zap.String("decision_id", result.DecisionID),
			zap.Error(err))
	}
	return RecordDecisionResult{
		StepID:              in.StepID,
		DecisionID:          result.DecisionID,
		DecidedAt:           result.DecidedAt,
		Transitioned:        result.Transitioned,
		TransitionAbandoned: result.TransitionAbandoned,
		FromStepID:          result.FromStepID,
		ToStepID:            result.ToStepID,
		Guards:              result.Guards,
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

// ResolveParticipantRole is the AC-2/3/4/4a engine entry point: resolves
// the calling agent's role and seat id at the given step by reusing the
// AC-50 slate construction (approver before reviewer, per AC-4). Unlike
// RecordDecision/EvaluateStepQuorum this needs no session resolution — the
// slate read has no session dependency.
func (d *Dispatcher) ResolveParticipantRole(
	ctx context.Context, taskID, stepID, agentProfileID string,
) (role, participantID string, err error) {
	return d.engine.ResolveParticipantRole(ctx, taskID, stepID, agentProfileID)
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

// resolveDeciderSessionID validates the decider's own calling session
// (RecordDecisionInput.SessionID) rather than resolving the task's active
// session. It returns "" (session_unresolvable, not an error) when the
// session is missing, belongs to a different task, or is not in one of the
// active states — mirroring resolveActiveSessionID's own never-an-error
// contract for "no session resolvable" (AC-16a) — so a stale or foreign
// session id degrades to the same "skip re-evaluation" behavior an
// unresolvable active session already gets, rather than rejecting the
// decision outright.
//
// The active-state set mirrors GetActiveTaskSessionByTaskID's own query
// (internal/task/repository/sqlite/session.go) — there is no shared
// models-level predicate for that set, so this local copy must be kept in
// sync with it.
func (d *Dispatcher) resolveDeciderSessionID(ctx context.Context, taskID, sessionID string) (string, error) {
	session, err := d.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, taskmodels.ErrTaskSessionNotFound) {
			d.logSessionUnresolvable(taskID, sessionID, "not_found")
			return "", nil
		}
		return "", err
	}
	if session == nil || session.TaskID != taskID {
		d.logSessionUnresolvable(taskID, sessionID, "foreign")
		return "", nil
	}
	if !isActiveSessionState(session.State) {
		d.logSessionUnresolvable(taskID, sessionID, "terminal")
		return "", nil
	}
	return session.ID, nil
}

func (d *Dispatcher) logSessionUnresolvable(taskID, sessionID, reason string) {
	d.logger.Debug("resolveDeciderSessionID: session unresolvable, falling back to blank",
		zap.String("task_id", taskID),
		zap.String("supplied_session_id", sessionID),
		zap.String("reason", reason))
}

func isActiveSessionState(state taskmodels.TaskSessionState) bool {
	switch state {
	case taskmodels.TaskSessionStateCreated,
		taskmodels.TaskSessionStateStarting,
		taskmodels.TaskSessionStateRunning,
		taskmodels.TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}

// resolveLatestSessionID returns the F38 "any session" id (the task's
// most recent session regardless of state), or "" when the task has never
// had one. Like resolveActiveSessionID and resolveDeciderSessionID, this is
// scoped to the given taskID — GetTaskSessionByTaskID's own query already
// filters by task, so unlike resolveDeciderSessionID (which validates a
// caller-supplied session id and must check task ownership itself), there
// is no cross-task session id to smuggle in here.
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
