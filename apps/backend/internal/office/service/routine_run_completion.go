package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type routineRunTaskStateChangedData struct {
	TaskID   string `json:"task_id"`
	OldState string `json:"old_state"`
	NewState string `json:"new_state"`
}

func (s *Service) handleRoutineRunTaskStateChanged(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[routineRunTaskStateChangedData](event)
	if err != nil || data.TaskID == "" || data.OldState == "" || data.OldState == data.NewState {
		return nil
	}
	status := routineRunTerminalStatus(data.NewState)
	if status == "" || s.routineRunSyncer == nil {
		return nil
	}
	if err := s.routineRunSyncer.SyncRunStatus(ctx, data.TaskID, status); err != nil {
		s.logger.Warn("sync routine run status",
			zap.String("task_id", data.TaskID),
			zap.String("status", status),
			zap.Error(err))
	}
	return nil
}

func routineRunTerminalStatus(taskState string) string {
	switch taskState {
	case "COMPLETED":
		return "done"
	case "FAILED":
		return "failed"
	case string(v1.TaskStateCancelled):
		return "cancelled"
	default:
		return ""
	}
}
