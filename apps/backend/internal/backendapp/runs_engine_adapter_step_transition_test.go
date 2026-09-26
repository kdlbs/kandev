package backendapp

// Covers AC-OFFICE-RUN-CAUSATION-001.25: the run causation chain must not
// break at a workflow step handoff. runsServiceEngineAdapter.QueueRun now
// resolves the causation carrier from the task_step_transitions ledger row
// named by req.CausingStepTransitionID (a step-entry wake) instead of the
// claimed-run lookup TaskBoundaryCarrierForRunQueue uses for an ordinary
// queue_run action, since a step-entry action has no claimed-run scope to
// key off. These tests drive the real adapter + real office repo + real
// runs service (internal/runs/service's causation-depth gate), seeding the
// ledger rows directly the way production's task/repository/sqlite writer
// would, so the depth ceiling from #3748 is proven to actually engage
// across successive step-entry-caused enqueues — the review<->rework loop
// scenario the causation-depth gate exists for.

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	workflowengine "github.com/kandev/kandev/internal/workflow/engine"
)

// seedTaskSession inserts a minimal task_sessions row so a
// task_step_transitions row can reference it under FK enforcement (this
// harness opens its database with db.OpenSQLite, which always enables FKs).
func seedTaskSession(t *testing.T, repo *officesqlite.Repository, sessionID, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := repo.ExecRaw(context.Background(), `
		INSERT INTO task_sessions (id, task_id, started_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, sessionID, taskID, now, now)
	if err != nil {
		t.Fatalf("seed task session %q: %v", sessionID, err)
	}
}

// seedStepTransitionLedgerRow inserts a task_step_transitions row the way
// task/repository/sqlite.recordStepTransition would, and returns its own
// id formatted as a string — the same shape DispatchStepEntry forwards as
// entryID / ActionInput.CausingStepTransitionID.
func seedStepTransitionLedgerRow(
	t *testing.T, repo *officesqlite.Repository, taskID, actorKind, actorID, sessionID string, occurredAt time.Time,
) string {
	t.Helper()
	var actorIDArg, sessionIDArg any
	if actorID != "" {
		actorIDArg = actorID
	}
	if sessionID != "" {
		sessionIDArg = sessionID
	}
	res, err := repo.ExecRaw(context.Background(), `
		INSERT INTO task_step_transitions
			(task_id, session_id, from_workflow_id, from_workflow_step_id,
			 to_workflow_id, to_workflow_step_id, trigger, actor_kind, actor_id,
			 contract_version, occurred_at)
		VALUES (?, ?, 'wf-1', 'from-step', 'wf-1', 'to-step', 'test', ?, ?, 1, ?)
	`, taskID, sessionIDArg, actorKind, actorIDArg, occurredAt)
	if err != nil {
		t.Fatalf("seed step transition ledger row: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return strconv.FormatInt(id, 10)
}

// claimAndBindSession moves runID to claimed at claimedAt and binds
// sessionID onto it, mirroring the production launch write
// (scheduler_integration.go's SetRunSessionID call after a claim) — the
// exact shape a step-transition ledger row's agent-actor resolution
// (GetRunBySessionAt) expects to find.
func claimAndBindSession(t *testing.T, repo *officesqlite.Repository, runID, sessionID string, claimedAt time.Time) {
	t.Helper()
	if err := repo.SetRunStatusForTest(context.Background(), runID, "claimed", &claimedAt, nil); err != nil {
		t.Fatalf("claim run %q: %v", runID, err)
	}
	if wrote, err := repo.SetRunSessionID(context.Background(), runID, sessionID); err != nil || !wrote {
		t.Fatalf("bind session %q onto run %q: wrote=%v err=%v", sessionID, runID, wrote, err)
	}
}

// findQueuedRunForAgent returns the run most recently queued against
// agentProfileID, for tests that queue one run per step and need its id to
// seed the next ledger row / assert its lineage.
func findQueuedRunForAgent(t *testing.T, repo *officesqlite.Repository, agentProfileID string) *officemodels.Run {
	t.Helper()
	runs, err := repo.ListRuns(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	var found *officemodels.Run
	for _, r := range runs {
		if r.AgentProfileID == agentProfileID {
			found = r
		}
	}
	if found == nil {
		t.Fatalf("no queued run found for agent %q", agentProfileID)
	}
	return found
}

// TestRunsServiceEngineAdapter_StepTransition_DepthAdvancesAcrossSuccessiveWakes
// pins AC-OFFICE-RUN-CAUSATION-001.25 end to end: a chain of step-entry-caused
// enqueues (mirroring a review -> reject -> rework -> review loop, where each
// hop's ledger row names the previous hop's own session) must advance
// causation_depth by exactly one hop at a time, through the real adapter and
// the real runs service — and with maxCausationDepth set low, the hop that
// would exceed it is refused by internal/runs/service's causation-depth
// gate (#3748), proving the containment introduced there can now see a
// step-entry-driven loop at all.
func TestRunsServiceEngineAdapter_StepTransition_DepthAdvancesAcrossSuccessiveWakes(t *testing.T) {
	adapter, taskSvc, officeRepo, _ := newRunsEngineAdapterActorTestHarness(t)
	adapter.svc.SetLaunchSafetyLimits(2, 0, 0)
	ctx := context.Background()

	initialAgent := &officemodels.AgentInstance{
		WorkspaceID: "ws-1", Name: "initial-agent",
		Role: officemodels.AgentRoleWorker, Status: officemodels.AgentStatusIdle,
	}
	reviewer := &officemodels.AgentInstance{
		WorkspaceID: "ws-1", Name: "reviewer-agent",
		Role: officemodels.AgentRoleWorker, Status: officemodels.AgentStatusIdle,
	}
	worker := &officemodels.AgentInstance{
		WorkspaceID: "ws-1", Name: "worker-agent",
		Role: officemodels.AgentRoleWorker, Status: officemodels.AgentStatusIdle,
	}
	for _, a := range []*officemodels.AgentInstance{initialAgent, reviewer, worker} {
		if err := officeRepo.CreateAgentInstance(ctx, a); err != nil {
			t.Fatalf("create agent %s: %v", a.Name, err)
		}
	}

	taskID := seedTaskWithCarrier(t, taskSvc, nil)
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	// Root run: the original work turn, as if task-assigned with no carrier
	// (depth 0, chain root) — the state before the review loop starts. Uses
	// its own agent (distinct from reviewer/worker) so later lookups by
	// agent profile id unambiguously find the freshly queued hop, not this
	// root run.
	rootRun := &officemodels.Run{
		ID: "run-root", AgentProfileID: initialAgent.ID, Reason: "task_assigned",
		Payload: `{"task_id":"` + taskID + `"}`,
	}
	if err := officeRepo.CreateRun(ctx, rootRun); err != nil {
		t.Fatalf("create root run: %v", err)
	}
	if err := officeRepo.SetRunStatusForTest(ctx, rootRun.ID, "queued", nil, nil); err != nil {
		t.Fatalf("normalize root run status: %v", err)
	}
	rootRun, err := officeRepo.GetRun(ctx, rootRun.ID)
	if err != nil {
		t.Fatalf("reload root run: %v", err)
	}
	seedTaskSession(t, officeRepo, "sess-root", taskID)
	claimAndBindSession(t, officeRepo, rootRun.ID, "sess-root", base)

	// Hop 1: Work -> Review, caused by the root turn's own session.
	// Resulting run's depth must be root(0) + 1 = 1.
	ledger1 := seedStepTransitionLedgerRow(t, officeRepo, taskID, "agent", "", "sess-root", base.Add(time.Minute))
	if _, err := adapter.QueueRun(ctx, workflowengine.QueueRunRequest{
		AgentProfileID:          reviewer.ID,
		TaskID:                  taskID,
		CausingTaskID:           taskID,
		Reason:                  "on_enter",
		CausingStepTransitionID: ledger1,
	}); err != nil {
		t.Fatalf("hop 1 QueueRun: %v", err)
	}
	hop1 := findQueuedRunForAgent(t, officeRepo, reviewer.ID)
	if hop1.CausationDepth != 1 || hop1.ParentRunID != rootRun.ID {
		t.Fatalf("hop 1 lineage = {depth=%d parent=%q}, want {depth=1 parent=%q}",
			hop1.CausationDepth, hop1.ParentRunID, rootRun.ID)
	}
	seedTaskSession(t, officeRepo, "sess-hop1", taskID)
	claimAndBindSession(t, officeRepo, hop1.ID, "sess-hop1", base.Add(2*time.Minute))

	// Hop 2: Review rejects -> Work -> Review again, caused by hop 1's own
	// session. Resulting depth must be hop1(1) + 1 = 2 — still within the
	// configured ceiling of 2.
	ledger2 := seedStepTransitionLedgerRow(t, officeRepo, taskID, "agent", "", "sess-hop1", base.Add(3*time.Minute))
	if _, err := adapter.QueueRun(ctx, workflowengine.QueueRunRequest{
		AgentProfileID:          worker.ID,
		TaskID:                  taskID,
		CausingTaskID:           taskID,
		Reason:                  "on_enter",
		CausingStepTransitionID: ledger2,
	}); err != nil {
		t.Fatalf("hop 2 QueueRun: %v", err)
	}
	hop2 := findQueuedRunForAgent(t, officeRepo, worker.ID)
	if hop2.ID == rootRun.ID {
		t.Fatal("hop 2 resolved to the root run, want a freshly queued run")
	}
	if hop2.CausationDepth != 2 || hop2.ParentRunID != hop1.ID {
		t.Fatalf("hop 2 lineage = {depth=%d parent=%q}, want {depth=2 parent=%q}",
			hop2.CausationDepth, hop2.ParentRunID, hop1.ID)
	}
	if hop2.ChainCausationID != hop1.ChainCausationID {
		t.Fatalf("hop 2 chain_causation_id = %q, want unchanged from hop 1 %q",
			hop2.ChainCausationID, hop1.ChainCausationID)
	}
	seedTaskSession(t, officeRepo, "sess-hop2", taskID)
	claimAndBindSession(t, officeRepo, hop2.ID, "sess-hop2", base.Add(4*time.Minute))

	// Hop 3: another rejection round would push depth to hop2(2) + 1 = 3,
	// exceeding the configured ceiling of 2 — the #3748 depth-refusal gate
	// must now be reachable through a step-entry-caused enqueue at all.
	ledger3 := seedStepTransitionLedgerRow(t, officeRepo, taskID, "agent", "", "sess-hop2", base.Add(5*time.Minute))
	_, err = adapter.QueueRun(ctx, workflowengine.QueueRunRequest{
		AgentProfileID:          reviewer.ID,
		TaskID:                  taskID,
		CausingTaskID:           taskID,
		Reason:                  "on_enter",
		CausingStepTransitionID: ledger3,
	})
	var refusal *runsservice.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("hop 3 QueueRun err = %v, want a *runsservice.RefusalError", err)
	}
	if refusal.Gate != runsservice.RefusalCausationDepth {
		t.Fatalf("hop 3 refusal gate = %q, want %q", refusal.Gate, runsservice.RefusalCausationDepth)
	}
}

// TestRunsServiceEngineAdapter_StepTransition_HumanMoveRootsAsHumanRootedUser
// pins the other half of AC-OFFICE-RUN-CAUSATION-001.25: a step-entry wake
// caused by a human moving the card (ledger actor_kind=human) must queue a
// fresh, human-rooted root — actor_kind=user, human_rooted=true, depth 0 —
// not an unattributed system root and not any run's lineage.
func TestRunsServiceEngineAdapter_StepTransition_HumanMoveRootsAsHumanRootedUser(t *testing.T) {
	adapter, taskSvc, officeRepo, _ := newRunsEngineAdapterActorTestHarness(t)
	ctx := context.Background()

	agent := &officemodels.AgentInstance{
		WorkspaceID: "ws-1", Name: "assignee-agent",
		Role: officemodels.AgentRoleWorker, Status: officemodels.AgentStatusIdle,
	}
	if err := officeRepo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent instance: %v", err)
	}
	taskID := seedTaskWithCarrier(t, taskSvc, nil)

	ledger := seedStepTransitionLedgerRow(t, officeRepo, taskID, "human", "user-42", "", time.Now().UTC())
	if _, err := adapter.QueueRun(ctx, workflowengine.QueueRunRequest{
		AgentProfileID:          agent.ID,
		TaskID:                  taskID,
		CausingTaskID:           taskID,
		Reason:                  "on_enter",
		CausingStepTransitionID: ledger,
	}); err != nil {
		t.Fatalf("QueueRun: %v", err)
	}

	run := findQueuedRunForAgent(t, officeRepo, agent.ID)
	if run.ActorKind != officemodels.ActorKindUser || run.ActorID != "user-42" {
		t.Fatalf("actor = %s/%s, want user/user-42", run.ActorKind, run.ActorID)
	}
	if !run.HumanRooted {
		t.Error("human_rooted = false, want true")
	}
	if run.ParentRunID != "" || run.CausationDepth != 0 {
		t.Fatalf("lineage = {parent=%q depth=%d}, want a fresh root", run.ParentRunID, run.CausationDepth)
	}
}
