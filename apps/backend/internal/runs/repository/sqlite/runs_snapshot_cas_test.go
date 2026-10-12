package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestUpdateRunRuntimeSnapshotCAS_WinsWhenPrevMatches covers the ordinary
// first-writer case: the run's current capabilities equal prevCapabilities,
// so the write takes effect and the bool reports true.
func TestUpdateRunRuntimeSnapshotCAS_WinsWhenPrevMatches(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, &models.Run{
		ID: "run-cas-win", AgentProfileID: "agent-1", Reason: "heartbeat",
		Capabilities: `{"task_scope_source":""}`,
	})

	won, err := repo.UpdateRunRuntimeSnapshotCAS(
		ctx, run.ID, `{"task_scope_source":""}`, `{"task_scope_source":"runner_set"}`, `{"in":"new"}`, "sess-1",
	)
	if err != nil {
		t.Fatalf("UpdateRunRuntimeSnapshotCAS: %v", err)
	}
	if !won {
		t.Fatal("expected CAS to win when prevCapabilities matches")
	}

	got := mustGetRun(t, repo, run.ID)
	checkString(t, "capabilities", got.Capabilities, `{"task_scope_source":"runner_set"}`)
	checkString(t, "input_snapshot", got.InputSnapshot, `{"in":"new"}`)
	checkString(t, "session_id", got.SessionID, "sess-1")
}

// TestUpdateRunRuntimeSnapshotCAS_LosesWhenPrevStale covers the race: a
// second processor already wrote a different capabilities value, so this
// call's prevCapabilities is stale and the write must not take effect.
func TestUpdateRunRuntimeSnapshotCAS_LosesWhenPrevStale(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, &models.Run{
		ID: "run-cas-lose", AgentProfileID: "agent-1", Reason: "heartbeat",
		Capabilities: `{"task_scope_source":"runner_set","allowed_task_ids":["t1"]}`,
	})

	won, err := repo.UpdateRunRuntimeSnapshotCAS(
		ctx, run.ID, `{"task_scope_source":""}`, `{"task_scope_source":"runner_set","allowed_task_ids":["t2"]}`, `{}`, "sess-2",
	)
	if err != nil {
		t.Fatalf("UpdateRunRuntimeSnapshotCAS: %v", err)
	}
	if won {
		t.Fatal("expected CAS to lose when prevCapabilities is stale")
	}

	got := mustGetRun(t, repo, run.ID)
	checkString(t, "capabilities unchanged", got.Capabilities, `{"task_scope_source":"runner_set","allowed_task_ids":["t1"]}`)
	checkString(t, "session_id unchanged", got.SessionID, "")
}

// TestUpdateRunRuntimeSnapshotCAS_UnaffectedRunsUntouched proves the swap
// is scoped to the target row only.
func TestUpdateRunRuntimeSnapshotCAS_UnaffectedRunsUntouched(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, &models.Run{
		ID: "run-cas-a", AgentProfileID: "agent-1", Reason: "heartbeat", Capabilities: `{}`,
	})
	other := mustCreateRun(t, repo, &models.Run{
		ID: "run-cas-b", AgentProfileID: "agent-2", Reason: "heartbeat", Capabilities: `{"untouched":true}`,
	})

	if _, err := repo.UpdateRunRuntimeSnapshotCAS(ctx, run.ID, `{}`, `{"cap":"new"}`, `{}`, "sess"); err != nil {
		t.Fatalf("UpdateRunRuntimeSnapshotCAS: %v", err)
	}

	untouched := mustGetRun(t, repo, other.ID)
	checkString(t, "other capabilities", untouched.Capabilities, `{"untouched":true}`)
}
