package service

// These tests pin TaskBoundaryCarrierMetadata's live-run preference: a run
// actively claimed against the task wins over the task's own
// already-resolved carrier, so a chain of create_child_task actions (child
// creates grandchild, ...) advances the causation depth ceiling hop by hop
// instead of freezing it at whatever the task's original creating run
// recorded. Verified end to end through a full task -> run -> child task ->
// child run chain.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestTaskBoundaryCarrierMetadata_PrefersLiveClaimedRunOverStaleTaskCarrier
// proves the fix directly: a task whose own stored metadata carries a
// stale carrier (as if it were created deep in an old chain) must NOT
// have that stale carrier forwarded once a run is actually executing its
// turn — the live run's own lineage wins.
func TestTaskBoundaryCarrierMetadata_PrefersLiveClaimedRunOverStaleTaskCarrier(t *testing.T) {
	svc, repo := newRunCausationFromTaskTestService(t)
	ctx := context.Background()

	agent := runCausationTestAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// parent-task's OWN stored carrier claims a shallow, stale lineage —
	// as if it were written long ago and never touched again.
	seedOfficeTaskWithMetadata(t, repo, "parent-task", map[string]interface{}{
		"office_carrier_causation_id":    "stale-causation",
		"office_carrier_causation_depth": 5,
		"office_carrier_creating_run_id": "stale-run",
		"office_carrier_human_rooted":    false,
		"office_carrier_routine_id":      "",
		"office_carrier_actor_kind":      "system",
		"office_carrier_actor_id":        "",
	})

	// A run is now actually executing parent-task's turn.
	if _, err := svc.QueueRunWithActor(ctx, agent.ID, "task_assigned", `{"task_id":"parent-task"}`, "",
		models.ActorKindAgent, "agent-9", ""); err != nil {
		t.Fatalf("queue live run: %v", err)
	}
	liveRun, err := svc.ClaimNextRun(ctx)
	if err != nil || liveRun == nil {
		t.Fatalf("claim live run: %v (run=%v)", err, liveRun)
	}

	got := svc.TaskBoundaryCarrierMetadata(ctx, "parent-task", agent.ID)

	if got["office_carrier_creating_run_id"] != liveRun.ID {
		t.Errorf("creating_run_id = %v, want the live run %q (not the stale forwarded value)",
			got["office_carrier_creating_run_id"], liveRun.ID)
	}
	if depth, ok := got["office_carrier_causation_depth"].(int); !ok || depth != liveRun.CausationDepth {
		t.Errorf("causation_depth = %v, want the live run's own depth %d (not the stale forwarded 5)",
			got["office_carrier_causation_depth"], liveRun.CausationDepth)
	}
	if got["office_carrier_causation_id"] != liveRun.ChainCausationID {
		t.Errorf("causation_id = %v, want the live run's %q", got["office_carrier_causation_id"], liveRun.ChainCausationID)
	}
}

// TestTaskBoundaryCarrierMetadata_ScopesToCausingAgentWhenTwoAgentsHoldClaimsOnSameTask
// proves the fix for the carrier lookup's task-only ambiguity: when two
// different agents each hold a claimed run against the same task, the
// resolved carrier must come from the causing agent's own claimed run,
// not whichever of the two happened to claim most recently.
func TestTaskBoundaryCarrierMetadata_ScopesToCausingAgentWhenTwoAgentsHoldClaimsOnSameTask(t *testing.T) {
	svc, repo := newRunCausationFromTaskTestService(t)
	ctx := context.Background()

	agentA := runCausationTestAgent("worker-a", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agentA); err != nil {
		t.Fatalf("create agent A: %v", err)
	}
	agentB := runCausationTestAgent("worker-b", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agentB); err != nil {
		t.Fatalf("create agent B: %v", err)
	}

	seedOfficeTaskWithMetadata(t, repo, "shared-task", map[string]interface{}{
		"unrelated_key": "value",
	})

	// agentA claims first, then agentB claims second — an unscoped
	// most-recently-claimed lookup would pick agentB's run regardless of
	// which agent is actually causing this resolution.
	if _, err := svc.QueueRunWithActor(ctx, agentA.ID, "task_assigned", `{"task_id":"shared-task"}`, "",
		models.ActorKindAgent, "agent-a-actor", ""); err != nil {
		t.Fatalf("queue agentA's run: %v", err)
	}
	runA, err := svc.ClaimNextRun(ctx)
	if err != nil || runA == nil {
		t.Fatalf("claim agentA's run: %v (run=%v)", err, runA)
	}
	if _, err := svc.QueueRunWithActor(ctx, agentB.ID, "task_assigned", `{"task_id":"shared-task"}`, "",
		models.ActorKindAgent, "agent-b-actor", ""); err != nil {
		t.Fatalf("queue agentB's run: %v", err)
	}
	runB, err := svc.ClaimNextRun(ctx)
	if err != nil || runB == nil {
		t.Fatalf("claim agentB's run: %v (run=%v)", err, runB)
	}

	got := svc.TaskBoundaryCarrierMetadata(ctx, "shared-task", agentA.ID)
	if got["office_carrier_creating_run_id"] != runA.ID {
		t.Errorf("creating_run_id = %v, want agentA's run %q (not agentB's more-recently-claimed %q)",
			got["office_carrier_creating_run_id"], runA.ID, runB.ID)
	}
}

