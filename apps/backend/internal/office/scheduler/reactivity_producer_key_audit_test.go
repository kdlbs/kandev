package scheduler

import (
	"context"
	"expvar"
	"testing"

	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/runs/commentkeys"
	"github.com/kandev/kandev/internal/runs/dedupkeys"
)

// newAuditScheduler wires a real repo + real *service.Service (reused from
// reactivity_children_completed_test.go's newChildrenCompletedTestScheduler)
// so every producer under audit — including the two (task_blockers_resolved,
// task_review_requested) that read participants/blockers off the repo, and
// task_mentioned, which resolves mentions via ss.svc.ListAgentInstances —
// can run unmodified.
func newAuditScheduler(t *testing.T) (*SchedulerService, *officesqlite.Repository) {
	t.Helper()
	repo := newReactivityTestRepo(t)
	return newChildrenCompletedTestScheduler(t, repo), repo
}

// requireOneCallKey finds the single recorded queue call for (reason, agentID)
// and returns its IdempotencyKey, failing the test if no such call exists.
func requireOneCallKey(t *testing.T, calls []recordedQueueCall, reason, agentID string) string {
	t.Helper()
	for _, c := range calls {
		if c.agentID == agentID && c.ctx.Reason == reason {
			return c.ctx.IdempotencyKey
		}
	}
	t.Fatalf("no %s call recorded for agent %q: calls=%#v", reason, agentID, calls)
	return ""
}

// requireOneCallKeyless is requireOneCallKey's keyless counterpart: it
// asserts the matching call carries NO idempotency key, rather than
// asserting a value.
func requireOneCallKeyless(t *testing.T, calls []recordedQueueCall, reason, agentID string) {
	t.Helper()
	for _, c := range calls {
		if c.agentID == agentID && c.ctx.Reason == reason {
			if c.ctx.IdempotencyKey != "" {
				t.Fatalf("%s call for agent %q carries key %q, want keyless", reason, agentID, c.ctx.IdempotencyKey)
			}
			return
		}
	}
	t.Fatalf("no %s call recorded for agent %q: calls=%#v", reason, agentID, calls)
}

// auditKeylessCounterValue returns the current value of the process-global
// office_run_dedup_keyless_total expvar map (internal/runs/service) for the
// exact "reason=<reason>;cause=<cause>" label, or 0 if that label has never
// been incremented. Duplicated locally rather than imported: the counter map
// is unexported and the sibling helper in internal/runs/service/dedup_test.go
// lives in a different package. Callers snapshot before and after a producer
// call and assert the delta, rather than merely that the label exists — a
// mere-existence check would pass even if this specific call site failed to
// report, as long as some other test already set that label.
func auditKeylessCounterValue(t *testing.T, reason, cause string) int64 {
	t.Helper()
	v := expvar.Get("office_run_dedup_keyless_total")
	if v == nil {
		t.Fatalf("expvar map office_run_dedup_keyless_total not registered")
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("office_run_dedup_keyless_total is not a *expvar.Map")
	}
	iv := m.Get("reason=" + reason + ";cause=" + cause)
	if iv == nil {
		return 0
	}
	i, ok := iv.(*expvar.Int)
	if !ok {
		t.Fatalf("counter value for reason=%s;cause=%s is not *expvar.Int", reason, cause)
	}
	return i.Value()
}

