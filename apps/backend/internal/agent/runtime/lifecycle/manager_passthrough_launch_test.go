package lifecycle

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// runnerBackend is a minimal ExecutorBackend that exposes a real
// InteractiveRunner. The runner is the designed injection seam for the
// passthrough launch paths; a runner with no started processes lets every
// failure and guard branch run without spawning a PTY.
type runnerBackend struct {
	MockExecutor
	runner *process.InteractiveRunner
}

func (b *runnerBackend) GetInteractiveRunner() *process.InteractiveRunner { return b.runner }

// terminalRecorder stands in for the terminal WebSocket writer so the banner
// text the manager emits can be asserted verbatim.
type terminalRecorder struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *terminalRecorder) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *terminalRecorder) Close() error { return nil }

func (w *terminalRecorder) text() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// newPassthroughRunnerManager returns a manager whose standalone backend
// exposes a real InteractiveRunner, plus a passthrough-capable execution whose
// workspace points at a directory that exists.
func newPassthroughRunnerManager(t *testing.T) (*Manager, *process.InteractiveRunner, *AgentExecution) {
	t.Helper()
	mgr, execution, _ := newClaudePassthroughMCPTestManager(t)

	log := newTestRegistryLogger()
	runner := process.NewInteractiveRunner(nil, log, 1024)
	registry := NewExecutorRegistry(log)
	registry.Register(&runnerBackend{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		runner:       runner,
	})
	mgr.executorRegistry = registry
	require.NoError(t, mgr.executionStore.Add(execution))
	return mgr, runner, execution
}

// missingWorkspace returns a path that does not exist. Pointing an execution at
// it makes the PTY spawn fail deterministically (the child cannot chdir),
// regardless of which agent binaries happen to be installed on the host, so no
// real agent process is ever started by these tests.
func missingWorkspace(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "does-not-exist")
}

func TestGetInteractiveRunnerResolvesStandaloneBackend(t *testing.T) {
	mgr, runner, _ := newPassthroughRunnerManager(t)

	require.Same(t, runner, mgr.GetInteractiveRunner())
}

// TestPassthroughRunnerErrorsSurfaceFromRunner pins that a live runner which
// does not know the recorded process ID produces the runner's own error rather
// than a generic "runner unavailable".
func TestPassthroughRunnerErrorsSurfaceFromRunner(t *testing.T) {
	ctx := context.Background()
	mgr, _, execution := newPassthroughRunnerManager(t)
	execution.PassthroughProcessID = "pty-gone"

	require.ErrorContains(t, mgr.WritePassthroughStdin(ctx, execution.SessionID, "\x03"),
		"process not found: pty-gone")
	require.ErrorContains(t, mgr.ResizePassthroughPTY(ctx, execution.SessionID, 80, 24),
		"no process found for session")

	_, err := mgr.GetPassthroughBuffer(ctx, execution.SessionID)
	require.ErrorContains(t, err, "passthrough process not found")
}

func TestResolvePassthroughConfigReturnsAgentConfig(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)

	pt, err := mgr.ResolvePassthroughConfig(context.Background(), execution.SessionID)

	require.NoError(t, err)
	require.NotEmpty(t, pt.SubmitSequence,
		"the resolved config must carry the submit sequence callers write to PTY stdin")
}

func TestResolvePassthroughConfigErrors(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)

	_, err := mgr.ResolvePassthroughConfig(context.Background(), "session-absent")
	require.ErrorContains(t, err, `no execution for session "session-absent"`)

	execution.AgentProfileID = ""
	_, err = mgr.ResolvePassthroughConfig(context.Background(), execution.SessionID)
	require.ErrorContains(t, err, "failed to get agent config")
}

// TestStartPassthroughSessionSurfacesLaunchFailure pins that a PTY that cannot
// be spawned is recorded on the execution as an error rather than leaving the
// session looking started.
func TestStartPassthroughSessionSurfacesLaunchFailure(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	execution.WorkspacePath = missingWorkspace(t)

	err := mgr.startPassthroughSession(context.Background(), execution, nil)

	require.ErrorContains(t, err, "failed to start passthrough session")
	require.Empty(t, execution.PassthroughProcessID,
		"a failed launch must not record a process ID the WS handler would then treat as live")
	require.Equal(t, v1.AgentStatusFailed, execution.Status)
	require.NotEmpty(t, execution.ErrorMessage)
}

