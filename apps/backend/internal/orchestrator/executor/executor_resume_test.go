package executor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestResumeSession_RejectsArchivedTask(t *testing.T) {
	repo := newMockRepository()
	agentMgr := &mockAgentManager{}
	exec := newTestExecutor(t, agentMgr, repo)

	now := time.Now().UTC()
	archivedAt := now.Add(-time.Minute)

	repo.tasks["task-1"] = &models.Task{
		ID:         "task-1",
		ArchivedAt: &archivedAt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	repo.sessions["sess-1"] = &models.TaskSession{
		ID:             "sess-1",
		TaskID:         "task-1",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateWaitingForInput,
	}
	repo.executorsRunning["sess-1"] = &models.ExecutorRunning{
		SessionID: "sess-1",
		TaskID:    "task-1",
	}

	_, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
	if err == nil {
		t.Fatal("expected error when task is archived, got nil")
	}
	if !errors.Is(err, ErrTaskArchived) {
		t.Fatalf("expected ErrTaskArchived, got: %v", err)
	}
}

// setupLiveResumeTestFixture seeds a repo + task + session + executor-running
// record suitable for exercising the ResumeSession launch path.
func setupLiveResumeTestFixture(repo *mockRepository) {
	now := time.Now().UTC()
	repo.tasks["task-1"] = &models.Task{
		ID:        "task-1",
		CreatedAt: now,
		UpdatedAt: now,
	}
	repo.sessions["sess-1"] = &models.TaskSession{
		ID:             "sess-1",
		TaskID:         "task-1",
		AgentProfileID: "profile-1",
		State:          models.TaskSessionStateWaitingForInput,
	}
	repo.executorsRunning["sess-1"] = &models.ExecutorRunning{
		ID:          "sess-1",
		SessionID:   "sess-1",
		TaskID:      "task-1",
		ResumeToken: "token-abc",
	}
}

// TestResumeSession_LiveAgentReturnsAlreadyRunning ensures ResumeSession returns
// ErrExecutionAlreadyRunning instead of killing the live agent subprocess when
// a live agent is already registered for the session.
//
// Post-refactor, this is detected earlier than before: validateAndLockResume's
// GetExecutionBySession now consults executors_running + IsAgentRunningForSession
// directly, short-circuiting *before* LaunchAgent is called. Pre-refactor the
// race only surfaced via LaunchAgent's "already has an agent running" error and
// a follow-up probe — so this test asserts the new short-circuit path.
func TestResumeSession_LiveAgentReturnsAlreadyRunning(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)

	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return nil, fmt.Errorf("session %q already has an agent running (execution: %s)", req.SessionID, "exec-live")
		},
		isAgentRunningForSessionFunc: func(_ context.Context, _ string) bool {
			return true
		},
	}
	exec := newTestExecutor(t, agentMgr, repo)

	_, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)

	if !errors.Is(err, ErrExecutionAlreadyRunning) {
		t.Fatalf("expected ErrExecutionAlreadyRunning, got: %v", err)
	}
	if agentMgr.cleanupStaleExecutionCallCount != 0 {
		t.Errorf("expected no stale-cleanup call on live agent, got %d", agentMgr.cleanupStaleExecutionCallCount)
	}
	// LaunchAgent must NOT be called: validation short-circuits before reaching it.
	// This is the regression guard — the pre-refactor LaunchAgent + probe path could
	// race and kill a live agent if probe timing was wrong; the early short-circuit
	// closes that window.
	if agentMgr.launchAgentCallCount != 0 {
		t.Errorf("expected LaunchAgent NOT called (early short-circuit), got %d", agentMgr.launchAgentCallCount)
	}
	if len(agentMgr.isAgentRunningForSessionCallArgs) == 0 {
		t.Errorf("expected IsAgentRunningForSession to be consulted at least once")
	}
}

