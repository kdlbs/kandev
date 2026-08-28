package sqlite_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seedWakeCandidate creates a parent task with one non-archived, terminal
// (COMPLETED) child and a resolvable runner backed by a real agent_profiles
// row, so it satisfies every one of ListStuckParents' predicates on its
// own — including the assignee filter (R2-A), the INNER JOIN against
// agent_profiles (R3-B), and the authoritative-Office-task predicate
// (project_id set, matching scheduler_recovery_test.go's own
// 'office-project' marker) — and would actually be woken end-to-end by the
// full reconciler, not just accepted by this SQL layer in isolation.
// Returns the child_set_key ListStuckParents will compute for it.
func seedWakeCandidate(t *testing.T, repo *sqlite.Repository, ctx context.Context, wsID, parentID string) string {
	t.Helper()
	insertTask(t, repo, ctx, parentID, wsID, "Parent", "", "")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, parentID,
	); err != nil {
		t.Fatalf("mark parent as Office task: %v", err)
	}
	childID := parentID + "-child-0"
	insertTask(t, repo, ctx, childID, wsID, "Child", "", "")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = ?, state = 'COMPLETED' WHERE id = ?`, parentID, childID,
	); err != nil {
		t.Fatalf("set child state: %v", err)
	}
	agentID := parentID + "-agent"
	seedWakeAgentProfile(t, repo, ctx, agentID, "idle")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id)
		VALUES (?, '', ?, 'runner', ?)
	`, "p-runner-"+parentID, parentID, agentID); err != nil {
		t.Fatalf("seed runner: %v", err)
	}
	return childID + ":COMPLETED"
}

// seedWakeAgentProfile inserts (or, for a second call against the same
// agentID, replaces) an agent_profiles row with the given status, using
// the same columns TestListStuckParents_ExcludesUnresolvedOrPausedAssignee
// always has: agent_profiles is created by settingsstore.Provide (see
// newSearchTestRepo) with its full production schema and NOT NULL columns.
func seedWakeAgentProfile(t *testing.T, repo *sqlite.Repository, ctx context.Context, agentID, status string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT OR REPLACE INTO agent_profiles (id, agent_id, name, agent_display_name, status, created_at, updated_at)
		VALUES (?, '', ?, ?, ?, datetime('now'), datetime('now'))
	`, agentID, agentID, agentID, status); err != nil {
		t.Fatalf("seed agent profile %s: %v", agentID, err)
	}
}

// seedWakeReceipt records a legacy run-backed receipt for parentID as already
// delivered for childSetKey. It also seeds a backing 'finished' run for the
// receipt's delivered_run_id. Workflow-engine receipts use the operation-id
// column instead; this helper keeps the legacy shape covered because existing
// databases can still contain those rows.
func seedWakeReceipt(t *testing.T, repo *sqlite.Repository, ctx context.Context, parentID, childSetKey string) {
	t.Helper()
	runID := "prior-run-" + parentID
	seedWakeRun(t, repo, ctx, runID, parentID, "task_children_completed", "finished")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO parent_child_wake_receipts (parent_task_id, child_set_key, delivered_run_id, delivered_at)
		VALUES (?, ?, ?, datetime('now'))
	`, parentID, childSetKey, runID); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
}

// seedWakeRun inserts a runs row for parentID the way CreateRunTx would,
// so tests can simulate a wake already queued/claimed/finished/failed for
// this parent without going through the full reconciler.
func seedWakeRun(t *testing.T, repo *sqlite.Repository, ctx context.Context, runID, parentID, reason, status string) {
	t.Helper()
	seedWakeRunAt(t, repo, ctx, runID, parentID, reason, status, "datetime('now')")
}

// seedWakeRunAt is seedWakeRun with an explicit requested_at expression
// (a SQL literal or expression, not a bound parameter — callers pass
// either "datetime('now')" or a quoted absolute timestamp literal), so
// tests can control a run's ordering relative to child updates without
// relying on wall-clock delay between statements.
func seedWakeRunAt(t *testing.T, repo *sqlite.Repository, ctx context.Context, runID, parentID, reason, status, requestedAtExpr string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, fmt.Sprintf(`
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES (?, 'agent-x', ?, ?, ?, %s)
	`, requestedAtExpr), runID, reason, fmt.Sprintf(`{"task_id":%q}`, parentID), status); err != nil {
		t.Fatalf("seed run: %v", err)
	}
}

