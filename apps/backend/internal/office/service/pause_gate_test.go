package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
)

// fakePauseGate is a scripted shared.PauseGate double with pop-front
// semantics, mirroring the convention used by office/pause's and
// office/routines' own gate tests: each call consumes the next scripted
// (active, err) pair, letting a test sequence a paused/gate-error read
// at one call site and a clean read at the next.
type fakePauseGate struct {
	active []*models.WorkspacePause
	errs   []error
	calls  []string
}

func (f *fakePauseGate) PauseState(_ context.Context, workspaceID string) (*models.WorkspacePause, error) {
	f.calls = append(f.calls, workspaceID)
	var active *models.WorkspacePause
	if len(f.active) > 0 {
		active, f.active = f.active[0], f.active[1:]
	}
	var err error
	if len(f.errs) > 0 {
		err, f.errs = f.errs[0], f.errs[1:]
	}
	return active, err
}

// -- QueueRun gate (site 2) --

func TestQueueRun_BlockedByPause_ReturnsErrWorkspacePaused(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-paused-ws", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.SetPauseGate(&fakePauseGate{active: []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}}})

	err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "")
	if !errors.Is(err, shared.ErrWorkspacePaused) {
		t.Fatalf("err = %v, want shared.ErrWorkspacePaused", err)
	}

	runs, _ := svc.ListRuns(ctx, "ws-1")
	if len(runs) != 0 {
		t.Errorf("want 0 runs queued for a paused workspace, got %d", len(runs))
	}
}

func TestQueueRun_PauseGateError_FailsClosed(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-gate-error", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.SetPauseGate(&fakePauseGate{errs: []error{errors.New("db unavailable")}})

	err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "")
	if !errors.Is(err, shared.ErrPauseGateUnavailable) {
		t.Fatalf("err = %v, want shared.ErrPauseGateUnavailable", err)
	}

	runs, _ := svc.ListRuns(ctx, "ws-1")
	if len(runs) != 0 {
		t.Errorf("want 0 runs queued on a gate-read error, got %d", len(runs))
	}
}

func TestQueueRun_NoPauseGateWired_Unaffected(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-no-gate", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	runs, _ := svc.ListRuns(ctx, "ws-1")
	if len(runs) != 1 {
		t.Errorf("want 1 run queued with no pause gate wired, got %d", len(runs))
	}
}

// -- processRun gate (site 4, F47) --

// TestSchedulerOutcome_WorkspacePaused_TakesPriorityOverAgentInactive proves
// F47: the pause gate sits BEFORE the agent-active check, so a run whose
// agent is ALSO inactive still finishes as workspace_paused, not
// agent_inactive, once the workspace is paused. Mirrors
// TestSchedulerOutcome_AgentInactive_WritesOutcome's claim-then-pause
// sequencing (ClaimNextRun excludes non-idle/working agents, so the agent
// is paused after claim via ProcessRunForTest).
func TestSchedulerOutcome_WorkspacePaused_TakesPriorityOverAgentInactive(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-paused-priority", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if run == nil {
		t.Fatal("expected a run")
	}
	if _, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusPaused, "test"); err != nil {
		t.Fatalf("pause agent: %v", err)
	}
	svc.SetPauseGate(&fakePauseGate{active: []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}}})

	service.ProcessRunForTest(svc, ctx, run)

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	assertOutcome(t, got, service.RunOutcomeWorkspacePaused)
}

// TestSchedulerIntegration_ProcessRun_PauseGateError_RequeuesRun proves a
// gate-read error at the F47 site requeues the claimed run (fails closed,
// retryable) rather than finishing or failing it — nothing about the run
// itself was wrong.
func TestSchedulerIntegration_ProcessRun_PauseGateError_RequeuesRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-gate-error-process", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"t1"}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if run == nil {
		t.Fatal("expected a run")
	}
	svc.SetPauseGate(&fakePauseGate{errs: []error{errors.New("db unavailable")}})

	service.ProcessRunForTest(svc, ctx, run)

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != service.RunStatusQueued {
		t.Fatalf("status = %q, want %q (requeued)", got.Status, service.RunStatusQueued)
	}
	if got.Outcome != nil {
		t.Fatalf("outcome = %v, want nil (requeued run is not terminal)", *got.Outcome)
	}
}

