package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/events/bus"
)

// Taskless turns have no task orchestrator to stop the execution after AgentReady.
// Keep the run claimed until its exact execution has stopped successfully.
//
// A task-owned launch can reach here only as the recovery path for a
// ceiling-deferred Office run whose replay-success settlement failed: its ready
// event carries the same causing run id, so the parked run can be settled
// without another launch. Ordinary task-owned launches are already claimed and
// settle as a no-op.
func (s *Service) handleAgentReady(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[AgentLifecycleData](event)
	if err != nil {
		return err
	}
	if data.TaskID != "" {
		return s.settleCeilingDeferredLaunchOnReady(ctx, data)
	}
	if data.OwnerKind != string(runtimeapi.ExecutionOwnerRun) ||
		!exactRunSessionEvent(data) || data.AgentExecutionID == "" {
		return nil
	}
	if _, err := s.resolveLifecycleRun(ctx, *data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if s.runStopper == nil {
		return errors.New("taskless completion requires a runtime stopper")
	}
	if err := s.runStopper.Stop(ctx, data.AgentExecutionID, "office_turn_complete"); err != nil && !runtimeapi.IsNotFound(err) {
		return fmt.Errorf("stop completed taskless execution: %w", err)
	}
	return s.handleTasklessAgentCompleted(ctx, data)
}
