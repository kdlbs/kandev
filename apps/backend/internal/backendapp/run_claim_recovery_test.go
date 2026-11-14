package backendapp

import (
	"context"
	"testing"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/config"
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
	turns    map[string]*taskmodels.Turn
	err      error
	taskIDs  []string
}

func (s *runClaimSessionReaderStub) ListTaskSessions(_ context.Context, taskID string) ([]*taskmodels.TaskSession, error) {
	s.taskIDs = append(s.taskIDs, taskID)
	return s.sessions, s.err
}

func (s *runClaimSessionReaderStub) GetLatestTurnBySessionID(_ context.Context, sessionID string) (*taskmodels.Turn, error) {
	return s.turns[sessionID], s.err
}

type runClaimRuntimeProbeStub struct {
	live  bool
	err   error
	calls []string
}

type runClaimProcessorRuntimeStub struct {
	runtimeapi.Backend
	live  bool
	err   error
	calls []string
}

func (p *runClaimProcessorRuntimeStub) ProbeAgentRunningForSession(_ context.Context, sessionID string) (bool, error) {
	p.calls = append(p.calls, sessionID)
	return p.live, p.err
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

func TestRunClaimRecoveryWaitingCompletionFallback(t *testing.T) {
	runClaimRecoveryProductionWiring(t, taskmodels.TaskSessionStateWaitingForInput)
}

func TestRunClaimRecoveryCompletionAfterIdleWrite(t *testing.T) {
	runClaimRecoveryProductionWiring(t, taskmodels.TaskSessionStateIdle)
}

func runClaimRecoveryProductionWiring(t *testing.T, completionState taskmodels.TaskSessionState) {
	t.Helper()
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
			ID: "prior-ready-session", TaskID: taskID, AgentProfileID: agent.ID,
			AgentExecutionID: "prior-ready-execution", State: taskmodels.TaskSessionStateWaitingForInput,
		},
		{
			ID: "ready-agent-session", TaskID: taskID, AgentProfileID: agent.ID,
			AgentExecutionID: "ready-agent-execution", State: completionState,
		},
	}, turns: map[string]*taskmodels.Turn{
		"prior-ready-session": {ID: "turn-prior-session", TaskSessionID: "prior-ready-session", TaskID: taskID, StartedAt: time.Now().UTC().Add(-time.Hour)},
		"ready-agent-session": {ID: "turn-ready-session", TaskSessionID: "ready-agent-session", TaskID: taskID, StartedAt: time.Now().UTC()},
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
	wantRuntimeCalls := []string{"prior-ready-session", "ready-agent-session", "prior-ready-session", "ready-agent-session"}
	if len(runtime.calls) != len(wantRuntimeCalls) {
		t.Fatalf("runtime probes = %#v, want %#v", runtime.calls, wantRuntimeCalls)
	}
	for i, want := range wantRuntimeCalls {
		if runtime.calls[i] != want {
			t.Fatalf("runtime probes = %#v, want %#v", runtime.calls, wantRuntimeCalls)
		}
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

func TestNewRunProcessorServiceWiresClaimSessionProbe(t *testing.T) {
	taskSvc, taskRepo, officeRepo := newRunSubscriptionCheckHarness(t)
	workspaceID := seedOwnedWorkspace(t, taskSvc, "claim-constructor-owner")
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-constructor-wiring"
	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{
		ID: taskID, WorkspaceID: workspaceID, Title: "Claim recovery constructor", State: "todo", Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := officeRepo.ExecRaw(ctx,
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('claim-constructor-cli', 'Claim Constructor CLI', datetime('now'), datetime('now'))`,
	); err != nil {
		t.Fatalf("seed CLI agent: %v", err)
	}
	agent := &officeModels.AgentInstance{
		ID: "claim-constructor-agent", WorkspaceID: workspaceID, Name: "claim-constructor-agent",
		Role: officeModels.AgentRoleWorker, Status: officeModels.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	runtime := &runClaimProcessorRuntimeStub{}
	eventBus := bus.NewMemoryEventBus(log)
	svc := newRunProcessorService(
		&config.Config{}, &Repositories{Task: taskRepo, Office: officeRepo}, &Services{Task: taskSvc},
		nil, eventBus, "", nil, nil, runtime, log,
	)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: "constructor-session", TaskID: taskID, AgentProfileID: agent.ID,
		AgentExecutionID: "constructor-execution", State: taskmodels.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	if err := taskRepo.CreateTurn(ctx, &taskmodels.Turn{
		ID: "constructor-turn", TaskSessionID: "constructor-session", TaskID: taskID,
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create task turn: %v", err)
	}
	if _, err := svc.QueueRun(ctx, agent.ID, officeservice.RunReasonTaskAssigned,
		`{"task_id":"`+taskID+`"}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim run: run=%#v err=%v", run, err)
	}
	if _, err := officeRepo.ExecRaw(ctx, `UPDATE runs SET claimed_at = ?, session_id = '' WHERE id = ?`,
		time.Now().UTC().Add(-35*time.Minute), run.ID); err != nil {
		t.Fatalf("age run claim: %v", err)
	}

	officeservice.RunSchedulerTick(svc, ctx)

	got, err := svc.GetRun(ctx, run.ID)
	if err != nil || got.Status != officeservice.RunStatusClaimed {
		t.Fatalf("run after constructor-wired recovery = status %q err=%v, want claimed", got.Status, err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "constructor-session" {
		t.Fatalf("runtime probes = %#v, want constructor-session", runtime.calls)
	}
}

func TestOfficeRunClaimSessionProbeFindsIdleSessionDuringCompletion(t *testing.T) {
	probe := &officeRunClaimSessionProbe{
		sessions: &runClaimSessionReaderStub{sessions: []*taskmodels.TaskSession{{
			ID: "idle-completion-session", TaskID: "task-idle-completion", AgentProfileID: "office-agent",
			AgentExecutionID: "execution-idle-completion", State: taskmodels.TaskSessionStateIdle,
		}}, turns: map[string]*taskmodels.Turn{
			"idle-completion-session": {
				ID: "turn-idle-completion", TaskSessionID: "idle-completion-session", TaskID: "task-idle-completion",
				StartedAt: time.Now().UTC(),
			},
		}},
		runtime: &runClaimRuntimeProbeStub{},
	}

	claimAt := time.Now().UTC().Add(-time.Minute)
	evidence, err := probe.FindTaskSessionsForClaim(
		context.Background(), "task-idle-completion", "office-agent", "", claimAt,
	)
	if err != nil || len(evidence) != 1 || evidence[0].SessionID != "idle-completion-session" || !evidence[0].TurnStartedAfterClaim {
		t.Fatalf("session evidence during pending completion = %#v, %v; want an idle session with a post-claim turn", evidence, err)
	}
}
