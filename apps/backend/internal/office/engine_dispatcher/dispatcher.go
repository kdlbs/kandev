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
	if taskID == "" {
		return fmt.Errorf("task_id is required")
	}
	session, err := d.resolveSession(ctx, taskID, trigger)
	if err != nil {
		return fmt.Errorf("resolve session: %w", err)
	}
	if session == nil {
		d.logger.Debug("engine trigger skipped: no active session",
			zap.String("task_id", taskID),
			zap.String("trigger", string(trigger)))
		return ErrNoSession
	}
	in := engine.HandleInput{
		TaskID:      taskID,
		SessionID:   session.ID,
		Trigger:     trigger,
		OperationID: operationID,
		Payload:     payload,
	}
	if _, err := d.engine.HandleTrigger(ctx, in); err != nil {
		return fmt.Errorf("engine handle %s: %w", trigger, err)
	}
	return nil
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