// insertTaskAt is insertTask with an explicit created_at/updated_at
// timestamp, so tests can control a child's ordering relative to a run's
// requested_at without relying on wall-clock delay between statements.
func insertTaskAt(t *testing.T, repo *sqlite.Repository, ctx context.Context, id, wsID, timestamp string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, description, identifier, created_at, updated_at)
		VALUES (?, ?, 'Task', '', '', ?, ?)
	`, id, wsID, timestamp, timestamp); err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

// setChildStateAt sets a child's parent, state, and updated_at in one
// statement, mirroring production's state-transition writes (which always
// bump updated_at alongside state) rather than the plain
// `UPDATE tasks SET parent_id = ?, state = ?` other helpers in this package
// use — those are fine for tests that don't assert on timestamps, but a
// timestamp-comparison predicate needs a child's updated_at to actually
// move when its state does.
func setChildStateAt(t *testing.T, repo *sqlite.Repository, ctx context.Context, parentID, childID, state, timestamp string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = ?, state = ?, updated_at = ? WHERE id = ?`,
		parentID, state, timestamp, childID,
	); err != nil {
		t.Fatalf("set child state: %v", err)
	}
}

// seedRunner inserts a workflow_step_participants runner row for parentID,
// the same shape seedWakeCandidate writes, factored out so callers that
// build up a candidate's children by hand can still get a resolvable
// runner without duplicating this insert.
func seedRunner(t *testing.T, repo *sqlite.Repository, ctx context.Context, parentID string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id)
		VALUES (?, '', ?, 'runner', ?)
	`, "p-runner-"+parentID, parentID, parentID+"-agent"); err != nil {
		t.Fatalf("seed runner: %v", err)
	}
}

func TestGetChildSetKey_UsesActiveChildren(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-1", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "child-b", "ws-1", "Child B", "", "")
	insertTask(t, repo, ctx, "child-a", "ws-1", "Child A", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET parent_id = ?, state = ? WHERE id = ?
	`, "parent-1", "COMPLETED", "child-b"); err != nil {
		t.Fatalf("set child-b: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET parent_id = ?, state = ?, archived_at = CURRENT_TIMESTAMP WHERE id = ?
	`, "parent-1", "FAILED", "child-a"); err != nil {
		t.Fatalf("archive child-a: %v", err)
	}
	insertTask(t, repo, ctx, "child-c", "ws-1", "Child C", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET parent_id = ?, state = ? WHERE id = ?
	`, "parent-1", "CANCELLED", "child-c"); err != nil {
		t.Fatalf("set child-c: %v", err)
	}

	got, err := repo.GetChildSetKey(ctx, "parent-1")
	if err != nil {
		t.Fatalf("GetChildSetKey: %v", err)
	}
	if got != "child-b:COMPLETED,child-c:CANCELLED" {
		t.Fatalf("child set key = %q, want child-b:COMPLETED,child-c:CANCELLED", got)
	}
}

// TestListStuckParents_LimitDoesNotStarveLaterCandidates is R1-A's
// regression test: candidates with an already-current receipt (nothing to
// do) must not consume LIMIT slots ahead of candidates that genuinely need
// a wake. Before the fix, the receipt comparison ran in Go after the SQL
// LIMIT, so the first 5 "resting" parents in id order permanently starved
// every parent past them.
func TestListStuckParents_LimitDoesNotStarveLaterCandidates(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	// p1..p5: already delivered for their current (unchanged) child set —
	// not actionable.
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("p%d", i)
		key := seedWakeCandidate(t, repo, ctx, "ws-1", id)
		seedWakeReceipt(t, repo, ctx, id, key)
	}
	// p6..p8: no receipt at all — genuinely stuck, need a wake.
	want := map[string]bool{}
	for i := 6; i <= 8; i++ {
		id := fmt.Sprintf("p%d", i)
		seedWakeCandidate(t, repo, ctx, "ws-1", id)
		want[id] = true
	}

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 3 {
		t.Fatalf("candidates = %d, want 3 (p6,p7,p8): %#v", len(candidates), candidates)
	}
	got := map[string]bool{}
	for _, c := range candidates {
		got[c.ParentTaskID] = true
	}
	for id := range want {
		if !got[id] {
			t.Errorf("expected %s among candidates, got %v", id, got)
		}
	}
}

