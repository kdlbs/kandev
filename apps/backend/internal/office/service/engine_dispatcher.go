package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/workflow/engine"
)

// ErrEngineNoSession is the sentinel a WorkflowEngineDispatcher returns
// when a task has no active session and the engine cannot evaluate a
// trigger. After Phase 4 there is no legacy fallback — subscribers log
// and drop the trigger when the engine cannot evaluate it.
//
// Implementations (see internal/office/engine_dispatcher) MUST use this
// exact sentinel; subscribers compare via errors.Is.
var ErrEngineNoSession = errors.New("workflow engine: no active session for task")

// WorkflowEngineDispatcher is the office service's contract with the
// workflow engine. The interface lives here (not in the engine package)
// because office is the only caller that needs to dispatch from typed
// trigger payloads — the engine itself accepts a generic HandleInput.
//
// Implementations are responsible for:
//
//  1. Resolving the task's active session id (if any) — without it the
//     engine cannot LoadState.
//  2. Building engine.HandleInput from the typed payload.
//  3. Invoking engine.HandleTrigger and translating its result.
//
// The interface stays narrow to keep the office service test surface
// small — only the four event-subscriber triggers are exercised.
type WorkflowEngineDispatcher interface {
	HandleTrigger(
		ctx context.Context,
		taskID string,
		trigger engine.Trigger,
		payload any,
		operationID string,
	) error
}

// SetWorkflowEngineDispatcher wires a dispatcher onto the office service.
// After Phase 4, when a dispatcher is set, the four event subscribers
// (comment_created, blockers_resolved, children_completed,
// approval_resolved) route through the engine unconditionally — there is
// no legacy fallback path.
//
// Calling SetWorkflowEngineDispatcher with nil disables engine routing
// (useful in tests that don't need a workflow engine wired).
func (s *Service) SetWorkflowEngineDispatcher(d WorkflowEngineDispatcher) {
	s.engineDispatcher = d
}
