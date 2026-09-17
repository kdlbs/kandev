package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	runsservice "github.com/kandev/kandev/internal/runs/service"
)

func TestIndividuallyAddressedCallbacksDoNotOverwriteEachOther(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()
	for _, id := range []string{"first", "second", "first"} {
		_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{Reason: "workspace_task_callback", DisableCoalescing: true, IdempotencyKey: "callback:" + id, Payload: map[string]any{"agent_profile_id": "chief", "task_id": "conversation", "callback": map[string]string{"task_id": id}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for {
		run, err := repo.ClaimNextEligibleRun(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
		if err := repo.FinishRun(ctx, run.ID, "finished", nil); err != nil {
			t.Fatal(err)
		}
	}
	if count != 2 {
		t.Fatalf("expected two durable callback runs, got %d", count)
	}
}
