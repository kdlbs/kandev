package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// A workflow profile switch parks its source session in WAITING_FOR_INPUT
// while the replacement session takes over. A launch that was already in
// flight must observe that as a supersede, not as a launch failure.
func TestPersistSessionFullRowIfCurrentState_WorkflowParkIsSuperseded(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateWaitingForInput,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyWorkflowParking: models.WorkflowParking{
				Stamp:           "park-1",
				ParkedAt:        time.Now().UTC(),
				SourceSessionID: "session-1",
				DestinationID:   "session-2",
			},
		},
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	stale := &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated}

	err := exec.persistSessionFullRowIfCurrentState(ctx, stale, models.TaskSessionStateCreated)
	if !IsSessionStateSwitched(err) {
		t.Fatalf("persistSessionFullRowIfCurrentState() error = %v, want SessionStateSwitchedError", err)
	}
	var switched *SessionStateSwitchedError
	if !errors.As(err, &switched) {
		t.Fatalf("error %T does not unwrap to *SessionStateSwitchedError", err)
	}
	if switched.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("switched state = %q, want WAITING_FOR_INPUT", switched.State)
	}
}

// An unexpected state without a workflow switch record is still a real launch
// failure, so the supersede classification must not widen.
func TestPersistSessionFullRowIfCurrentState_UnexpectedStateWithoutSwitchIsError(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateWaitingForInput,
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	stale := &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated}

	err := exec.persistSessionFullRowIfCurrentState(ctx, stale, models.TaskSessionStateCreated)
	if err == nil {
		t.Fatal("persistSessionFullRowIfCurrentState() error = nil, want unexpected-state failure")
	}
	if IsSessionStateSwitched(err) {
		t.Fatalf("unexpected state was classified as a workflow switch: %v", err)
	}
}

// A parking marker that names another session (the destination) must not
// excuse this launch's state mismatch.
func TestPersistSessionFullRowIfCurrentState_ForeignParkingMarkerIsNotSuperseded(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateWaitingForInput,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyWorkflowParking: models.WorkflowParking{
				Stamp:           "park-1",
				ParkedAt:        time.Now().UTC(),
				SourceSessionID: "session-other",
			},
		},
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	stale := &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated}

	err := exec.persistSessionFullRowIfCurrentState(ctx, stale, models.TaskSessionStateCreated)
	if err == nil || IsSessionStateSwitched(err) {
		t.Fatalf("persistSessionFullRowIfCurrentState() error = %v, want a plain unexpected-state failure", err)
	}
}