// TestCreateChildTaskCarrier_DepthAdvancesAcrossChainedChildTasks is the
// end-to-end task -> run -> child task -> child run test the review round
// asked for: depth must actually increment when a chain of tasks calls
// create_child_task repeatedly, not stay pinned at whatever the first
// task's carrier said.
func TestCreateChildTaskCarrier_DepthAdvancesAcrossChainedChildTasks(t *testing.T) {
	svc, repo := newRunCausationFromTaskTestService(t)
	ctx := context.Background()

	agent := runCausationTestAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// A distinct agent claims the child run: parentRun is deliberately left
	// claimed (not finished) throughout, since create_child_task fires from
	// within its own still-running turn — claiming a second run against the
	// same agent while the first is still claimed would hit the per-agent
	// concurrency ceiling, which is an orthogonal concern to this test.
	childAgent := runCausationTestAgent("worker-2", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, childAgent); err != nil {
		t.Fatalf("create child agent: %v", err)
	}

	// Root: parent-task was created by a depth-0 run.
	seedOfficeTaskWithMetadata(t, repo, "parent-task", map[string]interface{}{
		"office_carrier_causation_id":    "root-causation",
		"office_carrier_causation_depth": 0,
		"office_carrier_creating_run_id": "root-run",
		"office_carrier_human_rooted":    false,
		"office_carrier_routine_id":      "",
		"office_carrier_actor_kind":      "system",
		"office_carrier_actor_id":        "",
	})

	// A run is dispatched for parent-task's own turn via the normal
	// task-boundary path (mirrors queueTaskAssignedRun), inheriting depth
	// 0+1=1 from parent-task's carrier.
	if err := svc.QueueRunFromTaskBoundary(ctx, agent.ID, RunReasonTaskAssigned,
		`{"task_id":"parent-task"}`, "key-1", "parent-task"); err != nil {
		t.Fatalf("queue parent-task run: %v", err)
	}
	parentRun, err := svc.ClaimNextRun(ctx)
	if err != nil || parentRun == nil {
		t.Fatalf("claim parent run: %v (run=%v)", err, parentRun)
	}
	if parentRun.CausationDepth != 1 {
		t.Fatalf("sanity check failed: parentRun depth = %d, want 1", parentRun.CausationDepth)
	}

	// create_child_task fires from parentRun's own turn: the resolved
	// carrier must come from parentRun (depth 1), not from parent-task's
	// original depth-0 carrier.
	childCarrier := svc.TaskBoundaryCarrierMetadata(ctx, "parent-task", agent.ID)
	if childCarrier["office_carrier_causation_depth"] != parentRun.CausationDepth {
		t.Fatalf("child task carrier depth = %v, want %d (parentRun's own depth)",
			childCarrier["office_carrier_causation_depth"], parentRun.CausationDepth)
	}
	seedOfficeTaskWithMetadata(t, repo, "child-task", childCarrier)

	// A run dispatched for child-task's own turn must land one hop deeper
	// than parentRun, i.e. depth 2 — proving depth keeps advancing across
	// the create_child_task boundary instead of resetting or freezing.
	if err := svc.QueueRunFromTaskBoundary(ctx, childAgent.ID, RunReasonTaskAssigned,
		`{"task_id":"child-task"}`, "key-2", "child-task"); err != nil {
		t.Fatalf("queue child-task run: %v", err)
	}
	childRun, err := svc.ClaimNextRun(ctx)
	if err != nil || childRun == nil {
		t.Fatalf("claim child run: %v (run=%v)", err, childRun)
	}
	if childRun.CausationDepth != 2 {
		t.Errorf("childRun depth = %d, want 2 (parentRun depth %d + 1 task-boundary hop + 1 create_child_task hop)",
			childRun.CausationDepth, parentRun.CausationDepth)
	}
	if childRun.ParentRunID != parentRun.ID {
		t.Errorf("childRun parent_run_id = %q, want %q (parentRun, the run that actually executed create_child_task)",
			childRun.ParentRunID, parentRun.ID)
	}
}
