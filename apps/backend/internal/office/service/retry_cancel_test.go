package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestCancelPendingRetriesForTask_RecordsTerminalShape is the Testing
// round 3 regression test. CancelPendingRetriesForTask calls
// repo.BulkCancelRuns directly, which bypassed office_loop_terminal_total
// entirely — the same bypass class Review round 1 (R1-1) fixed for
// HandleAgentFailure and the single-run CancelRun call sites, just missed
// here. A pending retry never launched, so cancelling it must classify
// as unlaunched_failed.
func TestCancelPendingRetriesForTask_RecordsTerminalShape(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("retry-cancel-worker", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	taskID := "task-retry-cancel"
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, title, description, created_at, updated_at)
		VALUES (?, ?, 'Build API', 'Implement endpoint', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		taskID, agent.WorkspaceID)

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: runs=%v err=%v", runs, err)
	}
	run := runs[0]
	if err := svc.RepoForTest().ScheduleRetry(ctx, run.ID, run.RequestedAt.Add(time.Hour), 1); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	key := service.LoopMetricLabel("workspace", agent.WorkspaceID, "shape", string(service.ShapeUnlaunchedFailed))
	before := terminalShapeExpvarInt(t, key)

	if err := svc.CancelPendingRetriesForTask(ctx, taskID); err != nil {
		t.Fatalf("cancel pending retries: %v", err)
	}

	after := terminalShapeExpvarInt(t, key)
	if after != before+1 {
		t.Fatalf("unlaunched_failed delta = %d, want 1", after-before)
	}
}
