package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskSessionWakeUpsertKeepsOneStableRowPerMarker(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-wake", Name: "Wake workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-wake", WorkspaceID: "ws-wake", Title: "Wake task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-wake", TaskID: "task-wake", State: models.TaskSessionStateWaitingForInput}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	const writers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			wake := &models.TaskSessionWake{
				TaskID:             "task-wake",
				SessionID:          "session-wake",
				Marker:             "standup",
				Prompt:             "WAKE:STANDUP",
				CronExpression:     "*/5 * * * *",
				Timezone:           "UTC",
				NextRunAt:          now.Add(time.Duration(i+1) * time.Minute),
				ExpiresAt:          now.Add(24 * time.Hour),
				LastDeliveryStatus: models.SessionWakeDeliveryPending,
			}
			_, err := repo.UpsertTaskSessionWake(ctx, wake)
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("UpsertTaskSessionWake: %v", err)
		}
	}
	wakes, err := repo.ListTaskSessionWakes(ctx, "task-wake", "session-wake")
	if err != nil {
		t.Fatalf("ListTaskSessionWakes: %v", err)
	}
	if len(wakes) != 1 {
		t.Fatalf("wake rows = %d, want 1", len(wakes))
	}
	firstID := wakes[0].ID

	update := &models.TaskSessionWake{
		TaskID:             "task-wake",
		SessionID:          "session-wake",
		Marker:             "standup",
		Prompt:             "WAKE:CYCLE",
		CronExpression:     "*/10 * * * *",
		Timezone:           "UTC",
		NextRunAt:          now.Add(10 * time.Minute),
		ExpiresAt:          now.Add(48 * time.Hour),
		LastDeliveryStatus: models.SessionWakeDeliveryPending,
	}
	if created, err := repo.UpsertTaskSessionWake(ctx, update); err != nil || created {
		t.Fatalf("update UpsertTaskSessionWake created=%v err=%v, want update", created, err)
	}
	wakes, err = repo.ListTaskSessionWakes(ctx, "task-wake", "session-wake")
	if err != nil {
		t.Fatalf("ListTaskSessionWakes after update: %v", err)
	}
	if len(wakes) != 1 {
		t.Fatalf("wake rows after update = %d, want 1", len(wakes))
	}
	if wakes[0].ID != firstID {
		t.Fatalf("wake ID changed from %q to %q", firstID, wakes[0].ID)
	}
	if wakes[0].Prompt != "WAKE:CYCLE" || wakes[0].CronExpression != "*/10 * * * *" {
		t.Fatalf("wake was not updated: %+v", wakes[0])
	}
}

func TestListDueTaskSessionWakesSkipsTerminalDeliveries(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-terminal-wake", Name: "Wake workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-terminal-wake", WorkspaceID: "ws-terminal-wake", Title: "Wake task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-terminal-wake", TaskID: "task-terminal-wake", State: models.TaskSessionStateCompleted}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	wake := &models.TaskSessionWake{
		ID:                 "wake-terminal",
		TaskID:             "task-terminal-wake",
		SessionID:          "session-terminal-wake",
		Marker:             "done",
		Prompt:             "WAKE:DONE",
		CronExpression:     "*/5 * * * *",
		Timezone:           "UTC",
		NextRunAt:          now.Add(-time.Minute),
		ExpiresAt:          now.Add(time.Hour),
		LastDeliveryStatus: models.SessionWakeDeliveryTerminal,
	}
	if _, err := repo.UpsertTaskSessionWake(ctx, wake); err != nil {
		t.Fatalf("UpsertTaskSessionWake: %v", err)
	}

	due, err := repo.ListDueTaskSessionWakes(ctx, now, 100)
	if err != nil {
		t.Fatalf("ListDueTaskSessionWakes: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("terminal wake returned as due: %+v", due)
	}
}
