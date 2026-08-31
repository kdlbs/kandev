package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/events"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// stubCredentialsManager returns canned credential values so buildPassthroughEnv's
// required-env loop can be observed at the boundary it actually writes.
type stubCredentialsManager struct {
	values    map[string]string
	requested []string
	mu        sync.Mutex
}

func (s *stubCredentialsManager) GetCredentialValue(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	s.requested = append(s.requested, key)
	s.mu.Unlock()
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", errors.New("no such credential")
}

// TestBuildPassthroughEnvInjectsRequiredCredentialsAndMCPEnv pins the two
// layers buildPassthroughEnv adds on top of the runtime environment: the
// agent's declared required credentials, and the env vars the passthrough MCP
// strategy contributed during command building (e.g. opencode's
// OPENCODE_CONFIG). Both must reach the PTY child process.
func TestBuildPassthroughEnvInjectsRequiredCredentialsAndMCPEnv(t *testing.T) {
	mgr := newTestManager(t)
	creds := &stubCredentialsManager{values: map[string]string{
		"ANTHROPIC_API_KEY": "sk-live",
	}}
	mgr.credsMgr = creds
	mgr.profileResolver = &mockPassthroughProfileResolver{
		agentName:      "claude-acp",
		cliPassthrough: true,
		envVars:        []settingsmodels.ProfileEnvVar{{Key: "PROFILE_ONLY", Value: "profile-value"}},
	}

	execution := &AgentExecution{
		ID:                   "exec-1",
		TaskID:               "task-1",
		SessionID:            "session-1",
		AgentProfileID:       "profile-1",
		OfficeAgentProfileID: "office-1",
	}
	execution.setRuntimeEnvironment(map[string]string{
		"ANTHROPIC_BASE_URL": "https://proxy.internal",
		"HTTPS_PROXY":        "http://proxy:3128",
		"PROFILE_ONLY":       "profile-value",
	})
	setPassthroughMCPEnv(execution, map[string]string{"OPENCODE_CONFIG": "/tmp/opencode.json"})

	env, err := mgr.buildPassthroughEnv(context.Background(), execution, []string{"ANTHROPIC_API_KEY", "MISSING_KEY"})
	require.NoError(t, err)

	require.Equal(t, "https://proxy.internal", env["ANTHROPIC_BASE_URL"],
		"Agent.Runtime().Env must reach the passthrough subprocess")
	require.Equal(t, "http://proxy:3128", env["HTTPS_PROXY"])
	require.Equal(t, "task-1", env["KANDEV_TASK_ID"])
	require.Equal(t, "session-1", env["KANDEV_SESSION_ID"])
	require.Equal(t, "office-1", env["KANDEV_AGENT_PROFILE_ID"],
		"the Office identity is the agent profile ID exposed to the agent")
	require.Equal(t, "profile-1", env["KANDEV_EXECUTION_PROFILE_ID"])
	require.Equal(t, "profile-value", env["PROFILE_ONLY"], "profile env vars are merged in")
	require.Equal(t, "sk-live", env["ANTHROPIC_API_KEY"], "required credentials are resolved and injected")
	require.NotContains(t, env, "MISSING_KEY", "an unresolvable credential must not be written as an empty value")
	require.Equal(t, "/tmp/opencode.json", env["OPENCODE_CONFIG"],
		"env contributed by the passthrough MCP strategy must be merged")

	require.Contains(t, creds.requested, "ANTHROPIC_API_KEY")
	require.Contains(t, creds.requested, "MISSING_KEY")
}

func TestBuildPassthroughEnvUsesRuntimeSnapshotWhenProfileSecretIsUnavailable(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockPassthroughProfileResolver{
		envVars: []settingsmodels.ProfileEnvVar{{Key: "PROFILE_ONLY", SecretID: "deleted-secret"}},
	}
	execution := &AgentExecution{
		TaskID:         "task-1",
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
	}
	execution.setRuntimeEnvironment(map[string]string{"PROFILE_ONLY": "captured-value"})

	env, err := mgr.buildPassthroughEnv(context.Background(), execution, nil)
	require.NoError(t, err)
	require.Equal(t, "captured-value", env["PROFILE_ONLY"])
}

