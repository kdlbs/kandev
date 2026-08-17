package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type recordingSessionWakeDeliverer struct {
	mu    sync.Mutex
	calls []string
	state string
	err   error
}

func (d *recordingSessionWakeDeliverer) DeliverSessionWake(_ context.Context, _, _, wakeID, _ string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, wakeID)
	return d.state, d.err
}

func (d *recordingSessionWakeDeliverer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

func TestTickSessionWakes_RecursWithoutCatchUpStorm(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateIdle}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	wake := &models.TaskSessionWake{
		ID:             "wake-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		Marker:         "daily",
		Prompt:         "continue",
		CronExpression: "*/5 * * * *",
		Timezone:       "UTC",
		NextRunAt:      now.Add(-time.Hour),
		ExpiresAt:      now.Add(time.Hour),
	}
	if _, err := repo.UpsertTaskSessionWake(ctx, wake); err != nil {
		t.Fatalf("UpsertTaskSessionWake: %v", err)
	}

	deliverer := &recordingSessionWakeDeliverer{state: models.SessionWakeDeliveryQueued}
	if err := svc.TickSessionWakes(ctx, deliverer, now); err != nil {
		t.Fatalf("first TickSessionWakes: %v", err)
	}
	if got := deliverer.count(); got != 1 {
		t.Fatalf("deliveries after delayed tick = %d, want 1", got)
	}
	if err := svc.TickSessionWakes(ctx, deliverer, now); err != nil {
		t.Fatalf("second TickSessionWakes: %v", err)
	}
	if got := deliverer.count(); got != 1 {
		t.Fatalf("deliveries after same tick = %d, want 1", got)
	}

	stored, err := repo.ListTaskSessionWakes(ctx, wake.TaskID, wake.SessionID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("ListTaskSessionWakes = %#v, %v", stored, err)
	}
	if !stored[0].NextRunAt.After(now) {
		t.Fatalf("next run %s must be after %s", stored[0].NextRunAt, now)
	}
	if stored[0].LastDeliveryStatus != models.SessionWakeDeliveryQueued {
		t.Fatalf("delivery status = %q, want queued", stored[0].LastDeliveryStatus)
	}
}

func TestTickSessionWakesRecordsTerminalDeliveryAndStopsRetrying(t *testing.T) {
	svc, _, repo := createTestService(t)
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
		NextRunAt:          now.Add(-time.Hour),
		ExpiresAt:          now.Add(time.Hour),
		LastDeliveryStatus: models.SessionWakeDeliveryPending,
	}
	if _, err := repo.UpsertTaskSessionWake(ctx, wake); err != nil {
		t.Fatalf("UpsertTaskSessionWake: %v", err)
	}
	deliverer := &recordingSessionWakeDeliverer{state: models.SessionWakeDeliveryTerminal, err: models.NewSessionWakeTerminalDeliveryError("session completed")}

	if err := svc.TickSessionWakes(ctx, deliverer, now); err != nil {
		t.Fatalf("TickSessionWakes: %v", err)
	}
	stored, err := repo.ListTaskSessionWakes(ctx, "task-terminal-wake", "session-terminal-wake")
	if err != nil {
		t.Fatalf("ListTaskSessionWakes: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored wakes = %d, want 1", len(stored))
	}
	if stored[0].LastDeliveryStatus != models.SessionWakeDeliveryTerminal {
		t.Fatalf("delivery status = %q, want terminal", stored[0].LastDeliveryStatus)
	}
	if stored[0].LastError == "" {
		t.Fatal("terminal wake should retain an inspectable error")
	}

	if err := svc.TickSessionWakes(ctx, deliverer, now.Add(10*time.Minute)); err != nil {
		t.Fatalf("second TickSessionWakes: %v", err)
	}
	if got := deliverer.count(); got != 1 {
		t.Fatalf("terminal wake retried %d times, want 1 delivery attempt", got)
	}
}

func TestUpsertSessionWakeRejectsInvalidTimezoneForEveryExpression(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-wake-tz", Name: "Wake workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-wake-tz", WorkspaceID: "ws-wake-tz", Title: "Wake task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-wake-tz", TaskID: "task-wake-tz", State: models.TaskSessionStateWaitingForInput}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	_, _, err := svc.UpsertSessionWake(ctx, "task-wake-tz", "session-wake-tz", UpsertSessionWakeRequest{
		Marker:         "cycle",
		Prompt:         "WAKE:CYCLE",
		CronExpression: "@every 1m",
		Timezone:       "Mars/Olympus",
		ExpiresAt:      time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Fatal("UpsertSessionWake accepted an invalid timezone")
	}
}
