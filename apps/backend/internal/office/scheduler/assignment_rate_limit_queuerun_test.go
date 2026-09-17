package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// agentActorPayload and userActorPayload build task_assigned payloads for
// the two actor shapes the predicate distinguishes.
func agentActorPayload(taskID string) string {
	return `{"task_id":"` + taskID + `","actor_type":"agent"}`
}

func userActorPayload(taskID string) string {
	return `{"task_id":"` + taskID + `","actor_type":"user"}`
}

// admitNAgentWakes queues AssignmentWakeAllowanceN agent-actor
// task_assigned wakes for taskID through the real QueueRun path, one per
// distinct agent instance. CoalesceRun scopes its merge by
// agent_profile_id, so — unlike repeating the same agent — calls from N
// different agents within the 5s coalescing window never merge into each
// other, giving N genuinely separate admitted rows the way N different
// agents assigned to the same task would in production. Returns the
// agent IDs used, so a caller can issue one more call from any of them.
func admitNAgentWakes(t *testing.T, repo *officesqlite.Repository, ss *SchedulerService, agentPrefix, taskID string) []string {
	t.Helper()
	ctx := context.Background()
	agentIDs := make([]string, AssignmentWakeAllowanceN)
	for i := 0; i < AssignmentWakeAllowanceN; i++ {
		agentID := fmt.Sprintf("%s-%d", agentPrefix, i)
		createChildrenCompletedAgent(t, repo, agentID)
		agentIDs[i] = agentID
		outcome, err := ss.QueueRun(ctx, agentID, RunReasonTaskAssigned, agentActorPayload(taskID), "")
		if err != nil {
			t.Fatalf("admitNAgentWakes[%d]: %v", i, err)
		}
		if outcome != runsservice.QueueOutcomeQueued {
			t.Fatalf("admitNAgentWakes[%d] outcome = %q, want queued", i, outcome)
		}
	}
	return agentIDs
}

// TestQueueRun_AssignmentRateLimit_AdmitsNThenRefusesNPlus1 is the
// headline integration test for REQ-OFFICE-ASSIGN-RATE-001: the first N
// agent-initiated task_assigned wakes for a task insert a row each; the
// N+1th is refused with QueueOutcomeRateLimited and inserts nothing.
func TestQueueRun_AssignmentRateLimit_AdmitsNThenRefusesNPlus1(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()
	taskID := "rl-task-1"

	admitNAgentWakes(t, repo, ss, "rl1-agent", taskID)
	if got := runsCountForReason(t, ss, RunReasonTaskAssigned); got != AssignmentWakeAllowanceN {
		t.Fatalf("runs inserted = %d, want %d", got, AssignmentWakeAllowanceN)
	}

	// A 6th, previously-unused agent still hits the task-scoped allowance.
	createChildrenCompletedAgent(t, repo, "rl1-agent-extra")
	outcome, err := ss.QueueRun(ctx, "rl1-agent-extra", RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun[N+1]: %v", err)
	}
	if outcome != runsservice.QueueOutcomeRateLimited {
		t.Fatalf("QueueRun[N+1] outcome = %q, want rate_limited", outcome)
	}
	if got := runsCountForReason(t, ss, RunReasonTaskAssigned); got != AssignmentWakeAllowanceN {
		t.Fatalf("runs inserted after refusal = %d, want %d (refusal must not insert)", got, AssignmentWakeAllowanceN)
	}
}

// TestQueueRun_AssignmentRateLimit_UserActorAdmitsAfterExhaustion covers
// AC-OFFICE-ASSIGN-RATE-001.1: the gate applies only to agent-initiated
// wakes. A user-actor wake for the same task admits even after the
// allowance is exhausted.
func TestQueueRun_AssignmentRateLimit_UserActorAdmitsAfterExhaustion(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()
	taskID := "rl-task-2"

	admitNAgentWakes(t, repo, ss, "rl2-agent", taskID)

	// A brand new agent, never used above: reusing an exhausted agent's ID
	// here would coalesce with that agent's own already-queued row
	// (CoalesceRun's 5s window) rather than exercising the rate-limit gate.
	createChildrenCompletedAgent(t, repo, "rl2-agent-new")
	outcome, err := ss.QueueRun(ctx, "rl2-agent-new", RunReasonTaskAssigned, userActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun: %v", err)
	}
	if outcome != runsservice.QueueOutcomeQueued {
		t.Fatalf("outcome = %q, want queued (user-actor wakes are never refused)", outcome)
	}
}