// @covers AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.1
func TestStartPassthroughSessionClaimsInitialPromptBeforeReadyPublication(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)
	const agentID = "pending-prompt-test-agent"
	require.NoError(t, mgr.registry.Register(&testAgent{
		id:      agentID,
		enabled: true,
		StandardPassthrough: agents.StandardPassthrough{Cfg: agents.PassthroughConfig{
			Supported:        true,
			PassthroughCmd:   agents.NewCommand("sh", "-c", "sleep 30"),
			IdleTimeout:      time.Second,
			SubmitSequence:   "\r",
			AutoInjectPrompt: true,
		}},
	}))
	mgr.profileResolver = &mockPassthroughProfileResolver{
		agentName:      agentID,
		cliPassthrough: true,
	}
	execution.AgentID = agentID
	execution.Status = v1.AgentStatusRunning
	execution.setMetadataValue("task_description", "Ship the release")

	var statusAtReadyPublication v1.AgentStatus
	bus := mgr.eventBus.(*MockEventBus)
	bus.OnPublish = func(subject string, _ *eventbus.Event) {
		if subject != events.AgentctlReady {
			return
		}
		mgr.handlePassthroughTurnComplete(execution.SessionID, execution.PassthroughProcessID)
		statusAtReadyPublication = execution.Status
	}

	require.NoError(t, mgr.startPassthroughSession(context.Background(), execution, nil))
	t.Cleanup(func() {
		_ = runner.Stop(context.Background(), execution.PassthroughProcessID)
	})

	require.Equal(t, v1.AgentStatusRunning, statusAtReadyPublication,
		"the launch must claim initial prompt delivery before later startup work can publish ready")
}

func TestResumePassthroughSessionSurfacesLaunchFailure(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	execution.WorkspacePath = missingWorkspace(t)

	err := mgr.ResumePassthroughSession(context.Background(), execution.SessionID)

	require.ErrorContains(t, err, "failed to start passthrough session")
	require.Empty(t, execution.PassthroughProcessID)
}

func TestResumePassthroughSessionUnknownSession(t *testing.T) {
	mgr, _, _ := newPassthroughRunnerManager(t)

	err := mgr.ResumePassthroughSession(context.Background(), "session-absent")

	require.ErrorIs(t, err, ErrNoExecutionForSession)
}

// TestRestartPassthroughProcessClearsProcessIDBeforeStopping pins the ordering
// documented on restartPassthroughProcess: the ID is cleared first so the
// deliberate kill does not trip handlePassthroughStatus's auto-restart.
func TestRestartPassthroughProcessClearsProcessIDBeforeStopping(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	execution.WorkspacePath = missingWorkspace(t)
	execution.PassthroughProcessID = "pty-old"
	execution.PassthroughStartedAt = time.Now()

	err := mgr.restartPassthroughProcess(context.Background(), execution)

	require.ErrorContains(t, err, "failed to restart passthrough session")
	require.Empty(t, execution.PassthroughProcessID,
		"the old process ID is cleared so the deliberate kill is not auto-restarted")
	require.True(t, execution.PassthroughStartedAt.IsZero())
	require.Equal(t, v1.AgentStatusFailed, execution.Status)
}

func TestRestartPassthroughProcessKeepsOldProcessWhenProfileSecretFails(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	mgr.profileResolver = &mockPassthroughProfileResolver{
		agentName:      "claude-acp",
		cliPassthrough: true,
		envVars:        []settingsmodels.ProfileEnvVar{{Key: "PROFILE_ONLY", SecretID: "deleted-secret"}},
	}
	execution.PassthroughProcessID = "pty-old"

	err := mgr.restartPassthroughProcess(context.Background(), execution)

	require.ErrorIs(t, err, ErrProfileSecretUnavailable)
	require.Equal(t, "pty-old", execution.PassthroughProcessID,
		"the old process must remain available when replacement preflight fails")
}