// TestListStuckParents_ExcludesParentWithCoveredRun is R1-B's
// regression test: a parent whose wake was already delivered via the
// edge-triggered path (queueChildrenCompletedRun / cascadeChildrenCompleted)
// never gets a receipt written for it — those paths don't write one — so
// only a runs-backed check, not the receipt, can tell the sweep the wake was
// already delivered. Terminal failures are covered too: the runtime contract
// requires an explicit user retry, so the reconciler must not retry them.
func TestListStuckParents_ExcludesParentWithCoveredRun(t *testing.T) {
	const reason = "task_children_completed"

	blocking := []string{"queued", "claimed", "finished", "failed", "cancelled"}
	for _, status := range blocking {
		t.Run(status, func(t *testing.T) {
			repo := newSearchTestRepo(t)
			ctx := context.Background()

			seedWakeCandidate(t, repo, ctx, "ws-1", "parent-1")
			seedWakeRunAt(t, repo, ctx, "run-1", "parent-1", reason, status, "datetime('now', '+1 second')")

			candidates, err := repo.ListStuckParents(ctx, reason, 5)
			if err != nil {
				t.Fatalf("ListStuckParents: %v", err)
			}
			if len(candidates) != 0 {
				t.Fatalf("status %q: expected the already-delivered parent to be excluded, got %#v", status, candidates)
			}
		})
	}
}

// TestListStuckParents_TerminalRunDoesNotRetry verifies that a receipt whose
// delivered run failed or was cancelled still suppresses automatic retries.
// The Office runtime contract makes adapter failures terminal until an
// explicit user action starts a new run.
func TestListStuckParents_TerminalRunDoesNotRetry(t *testing.T) {
	const reason = "task_children_completed"

	terminal := []string{"failed", "cancelled"}
	for _, status := range terminal {
		t.Run(status, func(t *testing.T) {
			repo := newSearchTestRepo(t)
			ctx := context.Background()

			key := seedWakeCandidate(t, repo, ctx, "ws-1", "parent-1")
			seedWakeRun(t, repo, ctx, "run-1", "parent-1", reason, status)
			if _, err := repo.ExecRaw(ctx, `
				INSERT INTO parent_child_wake_receipts (parent_task_id, child_set_key, delivered_run_id, delivered_at)
				VALUES ('parent-1', ?, 'run-1', datetime('now'))
			`, key); err != nil {
				t.Fatalf("seed receipt: %v", err)
			}

			candidates, err := repo.ListStuckParents(ctx, reason, 5)
			if err != nil {
				t.Fatalf("ListStuckParents: %v", err)
			}
			if len(candidates) != 0 {
				t.Fatalf("status %q: terminal run must not be retried automatically: candidates = %#v", status, candidates)
			}
		})
	}

	// Control: a receipt whose referenced run actually finished must still
	// permanently exclude the parent for that unchanged child set.
	t.Run("finished_stays_excluded", func(t *testing.T) {
		repo := newSearchTestRepo(t)
		ctx := context.Background()

		key := seedWakeCandidate(t, repo, ctx, "ws-1", "parent-1")
		seedWakeRun(t, repo, ctx, "run-1", "parent-1", reason, "finished")
		if _, err := repo.ExecRaw(ctx, `
			INSERT INTO parent_child_wake_receipts (parent_task_id, child_set_key, delivered_run_id, delivered_at)
			VALUES ('parent-1', ?, 'run-1', datetime('now'))
		`, key); err != nil {
			t.Fatalf("seed receipt: %v", err)
		}

		candidates, err := repo.ListStuckParents(ctx, reason, 5)
		if err != nil {
			t.Fatalf("ListStuckParents: %v", err)
		}
		if len(candidates) != 0 {
			t.Fatalf("finished delivery: candidates = %#v, want none", candidates)
		}
	})

	// An edge-triggered terminal run has no reconciler receipt, but it still
	// represents an attempted wake for the current child set. It must not be
	// retried on every tick.
	for _, status := range terminal {
		t.Run("edge_"+status, func(t *testing.T) {
			repo := newSearchTestRepo(t)
			ctx := context.Background()

			seedWakeCandidate(t, repo, ctx, "ws-1", "parent-1")
			seedWakeRunAt(t, repo, ctx, "run-1", "parent-1", reason, status, "datetime('now', '+1 second')")

			candidates, err := repo.ListStuckParents(ctx, reason, 5)
			if err != nil {
				t.Fatalf("ListStuckParents: %v", err)
			}
			if len(candidates) != 0 {
				t.Fatalf("status %q: edge-triggered terminal run must not be retried: candidates = %#v", status, candidates)
			}
		})
	}
}

