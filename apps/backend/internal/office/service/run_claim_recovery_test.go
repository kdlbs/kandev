package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

type runClaimSessionProbeStub struct {
	sessionID string
	findErr   error
	live      bool
	probeErr  error
	probed    []string
}

func (p *runClaimSessionProbeStub) FindActiveTaskSession(context.Context, string, string) (string, error) {
	return p.sessionID, p.findErr
}

func (p *runClaimSessionProbeStub) ProbeAgentRunningForSession(_ context.Context, sessionID string) (bool, error) {
	p.probed = append(p.probed, sessionID)
	return p.live, p.probeErr
}

func TestRunClaimRecoveryPreservesLiveExecution(t *testing.T) {
	svc, eventBus := newTestServiceWithBus(t)
	ctx := context.Background()
	launcher := &tasklessTestLauncher{svc: svc}
	svc.SetRunSessionLauncher(launcher)
	agent := &models.AgentInstance{
		ID: "claim-recovery-agent", WorkspaceID: "ws-1", Name: "claim-recovery-agent",
		Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, "claim-recovery"); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list queued run: runs=%#v err=%v", runs, err)
	}
	run := runs[0]

	service.RunSchedulerTick(svc, ctx)
	if len(launcher.calls) != 1 {
		t.Fatalf("initial launches = %d, want 1", len(launcher.calls))
	}
	svc.ExecSQL(t, `UPDATE runs SET claimed_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-35*time.Minute), run.ID)
	aged, err := svc.GetRun(ctx, run.ID)
	if err != nil || aged.ClaimedAt == nil {
		t.Fatalf("read aged claim: run=%#v err=%v", aged, err)
	}
	claimedAt := *aged.ClaimedAt

	service.RunSchedulerTick(svc, ctx)
	service.RunSchedulerTick(svc, ctx)

	stored, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("read run after stale sweeps: %v", err)
	}
	if stored.Status != service.RunStatusClaimed {
		t.Fatalf("run status after stale sweeps = %q, want claimed", stored.Status)
	}
	if stored.ClaimedAt == nil || !stored.ClaimedAt.Equal(claimedAt) {
		t.Fatalf("claimed_at after stale sweeps = %v, want original %s", stored.ClaimedAt, claimedAt)
	}
	if stored.SessionID != "run-session-1" {
		t.Fatalf("session_id after stale sweeps = %q, want original session", stored.SessionID)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("launches after stale sweeps = %d, want 1", len(launcher.calls))
	}

	if err := eventBus.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: "execution-run-session-1", AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: run.ID, RunSessionID: "run-session-1", RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: agent.WorkspaceID, Status: "COMPLETED",
	})); err != nil {
		t.Fatalf("publish original completion: %v", err)
	}
	finished, err := svc.GetRun(ctx, run.ID)
	if err != nil || finished.Status != service.RunStatusFinished {
		t.Fatalf("run after original completion = status %q err=%v, want finished", finished.Status, err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("launches after completion = %d, want 1", len(launcher.calls))
	}
}

func TestRunClaimRecoveryRequeuesAbandonedClaim(t *testing.T) {
	svc := newTestService(t, service.ServiceOptions{})
	ctx := context.Background()
	svc.SetRunClaimSessionProbe(&runClaimSessionProbeStub{})
	agent := &models.AgentInstance{
		ID: "claim-recovery-abandoned-agent", WorkspaceID: "ws-1", Name: "claim-recovery-abandoned-agent",
		Role: models.AgentRoleWorker, Status: models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-unlaunched"}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: run=%#v err=%v", run, err)
	}
	svc.ExecSQL(t, `UPDATE runs SET claimed_at = ? WHERE id = ?`, time.Now().UTC().Add(-35*time.Minute), run.ID)

	service.RunSchedulerTick(svc, ctx)
	got, err := svc.GetRun(ctx, run.ID)
	if err != nil || got.Status != service.RunStatusQueued {
		t.Fatalf("abandoned run = status %q err=%v, want queued", got.Status, err)
	}
}

func TestRunClaimRecoveryUnknownSession(t *testing.T) {
	for _, tc := range []struct {
		name      string
		probe     *runClaimSessionProbeStub
		persistID bool
	}{
		{name: "runtime probe error", probe: &runClaimSessionProbeStub{probeErr: errors.New("runtime unavailable")}, persistID: true},
		{name: "fallback lookup error", probe: &runClaimSessionProbeStub{findErr: errors.New("task session store unavailable")}},
		{name: "resolved session without runtime", probe: &runClaimSessionProbeStub{sessionID: "session-from-fallback"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t, service.ServiceOptions{})
			ctx := context.Background()
			svc.SetRunClaimSessionProbe(tc.probe)
			agent := &models.AgentInstance{
				ID: "claim-recovery-unknown-agent", WorkspaceID: "ws-1", Name: "claim-recovery-unknown-agent",
				Role: models.AgentRoleWorker, Status: models.AgentStatusIdle,
				ExecutorPreference: `{"type":"worktree"}`,
			}
			if err := svc.CreateAgentInstance(ctx, agent); err != nil {
				t.Fatalf("create agent: %v", err)
			}
			if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, `{"task_id":"task-unknown"}`, ""); err != nil {
				t.Fatalf("queue run: %v", err)
			}
			run, err := svc.ClaimNextRun(ctx)
			if err != nil || run == nil {
				t.Fatalf("claim run: run=%#v err=%v", run, err)
			}
			sessionID := ""
			if tc.persistID {
				sessionID = "session-unknown"
			}
			svc.ExecSQL(t, `UPDATE runs SET claimed_at = ?, session_id = ? WHERE id = ?`,
				time.Now().UTC().Add(-35*time.Minute), sessionID, run.ID)

			service.RunSchedulerTick(svc, ctx)
			got, err := svc.GetRun(ctx, run.ID)
			if err != nil || got.Status != service.RunStatusClaimed {
				t.Fatalf("uncertain run = status %q err=%v, want claimed", got.Status, err)
			}
		})
	}
}

func TestRunClaimRecoveryTasklessAttempt(t *testing.T) {
	for _, state := range []models.RunSessionState{
		models.RunSessionStatePreparing,
		models.RunSessionStateRunning,
	} {
		t.Run(string(state), func(t *testing.T) {
			svc := newTestService(t, service.ServiceOptions{})
			ctx := context.Background()
			agent := &models.AgentInstance{
				ID: "claim-recovery-taskless-agent", WorkspaceID: "ws-1", Name: "claim-recovery-taskless-agent",
				Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
				ExecutorPreference: `{"type":"local_pc"}`,
			}
			if err := svc.CreateAgentInstance(ctx, agent); err != nil {
				t.Fatalf("create agent: %v", err)
			}
			if _, err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
				t.Fatalf("queue run: %v", err)
			}
			run, err := svc.ClaimNextRun(ctx)
			if err != nil || run == nil {
				t.Fatalf("claim run: run=%#v err=%v", run, err)
			}
			session := &models.RunSession{
				ID: "session-" + string(state), WorkspaceID: agent.WorkspaceID,
				AgentProfileID: agent.ID, RunID: run.ID, Attempt: 1,
				State: state, CreatedAt: time.Now().UTC(), Version: 1,
			}
			reserved, err := svc.RepoForTest().ReserveRunSession(ctx, session)
			if err != nil || !reserved {
				t.Fatalf("reserve attempt: reserved=%v err=%v", reserved, err)
			}
			if state == models.RunSessionStateRunning {
				if _, err := svc.RepoForTest().BindRunSessionExecution(ctx, session.ID, "execution-1", "profile-1", "agent", "model", "acp-1"); err != nil {
					t.Fatalf("bind execution: %v", err)
				}
				if _, err := svc.RepoForTest().MarkRunSessionStarted(ctx, session.ID, "execution-1", "profile-1", "agent", "model", "acp-1"); err != nil {
					t.Fatalf("mark started: %v", err)
				}
			}
			svc.ExecSQL(t, `UPDATE runs SET claimed_at = ?, session_id = '' WHERE id = ?`,
				time.Now().UTC().Add(-35*time.Minute), run.ID)

			service.RunSchedulerTick(svc, ctx)
			got, err := svc.GetRun(ctx, run.ID)
			if err != nil || got.Status != service.RunStatusClaimed {
				t.Fatalf("run with %s attempt = status %q err=%v, want claimed", state, got.Status, err)
			}
		})
	}
}