func TestMarkPassthroughRunningPublishesOnceAndGuards(t *testing.T) {
	mgr := newTestManager(t)
	bus, ok := mgr.eventBus.(*MockEventBus)
	require.True(t, ok)

	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-acp", SessionID: "session-acp", Status: v1.AgentStatusReady,
	}))
	exec := &AgentExecution{
		ID: "exec-pty", SessionID: "session-pty", PassthroughProcessID: "pty-1", Status: v1.AgentStatusReady,
	}
	require.NoError(t, mgr.executionStore.Add(exec))

	require.ErrorContains(t, mgr.MarkPassthroughRunning("session-absent"),
		"no agent execution found for session: session-absent")
	require.ErrorContains(t, mgr.MarkPassthroughRunning("session-acp"),
		"is not in passthrough mode")
	require.Empty(t, bus.PublishedEvents, "guard failures must publish nothing")

	require.NoError(t, mgr.MarkPassthroughRunning("session-pty"))
	require.Equal(t, v1.AgentStatusRunning, exec.Status)
	require.Len(t, bus.PublishedEvents, 1)
	require.Equal(t, events.AgentRunning, bus.PublishedEvents[0].Type)

	require.NoError(t, mgr.MarkPassthroughRunning("session-pty"))
	require.Len(t, bus.PublishedEvents, 1,
		"an already-running execution must not publish a duplicate AgentRunning event")
}

func TestPreparePassthroughRunningDefersAndSnapshotsPublication(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	startedAt := time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)
	execution := &AgentExecution{
		ID:                   "exec-pty",
		RunID:                "run-before",
		TaskID:               "task-before",
		SessionID:            "session-pty",
		AgentID:              "agent-before",
		AgentProfileID:       "profile-before",
		PassthroughProcessID: "pty-1",
		Status:               v1.AgentStatusReady,
		StartedAt:            startedAt,
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	preparer, ok := interface{}(mgr).(interface {
		PreparePassthroughRunning(string) (func(), error)
	})
	require.True(t, ok, "the lifecycle manager must expose deferred passthrough preparation")
	publish, err := preparer.PreparePassthroughRunning("session-pty")
	require.NoError(t, err)
	require.NotNil(t, publish)
	require.Equal(t, v1.AgentStatusRunning, execution.Status)
	require.Empty(t, eventBus.PublishedEvents, "preparation must not publish while the caller still owns its guard")

	execution.RunID = "run-after"
	execution.TaskID = "task-after"
	execution.Status = v1.AgentStatusFailed

	publish()
	publish()

	require.Len(t, eventBus.PublishedEvents, 1, "deferred publication must be one-shot")
	payload, ok := eventBus.PublishedEvents[0].Event.Data.(AgentEventPayload)
	require.True(t, ok)
	require.Equal(t, "run-before", payload.RunID)
	require.Equal(t, "task-before", payload.TaskID)
	require.Equal(t, "session-pty", payload.SessionID)
	require.Equal(t, string(v1.AgentStatusRunning), payload.Status)
}

type competingExecutionWriter struct {
	mutate func()
}

func (w *competingExecutionWriter) GetExecutorRunningBySessionID(context.Context, string) (*taskmodels.ExecutorRunning, error) {
	if w.mutate != nil {
		w.mutate()
	}
	return nil, taskmodels.ErrExecutorRunningNotFound
}

func (*competingExecutionWriter) UpsertExecutorRunning(context.Context, *taskmodels.ExecutorRunning) error {
	return nil
}

func (*competingExecutionWriter) DeleteExecutorRunningBySessionID(context.Context, string) error {
	return nil
}

func (*competingExecutionWriter) RepairExecutorRunningDead(context.Context, string) error {
	return nil
}

func TestPreparePassthroughRunningCapturesSnapshotBeforeCompetingMutation(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := &AgentExecution{
		ID:                   "exec-pty",
		RunID:                "run-before",
		TaskID:               "task-before",
		SessionID:            "session-pty",
		PassthroughProcessID: "pty-1",
		Status:               v1.AgentStatusReady,
	}
	require.NoError(t, mgr.executionStore.Add(execution))
	mgr.SetExecutorRunningWriter(&competingExecutionWriter{
		mutate: func() {
			require.NoError(t, mgr.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
				current.RunID = "run-after"
				current.TaskID = "task-after"
				current.Status = v1.AgentStatusFailed
			}))
		},
	})

	publish, err := mgr.PreparePassthroughRunning(execution.SessionID)
	require.NoError(t, err)
	publish()

	require.Len(t, eventBus.PublishedEvents, 1)
	payload, ok := eventBus.PublishedEvents[0].Event.Data.(AgentEventPayload)
	require.True(t, ok)
	require.Equal(t, "run-before", payload.RunID)
	require.Equal(t, "task-before", payload.TaskID)
	require.Equal(t, string(v1.AgentStatusRunning), payload.Status)
}