// TestListStuckParents_ExcludesNonOfficeParent is the fixup regression test
// for the missing authoritative-Office-task predicate: HasOfficeAdoption
// (the Tick-level gate in ParentWakeReconciler) only proves some workspace
// somewhere has adopted Office, not that this parent's own task is an
// Office task, so ListStuckParents must apply its own row-level check —
// mirroring ListUnstartedTasks (tasks.go), the sibling query this
// reconciler mirrors. Without it, an ordinary Kanban parent with terminal
// children and a resolvable runner would receive an unrequested autonomous
// run merely because Office happens to be adopted somewhere in the install.
func TestListStuckParents_ExcludesNonOfficeParent(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	// A Kanban-only parent: seedWakeCandidate's shape, minus the project_id
	// marker, so it satisfies every other ListStuckParents predicate.
	insertTask(t, repo, ctx, "parent-kanban", "ws-1", "Parent", "", "")
	childID := "parent-kanban-child-0"
	insertTask(t, repo, ctx, childID, "ws-1", "Child", "", "")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = ?, state = 'COMPLETED' WHERE id = ?`, "parent-kanban", childID,
	); err != nil {
		t.Fatalf("set child state: %v", err)
	}
	seedWakeAgentProfile(t, repo, ctx, "parent-kanban-agent", "idle")
	seedRunner(t, repo, ctx, "parent-kanban")

	// Control: a genuine Office candidate, so an empty result can't be
	// mistaken for a broken query.
	seedWakeCandidate(t, repo, ctx, "ws-1", "parent-office")

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ParentTaskID != "parent-office" {
		t.Fatalf("candidates = %#v, want exactly [parent-office] (parent-kanban is not an Office task)", candidates)
	}
}

// TestListStuckParents_ExcludesArchivedOnlyChildren is R1-C's regression
// test: a parent whose only children are archived (including one that
// never reached a terminal state) must not be swept with a spurious empty
// child-set key.
func TestListStuckParents_ExcludesArchivedOnlyChildren(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	insertTask(t, repo, ctx, "parent-archived-only", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "parent-archived-only-child-0", "ws-1", "Child", "", "")
	if _, err := repo.ExecRaw(ctx, `
		UPDATE tasks SET parent_id = 'parent-archived-only', state = 'IN_PROGRESS', archived_at = datetime('now')
		WHERE id = 'parent-archived-only-child-0'
	`); err != nil {
		t.Fatalf("archive child: %v", err)
	}

	// Control: a normal candidate, so an empty result can't be mistaken
	// for a broken query.
	seedWakeCandidate(t, repo, ctx, "ws-1", "parent-normal")

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ParentTaskID != "parent-normal" {
		t.Fatalf("candidates = %#v, want exactly [parent-normal]", candidates)
	}
}

// TestListStuckParents_ExcludesUnresolvedOrPausedAssignee is R2-A's SQL-layer
// regression test: a candidate with no resolvable runner, or whose runner is
// paused/stopped/pending_approval, must never be returned at all — those
// states are sticky (nothing about the row changes on the next tick), so
// returning them lets them permanently occupy a LIMIT slot. See
// TestParentWakeReconciler_UnresolvedAndPausedDoNotStarveHealthyParent
// (scheduler_wake_reconciler_test.go) for the end-to-end version of this
// same defect.
func TestListStuckParents_ExcludesUnresolvedOrPausedAssignee(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	// No runner at all.
	insertTask(t, repo, ctx, "parent-no-runner", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "parent-no-runner-child-0", "ws-1", "Child", "", "")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = 'parent-no-runner', state = 'COMPLETED' WHERE id = 'parent-no-runner-child-0'`,
	); err != nil {
		t.Fatalf("set child state: %v", err)
	}

	// Runner resolves, but the agent_profiles row says paused. seedWakeCandidate
	// already inserted an 'idle' row for parent-paused-agent; seedWakeAgentProfile's
	// INSERT OR REPLACE overwrites it with 'paused'.
	seedWakeCandidate(t, repo, ctx, "ws-1", "parent-paused")
	seedWakeAgentProfile(t, repo, ctx, "parent-paused-agent", "paused")

	// Runner resolves to an agent_profile_id with no matching agent_profiles
	// row at all (R3-B): a dangling reference, not merely a paused one.
	insertTask(t, repo, ctx, "parent-dangling", "ws-1", "Parent", "", "")
	insertTask(t, repo, ctx, "parent-dangling-child-0", "ws-1", "Child", "", "")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = 'parent-dangling', state = 'COMPLETED' WHERE id = 'parent-dangling-child-0'`,
	); err != nil {
		t.Fatalf("set child state: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id)
		VALUES ('p-runner-parent-dangling', '', 'parent-dangling', 'runner', 'parent-dangling-nonexistent-agent')
	`); err != nil {
		t.Fatalf("seed dangling runner: %v", err)
	}

	// Control: a normal, healthy candidate.
	seedWakeCandidate(t, repo, ctx, "ws-1", "parent-normal")

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ParentTaskID != "parent-normal" {
		t.Fatalf("candidates = %#v, want exactly [parent-normal]", candidates)
	}
}