func TestRestartPassthroughProcessWithoutRunner(t *testing.T) {
	mgr, execution, _ := newClaudePassthroughMCPTestManager(t)
	mgr.executorRegistry = nil
	require.NoError(t, mgr.executionStore.Add(execution))
	execution.PassthroughProcessID = "pty-old"

	err := mgr.restartPassthroughProcess(context.Background(), execution)

	require.ErrorContains(t, err, "interactive runner not available")
	require.Equal(t, "pty-old", execution.PassthroughProcessID,
		"without a runner nothing was stopped, so the process ID must be left intact")
}

// TestNotifyFastFailExitWritesBannerToTerminal pins the exact user-facing copy
// and the values interpolated into it: without them the user has no way to tell
// a bad CLI flag from a crash loop.
func TestNotifyFastFailExitWritesBannerToTerminal(t *testing.T) {
	mgr, runner, _ := newPassthroughRunnerManager(t)
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket("session-1", recorder)

	mgr.notifyFastFailExit(runner, "session-1", 350*time.Millisecond, 127, 2*time.Second)

	banner := recorder.text()
	require.Contains(t, banner, "Agent exited (code 127) within 2s")
	require.Contains(t, banner, "bad CLI flag, missing binary, or auth failure")
}

func TestNotifyFastFailExitToleratesMissingTerminal(t *testing.T) {
	mgr, runner, _ := newPassthroughRunnerManager(t)

	// No session WebSocket registered: the write fails and must be swallowed.
	mgr.notifyFastFailExit(runner, "session-gone", time.Second, 1, 2*time.Second)
}

// TestNotifyFallbackInfrastructureFailureNamesTheStage pins that a kandev-side
// failure gets its own banner naming the stage, rather than the fast-fail copy
// that blames the user's profile.
func TestNotifyFallbackInfrastructureFailureNamesTheStage(t *testing.T) {
	mgr, runner, _ := newPassthroughRunnerManager(t)
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket("session-1", recorder)

	mgr.notifyFallbackInfrastructureFailure(runner, "session-1", "start fresh process",
		context.DeadlineExceeded)

	banner := recorder.text()
	require.Contains(t, banner, "Resume fallback failed: could not start fresh process")
	require.Contains(t, banner, context.DeadlineExceeded.Error())
	require.NotContains(t, banner, "bad CLI flag",
		"an infrastructure failure must not blame the user's agent profile")
}

// TestAttemptResumeFallbackAnnouncesAndRelaunchesFresh pins the recovery a user
// sees after `claude -c` fails on a stale conversation: a fresh-start banner
// followed by a relaunch attempt with no resume flag.
func TestAttemptResumeFallbackAnnouncesAndRelaunchesFresh(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)
	execution.WorkspacePath = missingWorkspace(t)
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket(execution.SessionID, recorder)

	mgr.attemptResumeFallback(execution, runner, execution.SessionID, 1, 400*time.Millisecond)

	banner := recorder.text()
	require.Contains(t, banner, "No prior conversation to resume")
	require.Contains(t, banner, "Resume fallback failed: could not start fresh process",
		"a fallback that cannot start must say so rather than leaving a silent dead terminal")
	require.False(t, execution.passthroughLaunchUsedResume)
}

// @covers AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.4
func TestAttemptResumeFallbackClaimsFreshInitialPrompt(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)
	const agentID = "fallback-pending-prompt-test-agent"
	require.NoError(t, mgr.registry.Register(&testAgent{
		id:      agentID,
		enabled: true,
		StandardPassthrough: agents.StandardPassthrough{Cfg: agents.PassthroughConfig{
			Supported:        true,
			PassthroughCmd:   agents.NewCommand("sh", "-c", "sleep 30"),
			IdleTimeout:      time.Second,
			SubmitSequence:   "\r",
			AutoInjectPrompt: true,
		}},
	}))
	mgr.profileResolver = &mockPassthroughProfileResolver{
		agentName:      agentID,
		cliPassthrough: true,
	}
	execution.AgentID = agentID
	execution.Status = v1.AgentStatusRunning
	execution.setMetadataValue("task_description", "Ship the release")
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket(execution.SessionID, recorder)

	mgr.attemptResumeFallback(execution, runner, execution.SessionID, 1, 400*time.Millisecond)

	execution.passthroughLifecycleMu.Lock()
	activeProcessID := execution.PassthroughProcessID
	pendingProcessID := execution.passthroughInitialPromptProcessID
	execution.passthroughLifecycleMu.Unlock()
	t.Cleanup(func() {
		_ = runner.Stop(context.Background(), activeProcessID)
	})
	require.NotEmpty(t, activeProcessID)
	require.Equal(t, activeProcessID, pendingProcessID,
		"a fresh fallback must own its initial prompt before the injection goroutine runs")
}

