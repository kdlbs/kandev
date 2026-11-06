package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events/bus"
)

// Task session states read from task_session.state_changed. Office keeps the
// literals local because it does not import the task models package here.
const (
	taskSessionStateRunning         = "RUNNING"
	taskSessionStateWaitingForInput = "WAITING_FOR_INPUT"
)

// TaskSessionStateData is the subset of a task_session.state_changed payload
// that Office reads.
type TaskSessionStateData struct {
	TaskID         string `json:"task_id"`
	SessionID      string `json:"session_id"`
	OldState       string `json:"old_state"`
	NewState       string `json:"new_state"`
	ErrorMessage   string `json:"error_message"`
	AgentProfileID string `json:"agent_profile_id"`
	// UpdatedAt is the committed timestamp of the state write that produced
	// this event. It orders the park against a run claim: a run claimed after
	// the park belongs to the next turn, not the turn that parked.
	UpdatedAt string `json:"updated_at"`
	// The separate Office settlement event carries its completing turn.
	TurnStartedAt string `json:"turn_started_at,omitempty"`
	TurnID        string `json:"turn_id,omitempty"`
}

// handleTaskSessionStateChanged settles per-launch turns on their park.
// Office sessions retain the run claim until their completion path confirms
// teardown, so the next run cannot configure a process that is still alive.
func (s *Service) handleTaskSessionStateChanged(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[TaskSessionStateData](event)
	if err != nil || data == nil {
		return nil
	}
	s.resolvePendingAgentFailure(ctx, *data)
	if !isSuccessfulTurnPark(*data) {
		return nil
	}
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil {
		return err
	}
	if fields.IsFromOffice {
		return nil
	}
	return s.settleParkedSessionRun(ctx, *data)
}

// handleOfficeTurnSettled receives only a completed turn's runtime settlement,
// after its stop succeeded or its completion owner chose a no-stop path.
func (s *Service) handleOfficeTurnSettled(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[TaskSessionStateData](event)
	if err != nil || data == nil || data.TaskID == "" || data.SessionID == "" ||
		data.AgentProfileID == "" || data.TurnStartedAt == "" {
		return nil
	}
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil {
		return err
	}
	if !fields.IsFromOffice {
		return nil
	}
	return s.settleParkedSessionRun(ctx, *data)
}

// isSuccessfulTurnPark reports a RUNNING -> WAITING_FOR_INPUT transition
// without an error. Routine and other per-launch sessions settle here; Office
// turns retain their claim until the separate teardown confirmation.
func isSuccessfulTurnPark(data TaskSessionStateData) bool {
	return data.TaskID != "" && data.SessionID != "" && data.AgentProfileID != "" &&
		data.OldState == taskSessionStateRunning &&
		data.NewState == taskSessionStateWaitingForInput &&
		data.ErrorMessage == ""
}

// settleParkedSessionRun finishes the agent's claimed run on the task only
// when that run launched the parked session, so a park on another session of
// the same task never settles a run it did not execute.
func (s *Service) settleParkedSessionRun(ctx context.Context, data TaskSessionStateData) error {
	run, err := s.repo.GetClaimedRunByTaskAndAgent(ctx, data.TaskID, data.AgentProfileID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if run.SessionID == "" || run.SessionID != data.SessionID {
		return nil
	}
	if data.TurnStartedAt != "" {
		// The turn start is immutable even if a reused session has advanced
		// while git-status capture or teardown was in progress. Never let an
		// old completion settle a newer claim on that same session.
		turnStartedAt, err := time.Parse(time.RFC3339Nano, data.TurnStartedAt)
		if err != nil || run.ClaimedAt == nil || run.ClaimedAt.After(turnStartedAt) {
			return nil
		}
		state, errorMessage, err := s.repo.GetTaskSessionState(ctx, data.SessionID)
		if err != nil {
			return err
		}
		if state == taskSessionStateFailed || errorMessage != "" {
			return nil
		}
	}
	if parkPredatesClaim(data.UpdatedAt, run.ClaimedAt) {
		// A run claimed after this park was recorded belongs to the next
		// turn on the same Office session. Settling it here would finish
		// work the old turn never ran.
		s.logger.Info("ignoring park event for a run claimed after it was recorded",
			zap.String("run_id", run.ID),
			zap.String("task_id", data.TaskID),
			zap.String("session_id", data.SessionID),
			zap.String("park_updated_at", data.UpdatedAt),
			zap.Time("run_claimed_at", *run.ClaimedAt))
		return nil
	}
	s.logger.Info("settling Office run after its launched session parked",
		zap.String("run_id", run.ID),
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID))
	return s.completeClaimedTaskRun(ctx, run, AgentLifecycleData{
		TaskID:         data.TaskID,
		RunID:          run.ID,
		AgentProfileID: data.AgentProfileID,
		SessionID:      data.SessionID,
		TurnID:         data.TurnID,
	})
}

// parkPredatesClaim reports whether the run was claimed after the park's
// committed state write. Office sessions are reused across runs, so the park
// timestamp is the ordering identity that distinguishes the turn that parked
// from a successor claimed afterwards. An absent or unparseable timestamp
// leaves the existing settlement behavior unchanged.
func parkPredatesClaim(parkUpdatedAt string, claimedAt *time.Time) bool {
	if parkUpdatedAt == "" || claimedAt == nil {
		return false
	}
	parkTime, err := time.Parse(time.RFC3339Nano, parkUpdatedAt)
	if err != nil {
		return false
	}
	return claimedAt.After(parkTime)
}