// TestResumeSession_StaleExecutionCleansUpAndRetries ensures the pre-existing
// stale-cleanup path still works when LaunchAgent reports "already has an agent
// running" but the lifecycle manager confirms no live agent exists.
func TestResumeSession_StaleExecutionCleansUpAndRetries(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)

	var launchCalls int
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchCalls++
			if launchCalls == 1 {
				return nil, fmt.Errorf("session %q already has an agent running (execution: %s)", req.SessionID, "exec-stale")
			}
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-new",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		isAgentRunningForSessionFunc: func(_ context.Context, _ string) bool {
			return false
		},
	}
	exec := newTestExecutor(t, agentMgr, repo)

	execution, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
	if err != nil {
		t.Fatalf("expected success after stale cleanup + retry, got: %v", err)
	}
	if execution == nil {
		t.Fatal("expected a non-nil execution after retry")
	}
	if execution.AgentExecutionID != "exec-new" {
		t.Errorf("expected AgentExecutionID=exec-new, got %q", execution.AgentExecutionID)
	}
	if agentMgr.cleanupStaleExecutionCallCount != 1 {
		t.Errorf("expected stale-cleanup called once, got %d", agentMgr.cleanupStaleExecutionCallCount)
	}
	if agentMgr.launchAgentCallCount != 2 {
		t.Errorf("expected LaunchAgent called twice, got %d", agentMgr.launchAgentCallCount)
	}
}

// TestResumeSession_FailedStateForceCleansUpStaleState covers the FAILED-task
// resume scenario where agentctl wrongly reports "starting" and the executionStore
// still tracks a stale AgentExecution. The pre-launch cleanup should wipe stale
// state so the fresh LaunchAgent succeeds without hitting the "resume race" path —
// and crucially without probing IsAgentRunningForSession (which would return true
// for the stale status).
func TestResumeSession_FailedStateForceCleansUpStaleState(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateFailed

	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-new",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		// Returns true if accidentally called; the assertion below proves it is not.
		isAgentRunningForSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	exec := newTestExecutor(t, agentMgr, repo)

	execution, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if execution == nil || execution.AgentExecutionID != "exec-new" {
		t.Fatalf("expected fresh execution exec-new, got %+v", execution)
	}
	if agentMgr.cleanupStaleExecutionCallCount != 1 {
		t.Errorf("expected stale-cleanup called exactly once before LaunchAgent, got %d",
			agentMgr.cleanupStaleExecutionCallCount)
	}
	if agentMgr.launchAgentCallCount != 1 {
		t.Errorf("expected LaunchAgent called once (no retry), got %d", agentMgr.launchAgentCallCount)
	}
	// Terminal-state resume must bypass the stale "starting" liveness probe
	// entirely — otherwise it would wrongly re-trigger ErrExecutionAlreadyRunning.
	if len(agentMgr.isAgentRunningForSessionCallArgs) != 0 {
		t.Errorf("expected IsAgentRunningForSession NOT called for terminal-state resume, got %v",
			agentMgr.isAgentRunningForSessionCallArgs)
	}
}

// TestResumeSession_CancelledStateForceCleansUpStaleState mirrors the FAILED
// scenario for CANCELLED sessions: stale state must not block the relaunch.
func TestResumeSession_CancelledStateForceCleansUpStaleState(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateCancelled

	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-new",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		isAgentRunningForSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	exec := newTestExecutor(t, agentMgr, repo)

	if _, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if agentMgr.cleanupStaleExecutionCallCount != 1 {
		t.Errorf("expected stale-cleanup called once, got %d", agentMgr.cleanupStaleExecutionCallCount)
	}
	if len(agentMgr.isAgentRunningForSessionCallArgs) != 0 {
		t.Errorf("expected IsAgentRunningForSession NOT called for CANCELLED resume, got %v",
			agentMgr.isAgentRunningForSessionCallArgs)
	}
}

// TestResumeSession_PropagatesTaskEnvironmentID is a regression test for a
// post-restart bug where `buildResumeRequest` did not copy
// `session.TaskEnvironmentID` onto the LaunchAgentRequest. The lifecycle's
// in-memory ExecutionStore indexes executions by TaskEnvironmentID for the
// shell terminal lookup (`GetByTaskEnvironmentID`); a request without the env
// ID produced an execution tagged with TaskEnvironmentID="" so the shell
// terminal handler hit the "task environment ... has no workspace path yet"
// 503 fallback and stayed stuck on "Connecting terminal..." after restart.
func TestResumeSession_PropagatesTaskEnvironmentID(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].TaskEnvironmentID = "env-1"

	var capturedReq *LaunchAgentRequest
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			capturedReq = req
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-new",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		isAgentRunningForSessionFunc: func(_ context.Context, _ string) bool { return false },
	}
	exec := newTestExecutor(t, agentMgr, repo)

	if _, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true); err != nil {
		t.Fatalf("expected resume success, got: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("expected LaunchAgent to be called with a request")
	}
	if capturedReq.TaskEnvironmentID != "env-1" {
		t.Errorf("TaskEnvironmentID = %q, want %q — without this the lifecycle execution is indexed under empty env_id and GetByTaskEnvironmentID never finds it",
			capturedReq.TaskEnvironmentID, "env-1")
	}
}