// TestReactivityProducerKeyAudit is the Part 2-required "producer-audit
// key-format table test" (docs/specs/office/system-design/
// run-dedup-generation-02.md#test-strategy): a table over every reactivity
// producer named in the "Generation sources per producer" table
// (…-01.md#generation-sources-per-producer), asserting each one derives the
// spec's exact key shape, or reports the spec's exact keyless cause when the
// reason has no durable occurrence to key on. Review round 1 found 6 of this
// file's 8 run reasons silently doing neither — this is the design's own
// named enforcement mechanism whose absence is why that gap shipped
// undetected, and what stops the next reason added to reactivity.go from
// reintroducing it silently.
func TestReactivityProducerKeyAudit(t *testing.T) {
	t.Run("task_assigned", func(t *testing.T) {
		ss, _ := newAuditScheduler(t)
		var calls []recordedQueueCall
		queue := func(agentID string, c RunContext) {
			calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
		}
		task := &TaskSnapshot{ID: "task-audit-assigned", WorkspaceID: "ws-1"}
		gen := int64(3)
		res := &ApplyTaskMutationResult{}
		ss.reactToAssigneeChange(task, "agent-assigned", TaskMutation{
			AssignmentGeneration: &gen, ActorID: "user-1", ActorType: "user",
		}, queue, res)

		key := requireOneCallKey(t, calls, RunReasonTaskAssigned, "agent-assigned")
		if want := dedupkeys.AssignmentKey(task.ID, "agent-assigned", gen); key != want {
			t.Fatalf("task_assigned key = %q, want %q", key, want)
		}
	})

	t.Run("task_blockers_resolved", func(t *testing.T) {
		ss, repo := newAuditScheduler(t)
		setupChildrenCompletedParent(t, ss, "blocked-audit", "agent-blocked")
		insertBlockerRelationship(t, repo, "blocked-audit", "blocker-audit-a", "COMPLETED")
		insertBlockerRelationship(t, repo, "blocked-audit", "blocker-audit-b", "COMPLETED")

		var calls []recordedQueueCall
		queue := func(agentID string, c RunContext) {
			calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
		}
		ss.cascadeBlockersResolved(context.Background(),
			&TaskSnapshot{ID: "blocker-audit-b", WorkspaceID: "ws-1"}, queue)

		key := requireOneCallKey(t, calls, RunReasonTaskBlockersResolved, "agent-blocked")
		digest := dedupkeys.BlockerDigest([]string{"blocker-audit-a", "blocker-audit-b"})
		want := RunReasonTaskBlockersResolved + ":blocked-audit:agent-blocked:" + digest
		if key != want {
			t.Fatalf("task_blockers_resolved key = %q, want %q", key, want)
		}
	})

	t.Run("task_comment", func(t *testing.T) {
		ss, _ := newAuditScheduler(t)
		task := &TaskSnapshot{ID: "task-audit-comment", WorkspaceID: "ws-1", AssigneeAgentProfileID: "agent-assignee"}
		comment := &MutationComment{ID: "comment-audit-1", Body: "status update", AuthorType: "user", AuthorID: "user-1"}
		var calls []recordedQueueCall
		queue := func(agentID string, c RunContext) {
			calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
		}
		ss.reactToComment(context.Background(), task, comment, queue)

		key := requireOneCallKey(t, calls, RunReasonTaskComment, "agent-assignee")
		want := commentkeys.TaskComment("comment-audit-1") + ":agent-assignee"
		if key != want {
			t.Fatalf("task_comment key = %q, want %q", key, want)
		}
	})

	t.Run("task_mentioned", func(t *testing.T) {
		ss, repo := newAuditScheduler(t)
		createChildrenCompletedAgent(t, repo, "agent-mentioned")
		task := &TaskSnapshot{ID: "task-audit-mentioned", WorkspaceID: "ws-1"}
		comment := &MutationComment{
			ID: "comment-audit-2", AuthorType: "user", AuthorID: "user-1",
			// The mention token regex allows internal spaces (for multi-word
			// agent names), so it greedily consumes everything after the "@"
			// up to end of string or a disallowed character. Keeping the
			// mention last with nothing trailing bounds the capture to
			// exactly the agent name.
			Body: "cc @agent-mentioned",
			// Isolate the mention path from the assignee-wake path (task has
			// no assignee here, but SkipAssigneeWake documents the intent).
			SkipAssigneeWake: true,
		}
		var calls []recordedQueueCall
		queue := func(agentID string, c RunContext) {
			calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
		}
		ss.reactToComment(context.Background(), task, comment, queue)

		key := requireOneCallKey(t, calls, RunReasonTaskMentioned, "agent-mentioned")
		want := RunReasonTaskMentioned + ":comment-audit-2:agent-mentioned"
		if key != want {
			t.Fatalf("task_mentioned key = %q, want %q", key, want)
		}
	})

	t.Run("task_reopened_via_comment", func(t *testing.T) {
		ss, _ := newAuditScheduler(t)
		task := &TaskSnapshot{ID: "task-audit-reopen-comment", WorkspaceID: "ws-1", State: "DONE", AssigneeAgentProfileID: "agent-reopen"}
		res := &ApplyTaskMutationResult{}
		var calls []recordedQueueCall
		queue := func(agentID string, c RunContext) {
			calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
		}
		change := TaskMutation{
			Comment:   &MutationComment{ID: "comment-audit-3", AuthorType: "user", AuthorID: "user-1"},
			ActorID:   "user-1",
			ActorType: "user",
		}
		ss.reactToStatusChange(context.Background(), task, "todo", change, queue, res)

		key := requireOneCallKey(t, calls, RunReasonTaskReopenedComment, "agent-reopen")
		want := RunReasonTaskReopenedComment + ":comment-audit-3:agent-reopen"
		if key != want {
			t.Fatalf("task_reopened_via_comment key = %q, want %q", key, want)
		}
	})

	t.Run("task_unblocked", func(t *testing.T) {
		ss, _ := newAuditScheduler(t)
		task := &TaskSnapshot{ID: "task-audit-unblocked", WorkspaceID: "ws-1", State: "BLOCKED", AssigneeAgentProfileID: "agent-unblocked"}
		res := &ApplyTaskMutationResult{}
		var calls []recordedQueueCall
		queue := func(agentID string, c RunContext) {
			calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
		}
		before := auditKeylessCounterValue(t, RunReasonTaskUnblocked, "by_design")
		change := TaskMutation{ActorID: "user-1", ActorType: "user"}
		ss.reactToStatusChange(context.Background(), task, "todo", change, queue, res)

		requireOneCallKeyless(t, calls, RunReasonTaskUnblocked, "agent-unblocked")
		if got, want := auditKeylessCounterValue(t, RunReasonTaskUnblocked, "by_design"), before+1; got != want {
			t.Fatalf("office_run_dedup_keyless_total{task_unblocked,by_design} = %d, want %d", got, want)
		}
	})

	t.Run("task_unblocked_no_assignee_does_not_report", func(t *testing.T) {
		// Regression test for Review round 2 Finding 3: an unassigned task
		// must not increment the keyless counter for an enqueue that never
		// happens. Goes through ApplyTaskMutation (not reactToStatusChange
		// directly) so the real queue closure's empty-agent-id guard
		// (reactivity.go's `if agentID == "" { return }`) is exercised —
		// that guard is exactly what silently swallowed the enqueue while
		// FINDING 3's ReportKeylessEnqueue call still fired unconditionally.
		ss, _ := newAuditScheduler(t)
		task := &TaskSnapshot{ID: "task-audit-unblocked-noassignee", WorkspaceID: "ws-1", State: "BLOCKED"}
		before := auditKeylessCounterValue(t, RunReasonTaskUnblocked, "by_design")
		newStatus := "todo"
		change := TaskMutation{NewStatus: &newStatus, ActorID: "user-1", ActorType: "user"}
		res, err := ss.ApplyTaskMutation(context.Background(), task, change)
		if err != nil {
			t.Fatalf("ApplyTaskMutation: %v", err)
		}

		if len(res.Runs) != 0 {
			t.Fatalf("expected no queued runs for an unassigned task_unblocked, got %#v", res.Runs)
		}
		if got := auditKeylessCounterValue(t, RunReasonTaskUnblocked, "by_design"); got != before {
			t.Fatalf("office_run_dedup_keyless_total{task_unblocked,by_design} = %d, want unchanged %d (no enqueue attempted)", got, before)
		}
	})

	t.Run("task_reopened", func(t *testing.T) {
		ss, _ := newAuditScheduler(t)
		task := &TaskSnapshot{ID: "task-audit-reopen-silent", WorkspaceID: "ws-1", State: "CANCELLED", AssigneeAgentProfileID: "agent-silent-reopen"}
		res := &ApplyTaskMutationResult{}
		var calls []recordedQueueCall
		queue := func(agentID string, c RunContext) {
			calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
		}
		before := auditKeylessCounterValue(t, RunReasonTaskReopened, "by_design")
		change := TaskMutation{ActorID: "user-1", ActorType: "user"}
		ss.reactToStatusChange(context.Background(), task, "in_progress", change, queue, res)

		requireOneCallKeyless(t, calls, RunReasonTaskReopened, "agent-silent-reopen")
		if got, want := auditKeylessCounterValue(t, RunReasonTaskReopened, "by_design"), before+1; got != want {
			t.Fatalf("office_run_dedup_keyless_total{task_reopened,by_design} = %d, want %d", got, want)
		}
	})

	t.Run("task_reopened_no_assignee_does_not_report", func(t *testing.T) {
		// Regression test for Review round 2 Finding 3, silent-reopen branch.
		// Goes through ApplyTaskMutation for the same reason as the
		// task_unblocked case above: the empty-agent-id guard lives in the
		// real queue closure, not in reactToStatusChange itself.
		ss, _ := newAuditScheduler(t)
		task := &TaskSnapshot{ID: "task-audit-reopen-silent-noassignee", WorkspaceID: "ws-1", State: "CANCELLED"}
		before := auditKeylessCounterValue(t, RunReasonTaskReopened, "by_design")
		newStatus := "in_progress"
		change := TaskMutation{NewStatus: &newStatus, ActorID: "user-1", ActorType: "user"}
		res, err := ss.ApplyTaskMutation(context.Background(), task, change)
		if err != nil {
			t.Fatalf("ApplyTaskMutation: %v", err)
		}

		if len(res.Runs) != 0 {
			t.Fatalf("expected no queued runs for an unassigned silent reopen, got %#v", res.Runs)
		}
		if got := auditKeylessCounterValue(t, RunReasonTaskReopened, "by_design"); got != before {
			t.Fatalf("office_run_dedup_keyless_total{task_reopened,by_design} = %d, want unchanged %d (no enqueue attempted)", got, before)
		}
	})

	t.Run("task_review_requested", func(t *testing.T) {
		ss, repo := newAuditScheduler(t)
		ctx := context.Background()
		if _, err := repo.ExecRaw(ctx, `
			INSERT INTO tasks (id, workspace_id, workflow_step_id) VALUES ('task-audit-review', 'ws-1', 'step-audit')
		`); err != nil {
			t.Fatalf("insert task: %v", err)
		}
		if _, err := repo.ExecRaw(ctx, `
			INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position)
			VALUES ('p-audit-reviewer', 'step-audit', 'task-audit-review', 'reviewer', 'agent-reviewer-audit', 1, 0)
		`); err != nil {
			t.Fatalf("insert participant: %v", err)
		}

		task := &TaskSnapshot{ID: "task-audit-review", WorkspaceID: "ws-1"}
		var calls []recordedQueueCall
		queue := func(agentID string, c RunContext) {
			calls = append(calls, recordedQueueCall{agentID: agentID, ctx: c})
		}
		before := auditKeylessCounterValue(t, RunReasonTaskReviewRequested, "by_design")
		ss.cascadeReviewRequested(ctx, task, TaskMutation{ActorID: "user-1", ActorType: "user"}, queue)

		requireOneCallKeyless(t, calls, RunReasonTaskReviewRequested, "agent-reviewer-audit")
		if got, want := auditKeylessCounterValue(t, RunReasonTaskReviewRequested, "by_design"), before+1; got != want {
			t.Fatalf("office_run_dedup_keyless_total{task_review_requested,by_design} = %d, want %d", got, want)
		}
	})
}