func TestPreparePassthroughRunningClaimsTransitionOnceConcurrently(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := &AgentExecution{
		ID:                   "exec-pty",
		RunID:                "run-before",
		TaskID:               "task-before",
		SessionID:            "session-pty",
		PassthroughProcessID: "pty-1",
		Status:               v1.AgentStatusReady,
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	start := make(chan struct{})
	results := make(chan struct {
		publish func()
		err     error
	}, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			publish, err := mgr.PreparePassthroughRunning(execution.SessionID)
			results <- struct {
				publish func()
				err     error
			}{publish: publish, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.publish)
		result.publish()
	}

	require.Len(t, eventBus.PublishedEvents, 1,
		"concurrent prepare calls must claim one Ready to Running transition")
	payload, ok := eventBus.PublishedEvents[0].Event.Data.(AgentEventPayload)
	require.True(t, ok)
	require.Equal(t, "run-before", payload.RunID)
	require.Equal(t, string(v1.AgentStatusRunning), payload.Status)
}

func TestPassthroughAccessorsRequirePassthroughSessionAndRunner(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-acp", SessionID: "session-acp"}))
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-pty", SessionID: "session-pty", PassthroughProcessID: "pty-1",
	}))

	require.False(t, mgr.IsPassthroughSession(ctx, "session-absent"))
	require.False(t, mgr.IsPassthroughSession(ctx, "session-acp"))
	require.True(t, mgr.IsPassthroughSession(ctx, "session-pty"))

	t.Run("write stdin", func(t *testing.T) {
		require.ErrorContains(t, mgr.WritePassthroughStdin(ctx, "session-absent", "x"),
			"no agent execution found for session")
		require.ErrorContains(t, mgr.WritePassthroughStdin(ctx, "session-acp", "x"),
			"is not in passthrough mode")
		require.ErrorContains(t, mgr.WritePassthroughStdin(ctx, "session-pty", "x"),
			"interactive runner not available")
	})

	t.Run("resize pty", func(t *testing.T) {
		require.ErrorContains(t, mgr.ResizePassthroughPTY(ctx, "session-absent", 80, 24),
			"no agent execution found for session")
		require.ErrorContains(t, mgr.ResizePassthroughPTY(ctx, "session-acp", 80, 24),
			"is not in passthrough mode")
		require.ErrorContains(t, mgr.ResizePassthroughPTY(ctx, "session-pty", 80, 24),
			"interactive runner not available")
	})

	t.Run("get buffer", func(t *testing.T) {
		_, err := mgr.GetPassthroughBuffer(ctx, "session-absent")
		require.ErrorContains(t, err, "no agent execution found for session")
		_, err = mgr.GetPassthroughBuffer(ctx, "session-acp")
		require.ErrorContains(t, err, "is not in passthrough mode")
		_, err = mgr.GetPassthroughBuffer(ctx, "session-pty")
		require.ErrorContains(t, err, "interactive runner not available")
	})
}

func TestGetInteractiveRunnerWithoutStandaloneBackend(t *testing.T) {
	mgr := newTestManager(t)
	require.Nil(t, mgr.GetInteractiveRunner(), "a manager without an executor registry has no runner")

	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{name: "docker"})
	mgr.executorRegistry = registry
	require.Nil(t, mgr.GetInteractiveRunner(), "passthrough requires the standalone backend specifically")
}

func TestStartPassthroughShellIsNoOpWithoutAgentctl(t *testing.T) {
	mgr := newTestManager(t)
	// No agentctl client: the shell start must be skipped silently, not panic.
	mgr.startPassthroughShell(context.Background(), &AgentExecution{ID: "exec-1"}, "warn")
}

func TestStartPassthroughShellCallsAgentctlShellStart(t *testing.T) {
	var shellStarts atomic.Int32
	var status atomic.Int32
	status.Store(http.StatusOK)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/shell/start" {
			shellStarts.Add(1)
			w.WriteHeader(int(status.Load()))
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	mgr := newTestManager(t)
	execution := &AgentExecution{ID: "exec-1", agentctl: newTestAgentctlClient(t, server.URL, newTestLogger())}

	mgr.startPassthroughShell(context.Background(), execution, "failed to start shell")
	require.Equal(t, int32(1), shellStarts.Load())

	// A failing shell start is logged, not propagated — the PTY session stays up.
	status.Store(http.StatusInternalServerError)
	mgr.startPassthroughShell(context.Background(), execution, "failed to start shell")
	require.Equal(t, int32(2), shellStarts.Load())
}

