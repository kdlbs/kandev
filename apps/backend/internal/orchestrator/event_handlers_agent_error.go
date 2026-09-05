package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// Stable, dispatch-owned log messages. Kept distinct from every
// other log line in this package so a test can key on the dispatch's own
// records without also matching a line the surrounding handler or a shared
// helper (e.g. otherWorkingSessionID) writes for itself.
const (
	msgAgentErrorUserInitiated         = "on_agent_error dispatch: failure was user-initiated"
	msgAgentErrorSessionVanished       = "on_agent_error dispatch: session no longer exists"
	msgAgentErrorSessionReloadFailed   = "on_agent_error dispatch: failed to reload session"
	msgAgentErrorTaskLoadFailed        = "on_agent_error dispatch: failed to load task"
	msgAgentErrorAnotherSessionWorking = "on_agent_error dispatch: another session is working"
	msgAgentErrorActionUnregistered    = "on_agent_error action has no registered callback"
	msgAgentErrorDispatched            = "on_agent_error dispatched"
	msgAgentErrorNoActions             = "on_agent_error dispatch: step declares no actions"
	msgAgentErrorDispatchFailed        = "on_agent_error dispatch failed"
	msgAgentErrorDispatchPanicked      = "on_agent_error dispatch panicked"
	msgAgentErrorMarkFailed            = "on_agent_error dispatch: failed to mark deferred operation applied"

	// defaultAgentFailedMessage backfills an empty AgentEventData.ErrorMessage
	// (e.g. a synthetic start-failure event). Shared with the two other
	// terminal-failure call sites in event_handlers_agent.go so the literal
	// stays a single source of truth (goconst).
	defaultAgentFailedMessage = "agent failed"
)

// agentErrorDispatchDeps bundles the workflow engine, the callback registry
// the pre-engine walk checks against, and the store both read — the three
// collaborators initWorkflowEngine rebuilds together on every reinit.
// Holding them as one struct, published through Service.agentErrorDeps in a
// single atomic.Pointer write and read exactly once per dispatch, closes the
// race three separate fields would otherwise leave open: a dispatch racing a
// Set*/reinit call could pair a new engine with a stale registry, or walk
// one version of a step while the engine executes another.
type agentErrorDispatchDeps struct {
	engine   *engine.Engine
	registry engine.CallbackRegistry
	store    *workflowStore
}

// agentErrorOperationID derives on_agent_error's idempotency key from the
// failure's identity rather than from a run. Both components
// matter: a session can host several executions in sequence (rotation,
// resume, dynamic re-route), each of which can fail, so a session-only key
// would suppress every failure after the first for the life of the process.
func agentErrorOperationID(sessionID, agentExecutionID string) string {
	if agentExecutionID == "" {
		return fmt.Sprintf("agent_error:session:%s", sessionID)
	}
	return fmt.Sprintf("agent_error:session:%s:%s", sessionID, agentExecutionID)
}

// agentErrorOperationLock is the ref-counted per-operation-id mutex entry
// serializing dispatchKanbanAgentErrorTrigger's load -> evaluate -> commit ->
// mark window for a given operation id. Same shape as
// childCompletionOperationLock, but a separate type and a separate map on
// Service: the two triggers' contention must not be coupled by sharing one
// lock's lifetime.
type agentErrorOperationLock struct {
	mu   sync.Mutex
	refs int
}

// lockAgentErrorOperation serializes concurrent dispatchKanbanAgentErrorTrigger
// calls carrying the same operation id. The returned unlock func must be
// deferred immediately by the caller: dispatchKanbanAgentErrorTrigger runs
// under dispatchKanbanAgentErrorTriggerRecovered's recover(), so a
// non-deferred unlock would leak the lock on panic.
func (s *Service) lockAgentErrorOperation(operationID string) func() {
	s.agentErrorOperationLocksMu.Lock()
	if s.agentErrorOperationLocks == nil {
		s.agentErrorOperationLocks = make(map[string]*agentErrorOperationLock)
	}
	entry := s.agentErrorOperationLocks[operationID]
	if entry == nil {
		entry = &agentErrorOperationLock{}
		s.agentErrorOperationLocks[operationID] = entry
	}
	entry.refs++
	s.agentErrorOperationLocksMu.Unlock()

	entry.mu.Lock()
	return func() {
		s.agentErrorOperationLocksMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(s.agentErrorOperationLocks, operationID)
		}
		s.agentErrorOperationLocksMu.Unlock()
		entry.mu.Unlock()
	}
}

