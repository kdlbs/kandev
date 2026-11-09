package orchestrator

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	runtimekinds "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	idleSwitchProfile    = "dynamic-idle-switch"
	idleSwitchCandidateA = "idle-candidate-a"
	idleSwitchCandidateB = "idle-candidate-b"
	idleSwitchExecution  = "idle-execution-a"
)

// idleSwitchFixture is a warm, idle dynamic session on candidate A whose
// runtime is standalone and whose transcript holds one completed turn.
type idleSwitchFixture struct {
	t        *testing.T
	ctx      context.Context
	repo     *sqliterepo.Repository
	svc      *Service
	agents   *mockAgentManager
	resolver *agentruntime.ProfileExecutionResolver
	circuits *dynamicruntime.CircuitRegistry
	taskID   string
	session  string

	running  atomic.Bool
	mu       sync.Mutex
	launches []*executor.LaunchAgentRequest
}

func newIdleSwitchFixture(t *testing.T, name string, keepModel *bool, probe client.ProbeResult) *idleSwitchFixture {
	t.Helper()
	f := &idleSwitchFixture{
		t: t, ctx: context.Background(), repo: setupTestRepo(t),
		taskID: "task-" + name, session: "session-" + name,
	}
	seedTaskAndSession(t, f.repo, f.taskID, f.session, models.TaskSessionStateWaitingForInput)
	f.circuits = dynamicruntime.NewCircuitRegistry()
	f.resolver = newWorkflowDynamicProfileResolverWithKeepModel(t, idleSwitchProfile,
		[]workflowDynamicCandidate{
			{executionProfileID: idleSwitchCandidateA, enabled: true},
			{executionProfileID: idleSwitchCandidateB, enabled: true},
		},
		keepModel,
		dynamicruntime.WithCircuitRegistry(f.circuits),
		dynamicruntime.WithPersistence(f.repo),
		dynamicruntime.WithStateLoader(f.repo),
	)
	f.running.Store(true)
	f.agents = &mockAgentManager{
		repoForExecutionLookup: f.repo,
		isAgentRunningFn:       func(context.Context, string) bool { return f.running.Load() },
		isAgentReadyFn:         func(context.Context, string) bool { return f.running.Load() },
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			f.running.Store(false)
			return nil
		},
		probeBackgroundWorkloadsFunc: func(context.Context, string) (client.ProbeResult, error) {
			return probe, nil
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			f.mu.Lock()
			f.launches = append(f.launches, req)
			f.mu.Unlock()
			f.running.Store(true)
			return &executor.LaunchAgentResponse{AgentExecutionID: "idle-execution-successor"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[f.taskID] = &v1.Task{ID: f.taskID, State: v1.TaskStateInProgress, Description: "idle switch task"}
	f.svc = createTestServiceWithScheduler(f.repo, newMockStepGetter(), taskRepo, f.agents)
	// Production wires the replacement-stop claim in Service.SetExecutor.
	f.svc.executor.SetOnExecutionReplacementStopOwnerRegistration(f.svc.RegisterExecutionReplacementStopOwner)
	f.svc.SetProfileExecutionResolver(f.resolver)
	f.svc.turnService = &repoTurnService{repo: f.repo}

	execution, err := f.resolver.Resolve(f.ctx, f.session, idleSwitchProfile, 0, "")
	if err != nil || execution.ExecutionProfileID != idleSwitchCandidateA {
		t.Fatalf("initial route = %+v, %v; want candidate A", execution, err)
	}
	if err := f.resolver.MarkRouteActive(f.ctx, f.session, execution.Generation); err != nil {
		t.Fatalf("MarkRouteActive: %v", err)
	}
	session := f.currentSession()
	session.AgentProfileID = idleSwitchProfile
	session.ExecutionProfileID = idleSwitchCandidateA
	session.RouteGeneration = execution.Generation
	session.RouteState = dynamicRouteStatusActive
	session.AgentExecutionID = idleSwitchExecution
	session.DownstreamACPSessionID = "acp-native-a"
	if err := f.repo.UpdateTaskSession(f.ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}
	if err := f.repo.UpsertExecutorRunning(f.ctx, &models.ExecutorRunning{
		ID: f.session, SessionID: f.session, TaskID: f.taskID, AgentExecutionID: idleSwitchExecution,
		ExecutionProfileID: idleSwitchCandidateA, Runtime: runtimekinds.RuntimeStandalone,
		ResumeToken: "acp-native-a", Status: "ready",
	}); err != nil {
		t.Fatalf("seed executors_running: %v", err)
	}
	markTargetPromptReady(t, f.svc, f.taskID, f.session, idleSwitchExecution)
	now := time.Now().UTC()
	requireNoError(t, f.repo.CreateTurn(f.ctx, &models.Turn{
		ID: "turn-prior-" + name, TaskSessionID: f.session, TaskID: f.taskID, StartedAt: now, CompletedAt: &now,
	}))
	for _, message := range []*models.Message{
		{ID: "user-prior-" + name, AuthorType: models.MessageAuthorUser, Content: "Rename the config loader"},
		{ID: "agent-prior-" + name, AuthorType: models.MessageAuthorAgent, Content: "Renamed loadConfig to readConfig"},
	} {
		message.TaskSessionID, message.TaskID, message.TurnID = f.session, f.taskID, "turn-prior-"+name
		message.Type = "message"
		message.CreatedAt = now
		requireNoError(t, f.repo.CreateMessage(f.ctx, message))
	}
	return f
}

func (f *idleSwitchFixture) currentSession() *models.TaskSession {
	f.t.Helper()
	session, err := f.repo.GetTaskSession(f.ctx, f.session)
	if err != nil {
		f.t.Fatalf("GetTaskSession: %v", err)
	}
	return session
}

// limitCandidateA makes the next comparison prefer candidate B.
func (f *idleSwitchFixture) limitCandidateA() {
	f.t.Helper()
	profile, err := f.resolver.LoadDynamicProfile(f.ctx, idleSwitchProfile)
	if err != nil {
		f.t.Fatalf("LoadDynamicProfile: %v", err)
	}
	for _, candidate := range profile.Candidates {
		if candidate.ID == idleSwitchCandidateA {
			f.circuits.Open(candidate.BindingKey, time.Now().Add(time.Hour), routingerr.CodeRateLimited)
			return
		}
	}
	f.t.Fatal("candidate A not found")
}

func (f *idleSwitchFixture) stops() []stopAgentCall {
	f.agents.mu.Lock()
	defer f.agents.mu.Unlock()
	return append([]stopAgentCall(nil), f.agents.stopAgentWithReasonArgs...)
}

func (f *idleSwitchFixture) warmPrompts() []promptCall {
	f.agents.mu.Lock()
	defer f.agents.mu.Unlock()
	return append([]promptCall(nil), f.agents.capturedPromptCalls...)
}

func TestIdleNewTurnSwitchesCandidateWithContinuationWhenKeepModelOff(t *testing.T) {
	keep := false
	f := newIdleSwitchFixture(t, "switch", &keep, client.ProbeResultSettled)
	f.limitCandidateA()
	before := f.currentSession().RouteGeneration

	if _, err := f.svc.PromptTask(f.ctx, f.taskID, f.session, "Now add a unit test", "", false, nil, true); err != nil {
		t.Fatalf("PromptTask: %v", err)
	}

	// The executor repeats the stop for the replaced profile; both target the
	// idle predecessor only.
	stops := f.stops()
	if len(stops) == 0 {
		t.Fatal("idle predecessor was not stopped")
	}
	for _, stop := range stops {
		if stop.ExecutionID != idleSwitchExecution {
			t.Fatalf("stop calls = %+v, want only the idle predecessor stopped", stops)
		}
	}
	if prompts := f.warmPrompts(); len(prompts) != 0 {
		t.Fatalf("warm prompts = %+v, want the turn handed to the successor launch", prompts)
	}
	session := f.currentSession()
	if session.ExecutionProfileID != idleSwitchCandidateB || session.RouteGeneration != before+1 {
		t.Fatalf("route = %s gen %d, want candidate B at generation %d", session.ExecutionProfileID, session.RouteGeneration, before+1)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.launches) != 1 {
		t.Fatalf("successor launches = %d, want 1", len(f.launches))
	}
	launch := f.launches[0]
	if launch.AgentProfileID != idleSwitchCandidateB {
		t.Fatalf("successor profile = %q, want candidate B", launch.AgentProfileID)
	}
	if launch.ACPSessionID != "" {
		t.Fatalf("successor native session = %q, want no provider-native resume across candidates", launch.ACPSessionID)
	}
	for _, want := range []string{
		"Now add a unit test", "Kandev continuation package",
		"Rename the config loader", "Renamed loadConfig to readConfig",
	} {
		if !strings.Contains(launch.TaskDescription, want) {
			t.Fatalf("successor prompt lacks %q:\n%s", want, launch.TaskDescription)
		}
	}
}

func TestIdleNewTurnKeepsWarmCandidateWhenComparisonPicksIt(t *testing.T) {
	keep := false
	f := newIdleSwitchFixture(t, "same", &keep, client.ProbeResultSettled)
	before := f.currentSession().RouteGeneration

	if _, err := f.svc.PromptTask(f.ctx, f.taskID, f.session, "Continue", "", false, nil, true); err != nil {
		t.Fatalf("PromptTask: %v", err)
	}

	if stops := f.stops(); len(stops) != 0 {
		t.Fatalf("stop calls = %+v, want the warm runtime kept", stops)
	}
	if prompts := f.warmPrompts(); len(prompts) != 1 || prompts[0].ExecutionID != idleSwitchExecution {
		t.Fatalf("warm prompts = %+v, want the prompt on the warm execution", prompts)
	}
	session := f.currentSession()
	if session.ExecutionProfileID != idleSwitchCandidateA || session.RouteGeneration != before+1 ||
		session.DownstreamACPSessionID != "acp-native-a" {
		t.Fatalf("route = %s gen %d acp %q, want candidate A re-claimed at generation %d with its native session",
			session.ExecutionProfileID, session.RouteGeneration, session.DownstreamACPSessionID, before+1)
	}
	if state := f.routeStatus(); state != dynamicRouteStatusActive {
		t.Fatalf("route status = %q, want active", state)
	}
}

func TestIdleNewTurnDoesNotSwitchWhileWorkIsRunningOrKeepModelIsOn(t *testing.T) {
	keep := false
	cases := []struct {
		name      string
		keepModel *bool
		probe     client.ProbeResult
		prepare   func(*idleSwitchFixture)
	}{
		{name: "keep-model-default", probe: client.ProbeResultSettled},
		{name: "background-live", keepModel: &keep, probe: client.ProbeResultLive},
		{name: "background-unknown", keepModel: &keep, probe: client.ProbeResultUnknown},
		{name: "remote-runtime", keepModel: &keep, probe: client.ProbeResultSettled, prepare: func(f *idleSwitchFixture) {
			running, err := f.repo.GetExecutorRunningBySessionID(f.ctx, f.session)
			requireNoError(f.t, err)
			running.Runtime = runtimekinds.RuntimeDocker
			requireNoError(f.t, f.repo.UpsertExecutorRunning(f.ctx, running))
		}},
		{name: "turn-running", keepModel: &keep, probe: client.ProbeResultSettled, prepare: func(f *idleSwitchFixture) {
			session := f.currentSession()
			session.State = models.TaskSessionStateRunning
			requireNoError(f.t, f.repo.UpdateTaskSession(f.ctx, session))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newIdleSwitchFixture(t, "keep-"+tc.name, tc.keepModel, tc.probe)
			f.limitCandidateA()
			if tc.prepare != nil {
				tc.prepare(f)
			}
			before := f.currentSession()
			sessionBefore := *before
			_, handled, err := f.svc.switchIdleDynamicCandidateForNewTurn(
				f.ctx, f.taskID, f.session, "Next request", "", false, nil, launchOriginManual, promptTaskOptions{},
			)
			if err != nil || handled {
				t.Fatalf("switch handled=%v err=%v, want the ordinary prompt path", handled, err)
			}
			if stops := f.stops(); len(stops) != 0 {
				t.Fatalf("stop calls = %+v, want no interruption", stops)
			}
			after := f.currentSession()
			if after.ExecutionProfileID != sessionBefore.ExecutionProfileID || after.RouteGeneration != sessionBefore.RouteGeneration {
				t.Fatalf("route moved to %s gen %d, want %s gen %d unchanged", after.ExecutionProfileID,
					after.RouteGeneration, sessionBefore.ExecutionProfileID, sessionBefore.RouteGeneration)
			}
		})
	}
}

func (f *idleSwitchFixture) routeStatus() string {
	f.t.Helper()
	state, err := f.repo.LoadRouteState(f.ctx, f.session)
	if err != nil || state == nil {
		f.t.Fatalf("LoadRouteState: %v %#v", err, state)
	}
	return state.Status
}
