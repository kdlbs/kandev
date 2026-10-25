package service_test

// This file proves the specific defect run-dedup-generation fixes: a
// task_assigned dedup key that used to be stable for the life of a
// (task, agent) pair (docs/specs/office/requirements/run-dedup-generation.md,
// AC-OFFICE-SCHEDULER-001.7). idx_run_idempotency has no time bound, so a
// stable key collided with the UNIQUE index forever, making a re-assignment
// of the same task to the same agent a silent, permanent no-op. These tests
// exercise the real bus -> handleTaskUpdated -> queueTaskAssignedRun path
// (not just the dedupkeys.AssignmentKey unit) and assert on the persisted
// idempotency_key, which none of the existing task_assigned tests do.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/runs/dedupkeys"
)

func publishTaskAssigned(
	t *testing.T, ctx context.Context, eb bus.EventBus, taskID, agentID string, generation *int64,
) {
	t.Helper()
	data := map[string]any{
		"task_id":                   taskID,
		"assignee_agent_profile_id": agentID,
	}
	if generation != nil {
		data["assignment_generation"] = *generation
	}
	event := bus.NewEvent(events.TaskUpdated, "test", data)
	if err := eb.Publish(ctx, events.TaskUpdated, event); err != nil {
		t.Fatalf("publish task updated event: %v", err)
	}
}

func gen(n int64) *int64 { return &n }

