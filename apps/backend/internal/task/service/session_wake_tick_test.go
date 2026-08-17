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
}

func (d *recordingSessionWakeDeliverer) DeliverSessionWake(_ context.Context, _, _, wakeID, _ string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, wakeID)
	return d.state, nil
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