// TestQueueRun_AssignmentRateLimit_NoActorTypeAdmitsAfterExhaustion covers
// the 4 out-of-scope producers' payload shape ({"task_id":...}, no
// actor_type): it must admit even after the allowance is exhausted, and
// must never move the rate-limit counter (proven indirectly: the
// scheduler-level unit test already asserts out-of-scope never touches
// the DB at all).
func TestQueueRun_AssignmentRateLimit_NoActorTypeAdmitsAfterExhaustion(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()
	taskID := "rl-task-3"

	admitNAgentWakes(t, repo, ss, "rl3-agent", taskID)

	createChildrenCompletedAgent(t, repo, "rl3-agent-new")
	outcome, err := ss.QueueRun(ctx, "rl3-agent-new", RunReasonTaskAssigned, `{"task_id":"`+taskID+`"}`, "")
	if err != nil {
		t.Fatalf("QueueRun: %v", err)
	}
	if outcome != runsservice.QueueOutcomeQueued {
		t.Fatalf("outcome = %q, want queued (no actor_type is out of scope)", outcome)
	}
}

// TestQueueRun_AssignmentRateLimit_SharedAcrossAgents covers
// AC-OFFICE-ASSIGN-RATE-001.3: the allowance is scoped to the task, not
// the acting agent — N wakes from N distinct agents against the same
// task exhaust one shared allowance, and a further wake from any agent
// (including one already used) is refused.
func TestQueueRun_AssignmentRateLimit_SharedAcrossAgents(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()
	taskID := "rl-task-4"

	agentIDs := admitNAgentWakes(t, repo, ss, "rl4-agent", taskID)
	// Past CoalesceRun's 5s window but inside the 10-minute rate-limit
	// window, so reusing agentIDs[0] below reaches the gate instead of
	// coalescing with that agent's own already-queued row.
	ageRunsRequestedAt(t, ss, time.Minute)

	outcome, err := ss.QueueRun(ctx, agentIDs[0], RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun[N+1] agentIDs[0]: %v", err)
	}
	if outcome != runsservice.QueueOutcomeRateLimited {
		t.Fatalf("outcome = %q, want rate_limited (allowance is task-scoped, shared across agents)", outcome)
	}

	createChildrenCompletedAgent(t, repo, "rl4-agent-new")
	outcome, err = ss.QueueRun(ctx, "rl4-agent-new", RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun[N+1] new agent: %v", err)
	}
	if outcome != runsservice.QueueOutcomeRateLimited {
		t.Fatalf("outcome = %q, want rate_limited for a brand new agent too", outcome)
	}
}

// TestQueueRun_AssignmentRateLimit_RefusalsDoNotConsumeAllowance proves a
// refusal is not itself counted toward the allowance: repeated refused
// attempts never change the outcome or insert a row.
func TestQueueRun_AssignmentRateLimit_RefusalsDoNotConsumeAllowance(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()
	taskID := "rl-task-5"

	agentIDs := admitNAgentWakes(t, repo, ss, "rl5-agent", taskID)
	// Past CoalesceRun's 5s window but inside the 10-minute rate-limit
	// window, so the repeated calls below reach the gate instead of
	// coalescing with agentIDs[0]'s own already-queued row.
	ageRunsRequestedAt(t, ss, time.Minute)

	for i := 0; i < 3; i++ {
		outcome, err := ss.QueueRun(ctx, agentIDs[0], RunReasonTaskAssigned, agentActorPayload(taskID), "")
		if err != nil {
			t.Fatalf("QueueRun refusal[%d]: %v", i, err)
		}
		if outcome != runsservice.QueueOutcomeRateLimited {
			t.Fatalf("QueueRun refusal[%d] outcome = %q, want rate_limited", i, outcome)
		}
	}
	if got := runsCountForReason(t, ss, RunReasonTaskAssigned); got != AssignmentWakeAllowanceN {
		t.Fatalf("runs inserted = %d, want %d (repeated refusals must not insert)", got, AssignmentWakeAllowanceN)
	}
}