// -- prepareAndLaunch gate (site 5) --

// TestSchedulerIntegration_PrepareAndLaunch_BlockedByPause_ReleasesCheckoutAndFinishes
// proves the final gate, immediately before launch, still stops a pause
// confirmed after processRun's own earlier read — and that the checkout
// this run holds is released before the run finishes as workspace_paused.
// The scripted gate reports clean on the first (processRun) read and
// paused on the second (prepareAndLaunch) read.
func TestSchedulerIntegration_PrepareAndLaunch_BlockedByPause_ReleasesCheckoutAndFinishes(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := makeAgent("worker-launch-paused", models.AgentRoleWorker)
	agent.ExecutorPreference = `{"type":"local_pc"}`
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks
		(id, workspace_id, title, created_at, updated_at)
		VALUES ('task-launch-paused', 'ws-1', 'Launch paused task',
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-launch-paused"}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if run == nil {
		t.Fatal("expected a run")
	}
	// A single ProcessRunForTest pass exercises both processRun's gate
	// (clean, first read) and prepareAndLaunch's gate (paused, second
	// read) exactly once — RunSchedulerTick's drain loop would reclaim
	// and reprocess a requeued run within the same tick, which would
	// consume the script twice and hide the launch-time block being
	// tested here.
	svc.SetPauseGate(&fakePauseGate{active: []*models.WorkspacePause{
		nil,
		{ID: "pause-1", WorkspaceID: "ws-1"},
	}})

	service.ProcessRunForTest(svc, ctx, run)

	if mock.callCount() != 0 {
		t.Fatalf("StartTask calls = %d, want 0 (launch must not happen once paused)", mock.callCount())
	}

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	assertOutcome(t, got, service.RunOutcomeWorkspacePaused)

	var checkoutAgentID *string
	if err := svc.RepoForTest().ReaderDB().Get(
		&checkoutAgentID, `SELECT checkout_agent_id FROM tasks WHERE id = ?`, "task-launch-paused",
	); err != nil {
		t.Fatalf("read checkout_agent_id: %v", err)
	}
	if checkoutAgentID != nil {
		t.Fatalf("checkout_agent_id = %q, want released (nil)", *checkoutAgentID)
	}
}

// TestSchedulerIntegration_PrepareAndLaunch_PauseGateError_ReleasesCheckoutAndRequeues
// mirrors the above for a gate-read error at the same site: the checkout
// is still released, but the run is requeued rather than finished.
func TestSchedulerIntegration_PrepareAndLaunch_PauseGateError_ReleasesCheckoutAndRequeues(t *testing.T) {
	mock := &mockTaskStarter{}
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock})
	ctx := context.Background()

	agent := makeAgent("worker-launch-gate-error", models.AgentRoleWorker)
	agent.ExecutorPreference = `{"type":"local_pc"}`
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks
		(id, workspace_id, title, created_at, updated_at)
		VALUES ('task-launch-gate-error', 'ws-1', 'Launch gate error task',
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-launch-gate-error"}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if run == nil {
		t.Fatal("expected a run")
	}
	svc.SetPauseGate(&fakePauseGate{errs: []error{nil, errors.New("db unavailable")}})

	service.ProcessRunForTest(svc, ctx, run)

	if mock.callCount() != 0 {
		t.Fatalf("StartTask calls = %d, want 0", mock.callCount())
	}

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != service.RunStatusQueued {
		t.Fatalf("status = %q, want %q (requeued)", got.Status, service.RunStatusQueued)
	}

	var checkoutAgentID *string
	if err := svc.RepoForTest().ReaderDB().Get(
		&checkoutAgentID, `SELECT checkout_agent_id FROM tasks WHERE id = ?`, "task-launch-gate-error",
	); err != nil {
		t.Fatalf("read checkout_agent_id: %v", err)
	}
	if checkoutAgentID != nil {
		t.Fatalf("checkout_agent_id = %q, want released (nil)", *checkoutAgentID)
	}
}
