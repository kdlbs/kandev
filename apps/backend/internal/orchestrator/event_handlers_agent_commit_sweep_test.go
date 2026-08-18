package orchestrator

import (
	"context"
	"testing"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// Regression: the live commit-created event can miss a commit (e.g. pushed
// just before agentctl's next poll tick, see filterLocalCommits' upstream
// filter). captureSessionCommitsSweep is the per-turn reconcile pass that
// runs while the agent process is still alive - unlike archive capture,
// whose GetGitLog call usually finds it already gone - and must persist any
// commit the live path missed.
func TestCaptureSessionCommitsSweep_FillsCommitLivePathMissed(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-sweep", "s-sweep", "step1")

	session, err := testRepo.GetTaskSession(ctx, "s-sweep")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.BaseCommitSHA = "base-sha"
	if err := testRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set base commit: %v", err)
	}

	agentMgr := &mockAgentManager{
		getGitLogFunc: func(_ context.Context, sessionID, baseCommit string, _ int, _ string) (*client.GitLogResult, error) {
			if sessionID != "s-sweep" || baseCommit != "base-sha" {
				t.Fatalf("GetGitLog called with (%q, %q), want (s-sweep, base-sha)", sessionID, baseCommit)
			}
			return &client.GitLogResult{
				Success: true,
				Commits: []*client.GitCommitInfo{
					{CommitSHA: "missed-sha", CommitMessage: "feat: missed by live path", CommittedAt: "2026-04-02T03:04:05Z", Insertions: 5},
				},
			}, nil
		},
	}
	svc := createTestServiceWithAgent(testRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.captureSessionCommitsSweep(ctx, "s-sweep")

	commits, err := testRepo.GetSessionCommits(ctx, "s-sweep")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("commit rows = %d, want 1", len(commits))
	}
	if commits[0].CommitSHA != "missed-sha" {
		t.Errorf("CommitSHA = %q, want missed-sha", commits[0].CommitSHA)
	}
}

// Regression: when the agent process is gone (GetGitLog returns a nil
// result with a nil error, matching the documented "agent not running"
// contract), the sweep must not error or persist anything - archive capture
// remains the final safety net.
func TestCaptureSessionCommitsSweep_AgentNotRunningIsNoop(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-sweep-gone", "s-sweep-gone", "step1")

	session, err := testRepo.GetTaskSession(ctx, "s-sweep-gone")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.BaseCommitSHA = "base-sha"
	if err := testRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set base commit: %v", err)
	}

	agentMgr := &mockAgentManager{
		getGitLogFunc: func(context.Context, string, string, int, string) (*client.GitLogResult, error) {
			return nil, nil
		},
	}
	svc := createTestServiceWithAgent(testRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.captureSessionCommitsSweep(ctx, "s-sweep-gone")

	commits, err := testRepo.GetSessionCommits(ctx, "s-sweep-gone")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("commit rows = %d, want 0", len(commits))
	}
}

// Regression: captureSessionCommitsSweep runs inside handleAgentCompletedLocked
// while it holds the per-session cancel-in-flight mutex (see
// acquireCancelInFlightGuard) - the same mutex stopTaskSessionForCoordinator
// and DeleteSession must acquire to serve a user's Stop/Cancel/Delete for
// this session. GetGitLog is a real `git log --shortstat` shellout through
// agentctl, bounded only by the agentctl HTTP client's 60s default timeout
// when passed an undecorated context. The archive-capture call site
// (service_tasks.go's CaptureArchiveSnapshot) deliberately wraps the same
// GetGitLog call in a 10s context.WithTimeout for exactly this reason
// ("prevent blocking the archive operation if agentctl is stuck") - the
// sweep must do the same so a stuck agentctl cannot block Stop/Cancel/Delete
// for up to a minute on every turn completion.
func TestCaptureSessionCommitsSweep_BoundsGetGitLogWithTimeout(t *testing.T) {
	ctx := context.Background()
	testRepo := setupTestRepo(t)
	seedSession(t, testRepo, "t-sweep-bound", "s-sweep-bound", "step1")

	session, err := testRepo.GetTaskSession(ctx, "s-sweep-bound")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.BaseCommitSHA = "base-sha"
	if err := testRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set base commit: %v", err)
	}

	var sawDeadline bool
	agentMgr := &mockAgentManager{
		getGitLogFunc: func(callCtx context.Context, _, _ string, _ int, _ string) (*client.GitLogResult, error) {
			deadline, ok := callCtx.Deadline()
			sawDeadline = ok
			if ok && time.Until(deadline) > 11*time.Second {
				t.Errorf("GetGitLog context deadline is %s out, want <= ~10s (matching the archive-capture bound)",
					time.Until(deadline))
			}
			return &client.GitLogResult{Success: true}, nil
		},
	}
	svc := createTestServiceWithAgent(testRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.captureSessionCommitsSweep(ctx, "s-sweep-bound")

	if !sawDeadline {
		t.Fatal("captureSessionCommitsSweep called GetGitLog with an unbounded context (no deadline) - " +
			"it runs inside the per-session cancel mutex and must not be able to block Stop/Cancel/Delete " +
			"for up to the agentctl client's full 60s timeout")
	}
}