// TestQueueRun_AssignmentRateLimit_CoalescedWakeDoesNotConsumeAllowance
// covers the ordering contract: coalescing happens before the rate-limit
// gate, so a coalesced wake (merged into an already-queued row) never
// reaches the gate and never inserts a second row.
func TestQueueRun_AssignmentRateLimit_CoalescedWakeDoesNotConsumeAllowance(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()
	taskID := "rl-task-6"

	first, err := ss.QueueRun(ctx, "agent-1", RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun first: %v", err)
	}
	if first != runsservice.QueueOutcomeQueued {
		t.Fatalf("first outcome = %q, want queued", first)
	}

	for i := 0; i < AssignmentWakeAllowanceN+2; i++ {
		outcome, err := ss.QueueRun(ctx, "agent-1", RunReasonTaskAssigned, agentActorPayload(taskID), "")
		if err != nil {
			t.Fatalf("QueueRun coalesce[%d]: %v", i, err)
		}
		if outcome != runsservice.QueueOutcomeCoalesced {
			t.Fatalf("QueueRun coalesce[%d] outcome = %q, want coalesced", i, outcome)
		}
	}
	if got := runsCountForReason(t, ss, RunReasonTaskAssigned); got != 1 {
		t.Fatalf("runs inserted = %d, want 1 (every repeat coalesced into the first row)", got)
	}
}

// TestQueueRun_AssignmentRateLimit_DedupedWakeDoesNotConsumeAllowance
// covers the same ordering contract for the idempotency-key path: a
// windowed-duplicate wake is suppressed before the rate-limit gate runs.
func TestQueueRun_AssignmentRateLimit_DedupedWakeDoesNotConsumeAllowance(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()
	taskID := "rl-task-7"

	first, err := ss.QueueRun(ctx, "agent-1", RunReasonTaskAssigned, agentActorPayload(taskID), "dedup-key-1")
	if err != nil {
		t.Fatalf("QueueRun first: %v", err)
	}
	if first != runsservice.QueueOutcomeQueued {
		t.Fatalf("first outcome = %q, want queued", first)
	}

	for i := 0; i < AssignmentWakeAllowanceN+2; i++ {
		outcome, err := ss.QueueRun(ctx, "agent-1", RunReasonTaskAssigned, agentActorPayload(taskID), "dedup-key-1")
		if err != nil {
			t.Fatalf("QueueRun dedup[%d]: %v", i, err)
		}
		if outcome != runsservice.QueueOutcomeDeduped {
			t.Fatalf("QueueRun dedup[%d] outcome = %q, want deduped", i, outcome)
		}
	}
	if got := runsCountForReason(t, ss, RunReasonTaskAssigned); got != 1 {
		t.Fatalf("runs inserted = %d, want 1 (every repeat deduped)", got)
	}
}

// TestQueueRun_AssignmentRateLimit_KeylessRedeliveryConsumesAllowanceAgain
// covers AC-OFFICE-ASSIGN-RATE-002.5's second clause: a keyless wake has no
// dedup identity, so redelivering the same keyless occurrence consumes the
// allowance again rather than deduping — the opposite of
// TestQueueRun_AssignmentRateLimit_DedupedWakeDoesNotConsumeAllowance, which
// covers the keyed case.
func TestQueueRun_AssignmentRateLimit_KeylessRedeliveryConsumesAllowanceAgain(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()
	taskID := "rl-task-11"

	first, err := ss.QueueRun(ctx, "agent-1", RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun first: %v", err)
	}
	if first != runsservice.QueueOutcomeQueued {
		t.Fatalf("first outcome = %q, want queued", first)
	}

	// Past CoalesceRun's 5s window but inside the 10-minute rate-limit
	// window, so the redelivery below reaches the gate as a genuinely new
	// wake instead of coalescing with the first row.
	ageRunsRequestedAt(t, ss, time.Minute)

	second, err := ss.QueueRun(ctx, "agent-1", RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun redelivery: %v", err)
	}
	if second != runsservice.QueueOutcomeQueued {
		t.Fatalf("redelivery outcome = %q, want queued (keyless redelivery has no identity to dedup on)", second)
	}

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, taskID, RunReasonTaskAssigned, time.Now().UTC().Add(-AssignmentWakeAllowanceWindow))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2 (both keyless deliveries consumed the allowance independently)", count)
	}
}

