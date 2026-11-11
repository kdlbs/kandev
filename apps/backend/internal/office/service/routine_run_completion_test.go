package service_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type routineRunSyncCall struct {
	taskID string
	status string
}

type recordingRoutineRunSyncer struct {
	calls []routineRunSyncCall
}

func (f *recordingRoutineRunSyncer) SyncRunStatus(_ context.Context, taskID, status string) error {
	f.calls = append(f.calls, routineRunSyncCall{taskID: taskID, status: status})
	return nil
}

func TestRoutineRunCompletionFollowsTerminalTaskState(t *testing.T) {
	terminalStates := []struct {
		state  string
		status string
	}{
		{state: "COMPLETED", status: "done"},
		{state: "FAILED", status: "failed"},
		{state: string(v1.TaskStateCancelled), status: "cancelled"},
	}
	assignees := []string{"worker-1", ""}

	for _, terminal := range terminalStates {
		for _, assignee := range assignees {
			name := terminal.state + "/assignee=" + assignee
			if assignee == "" {
				name = terminal.state + "/unassigned"
			}
			t.Run(name, func(t *testing.T) {
				svc, eb := newTestServiceWithBus(t)
				syncer := &recordingRoutineRunSyncer{}
				svc.SetRoutineRunSyncer(syncer)

				taskID := "task-" + terminal.state
				event := bus.NewEvent(events.TaskStateChanged, "test", map[string]any{
					"task_id":                   taskID,
					"old_state":                 "IN_PROGRESS",
					"new_state":                 terminal.state,
					"to_step_name":              "Shipped",
					"assignee_agent_profile_id": assignee,
				})
				if err := eb.Publish(context.Background(), events.TaskStateChanged, event); err != nil {
					t.Fatalf("publish task.state_changed: %v", err)
				}

				if len(syncer.calls) != 1 {
					t.Fatalf("SyncRunStatus calls = %d, want 1", len(syncer.calls))
				}
				if got := syncer.calls[0]; got.taskID != taskID || got.status != terminal.status {
					t.Fatalf("SyncRunStatus call = %+v, want task %q status %q", got, taskID, terminal.status)
				}
			})
		}
	}
}

func TestRoutineRunCompletionIgnoresNonTerminalOrInvalidTransitions(t *testing.T) {
	transitions := []map[string]any{
		{"task_id": "task-1", "old_state": "IN_PROGRESS", "new_state": "WAITING_FOR_INPUT"},
		{"task_id": "task-2", "old_state": "", "new_state": "COMPLETED"},
		{"task_id": "task-3", "old_state": "COMPLETED", "new_state": "COMPLETED"},
		{"task_id": "", "old_state": "IN_PROGRESS", "new_state": "FAILED"},
	}
	for i, transition := range transitions {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			svc, eb := newTestServiceWithBus(t)
			syncer := &recordingRoutineRunSyncer{}
			svc.SetRoutineRunSyncer(syncer)
			event := bus.NewEvent(events.TaskStateChanged, "test", transition)
			if err := eb.Publish(context.Background(), events.TaskStateChanged, event); err != nil {
				t.Fatalf("publish task.state_changed: %v", err)
			}
			if len(syncer.calls) != 0 {
				t.Fatalf("SyncRunStatus calls = %d, want 0", len(syncer.calls))
			}
		})
	}
}

func TestRoutineRunCompletionWithoutSyncerIsANoop(t *testing.T) {
	_, eb := newTestServiceWithBus(t)
	event := bus.NewEvent(events.TaskStateChanged, "test", map[string]string{
		"task_id":   "task-without-syncer",
		"old_state": "IN_PROGRESS",
		"new_state": "COMPLETED",
	})
	if err := eb.Publish(context.Background(), events.TaskStateChanged, event); err != nil {
		t.Fatalf("publish task.state_changed: %v", err)
	}
}

func TestRoutineRunCompletionClosesLinkedRunAndIgnoresDuplicateDelivery(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open routine sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	repo, err := sqlite.NewWithDB(db, db, logger.Default())
	if err != nil {
		t.Fatalf("create routine repository: %v", err)
	}
	syncer := routines.NewRoutineService(repo, logger.Default(), nil)
	svc, eb := newTestServiceWithBus(t)
	svc.SetRoutineRunSyncer(syncer)

	terminalStates := []struct {
		state  string
		status models.RoutineRunStatus
	}{
		{state: "COMPLETED", status: models.RoutineRunStatusDone},
		{state: "FAILED", status: models.RoutineRunStatusFailed},
		{state: string(v1.TaskStateCancelled), status: models.RoutineRunStatusCancelled},
	}
	assignees := []string{"worker-1", ""}
	for _, terminal := range terminalStates {
		for _, assignee := range assignees {
			taskID := "linked-" + terminal.state + "-" + assignee
			if assignee == "" {
				taskID = "linked-" + terminal.state + "-unassigned"
			}
			routine := &models.Routine{
				WorkspaceID:       "ws-1",
				Name:              taskID,
				TaskTemplate:      "{}",
				Status:            "active",
				ConcurrencyPolicy: models.ConcurrencyPolicySkipIfActive,
				Variables:         "{}",
			}
			if err := repo.CreateRoutine(context.Background(), routine); err != nil {
				t.Fatalf("create routine %s: %v", taskID, err)
			}
			run := &models.RoutineRun{
				RoutineID:           routine.ID,
				Source:              "manual",
				Status:              models.RoutineRunStatusTaskCreated,
				TriggerPayload:      "{}",
				DispatchFingerprint: taskID,
				LinkedTaskID:        taskID,
			}
			if err := repo.CreateRoutineRun(context.Background(), run); err != nil {
				t.Fatalf("create routine run for %s: %v", taskID, err)
			}

			event := bus.NewEvent(events.TaskStateChanged, "test", map[string]string{
				"task_id":                   taskID,
				"old_state":                 "IN_PROGRESS",
				"new_state":                 terminal.state,
				"to_step_name":              "Shipped",
				"assignee_agent_profile_id": assignee,
			})
			if err := eb.Publish(context.Background(), events.TaskStateChanged, event); err != nil {
				t.Fatalf("publish completion for %s: %v", taskID, err)
			}
			closed, err := repo.GetRoutineRunByLinkedTaskID(context.Background(), taskID)
			if err != nil {
				t.Fatalf("read closed run for %s: %v", taskID, err)
			}
			if closed.Status != terminal.status || closed.CompletedAt == nil {
				t.Fatalf("closed run = status %q completed_at %v, want status %q and completion time", closed.Status, closed.CompletedAt, terminal.status)
			}
			completedAt := *closed.CompletedAt

			if err := eb.Publish(context.Background(), events.TaskStateChanged, event); err != nil {
				t.Fatalf("republish completion for %s: %v", taskID, err)
			}
			redelivered, err := repo.GetRoutineRunByLinkedTaskID(context.Background(), taskID)
			if err != nil {
				t.Fatalf("read redelivered run for %s: %v", taskID, err)
			}
			if redelivered.Status != terminal.status || redelivered.CompletedAt == nil || !redelivered.CompletedAt.Equal(completedAt) {
				t.Fatalf("redelivered run = status %q completed_at %v, want unchanged status %q and time %v", redelivered.Status, redelivered.CompletedAt, terminal.status, completedAt)
			}
		}
	}
}