// Regression: handleAgentCompletedLocked must run captureSessionCommitsSweep
// for every completed turn - including one that ends with a pending
// clarification. The pending-clarification branch returns early (deferring
// the workflow transition, captureGitStatusSnapshot, and
// finalizeAutomationRun to a later turn), but the agent process is still
// alive for THIS turn and any commit made during it would otherwise never be
// captured by the live event path alone (see
// TestCaptureSessionCommitsSweep_FillsCommitLivePathMissed). Skipping the
// sweep here silently drops that turn's commits from task_session_commits.
func TestHandleAgentCompleted_CommitSweepRunsEvenWhileClarificationPending(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-sweep-clarify", "s-sweep-clarify", "step1")
	setSessionExecID(t, repo, "s-sweep-clarify", "exec-1")

	session, err := repo.GetTaskSession(ctx, "s-sweep-clarify")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.BaseCommitSHA = "base-sha"
	session.State = models.TaskSessionStateRunning
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set base commit and session state: %v", err)
	}

	now := time.Now().UTC()
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-sweep-clarify", TaskSessionID: "s-sweep-clarify", TaskID: "t-sweep-clarify", StartedAt: now,
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "clarify-sweep-1", TaskSessionID: "s-sweep-clarify", TaskID: "t-sweep-clarify", TurnID: "turn-sweep-clarify",
		AuthorType: models.MessageAuthorAgent, Type: "clarification_request", Content: "Which approach?",
		CreatedAt: now,
		Metadata:  map[string]interface{}{"pending_id": "pending-1", "question_id": "q1", "status": "pending"},
	}); err != nil {
		t.Fatalf("create pending clarification message: %v", err)
	}

	gitLogCalls := 0
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		getGitLogFunc: func(_ context.Context, sessionID, baseCommit string, _ int, _ string) (*client.GitLogResult, error) {
			gitLogCalls++
			if sessionID != "s-sweep-clarify" || baseCommit != "base-sha" {
				t.Fatalf("GetGitLog called with (%q, %q), want (s-sweep-clarify, base-sha)", sessionID, baseCommit)
			}
			return &client.GitLogResult{
				Success: true,
				Commits: []*client.GitCommitInfo{
					{CommitSHA: "mid-turn-sha", CommitMessage: "feat: made before the clarification question", CommittedAt: "2026-04-02T03:04:05Z", Insertions: 3},
				},
			}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.turnService = &repoBackedTurnService{repo: repo}

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID: "t-sweep-clarify", SessionID: "s-sweep-clarify", AgentExecutionID: "exec-1",
	})

	if gitLogCalls != 1 {
		t.Fatalf("GetGitLog calls = %d, want 1 (sweep must run even when a turn ends with pending clarification)", gitLogCalls)
	}
	commits, err := repo.GetSessionCommits(ctx, "s-sweep-clarify")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 1 || commits[0].CommitSHA != "mid-turn-sha" {
		t.Fatalf("commits = %+v, want exactly [mid-turn-sha]", commits)
	}
}

// Regression: handleAgentCompletedLocked's rotated-execution guard (a
// profile switch already replaced this session's live execution before this
// stale agent.completed event arrived) must not skip captureSessionCommitsSweep.
// GetGitLog is resolved by session ID, not execution ID, and a session's
// worktree is shared across its executions (see internal/worktree/Manager),
// so a live GetGitLog call issued through the NEW execution still reads the
// commits the OLD execution made before rotation - the exact commits this
// stale event's own turn would otherwise never get another chance to record.
func TestHandleAgentCompleted_CommitSweepRunsWhenExecutionRotated(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-sweep-rotate", "s-sweep-rotate", "step1")
	seedExecutorRunning(t, repo, "s-sweep-rotate", "t-sweep-rotate", "exec-new")

	session, err := repo.GetTaskSession(ctx, "s-sweep-rotate")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.BaseCommitSHA = "base-sha"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set base commit: %v", err)
	}

	gitLogCalls := 0
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		getGitLogFunc: func(_ context.Context, sessionID, baseCommit string, _ int, _ string) (*client.GitLogResult, error) {
			gitLogCalls++
			if sessionID != "s-sweep-rotate" || baseCommit != "base-sha" {
				t.Fatalf("GetGitLog called with (%q, %q), want (s-sweep-rotate, base-sha)", sessionID, baseCommit)
			}
			return &client.GitLogResult{
				Success: true,
				Commits: []*client.GitCommitInfo{
					{CommitSHA: "pre-rotation-sha", CommitMessage: "feat: made by the old execution", CommittedAt: "2026-04-02T03:04:05Z", Insertions: 4},
				},
			}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	// The event's AgentExecutionID ("exec-old") differs from the seeded live
	// execution ("exec-new"), so this fires the rotated-execution guard.
	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID: "t-sweep-rotate", SessionID: "s-sweep-rotate", AgentExecutionID: "exec-old",
	})

	if gitLogCalls != 1 {
		t.Fatalf("GetGitLog calls = %d, want 1 (sweep must run even when the rotated-execution guard fires)", gitLogCalls)
	}
	commits, err := repo.GetSessionCommits(ctx, "s-sweep-rotate")
	if err != nil {
		t.Fatalf("GetSessionCommits: %v", err)
	}
	if len(commits) != 1 || commits[0].CommitSHA != "pre-rotation-sha" {
		t.Fatalf("commits = %+v, want exactly [pre-rotation-sha]", commits)
	}
}

