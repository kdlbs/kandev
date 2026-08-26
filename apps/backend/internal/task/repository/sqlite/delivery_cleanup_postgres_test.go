package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresDeleteSessionCancelsDeliveryReceipt proves session cleanup uses
// the same durable cancellation rule on PostgreSQL as SQLite.
func TestPostgresDeleteSessionCancelsDeliveryReceipt(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-pg-delivery", Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-pg-delivery", TaskID: "task-pg-delivery"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	queue, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	ledger := queue.(messagequeue.DeliveryLedger)
	delivery, _, err := ledger.CreateOrGetDelivery(ctx, messagequeue.Delivery{
		SenderTaskID: "source-task", SenderSessionID: "source-session", SourceTurnID: "source-turn",
		IdempotencyKey: "report-v1", TargetTaskID: "task-pg-delivery", TargetSessionID: "session-pg-delivery",
		Content: "review is ready", State: messagequeue.DeliveryPendingCapacity,
	})
	if err != nil {
		t.Fatalf("CreateOrGetDelivery: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, "session-pg-delivery"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	stored, err := ledger.GetDelivery(ctx, delivery.ID)
	if err != nil {
		t.Fatalf("GetDelivery: %v", err)
	}
	if stored.State != messagequeue.DeliveryCancelled {
		t.Fatalf("delivery state = %q, want %q", stored.State, messagequeue.DeliveryCancelled)
	}
}