// TestResumeSession_TerminalStateSkipsLivenessProbeOnFallback covers the
// residual path where the preemptive CleanupStaleExecutionBySessionID is not
// enough (e.g. the first LaunchAgent still returns "already has an agent running"
// because a stale executionStore entry survived). For terminal-state sessions
// the fallback block MUST skip the IsAgentRunningForSession probe and go
// straight to cleanup+retry, otherwise a stale agentctl "starting" status
// silently regresses to ErrExecutionAlreadyRunning — the original bug.
func TestResumeSession_TerminalStateSkipsLivenessProbeOnFallback(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateFailed

	var launchCalls int
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchCalls++
			if launchCalls == 1 {
				return nil, fmt.Errorf("session %q already has an agent running (execution: %s)", req.SessionID, "exec-stale")
			}
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-new",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		// Live-probe would return true (stale "starting") — if we wrongly call
		// it, the test fails because err is ErrExecutionAlreadyRunning.
		isAgentRunningForSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	exec := newTestExecutor(t, agentMgr, repo)

	execution, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
	if err != nil {
		t.Fatalf("expected success after fallback cleanup+retry, got: %v", err)
	}
	if execution == nil || execution.AgentExecutionID != "exec-new" {
		t.Fatalf("expected fresh execution exec-new, got %+v", execution)
	}
	if agentMgr.launchAgentCallCount != 2 {
		t.Errorf("expected LaunchAgent called twice (first fails, second succeeds), got %d", agentMgr.launchAgentCallCount)
	}
	// 1 preemptive + 1 fallback cleanup.
	if agentMgr.cleanupStaleExecutionCallCount != 2 {
		t.Errorf("expected stale-cleanup called twice, got %d", agentMgr.cleanupStaleExecutionCallCount)
	}
	if len(agentMgr.isAgentRunningForSessionCallArgs) != 0 {
		t.Errorf("expected IsAgentRunningForSession NOT called for terminal-state fallback, got %v",
			agentMgr.isAgentRunningForSessionCallArgs)
	}
}

// TestResumeSession_ConcurrentResumeReReadsFreshState exercises the concurrent-
// resume race: the caller's session object has a stale State=FAILED, but a
// concurrent resume already transitioned the DB to STARTING. validateAndLockResume
// MUST re-read the session state inside the lock — otherwise isTerminalSessionState
// wrongly bypasses the live-execution guard, CleanupStaleExecutionBySessionID
// wipes the live AgentExecution, and LaunchAgent launches a duplicate agent.
func TestResumeSession_ConcurrentResumeReReadsFreshState(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)

	// Caller's in-memory session carries a stale FAILED state.
	callerSession := *repo.sessions["sess-1"]
	callerSession.State = models.TaskSessionStateFailed

	// DB truth: a concurrent resume already transitioned this session to STARTING.
	repo.sessions["sess-1"].State = models.TaskSessionStateStarting
	repo.sessions["sess-1"].UpdatedAt = time.Now().UTC()
	repo.sessions["sess-1"].AgentExecutionID = "exec-live"

	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			t.Fatal("LaunchAgent must not be called — the live execution guard must reject the duplicate resume")
			return nil, nil
		},
		// Live execution was registered by the first resume; probe returns true.
		isAgentRunningForSessionFunc: func(_ context.Context, _ string) bool { return true },
	}
	exec := newTestExecutor(t, agentMgr, repo)

	_, err := exec.ResumeSession(context.Background(), &callerSession, true)
	if !errors.Is(err, ErrExecutionAlreadyRunning) {
		t.Fatalf("expected ErrExecutionAlreadyRunning (live agent protected), got: %v", err)
	}
	if agentMgr.cleanupStaleExecutionCallCount != 0 {
		t.Errorf("cleanup must NOT be called when a live execution is detected, got %d",
			agentMgr.cleanupStaleExecutionCallCount)
	}
}