// Regression: handleAgentCompletedLocked's terminal-session-state guard (the
// session was already deliberately stopped via completeAndStopSession before
// this stale agent.completed event arrived) must not skip
// captureSessionCommitsSweep either. In this scenario the underlying agent
// process is very likely already torn down, so the sweep's GetGitLog call is
// expected to be attempted and to safely no-op (see
// TestCaptureSessionCommitsSweep_AgentNotRunningIsNoop) - the point of this
// test is that the sweep is reached at all, not skipped by the same early
// return that skips the workflow-transition logic.
func TestHandleAgentCompleted_CommitSweepRunsWhenSessionAlreadyTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-sweep-terminal", "s-sweep-terminal", "step1")

	session, err := repo.GetTaskSession(ctx, "s-sweep-terminal")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.BaseCommitSHA = "base-sha"
	session.State = models.TaskSessionStateCompleted
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set base commit and terminal state: %v", err)
	}

	gitLogCalls := 0
	agentMgr := &mockAgentManager{
		getGitLogFunc: func(_ context.Context, sessionID, baseCommit string, _ int, _ string) (*client.GitLogResult, error) {
			gitLogCalls++
			if sessionID != "s-sweep-terminal" || baseCommit != "base-sha" {
				t.Fatalf("GetGitLog called with (%q, %q), want (s-sweep-terminal, base-sha)", sessionID, baseCommit)
			}
			// The old execution is torn down by the time this stale event
			// arrives - matches the documented "agent not running" contract.
			return nil, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID: "t-sweep-terminal", SessionID: "s-sweep-terminal", AgentExecutionID: "exec-old",
	})

	if gitLogCalls != 1 {
		t.Fatalf("GetGitLog calls = %d, want 1 (sweep must be attempted even when the terminal-state guard fires)", gitLogCalls)
	}
}

// Regression: captureSessionCommitsSweep must not run while
// handleAgentCompleted holds the per-session cancel-in-flight mutex
// (acquireCancelInFlightGuard) - the same mutex ~20 other call sites across
// internal/orchestrator/ need to serve a user's Stop/Cancel/Delete for this
// session. GetGitLog is a real git shellout through agentctl that can take
// up to the sweep's 10s bound (see
// TestCaptureSessionCommitsSweep_BoundsGetGitLogWithTimeout); holding the
// mutex for that long blocks those operations. This test blocks GetGitLog
// mid-flight and asserts a concurrent caller can still acquire the same
// per-session mutex immediately, rather than waiting behind the sweep.
func TestHandleAgentCompleted_SweepDoesNotBlockCancelInFlightMutex(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-sweep-nolock", "s-sweep-nolock", "step1")

	session, err := repo.GetTaskSession(ctx, "s-sweep-nolock")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.BaseCommitSHA = "base-sha"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set base commit: %v", err)
	}

	gitLogStarted := make(chan struct{})
	releaseGitLog := make(chan struct{})
	agentMgr := &mockAgentManager{
		getGitLogFunc: func(context.Context, string, string, int, string) (*client.GitLogResult, error) {
			close(gitLogStarted)
			<-releaseGitLog
			return &client.GitLogResult{Success: true}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.handleAgentCompleted(ctx, watcher.AgentEventData{
			TaskID: "t-sweep-nolock", SessionID: "s-sweep-nolock", AgentExecutionID: "exec-1",
		})
	}()

	select {
	case <-gitLogStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("GetGitLog was never called")
	}

	lock, release := svc.acquireCancelInFlightGuard("s-sweep-nolock")
	defer release()
	acquired := make(chan struct{})
	go func() {
		lock.Lock()
		close(acquired)
	}()
	select {
	case <-acquired:
		lock.Unlock()
	case <-time.After(200 * time.Millisecond):
		t.Fatal("acquireCancelInFlightGuard's mutex was held by the in-flight commit sweep - " +
			"Stop/Cancel/Delete on this session would block for up to the sweep's full duration")
	}

	close(releaseGitLog)
	<-done
}
