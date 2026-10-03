package lifecycle

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// recordedWorkspaceCallback captures which handler fired, for which execution,
// and with what payload — the three things a dropped wiring hop silently loses.
type recordedWorkspaceCallback struct {
	name      string
	execution *AgentExecution
	payload   any
}

func newRecordingStreamManager(t *testing.T, recorded *[]recordedWorkspaceCallback) *StreamManager {
	t.Helper()
	record := func(name string) func(*AgentExecution, any) {
		return func(execution *AgentExecution, payload any) {
			*recorded = append(*recorded, recordedWorkspaceCallback{name: name, execution: execution, payload: payload})
		}
	}
	stopCh := newTestStopCh(t)
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnShellOutput: func(e *AgentExecution, data string) { record("shell_output")(e, data) },
		OnShellExit:   func(e *AgentExecution, code int) { record("shell_exit")(e, code) },
		OnGitStatus: func(e *AgentExecution, u *agentctl.GitStatusUpdate) {
			record("git_status")(e, u)
		},
		OnGitCommit: func(e *AgentExecution, c *agentctl.GitCommitNotification) {
			record("git_commit")(e, c)
		},
		OnGitReset: func(e *AgentExecution, r *agentctl.GitResetNotification) {
			record("git_reset")(e, r)
		},
		OnBranchSwitch: func(e *AgentExecution, b *agentctl.GitBranchSwitchNotification) {
			record("branch_switch")(e, b)
		},
		OnFileChange: func(e *AgentExecution, n *agentctl.FileChangeNotification) {
			record("file_change")(e, n)
		},
		OnProcessOutput: func(e *AgentExecution, o *agentctl.ProcessOutput) {
			record("process_output")(e, o)
		},
		OnProcessStatus: func(e *AgentExecution, s *agentctl.ProcessStatusUpdate) {
			record("process_status")(e, s)
		},
	}, nil, stopCh)
	cleanupStreamManager(t, stopCh, sm)
	return sm
}

// TestBuildWorkspaceCallbacksForwardsEveryHopWithItsExecution pins the full
// wiring chain from the agentctl workspace stream to the manager's handlers.
// A dropped hop here is invisible to any test that calls the manager handler
// directly: the stream would simply stop delivering that notification.
func TestBuildWorkspaceCallbacksForwardsEveryHopWithItsExecution(t *testing.T) {
	var recorded []recordedWorkspaceCallback
	sm := newRecordingStreamManager(t, &recorded)
	execution := &AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "session-1"}

	callbacks := sm.buildWorkspaceCallbacks(execution)

	status := &agentctl.GitStatusUpdate{Branch: "feature/cover", Timestamp: time.Now()}
	commit := &agentctl.GitCommitNotification{CommitSHA: "abc123"}
	reset := &agentctl.GitResetNotification{PreviousHead: "head-after"}
	branchSwitch := &agentctl.GitBranchSwitchNotification{CurrentBranch: "feature/cover"}
	fileChange := &agentctl.FileChangeNotification{Path: "src/main.go"}
	output := &agentctl.ProcessOutput{ProcessID: "proc-1", Data: "hello"}
	procStatus := &agentctl.ProcessStatusUpdate{ProcessID: "proc-1", Status: "failed"}

	callbacks.OnShellOutput("build ok\n")
	callbacks.OnShellExit(137)
	callbacks.OnGitStatus(status)
	callbacks.OnGitCommit(commit)
	callbacks.OnGitReset(reset)
	callbacks.OnBranchSwitch(branchSwitch)
	callbacks.OnFileChange(fileChange)
	callbacks.OnProcessOutput(output)
	callbacks.OnProcessStatus(procStatus)
	// These two only log; they must not panic or reach a handler.
	callbacks.OnConnected()
	callbacks.OnError("stream reset by peer")

	require.Len(t, recorded, 9)
	wantOrder := []string{
		"shell_output", "shell_exit", "git_status", "git_commit", "git_reset",
		"branch_switch", "file_change", "process_output", "process_status",
	}
	for i, want := range wantOrder {
		require.Equal(t, want, recorded[i].name, "hop %d", i)
		require.Same(t, execution, recorded[i].execution,
			"%s must be attributed to the execution the stream belongs to", want)
	}
	require.Equal(t, "build ok\n", recorded[0].payload)
	require.Equal(t, 137, recorded[1].payload)
	require.Same(t, status, recorded[2].payload)
	require.Same(t, commit, recorded[3].payload)
	require.Same(t, reset, recorded[4].payload)
	require.Same(t, branchSwitch, recorded[5].payload)
	require.Same(t, fileChange, recorded[6].payload)
	require.Same(t, output, recorded[7].payload)
	require.Same(t, procStatus, recorded[8].payload)
}