// taskAssignedRunsFor returns every task_assigned run queued for agentID on
// the given task, in insertion order (ListRuns has no ordering guarantee
// this test relies on beyond "all rows present").
func taskAssignedRunsFor(t *testing.T, svc *service.Service, wsID, taskID, agentID string) []string {
	t.Helper()
	runs, err := svc.ListRuns(context.Background(), wsID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var keys []string
	for _, run := range runs {
		if run.AgentProfileID != agentID || run.Reason != service.RunReasonTaskAssigned {
			continue
		}
		var payload struct {
			TaskID string `json:"task_id"`
		}
		if err := json.Unmarshal([]byte(run.Payload), &payload); err != nil {
			t.Fatalf("decode run payload: %v", err)
		}
		if payload.TaskID != taskID {
			continue
		}
		if run.IdempotencyKey == nil {
			keys = append(keys, "")
			continue
		}
		keys = append(keys, *run.IdempotencyKey)
	}
	return keys
}

func TestTaskAssigned_SameGenerationRedelivery_DedupesForever(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-gen")
	insertTestTask(t, svc, "task-gen-redeliver", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, "task-gen-redeliver")

	publishTaskAssigned(t, ctx, eb, "task-gen-redeliver", "worker-gen", gen(0))
	publishTaskAssigned(t, ctx, eb, "task-gen-redeliver", "worker-gen", gen(0))

	keys := taskAssignedRunsFor(t, svc, "ws-1", "task-gen-redeliver", "worker-gen")
	if len(keys) != 1 {
		t.Fatalf("redelivering the same generation queued %d runs, want exactly 1 (deduped): keys=%#v", len(keys), keys)
	}
	want := dedupkeys.AssignmentKey("task-gen-redeliver", "worker-gen", 0)
	if keys[0] != want {
		t.Fatalf("idempotency key = %q, want %q", keys[0], want)
	}
}

// TestTaskAssigned_GenerationBump_AvoidsPermanentCollision is the direct
// regression test for the reported defect: reassigning the same task to the
// same agent must NOT be a permanent no-op once the durable unique index
// already holds a row for an earlier generation's key. Unlike
// TestTaskAssigned_ReassignmentUsesAgentScopedIdempotency (which reassigns to
// a *different* agent and never sets assignment_generation, so it never
// exercised the keyed path at all), this drives the same agent through two
// generations to prove the key itself changed.
func TestTaskAssigned_GenerationBump_AvoidsPermanentCollision(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-regen")
	insertTestTask(t, svc, "task-regen", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, "task-regen")

	// Occurrence 1: original assignment at generation 0. In production this
	// row's idempotency_key (task_assigned:task-regen:worker-regen:0) is
	// exactly the durable UNIQUE row that used to collide forever under the
	// pre-fix key format (task_assigned:<task>:<agent>, no generation).
	publishTaskAssigned(t, ctx, eb, "task-regen", "worker-regen", gen(0))

	// The first run is claimed and finishes (agent did the work) long before
	// the reassignment — CoalesceRun only merges into a still-'queued' row,
	// so this ensures the second occurrence exercises the durable
	// idx_run_idempotency INSERT path, not the 5-second coalesce window.
	// This is exactly the "more than 24 hours later" gap in the bug report.
	svc.ExecSQL(t, `UPDATE runs SET status = 'completed' WHERE status = 'queued'`)

	// Simulate a real reassignment: UpdateTaskAssignee bumps the stored
	// generation before the task.updated event carrying it is published.
	svc.ExecSQL(t, `UPDATE tasks SET assignment_generation = 1 WHERE id = ?`, "task-regen")
	publishTaskAssigned(t, ctx, eb, "task-regen", "worker-regen", gen(1))

	keys := taskAssignedRunsFor(t, svc, "ws-1", "task-regen", "worker-regen")
	if len(keys) != 2 {
		t.Fatalf("reassigning the same agent at a new generation queued %d runs, want 2 (no false collision): keys=%#v", len(keys), keys)
	}
	wantGen0 := dedupkeys.AssignmentKey("task-regen", "worker-regen", 0)
	wantGen1 := dedupkeys.AssignmentKey("task-regen", "worker-regen", 1)
	matchesInOrder := keys[0] == wantGen0 && keys[1] == wantGen1
	matchesReversed := keys[0] == wantGen1 && keys[1] == wantGen0
	if !matchesInOrder && !matchesReversed {
		t.Fatalf("keys = %#v, want %q and %q (one run per generation)", keys, wantGen0, wantGen1)
	}
	if keys[0] == keys[1] {
		t.Fatalf("both runs share idempotency key %q — this is the reported defect: a reassignment collided with the durable unique index", keys[0])
	}

	// Redelivering generation 1 again — past the coalesce window and after
	// the generation-1 run has itself been claimed — must still dedupe via
	// the durable unique index: the fix must not turn every event into a
	// fresh key, only a genuine generation change.
	svc.ExecSQL(t, `UPDATE runs SET status = 'completed' WHERE status = 'queued'`)
	publishTaskAssigned(t, ctx, eb, "task-regen", "worker-regen", gen(1))
	keysAfterRedeliver := taskAssignedRunsFor(t, svc, "ws-1", "task-regen", "worker-regen")
	if len(keysAfterRedeliver) != 2 {
		t.Fatalf("redelivering generation 1 queued a 3rd run: keys=%#v", keysAfterRedeliver)
	}
}

// TestTaskAssigned_MissingGeneration_EnqueuesKeyless proves SR-55's
// resolution: an event carrying no assignment_generation must never mint a
// key with a guessed or zero generation — it goes keyless (empty
// idempotency_key) so a possible duplicate wake is accepted (AC-003.2)
// instead of a mis-keyed row that could collide with an unrelated
// occurrence.
func TestTaskAssigned_MissingGeneration_EnqueuesKeyless(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "worker-nogen")
	insertTestTask(t, svc, "task-nogen", "ws-1")
	svc.ExecSQL(t, `UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, "task-nogen")

	publishTaskAssigned(t, ctx, eb, "task-nogen", "worker-nogen", nil)

	keys := taskAssignedRunsFor(t, svc, "ws-1", "task-nogen", "worker-nogen")
	if len(keys) != 1 {
		t.Fatalf("got %d runs, want 1: keys=%#v", len(keys), keys)
	}
	if keys[0] != "" {
		t.Fatalf("idempotency key = %q, want empty (keyless enqueue)", keys[0])
	}
}