// TestQueueRun_AssignmentRateLimit_OrderIndependenceAtBoundary covers
// AC-OFFICE-ASSIGN-RATE-001's window-count query having no ORDER BY: N
// admitted wakes sharing one requested_at timestamp (a real possibility
// since requested_at has millisecond-or-coarser resolution) must all
// count, regardless of any incidental row order.
func TestQueueRun_AssignmentRateLimit_OrderIndependenceAtBoundary(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()
	taskID := "rl-task-8"

	// Insert N admitted wakes directly (all as agent-1, all sharing one
	// requested_at timestamp), then confirm the (N+1)th QueueRun call from
	// a different agent still refuses. createAssignmentWakeRun's rows
	// don't go through CoalesceRun, so using the same agent for all of
	// them here is fine; the follow-up call below uses a fresh agent so
	// it reaches the gate instead of coalescing with agent-1's row.
	shared := time.Now().UTC()
	for i := 0; i < AssignmentWakeAllowanceN; i++ {
		createAssignmentWakeRun(t, repo, taskID, shared)
	}

	createChildrenCompletedAgent(t, repo, "rl8-agent-new")
	outcome, err := ss.QueueRun(ctx, "rl8-agent-new", RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun: %v", err)
	}
	if outcome != runsservice.QueueOutcomeRateLimited {
		t.Fatalf("outcome = %q, want rate_limited", outcome)
	}
}

// TestQueueRun_AssignmentRateLimit_MixedActorCoalesce_AgentOverwritesUser
// pins the "## Accepted consequence" mixed-actor coalesce behavior in one
// direction: a user-actor wake queued first, then coalesced with an
// agent-actor wake for the same task within the coalescing window, ends
// up counted as an agent-initiated admission even though only the
// coalesced (not a newly inserted) row exists — a bounded one-high
// miscount, not a bug to fix.
func TestQueueRun_AssignmentRateLimit_MixedActorCoalesce_AgentOverwritesUser(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()
	taskID := "rl-task-9"

	first, err := ss.QueueRun(ctx, "agent-1", RunReasonTaskAssigned, userActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun user: %v", err)
	}
	if first != runsservice.QueueOutcomeQueued {
		t.Fatalf("first outcome = %q, want queued", first)
	}

	second, err := ss.QueueRun(ctx, "agent-1", RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun agent (coalesces): %v", err)
	}
	if second != runsservice.QueueOutcomeCoalesced {
		t.Fatalf("second outcome = %q, want coalesced", second)
	}

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, taskID, RunReasonTaskAssigned, time.Now().UTC().Add(-time.Minute))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (the coalesced row now reads actor_type=agent)", count)
	}
}

// TestQueueRun_AssignmentRateLimit_MixedActorCoalesce_UserOverwritesAgent
// pins the same consequence in the other direction: an agent-actor wake
// queued first (an admitted, counted wake), then coalesced with a
// user-actor wake for the same task, ends up NOT counted — a bounded
// one-low miscount.
func TestQueueRun_AssignmentRateLimit_MixedActorCoalesce_UserOverwritesAgent(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ctx := context.Background()
	taskID := "rl-task-10"

	first, err := ss.QueueRun(ctx, "agent-1", RunReasonTaskAssigned, agentActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun agent: %v", err)
	}
	if first != runsservice.QueueOutcomeQueued {
		t.Fatalf("first outcome = %q, want queued", first)
	}

	second, err := ss.QueueRun(ctx, "agent-1", RunReasonTaskAssigned, userActorPayload(taskID), "")
	if err != nil {
		t.Fatalf("QueueRun user (coalesces): %v", err)
	}
	if second != runsservice.QueueOutcomeCoalesced {
		t.Fatalf("second outcome = %q, want coalesced", second)
	}

	count, err := repo.CountAgentInitiatedAssignmentWakes(ctx, taskID, RunReasonTaskAssigned, time.Now().UTC().Add(-time.Minute))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 (the coalesced row now reads actor_type=user)", count)
	}
}