func TestHandlePassthroughOutputPublishesToEventBus(t *testing.T) {
	mgr := newTestManager(t)
	bus, ok := mgr.eventBus.(*MockEventBus)
	require.True(t, ok)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
	}))

	mgr.handlePassthroughOutput(nil)
	mgr.handlePassthroughOutput(&agentctltypes.ProcessOutput{SessionID: "session-absent"})
	require.Empty(t, bus.PublishedEvents, "nil output and unknown sessions publish nothing")

	mgr.handlePassthroughOutput(&agentctltypes.ProcessOutput{
		SessionID: "session-1",
		ProcessID: "pty-1",
		Stream:    "stdout",
		Data:      "hello",
		Timestamp: time.Now(),
	})

	require.Len(t, bus.PublishedEvents, 1)
	require.Equal(t, events.ProcessOutput, bus.PublishedEvents[0].Type)
	payload, ok := bus.PublishedEvents[0].Data.(ProcessOutputEventPayload)
	require.True(t, ok, "payload type = %T", bus.PublishedEvents[0].Data)
	require.Equal(t, "task-1", payload.TaskID)
	require.Equal(t, "session-1", payload.SessionID)
	require.Equal(t, "pty-1", payload.ProcessID)
	require.Equal(t, "stdout", payload.Stream)
	require.Equal(t, "hello", payload.Data)
}

// TestHandlePassthroughStatusSkipsRestartForShellTerminals pins the guard that
// keeps a user shell terminal's exit from restarting the agent PTY.
func TestHandlePassthroughStatusSkipsRestartForShellTerminals(t *testing.T) {
	mgr := newTestManager(t)
	bus, ok := mgr.eventBus.(*MockEventBus)
	require.True(t, ok)
	exec := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		PassthroughProcessID: "pty-agent",
		PassthroughStartedAt: time.Now(),
	}
	exec.passthroughLaunchUsedResume = true
	require.NoError(t, mgr.executionStore.Add(exec))

	mgr.handlePassthroughStatus(nil)
	mgr.handlePassthroughStatus(&agentctltypes.ProcessStatusUpdate{SessionID: "session-absent"})
	require.Empty(t, bus.PublishedEvents)

	exitCode := 1
	mgr.handlePassthroughStatus(&agentctltypes.ProcessStatusUpdate{
		SessionID: "session-1",
		ProcessID: "shell-terminal-7",
		Status:    agentctltypes.ProcessStatusExited,
		ExitCode:  &exitCode,
		Timestamp: time.Now(),
	})

	require.Len(t, bus.PublishedEvents, 1, "the status is still published for the shell terminal")
	require.Equal(t, events.ProcessStatus, bus.PublishedEvents[0].Type)
	require.False(t, exec.passthroughResumeFailed,
		"a user shell terminal's exit must not mark the agent's resume as failed")
	require.True(t, exec.passthroughLaunchUsedResume,
		"the agent PTY's resume intent must survive an unrelated process exit")
}

// TestHandlePassthroughStatusFlipsResumeFailedSynchronously pins the
// synchronous flag flip that beats a racing WebSocket reconnect.
func TestHandlePassthroughStatusFlipsResumeFailedSynchronously(t *testing.T) {
	mgr := newTestManager(t)
	mgr.shuttingDown.Store(true) // Short-circuits the detached restart goroutine.
	exec := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		PassthroughProcessID: "pty-agent",
		PassthroughStartedAt: time.Now(),
		isResumedSession:     true,
	}
	exec.passthroughLaunchUsedResume = true
	require.NoError(t, mgr.executionStore.Add(exec))

	exitCode := 1
	mgr.handlePassthroughStatus(&agentctltypes.ProcessStatusUpdate{
		SessionID: "session-1",
		ProcessID: "pty-agent",
		Status:    agentctltypes.ProcessStatusFailed,
		ExitCode:  &exitCode,
		Timestamp: time.Now(),
	})

	require.True(t, exec.passthroughResumeFailed,
		"a non-zero exit of a resume launch must set the sticky resume-failed flag before returning")
	require.False(t, exec.isResumedSession)
	require.False(t, exec.passthroughLaunchUsedResume)
}

func TestHandlePassthroughStatusKeepsResumeIntentOnCleanExit(t *testing.T) {
	mgr := newTestManager(t)
	mgr.shuttingDown.Store(true)
	exec := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		PassthroughProcessID: "pty-agent",
		PassthroughStartedAt: time.Now(),
	}
	exec.passthroughLaunchUsedResume = true
	require.NoError(t, mgr.executionStore.Add(exec))

	zero := 0
	mgr.handlePassthroughStatus(&agentctltypes.ProcessStatusUpdate{
		SessionID: "session-1",
		ProcessID: "pty-agent",
		Status:    agentctltypes.ProcessStatusExited,
		ExitCode:  &zero,
		Timestamp: time.Now(),
	})

	require.False(t, exec.passthroughResumeFailed,
		"a clean exit keeps the resume intent so a quit session is auto-restarted as a resume")
	require.True(t, exec.passthroughLaunchUsedResume)
}

