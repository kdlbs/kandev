// Package engine_dispatcher provides the production implementation of
// office/service.WorkflowEngineDispatcher. It bridges the office service's
// typed event subscribers to the workflow engine's HandleInput envelope
// by resolving the task's active session id and invoking
// engine.HandleTrigger.
//
// Constructed in cmd/kandev/main.go and passed to office service via
// SetWorkflowEngineDispatcher. The engine path is unconditional.
package engine_dispatcher

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/common/logger"
	officeservice "github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	"go.uber.org/zap"
)

// ErrNoSession is re-exported from office/service so callers can compare
// via errors.Is without importing this package directly. Returned when a
// trigger arrives for a task with no active session — engine state is
// keyed on (taskID, sessionID), so the dispatcher cannot proceed without
// one.
var ErrNoSession = officeservice.ErrEngineNoSession

// SessionResolver looks up a task's active session.
type SessionResolver interface {
	GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*taskmodels.TaskSession, error)
}

// EngineHandle is the engine surface the dispatcher needs. Defined as a
// minimal interface so tests can pass a fake.
type EngineHandle interface {
	HandleTrigger(ctx context.Context, in engine.HandleInput) (engine.HandleResult, error)
}

// Dispatcher resolves a task's active session and invokes the workflow
// engine. It implements office/service.WorkflowEngineDispatcher.
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

// HandleTrigger satisfies office/service.WorkflowEngineDispatcher.
//
// Resolves the task's active session — returning ErrNoSession if none
// exists — then invokes engine.HandleTrigger. Errors from the engine
// (e.g. queue_run resolver failures) bubble up so the office event
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
	session, err := d.sessions.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil || session == nil {
		// No active session — engine cannot LoadState. Caller falls
		// back to the legacy QueueRun path.
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