func TestAttemptResumeFallbackSkipsDuringShutdown(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)
	mgr.shuttingDown.Store(true)
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket(execution.SessionID, recorder)

	mgr.attemptResumeFallback(execution, runner, execution.SessionID, 1, time.Second)

	require.Empty(t, recorder.text(), "a clean shutdown must not print a recovery banner")
}

func TestAttemptResumeFallbackSkipsWithoutTerminal(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)

	// No session WebSocket: nobody is watching, so no relaunch is attempted.
	mgr.attemptResumeFallback(execution, runner, execution.SessionID, 1, time.Second)

	require.Empty(t, execution.PassthroughProcessID)
}

// TestAttemptPassthroughRestartAnnouncesAndReportsFailure pins the restart
// banner pair the user sees when the agent exits and the relaunch also fails.
func TestAttemptPassthroughRestartAnnouncesAndReportsFailure(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)
	execution.WorkspacePath = missingWorkspace(t)
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket(execution.SessionID, recorder)

	mgr.attemptPassthroughRestart(execution, runner, execution.SessionID, 1, time.Millisecond)

	banner := recorder.text()
	require.Contains(t, banner, "[Agent exited. Restarting...]")
	require.Contains(t, banner, "[Restart failed:",
		"a failed restart must be reported on the terminal, not only in backend logs")
}

func TestAttemptPassthroughRestartAbortsDuringShutdown(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket(execution.SessionID, recorder)
	mgr.shuttingDown.Store(true)

	mgr.attemptPassthroughRestart(execution, runner, execution.SessionID, 1, time.Millisecond)

	require.Contains(t, recorder.text(), "[Agent exited. Restarting...]")
	require.NotContains(t, recorder.text(), "[Restart failed:",
		"shutdown aborts before the relaunch, so no failure banner is emitted")
}

func TestAttemptPassthroughRestartAbortsWhenTerminalDisconnects(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)

	// No session WebSocket at all: the announce write fails and the
	// post-delay check aborts the restart.
	mgr.attemptPassthroughRestart(execution, runner, execution.SessionID, 1, time.Millisecond)

	require.Empty(t, execution.PassthroughProcessID)
}

// TestHandlePassthroughExitFastFailsResumeLaunch drives the exit handler end to
// end for the issue-#2330 shape: a resume launch that exits non-zero quickly
// must take the fallback path, not the plain restart path.
func TestHandlePassthroughExitFastFailsResumeLaunch(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)
	execution.WorkspacePath = missingWorkspace(t)
	execution.PassthroughProcessID = "pty-1"
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket(execution.SessionID, recorder)

	startedAt := time.Now()
	exitCode := 1
	mgr.handlePassthroughExit(execution, &agentctltypes.ProcessStatusUpdate{
		SessionID: execution.SessionID,
		ProcessID: "pty-1",
		Status:    agentctltypes.ProcessStatusExited,
		ExitCode:  &exitCode,
		Timestamp: startedAt.Add(200 * time.Millisecond),
	}, startedAt, true)

	require.Contains(t, recorder.text(), "No prior conversation to resume",
		"a fast-failed resume must retry once without the resume flag")
	require.NotContains(t, recorder.text(), "[Agent exited. Restarting...]",
		"the fast-fail short-circuit must skip the plain restart path")
}

func TestHandlePassthroughExitFastFailsFreshLaunch(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)
	execution.PassthroughProcessID = "pty-1"
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket(execution.SessionID, recorder)

	startedAt := time.Now()
	exitCode := 127
	mgr.handlePassthroughExit(execution, &agentctltypes.ProcessStatusUpdate{
		SessionID: execution.SessionID,
		ProcessID: "pty-1",
		ExitCode:  &exitCode,
		Timestamp: startedAt.Add(100 * time.Millisecond),
	}, startedAt, false)

	require.Contains(t, recorder.text(), "Agent exited (code 127)")
	require.NotContains(t, recorder.text(), "No prior conversation to resume",
		"a launch that never used a resume flag has nothing to fall back from")
}