// agentErrorFailedAgentID resolves OnAgentErrorPayload.FailedAgentID.
// The synthetic failure events built by handleAgentStartFailed and
// the transient-loop exits carry no AgentProfileID, so the session fallback
// is the normal path for four of the five dispatching routes.
func agentErrorFailedAgentID(data watcher.AgentEventData, session *models.TaskSession) string {
	if data.AgentProfileID != "" {
		return data.AgentProfileID
	}
	if session != nil {
		return session.AgentProfileID
	}
	return ""
}

// agentErrorMessage resolves OnAgentErrorPayload.ErrorMessage.
func agentErrorMessage(data watcher.AgentEventData) string {
	if data.ErrorMessage != "" {
		return data.ErrorMessage
	}
	return defaultAgentFailedMessage
}

// isAgentErrorTransitionActionKind reports whether kind is one of the three
// structural transition kinds, which the engine executes without a
// registered callback regardless of position, guard, or requires_approval.
func isAgentErrorTransitionActionKind(kind engine.ActionKind) bool {
	switch kind {
	case engine.ActionMoveToNext, engine.ActionMoveToPrevious, engine.ActionMoveToStep:
		return true
	default:
		return false
	}
}

// warnUnregisteredAgentErrorActions is the pre-engine walk: it reads the
// current step's compiled on_agent_error actions through the same store the
// engine will use and warns on any non-transition action with no
// registered callback. A failed read is advisory-only — the walk is
// skipped silently and the dispatch still proceeds to the engine.
func (s *Service) warnUnregisteredAgentErrorActions(
	ctx context.Context, deps *agentErrorDispatchDeps, task *models.Task, sessionID string,
) {
	step, err := deps.store.LoadStep(ctx, task.WorkflowID, task.WorkflowStepID)
	if err != nil {
		return
	}
	for _, action := range step.Events[engine.TriggerOnAgentError] {
		if isAgentErrorTransitionActionKind(action.Kind) {
			continue
		}
		if _, ok := deps.registry.Get(action.Kind); ok {
			continue
		}
		s.logger.Warn(msgAgentErrorActionUnregistered,
			zap.String("workflow_id", task.WorkflowID),
			zap.String("step_id", task.WorkflowStepID),
			zap.String("step_name", step.Name),
			zap.String("action_type", string(action.Kind)),
			zap.String("task_id", task.ID),
			zap.String("session_id", sessionID))
	}
}

// resolveAgentErrorDispatchTarget reloads the session and task for a failure
// and evaluates the ownership/shape guards. ok is
// false when dispatch must stop; every stop path has already logged at the
// correct level (or, for the silent shape guards, deliberately logs
// nothing).
func (s *Service) resolveAgentErrorDispatchTarget(
	ctx context.Context, data watcher.AgentEventData,
) (session *models.TaskSession, task *models.Task, ok bool) {
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			s.logger.Debug(msgAgentErrorSessionVanished,
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID))
			return nil, nil, false
		}
		s.logger.Warn(msgAgentErrorSessionReloadFailed,
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return nil, nil, false
	}
	if session == nil {
		s.logger.Debug(msgAgentErrorSessionVanished,
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		return nil, nil, false
	}

	task, err = s.repo.GetTask(ctx, data.TaskID)
	if err != nil || task == nil {
		s.logger.Warn(msgAgentErrorTaskLoadFailed,
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.Error(err))
		return nil, nil, false
	}

	if task.IsFromOffice || task.IsEphemeral || task.WorkflowStepID == "" || taskArchived(task) {
		return nil, nil, false
	}

	blockingSessionID, okOther := s.otherWorkingSessionID(ctx, data.TaskID, data.SessionID)
	if !okOther {
		// otherWorkingSessionID already logged its own WARNING.
		return nil, nil, false
	}
	if blockingSessionID != "" {
		s.logger.Debug(msgAgentErrorAnotherSessionWorking,
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("blocking_session_id", blockingSessionID))
		return nil, nil, false
	}
	return session, task, true
}