// TestResumeSession_AbortsIfSessionReReadFails locks down the abort-on-error
// behavior of the in-lock session re-read. If GetTaskSession fails transiently,
// silently falling back to the caller's (potentially stale) session.State would
// reintroduce the concurrent-resume duplicate-launch race. The resume MUST
// return the fetch error without calling LaunchAgent or CleanupStaleExecutionBySessionID.
func TestResumeSession_AbortsIfSessionReReadFails(t *testing.T) {
	repo := newMockRepository()
	setupLiveResumeTestFixture(repo)
	repo.sessions["sess-1"].State = models.TaskSessionStateFailed

	wantErr := fmt.Errorf("transient DB error")
	var callCount int
	repo.getTaskSessionFunc = func(_ context.Context, _ string) (*models.TaskSession, error) {
		callCount++
		// First call is the caller's pre-lock fetch (via the service, not
		// reached directly in this test). The re-read inside the lock is the
		// only GetTaskSession the executor makes, so fail it unconditionally.
		return nil, wantErr
	}

	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, _ *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			t.Fatal("LaunchAgent must not be called when the session re-read fails")
			return nil, nil
		},
	}
	exec := newTestExecutor(t, agentMgr, repo)

	_, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected transient DB error to be returned, got: %v", err)
	}
	if agentMgr.launchAgentCallCount != 0 {
		t.Errorf("expected LaunchAgent NOT called, got %d", agentMgr.launchAgentCallCount)
	}
	if agentMgr.cleanupStaleExecutionCallCount != 0 {
		t.Errorf("expected CleanupStaleExecutionBySessionID NOT called, got %d",
			agentMgr.cleanupStaleExecutionCallCount)
	}
	if callCount == 0 {
		t.Error("expected the in-lock session re-read to be called at least once")
	}
}

// TestIsTerminalSessionState is a small unit test for the helper that drives
// both the preemptive cleanup and the validateAndLockResume carve-out.
// TestApplyResumeRepoConfig_BaseBranchByExecutorType locks in the per-executor
// rules for BaseBranch propagation on resume:
//   - clone-based remote executors (Sprites, Docker variants) MUST receive
//     BaseBranch so the in-sandbox prepare script's `git clone --branch <X>`
//     resolves;
//   - the worktree executor receives BaseBranch via its dedicated block;
//   - the local executor MUST NOT receive BaseBranch — LocalPreparer reads it
//     and would force a `git fetch && git checkout` against the user's actual
//     workspace, clobbering the "use current state" UX.
func TestApplyResumeRepoConfig_BaseBranchByExecutorType(t *testing.T) {
	cases := []struct {
		executorType        string
		wantBaseBranch      string
		wantWorktreeApplied bool
	}{
		{executorType: "sprites", wantBaseBranch: "main"},
		{executorType: "local_docker", wantBaseBranch: "main"},
		{executorType: "remote_docker", wantBaseBranch: "main"},
		{executorType: "worktree", wantBaseBranch: "main", wantWorktreeApplied: true},
		{executorType: "local", wantBaseBranch: ""},
	}

	for _, tc := range cases {
		t.Run(tc.executorType, func(t *testing.T) {
			repo := newMockRepository()
			repo.repositories["repo-1"] = &models.Repository{
				ID:            "repo-1",
				LocalPath:     "/tmp/repo",
				Provider:      "github",
				ProviderOwner: "kdlbs",
				ProviderName:  "kandev",
			}
			task := &v1.Task{ID: "task-1"}
			repo.tasks["task-1"] = &models.Task{ID: "task-1"}
			session := &models.TaskSession{
				ID:           "sess-1",
				TaskID:       "task-1",
				RepositoryID: "repo-1",
				BaseBranch:   "main",
			}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)

			req := &LaunchAgentRequest{
				TaskID:       "task-1",
				SessionID:    "sess-1",
				ExecutorType: tc.executorType,
			}

			if _, err := exec.applyResumeRepoConfig(context.Background(), task, session, req); err != nil {
				t.Fatalf("applyResumeRepoConfig: %v", err)
			}

			if req.BaseBranch != tc.wantBaseBranch {
				t.Fatalf("BaseBranch = %q, want %q", req.BaseBranch, tc.wantBaseBranch)
			}
			if tc.wantWorktreeApplied != req.UseWorktree {
				t.Fatalf("UseWorktree = %v, want %v", req.UseWorktree, tc.wantWorktreeApplied)
			}
		})
	}
}

func TestIsTerminalSessionState(t *testing.T) {
	cases := []struct {
		state models.TaskSessionState
		want  bool
	}{
		{models.TaskSessionStateFailed, true},
		{models.TaskSessionStateCancelled, true},
		{models.TaskSessionStateWaitingForInput, false},
		{models.TaskSessionStateRunning, false},
		{models.TaskSessionStateStarting, false},
		{models.TaskSessionStateCompleted, false},
	}
	for _, c := range cases {
		if got := isTerminalSessionState(c.state); got != c.want {
			t.Errorf("isTerminalSessionState(%q) = %v, want %v", c.state, got, c.want)
		}
	}
}