// TestBuildWorkspaceCallbacksToleratesUnwiredHandlers pins the nil guards: a
// StreamManager built without a given handler must drop the notification, not
// panic and tear down the workspace stream goroutine.
func TestBuildWorkspaceCallbacksToleratesUnwiredHandlers(t *testing.T) {
	stopCh := newTestStopCh(t)
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, stopCh)
	cleanupStreamManager(t, stopCh, sm)

	callbacks := sm.buildWorkspaceCallbacks(&AgentExecution{ID: "exec-1"})

	callbacks.OnShellOutput("data")
	callbacks.OnShellExit(0)
	callbacks.OnGitStatus(&agentctl.GitStatusUpdate{})
	callbacks.OnGitCommit(&agentctl.GitCommitNotification{})
	callbacks.OnGitReset(&agentctl.GitResetNotification{})
	callbacks.OnBranchSwitch(&agentctl.GitBranchSwitchNotification{})
	callbacks.OnFileChange(&agentctl.FileChangeNotification{})
	callbacks.OnProcessOutput(&agentctl.ProcessOutput{})
	callbacks.OnProcessStatus(&agentctl.ProcessStatusUpdate{})
	callbacks.OnConnected()
	callbacks.OnError("boom")
}

// AC-PLATFORM-WORKSPACE-GIT-STATUS-001.43: the attached workspace stream's
// callbacks remain valid while the same execution is promoted and restarted.
func TestBuildWorkspaceCallbacksContinueAfterRepeatedStartup(t *testing.T) {
	var recorded []recordedWorkspaceCallback
	sm := newRecordingStreamManager(t, &recorded)
	client := &agentctl.Client{}
	execution := &AgentExecution{ID: "exec-1", SessionID: "session-1", agentctl: client}
	callbacks := sm.buildWorkspaceCallbacks(execution, client)
	execution.SetWorkspaceStream(&agentctl.WorkspaceStream{})

	for attempt := 1; attempt <= 2; attempt++ {
		execution.beginStartupAttemptWithID("promotion")
		ready := make(chan struct{})
		sm.connectWorkspaceStream(execution, ready)
		select {
		case <-ready:
		default:
			t.Fatalf("attached stream reuse did not settle on startup attempt %d", attempt)
		}
	}

	status := &agentctl.GitStatusUpdate{Branch: "promoted"}
	commit := &agentctl.GitCommitNotification{CommitSHA: "promoted-commit"}
	reset := &agentctl.GitResetNotification{PreviousHead: "before", CurrentHead: "after"}
	branchSwitch := &agentctl.GitBranchSwitchNotification{CurrentBranch: "promoted"}
	fileChange := &agentctl.FileChangeNotification{Path: "after-promotion.go"}
	processOutput := &agentctl.ProcessOutput{ProcessID: "proc-1", Data: "promoted output"}
	processStatus := &agentctl.ProcessStatusUpdate{ProcessID: "proc-1", Status: "running"}
	callbacks.OnShellOutput("promoted shell output")
	callbacks.OnShellExit(0)
	callbacks.OnGitStatus(status)
	callbacks.OnGitCommit(commit)
	callbacks.OnGitReset(reset)
	callbacks.OnBranchSwitch(branchSwitch)
	callbacks.OnFileChange(fileChange)
	callbacks.OnProcessOutput(processOutput)
	callbacks.OnProcessStatus(processStatus)

	require.Len(t, recorded, 9)
	wantOrder := []string{
		"shell_output", "shell_exit", "git_status", "git_commit", "git_reset",
		"branch_switch", "file_change", "process_output", "process_status",
	}
	wantPayloads := []any{
		"promoted shell output", 0, status, commit, reset, branchSwitch,
		fileChange, processOutput, processStatus,
	}
	for i := range recorded {
		require.Equal(t, wantOrder[i], recorded[i].name)
		require.Same(t, execution, recorded[i].execution)
		if i < 2 {
			require.Equal(t, wantPayloads[i], recorded[i].payload)
		} else {
			require.Same(t, wantPayloads[i], recorded[i].payload)
		}
	}
}

func TestBuildWorkspaceCallbacksRejectsReplacedAgentctlClient(t *testing.T) {
	var recorded []recordedWorkspaceCallback
	sm := newRecordingStreamManager(t, &recorded)
	firstClient := &agentctl.Client{}
	replacementClient := &agentctl.Client{}
	execution := &AgentExecution{ID: "exec-1", SessionID: "session-1", agentctl: firstClient}
	callbacks := sm.buildWorkspaceCallbacks(execution, firstClient)

	execution.agentctlLifecycleMu.Lock()
	execution.replaceAgentctlClient(replacementClient)
	execution.agentctlLifecycleMu.Unlock()
	callbacks.OnGitStatus(&agentctl.GitStatusUpdate{Branch: "retired"})

	require.Empty(t, recorded)
}