// TestListStuckParents_RecoversAfterChildSetChangesPastFinishedRun is R3-A's
// regression test: a parent whose task_children_completed wake was already
// delivered by a finished run must become a candidate again once its child
// set changes afterward. Before this fix, the NOT EXISTS against runs
// treated any 'finished' run for the parent+reason as permanent evidence of
// delivery, regardless of which child set it was requested for, so a
// second child completing after that run finished could never re-trigger
// the sweep — the exact "lost wake, no recovery path" failure mode this
// reconciler exists to fix, but for a wake it delivered itself.
func TestListStuckParents_RecoversAfterChildSetChangesPastFinishedRun(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	const (
		parentID = "parent-1"
		wsID     = "ws-1"
		oldTime  = "2026-01-01 00:00:00"
		runTime  = "2026-01-01 00:05:00"
		newTime  = "2026-01-01 00:10:00"
	)

	insertTaskAt(t, repo, ctx, parentID, wsID, oldTime)
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, parentID,
	); err != nil {
		t.Fatalf("mark parent as Office task: %v", err)
	}
	child0 := parentID + "-child-0"
	insertTaskAt(t, repo, ctx, child0, wsID, oldTime)
	setChildStateAt(t, repo, ctx, parentID, child0, "COMPLETED", oldTime)
	seedWakeAgentProfile(t, repo, ctx, parentID+"-agent", "idle")
	seedRunner(t, repo, ctx, parentID)

	seedWakeRunAt(t, repo, ctx, "run-1", parentID, "task_children_completed", "finished", fmt.Sprintf("'%s'", runTime))

	before, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents (before child set changed): %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("before child set changed: candidates = %#v, want none (the finished run already reflects this child set)", before)
	}

	// A second child completes after the run finished: the child set has
	// changed, and no run reflects it yet.
	child1 := parentID + "-child-1"
	insertTaskAt(t, repo, ctx, child1, wsID, newTime)
	setChildStateAt(t, repo, ctx, parentID, child1, "COMPLETED", newTime)

	after, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents (after child set changed): %v", err)
	}
	if len(after) != 1 || after[0].ParentTaskID != parentID {
		t.Fatalf("LOST WAKE NOT RECOVERABLE: after the child set changed, ListStuckParents returned %#v; want exactly [%s]", after, parentID)
	}
}

// TestListStuckParents_OrdersDeterministically is R2-C's regression test:
// the capped query must not depend on incidental scan order.
func TestListStuckParents_OrdersDeterministically(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	ids := []string{"p-z", "p-a", "p-m"}
	for _, id := range ids {
		seedWakeCandidate(t, repo, ctx, "ws-1", id)
	}

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 3 {
		t.Fatalf("candidates = %d, want 3: %#v", len(candidates), candidates)
	}
	want := []string{"p-a", "p-m", "p-z"}
	for i, c := range candidates {
		if c.ParentTaskID != want[i] {
			t.Fatalf("candidates[%d] = %q, want %q (candidates not ordered): %#v", i, c.ParentTaskID, want[i], candidates)
		}
	}
}