// TestHandlePassthroughExitSkipsWithoutTerminal pins the no-subscriber guard.
// The observable is that none of the relaunch machinery ran: building a fresh
// command materializes an MCP config file and records it on the execution, so
// an empty file list proves the handler returned before that work.
func TestHandlePassthroughExitSkipsWithoutTerminal(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)

	exitCode := 1
	mgr.handlePassthroughExit(execution, &agentctltypes.ProcessStatusUpdate{
		SessionID: execution.SessionID,
		ProcessID: "pty-1",
		ExitCode:  &exitCode,
		Timestamp: time.Now(),
	}, time.Now(), true)

	require.Empty(t, getPassthroughMCPFiles(execution),
		"with nobody watching the terminal, no relaunch command may be built")
	require.Empty(t, execution.PassthroughProcessID)
}

// TestHandlePassthroughExitUsesStatusTimestampNotWallClock pins that the
// measured uptime comes from the exit event, so the handler's own 100ms cleanup
// delay cannot push a genuine fast fail outside the window.
func TestHandlePassthroughExitUsesStatusTimestampNotWallClock(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)
	execution.PassthroughProcessID = "pty-1"
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket(execution.SessionID, recorder)

	startedAt := time.Now().Add(-time.Hour)
	exitCode := 3
	mgr.handlePassthroughExit(execution, &agentctltypes.ProcessStatusUpdate{
		SessionID: execution.SessionID,
		ProcessID: "pty-1",
		ExitCode:  &exitCode,
		// The child really lived 50ms, an hour ago.
		Timestamp: startedAt.Add(50 * time.Millisecond),
	}, startedAt, false)

	require.True(t, strings.Contains(recorder.text(), "Agent exited (code 3)"),
		"uptime must be measured from the exit event, not from time.Now()")
}

func TestHandlePassthroughExitSkipsReplacedProcess(t *testing.T) {
	mgr, runner, execution := newPassthroughRunnerManager(t)
	recorder := &terminalRecorder{}
	runner.TrackSessionWebSocket(execution.SessionID, recorder)
	execution.PassthroughProcessID = "pty-new"

	exitCode := 1
	startedAt := time.Now().Add(-time.Hour)
	mgr.handlePassthroughExit(execution, &agentctltypes.ProcessStatusUpdate{
		SessionID: execution.SessionID,
		ProcessID: "pty-old",
		Status:    agentctltypes.ProcessStatusExited,
		ExitCode:  &exitCode,
		Timestamp: time.Now(),
	}, startedAt, false)

	require.Empty(t, recorder.text(), "a delayed exit from a replaced process must not restart the active PTY")
	require.Equal(t, "pty-new", execution.PassthroughProcessID)
}

// TestStopPassthroughProcessLeavesExecutionStateToItsCaller pins that a failed
// runner stop is swallowed AND that this helper never edits the execution:
// StopAgentWithReason still needs the process ID to build the ExecutorInstance
// it hands to the backend teardown immediately afterwards.
func TestStopPassthroughProcessLeavesExecutionStateToItsCaller(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	backend, err := mgr.executorRegistry.GetBackend(executor.NameStandalone)
	require.NoError(t, err)
	execution.PassthroughProcessID = "pty-gone"

	// The runner does not know this process; the failure is logged and
	// teardown continues rather than aborting the stop.
	mgr.stopPassthroughProcess(context.Background(), execution.ID, execution, backend)

	require.Equal(t, "pty-gone", execution.PassthroughProcessID,
		"clearing the process ID is the caller's job, not this helper's")
}

func TestStopPassthroughProcessSkipsNonPassthroughExecutions(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	backend, err := mgr.executorRegistry.GetBackend(executor.NameStandalone)
	require.NoError(t, err)

	// No passthrough process ID: nothing to stop.
	mgr.stopPassthroughProcess(context.Background(), execution.ID, execution, backend)

	// A backend without an interactive runner is also a no-op.
	execution.PassthroughProcessID = "pty-1"
	mgr.stopPassthroughProcess(context.Background(), execution.ID, execution,
		&MockExecutor{name: executor.NameDocker})
}