func TestBuildWorkspaceCallbacksRejectsDetachedAgentctlClient(t *testing.T) {
	var recorded []recordedWorkspaceCallback
	sm := newRecordingStreamManager(t, &recorded)
	client := &agentctl.Client{}
	execution := &AgentExecution{ID: "exec-1", SessionID: "session-1", agentctl: client}
	callbacks := sm.buildWorkspaceCallbacks(execution, client)

	execution.agentctlLifecycleMu.Lock()
	execution.detachAgentctlClient()
	execution.agentctlLifecycleMu.Unlock()
	callbacks.OnGitStatus(&agentctl.GitStatusUpdate{Branch: "detached"})

	require.Empty(t, recorded)
}

func TestBuildWorkspaceCallbacksLeaseFencesClientReplacement(t *testing.T) {
	stopCh := newTestStopCh(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls int
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{
		OnGitStatus: func(*AgentExecution, *agentctl.GitStatusUpdate) {
			calls++
			if calls == 1 {
				close(entered)
				<-release
			}
		},
	}, nil, stopCh)
	cleanupStreamManager(t, stopCh, sm)

	streamClient := &agentctl.Client{}
	replacementClient := &agentctl.Client{}
	execution := &AgentExecution{ID: "exec-1", SessionID: "session-1", agentctl: streamClient}
	callbacks := sm.buildWorkspaceCallbacks(execution, streamClient)

	callbackDone := make(chan struct{})
	go func() {
		callbacks.OnGitStatus(&agentctl.GitStatusUpdate{Branch: "in-flight"})
		close(callbackDone)
	}()
	<-entered
	leaseHeld := !execution.agentctlSourceMu.TryLock()
	if !leaseHeld {
		execution.agentctlSourceMu.Unlock()
	}
	require.True(t, leaseHeld,
		"the active callback must hold the client source lease through its handler")

	replacementDone := make(chan struct{})
	replacementReady := make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseCallback()
	go func() {
		execution.agentctlLifecycleMu.Lock()
		close(replacementReady)
		execution.replaceAgentctlClient(replacementClient)
		execution.agentctlLifecycleMu.Unlock()
		close(replacementDone)
	}()
	<-replacementReady

	writerWaiting := make(chan struct{})
	observerStop := make(chan struct{})
	defer close(observerStop)
	go func() {
		for {
			if !execution.agentctlSourceMu.TryRLock() {
				close(writerWaiting)
				return
			}
			execution.agentctlSourceMu.RUnlock()
			select {
			case <-observerStop:
				return
			default:
			}
			runtime.Gosched()
		}
	}()
	select {
	case <-writerWaiting:
	case <-time.After(5 * time.Second):
		t.Fatal("client replacement did not reach the active source lease")
	}
	select {
	case <-replacementDone:
		t.Fatal("client replacement crossed an in-flight workspace callback")
	default:
	}

	releaseCallback()
	select {
	case <-callbackDone:
	case <-time.After(5 * time.Second):
		t.Fatal("workspace callback did not finish after releasing its handler")
	}
	select {
	case <-replacementDone:
	case <-time.After(5 * time.Second):
		t.Fatal("client replacement did not finish after the callback lease released")
	}

	callbacks.OnGitStatus(&agentctl.GitStatusUpdate{Branch: "retired"})
	require.Equal(t, 1, calls, "the old stream client must be rejected after replacement")
}

// TestStreamManagerStartRejectsWorkAfterWait pins the drain barrier: once Wait
// has run, no new stream goroutine may be spawned, and a caller waiting on
// `ready` is still released rather than blocked forever.
func TestStreamManagerStartRejectsWorkAfterWait(t *testing.T) {
	stopCh := newTestStopCh(t)
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, stopCh)
	// Registered before the Wait below and before any t.Fatal path, so the
	// drain still runs if this test fails early.
	cleanupStreamManager(t, stopCh, sm)
	sm.Wait()

	execution := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	ready := make(chan struct{})
	sm.ConnectWorkspaceStream(execution, ready)

	select {
	case <-ready:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ready was not closed after a rejected start; callers would block forever")
	}

	// ConnectMCPStream has no ready channel — it simply must not spawn.
	// The registered cleanup drains; no trailing Wait needed here.
	sm.ConnectMCPStream(execution)
}

func TestStopChannelContextErrReportsFirstClosedSignal(t *testing.T) {
	primary := make(chan struct{})
	secondary := make(chan struct{})
	ctx := &stopChannelContext{Context: context.Background(), primary: primary, secondary: secondary}

	require.NoError(t, ctx.Err(), "no signal has fired yet")

	close(secondary)
	require.ErrorIs(t, ctx.Err(), context.Canceled,
		"the StreamManager's internal drain signal must cancel the stream context")

	close(primary)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestStopChannelContextErrFallsBackToParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx := &stopChannelContext{Context: parent}

	require.NoError(t, ctx.Err())

	cancel()
	require.ErrorIs(t, ctx.Err(), context.Canceled,
		"with no stop channels the parent context still governs")
}
