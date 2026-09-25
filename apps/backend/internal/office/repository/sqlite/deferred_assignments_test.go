package sqlite_test

import (
	"context"
	"testing"
)

// TestResolveDeferredAssignment_CASKeysOnFullIdentityNotJustTaskID is the
// SQLite twin of the Postgres-gated CAS-only-once coverage: it proves the
// resolve CAS keys on the full identity a reader captured — task_id,
// assignment_generation, agent_profile_id, pause_id — not task_id alone
// (R1-F2). Interleave: a replay reads row (task-1, gen1), a fresh deferral
// for the same task overwrites it to gen2 before the replay resolves, and
// the replay's stale gen1 resolve must lose the CAS rather than marking
// the still-pending gen2 row resolved.
func TestResolveDeferredAssignment_CASKeysOnFullIdentityNotJustTaskID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.RecordDeferredAssignment(ctx, "task-1", "ws-1", "agent-1", 1, "pause-1"); err != nil {
		t.Fatalf("record gen1: %v", err)
	}

	// A reassignment during the same pause overwrites the row to gen2
	// before the gen1 reader below gets to resolve it.
	if _, err := repo.RecordDeferredAssignment(ctx, "task-1", "ws-1", "agent-2", 2, "pause-1"); err != nil {
		t.Fatalf("record gen2 (overwrite): %v", err)
	}

	// Resolve using the stale gen1 identity an earlier reader captured
	// before the overwrite above landed.
	won, err := repo.ResolveDeferredAssignment(ctx, "task-1", 1, "agent-1", "pause-1", "replayed")
	if err != nil {
		t.Fatalf("resolve with stale gen1 identity: %v", err)
	}
	if won {
		t.Fatal("resolve with the stale gen1 identity must lose the CAS: the row now holds gen2")
	}

	pending, err := repo.ListPendingDeferredAssignmentsForWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending count = %d, want 1: %+v", len(pending), pending)
	}
	row := pending[0]
	if row.AssignmentGeneration != 2 || row.AgentProfileID != "agent-2" {
		t.Fatalf("gen2's deferral was lost to the stale gen1 resolve: %+v, want gen2/agent-2", row)
	}
	if row.ResolvedAt != nil {
		t.Fatalf("gen2's row must remain pending after the stale resolve, got resolved_at=%v", row.ResolvedAt)
	}
}

// TestRecordDeferredAssignment_DoesNotRegressToOlderGeneration is the
// SQLite twin of the Postgres-gated upsert coverage: it proves the upsert
// fallback never regresses a still-pending row to an older generation
// (R1-F3). Interleave: replay-time reader A read gen1 for task-1; before
// A's own deferral write lands, a second assignment (B) already committed
// gen2 for the same task. A's late write must not overwrite B's gen2.
func TestRecordDeferredAssignment_DoesNotRegressToOlderGeneration(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// B's assignment (gen2) commits first...
	if _, err := repo.RecordDeferredAssignment(ctx, "task-1", "ws-1", "agent-2", 2, "pause-1"); err != nil {
		t.Fatalf("record gen2: %v", err)
	}
	// ...then A's stale write (gen1, captured before B committed) lands —
	// the generation guard must suppress it and report recorded=false, so
	// the caller knows not to log a "deferred" activity entry for it.
	recorded, err := repo.RecordDeferredAssignment(ctx, "task-1", "ws-1", "agent-1", 1, "pause-1")
	if err != nil {
		t.Fatalf("record gen1 (stale, must not regress): %v", err)
	}
	if recorded {
		t.Fatal("stale gen1 write must report recorded=false: the guard suppressed it")
	}

	pending, err := repo.ListPendingDeferredAssignmentsForWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending count = %d, want 1: %+v", len(pending), pending)
	}
	row := pending[0]
	if row.AssignmentGeneration != 2 || row.AgentProfileID != "agent-2" {
		t.Fatalf("stale gen1 write regressed the row: %+v, want gen2/agent-2", row)
	}
}

// TestRecordDeferredAssignment_OverwritesAResolvedRowRegardlessOfGeneration
// proves the generation guard in RecordDeferredAssignment only blocks
// regressing a still-*pending* row — a fresh pause after an earlier
// deferral already resolved must still record normally even if its
// generation happens to be lower than the resolved row's (e.g. the task's
// assignment_generation counter is scoped independently of this guard).
func TestRecordDeferredAssignment_OverwritesAResolvedRowRegardlessOfGeneration(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.RecordDeferredAssignment(ctx, "task-1", "ws-1", "agent-1", 5, "pause-1"); err != nil {
		t.Fatalf("record gen5: %v", err)
	}
	won, err := repo.ResolveDeferredAssignment(ctx, "task-1", 5, "agent-1", "pause-1", "replayed")
	if err != nil {
		t.Fatalf("resolve gen5: %v", err)
	}
	if !won {
		t.Fatal("expected the resolve to win the CAS")
	}

	// A fresh pause + deferral for the same task, generation lower than
	// the already-resolved row, must still overwrite: the row is resolved,
	// so the generation guard does not apply.
	if _, err := repo.RecordDeferredAssignment(ctx, "task-1", "ws-1", "agent-3", 2, "pause-2"); err != nil {
		t.Fatalf("record gen2 over a resolved row: %v", err)
	}

	pending, err := repo.ListPendingDeferredAssignmentsForWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending count = %d, want 1: %+v", len(pending), pending)
	}
	row := pending[0]
	if row.AssignmentGeneration != 2 || row.AgentProfileID != "agent-3" || row.PauseID != "pause-2" {
		t.Fatalf("resolved row was not overwritten by the fresh deferral: %+v", row)
	}
}