// dispatchKanbanAgentErrorTrigger is the non-Office fire site for
// engine.TriggerOnAgentError — the counterpart to Office's
// dispatchAgentErrorTrigger. It is called as the final action of terminal
// agent-session-failure handling, after that failure's own
// bookkeeping is already committed, so a dispatch failure here must never
// mask it.
//
// Guards run first-match-wins in the order fixed by "Guard evaluation
// order" in docs/specs/workflow-on-agent-error-dispatch/spec.md: the engine
// snapshot, the session id, the user-initiated marker, then a single
// session-then-task reload feeding the ownership and shape guards
// (resolveAgentErrorDispatchTarget).
//
// The operation id is computed and its lock acquired before the
// task/session/MachineState load, and held across the engine call, the
// commit, and this dispatch's own mark: the engine consumes PreloadedState
// verbatim, so a lock taken only around the engine call would let a blocked
// racer wake holding state built before a winner's commit and re-evaluate
// the step the task has already left.
//
// ctx is stripped of the caller's cancellation (see the call site in
// handleRecoverableFailureLocked): this dispatch is the recovery path for an
// already-failed session, so a canceled request context must not suppress
// the recovery it exists to run.
func (s *Service) dispatchKanbanAgentErrorTrigger(ctx context.Context, data watcher.AgentEventData) {
	deps := s.agentErrorDeps.Load()
	if deps == nil || deps.engine == nil {
		return
	}
	if data.SessionID == "" {
		return
	}
	if data.UserInitiated {
		s.logger.Debug(msgAgentErrorUserInitiated,
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID))
		return
	}

	operationID := agentErrorOperationID(data.SessionID, data.AgentExecutionID)
	unlock := s.lockAgentErrorOperation(operationID)
	defer unlock()

	session, task, ok := s.resolveAgentErrorDispatchTarget(ctx, data)
	if !ok {
		return
	}

	state := s.buildMachineState(ctx, task, session)

	s.warnUnregisteredAgentErrorActions(ctx, deps, task, data.SessionID)

	result, err := deps.engine.HandleTrigger(ctx, engine.HandleInput{
		TaskID:         data.TaskID,
		SessionID:      data.SessionID,
		Trigger:        engine.TriggerOnAgentError,
		OperationID:    operationID,
		EvaluateOnly:   true,
		PreloadedState: &state,
		Payload: engine.OnAgentErrorPayload{
			FailedAgentID:   agentErrorFailedAgentID(data, session),
			FailedSessionID: data.SessionID,
			ErrorMessage:    agentErrorMessage(data),
		},
	})
	if err != nil {
		s.logger.Error(msgAgentErrorDispatchFailed,
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("operation_id", operationID),
			zap.Error(err))
		return
	}
	if result.Idempotent {
		return
	}

	if result.Transitioned {
		applied := s.applyEngineTransition(ctx, data.TaskID, session, result, engine.TriggerOnAgentError, task.Description, true)
		if applied && result.OperationMarkDeferred {
			if markErr := deps.store.MarkOperationApplied(ctx, operationID); markErr != nil {
				s.logger.Warn(msgAgentErrorMarkFailed,
					zap.String("task_id", data.TaskID),
					zap.String("session_id", data.SessionID),
					zap.String("operation_id", operationID),
					zap.Error(markErr))
			}
		}
	}

	if result.ActionCount > 0 {
		s.logger.Info(msgAgentErrorDispatched,
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("step_id", task.WorkflowStepID),
			zap.String("operation_id", operationID))
		return
	}
	s.logger.Debug(msgAgentErrorNoActions,
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID),
		zap.String("step_id", task.WorkflowStepID),
		zap.String("operation_id", operationID))
}

// dispatchKanbanAgentErrorTriggerRecovered wraps dispatchKanbanAgentErrorTrigger
// with panic recovery. The dispatch now runs synchronously after the caller
// has released the failed session's cancelInFlight guard (see
// handleRecoverableFailureLockedState), so — unlike an agent.failed delivery
// that arrives through the event bus's own recover-and-log invokeHandler
// (events/bus/safe_handler.go) — routes R2-R5 reach this call with no
// recover above them on the stack. A panicking callback (e.g. auto_start_agent)
// would otherwise take the whole backend process down.
func (s *Service) dispatchKanbanAgentErrorTriggerRecovered(ctx context.Context, data watcher.AgentEventData) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error(msgAgentErrorDispatchPanicked,
				zap.String("task_id", data.TaskID),
				zap.String("session_id", data.SessionID),
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	s.dispatchKanbanAgentErrorTrigger(ctx, data)
}