func TestPassthroughExitCodeTreatsAbsentCodeAsClean(t *testing.T) {
	require.Equal(t, 0, passthroughExitCode(&agentctltypes.ProcessStatusUpdate{}))

	code := 127
	require.Equal(t, 127, passthroughExitCode(&agentctltypes.ProcessStatusUpdate{ExitCode: &code}))
}

func TestHandlePassthroughTurnCompleteUnknownSession(t *testing.T) {
	mgr := newTestManager(t)
	bus, ok := mgr.eventBus.(*MockEventBus)
	require.True(t, ok)

	mgr.handlePassthroughTurnComplete("session-absent", "pty-1")

	require.Empty(t, bus.PublishedEvents)
}

// TestAutoInjectMarksRunningBeforeWriting pins the ordering that blocks a
// racing composer submission: the execution must be RUNNING before the first
// chunk lands on the PTY, so checkSessionPromptable rejects a concurrent
// message.add instead of letting it race into the same PTY mid-submit.
func TestAutoInjectMarksRunningBeforeWriting(t *testing.T) {
	mgr := newTestManager(t)
	execution := newAutoInjectExecution("Ship the release")
	execution.Status = v1.AgentStatusReady
	require.NoError(t, mgr.executionStore.Add(execution))

	var statusAtFirstWrite v1.AgentStatus
	runner := &statusObservingRunner{onWrite: func() { statusAtFirstWrite = execution.Status }}

	mgr.autoInjectInitialPromptWith(runner, execution, agents.PassthroughConfig{SubmitSequence: "\r"})

	require.Equal(t, []string{"Ship the release\r"}, runner.writes)
	require.Equal(t, v1.AgentStatusRunning, statusAtFirstWrite,
		"the execution must already be RUNNING when the first PTY chunk is written")
}

func TestAutoInjectStopsAfterWriteFailure(t *testing.T) {
	mgr := newTestManager(t)
	execution := newAutoInjectExecution("Ship the release")
	execution.Status = v1.AgentStatusReady
	require.NoError(t, mgr.executionStore.Add(execution))
	runner := &fakePassthroughRunner{writeErr: errors.New("process not found")}

	mgr.autoInjectInitialPromptWith(runner, execution, agents.PassthroughConfig{
		SubmitSequence:        "\r",
		DisableBracketedPaste: true,
		SubmitDelay:           time.Millisecond,
	})

	require.Len(t, runner.writes, 1,
		"a failed write must abort the chunk loop instead of continuing to the submit byte")
}

func TestAutoInjectSkipsDuringShutdown(t *testing.T) {
	mgr := newTestManager(t)
	mgr.shuttingDown.Store(true)
	runner := &fakePassthroughRunner{}

	mgr.autoInjectInitialPromptWith(runner, newAutoInjectExecution("Ship it"), agents.PassthroughConfig{
		SubmitSequence: "\r",
	})

	require.False(t, runner.writeCalled, "teardown must not race a stdin write into a dying PTY")
}

// TestAutoInjectInitialPromptWithoutRunnerIsNoOp covers the outer wrapper: with
// no standalone backend there is no interactive runner, and the goroutine
// startPassthroughSession spawns must exit without touching the execution.
func TestAutoInjectInitialPromptWithoutRunnerIsNoOp(t *testing.T) {
	mgr := newTestManager(t)
	execution := newAutoInjectExecution("Ship it")
	execution.Status = v1.AgentStatusReady
	require.NoError(t, mgr.executionStore.Add(execution))

	mgr.autoInjectInitialPrompt(execution, agents.PassthroughConfig{SubmitSequence: "\r"})

	require.Equal(t, v1.AgentStatusReady, execution.Status,
		"without a runner the execution must not be flipped to RUNNING")
}

// statusObservingRunner records the execution status observed at the moment of
// the first stdin write, which is what the mark-running-first ordering is about.
type statusObservingRunner struct {
	onWrite func()
	writes  []string
}

func (r *statusObservingRunner) WaitForFirstIdle(context.Context, string) error { return nil }

func (r *statusObservingRunner) WriteStdin(_ string, data string) error {
	if r.onWrite != nil {
		r.onWrite()
	}
	r.writes = append(r.writes, data)
	return nil
}
