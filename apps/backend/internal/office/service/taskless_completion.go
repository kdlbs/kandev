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
func (s *Service) handleTasklessAgentReady(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[AgentLifecycleData](event)
	if err != nil {
		return err
	}
	if data.OwnerKind != string(runtimeapi.ExecutionOwnerRun) || data.TaskID != "" ||
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
