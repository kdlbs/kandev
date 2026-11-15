package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestHasActiveExecutorRunningRowKeepsFailedRowsActive(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-active-row", "session-active-row", "turn-active-row")

	if active, err := repo.HasActiveExecutorRunningRow(ctx, "session-active-row"); err != nil || active {
		t.Fatalf("no row: active = %v, err = %v, want false", active, err)
	}

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: "session-active-row", TaskID: "task-active-row",
		AgentExecutionID: "execution-one", Status: models.ExecutorRunningStatusRunning,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	cases := []struct {
		status string
		active bool
	}{
		{models.ExecutorRunningStatusPrepared, true},
		{models.ExecutorRunningStatusStarting, true},
		{models.ExecutorRunningStatusRunning, true},
		{models.ExecutorRunningStatusReady, true},
		{models.ExecutorRunningStatusStopped, false},
		{models.ExecutorRunningStatusFailed, true},
		{models.ExecutorRunningStatusComplete, false},
	}
	for _, tc := range cases {
		if err := repo.UpdateExecutorRunningStatus(ctx, "session-active-row", tc.status); err != nil {
			t.Fatalf("UpdateExecutorRunningStatus(%s): %v", tc.status, err)
		}
		active, err := repo.HasActiveExecutorRunningRow(ctx, "session-active-row")
		if err != nil || active != tc.active {
			t.Fatalf("status %q: active = %v, err = %v, want %v", tc.status, active, err, tc.active)
		}
		// The row itself is still retained for resume, so the launched check stays true.
		if exists, err := repo.HasExecutorRunningRow(ctx, "session-active-row"); err != nil || !exists {
			t.Fatalf("status %q: HasExecutorRunningRow = %v, err = %v, want true", tc.status, exists, err)
		}
	}
}
