package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	officeModels "github.com/kandev/kandev/internal/office/models"
	officeservice "github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type runClaimSessionReaderStub struct {
	sessions []*taskmodels.TaskSession
	err      error
	taskIDs  []string
}

func (s *runClaimSessionReaderStub) ListTaskSessions(_ context.Context, taskID string) ([]*taskmodels.TaskSession, error) {
	s.taskIDs = append(s.taskIDs, taskID)
	return s.sessions, s.err
}

type runClaimRuntimeProbeStub struct {
	live  bool
	err   error
	calls []string
}

type runClaimTaskStarterStub struct {
	calls int
}

func (s *runClaimTaskStarterStub) StartTask(
	context.Context, string, string, string, string, string, string, string, bool, []v1.MessageAttachment,
) error {
	s.calls++
	return nil
}

func (p *runClaimRuntimeProbeStub) ProbeAgentRunningForSession(_ context.Context, sessionID string) (bool, error) {
	p.calls = append(p.calls, sessionID)
	return p.live, p.err
}

func TestRunClaimRecoveryProductionWiring(t *testing.T) {
	taskSvc, taskRepo, repo := newRunSubscriptionCheckHarness(t)
	workspaceID := seedOwnedWorkspace(t, taskSvc, "claim-recovery-owner")
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-wired"
	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{
		ID: taskID, WorkspaceID: workspaceID, Title: "Claim recovery", State: "todo", Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	starter := &runClaimTaskStarterStub{}
	eventBus := bus.NewMemoryEventBus(log)
	svc := officeservice.NewService(officeservice.ServiceOptions{
		Repo: repo, Logger: log, TaskStarter: starter, EventBus: eventBus,
	})
	svc.SetSyncHandlers(true)
	if err := svc.RegisterEventSubscribers(eventBus); err != nil {
		t.Fatalf("register event subscribers: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('claim-test-cli', 'Claim Test CLI', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed CLI agent: %v", err)
	}
	agent := &officeModels.AgentInstance{
		ID: "claim-recovery-wired-agent", WorkspaceID: workspaceID, Name: "claim-recovery-wired-agent",
		Role: officeModels.AgentRoleWorker, Status: officeModels.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, officeservice.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: run=%#v err=%v", run, err)
	}
	if _, err := repo.ExecRaw(ctx, `UPDATE runs SET claimed_at = ?, session_id = '' WHERE id = ?`,
		time.Now().UTC().Add(-35*time.Minute), run.ID); err != nil {
		t.Fatalf("age run claim: %v", err)
	}
	sessions := &runClaimSessionReaderStub{sessions: []*taskmodels.TaskSession{
		{ID: "other-agent-session", TaskID: taskID, AgentProfileID: "other-agent", State: taskmodels.TaskSessionStateRunning},
		{
			ID: "ready-agent-session", TaskID: taskID, AgentProfileID: agent.ID,
			AgentExecutionID: "ready-agent-execution", State: taskmodels.TaskSessionStateWaitingForInput,
		},
	}}
	runtime := &runClaimRuntimeProbeStub{}
	wireOfficeRunClaimSessionProbe(svc, sessions, runtime)

	officeservice.RunSchedulerTick(svc, ctx)
	officeservice.RunSchedulerTick(svc, ctx)
	got, err := svc.GetRun(ctx, run.ID)
	if err != nil || got.Status != officeservice.RunStatusClaimed {
		t.Fatalf("run after production guard = status %q err=%v, want claimed", got.Status, err)
	}
	if got.SessionID != "" {
		t.Fatalf("run session id = %q, want empty to exercise fallback", got.SessionID)
	}
	if starter.calls != 0 {
		t.Fatalf("task launches after stale sweeps = %d, want 0", starter.calls)
	}
	if len(sessions.taskIDs) != 2 || sessions.taskIDs[0] != taskID || sessions.taskIDs[1] != taskID {
		t.Fatalf("task session lookups = %#v, want two lookups for %s", sessions.taskIDs, taskID)
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "ready-agent-session" || runtime.calls[1] != "ready-agent-session" {
		t.Fatalf("runtime probes = %#v, want the ready task session on both sweeps", runtime.calls)
	}

	event := bus.NewEvent(events.AgentCompleted, "test", map[string]string{
		"task_id": taskID, "session_id": "ready-agent-session", "agent_profile_id": agent.ID,
	})
	if err := eventBus.Publish(ctx, events.AgentCompleted, event); err != nil {
		t.Fatalf("publish delayed completion: %v", err)
	}
	got, err = svc.GetRun(ctx, run.ID)
	if err != nil || got.Status != officeservice.RunStatusFinished {
		t.Fatalf("run after delayed completion = status %q err=%v, want finished", got.Status, err)
	}
}
