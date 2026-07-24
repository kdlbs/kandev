package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ADR-0049 narrowed the busy gate so a RUNNING session whose foreground turn has
// yielded to background work accepts a new prompt. checkSessionPromptable only
// *reads* that substate, though, and PromptTask does real work between the read
// and the point the turn is marked generating (session reload, ensureSessionRunning,
// an optionally network-bound model switch). Two prompts arriving in that window —
// a double-send, or two tabs on the same session — would both pass the read and
// both reach executor.Prompt, starting overlapping turns on one ACP session.
//
// claimForegroundTurn closes the window: the check and the claim happen under one
// lock, so exactly one caller wins.

// TestClaimForegroundTurn_OnlyOneConcurrentPromptWins is the regression test for
// that race. Every claim but one must lose, no matter how many race in at once.
func TestClaimForegroundTurn_OnlyOneConcurrentPromptWins(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-race"
	const contenders = 32

	// The agent spawned background work and went idle in the foreground: the gate
	// is open, and every contender below is about to read it as open.
	svc.registerBackgroundTask(sessionID, "tool-subagent-1")
	svc.markForegroundIdle(sessionID)

	var (
		start sync.WaitGroup
		done  sync.WaitGroup
		mu    sync.Mutex
		won   int
	)
	start.Add(1)
	for range contenders {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait() // release them all into the window together
			if svc.claimForegroundTurn(sessionID) != nil {
				mu.Lock()
				won++
				mu.Unlock()
			}
		}()
	}
	start.Done()
	done.Wait()

	if won != 1 {
		t.Fatalf("exactly one concurrent prompt may claim the background-idle turn, got %d winners", won)
	}
	// The winner drives the turn, so the session now reads as foreground-generating
	// and every later prompt is gated again.
	if !svc.isForegroundTurnGenerating(sessionID) {
		t.Fatal("after the claim the foreground turn must read as generating")
	}
}

// TestPromptTask_ConcurrentPromptsIntoBackgroundIdleStartOneTurn is the same
// regression driven through the REAL operator entrypoint. It is the assertion
// that actually matters: no matter how many prompts land in the background-idle
// window at once, exactly one may reach the agent. The rest must be rejected with
// ErrAgentPromptInProgress — overlapping turns on a single ACP session are the
// failure this prevents.
//
// Note the serial version of this cannot fail: the first PromptTask marks the
// foreground generating on its way through, so a *subsequent* prompt is gated even
// without the claim. Only genuine concurrency exposes the check-then-act window,
// which is wide — a session reload and ensureSessionRunning sit inside it.
func TestPromptTask_ConcurrentPromptsIntoBackgroundIdleStartOneTurn(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-concurrent"
		prompters = 8
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-1")
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	// The agent kicks off a run_in_background shell and goes idle in the foreground,
	// so the gate opens: every prompt below is about to read it as open.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:      taskID,
		SessionID:   sessionID,
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "bash-1",
			ToolStatus: "in_progress",
			Normalized: streams.NewShellExec("npm run dev", "", "", 0, true),
		},
	})
	emitForegroundIdle(svc, taskID, sessionID)

	// The operator double-sends, or two tabs fire at once.
	var (
		start sync.WaitGroup
		done  sync.WaitGroup
		mu    sync.Mutex
		accepted,
		rejectedBusy int
	)
	start.Add(1)
	for range prompters {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			_, err := svc.PromptTask(context.Background(), taskID, sessionID, "are you still working?", "", false, nil, false)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				accepted++
			case errors.Is(err, ErrAgentPromptInProgress):
				rejectedBusy++
			default:
				t.Errorf("unexpected prompt error: %v", err)
			}
		}()
	}
	start.Done()
	done.Wait()

	if accepted != 1 {
		t.Fatalf("exactly one concurrent prompt may open a turn, got %d accepted (%d rejected busy)", accepted, rejectedBusy)
	}
	if rejectedBusy != prompters-1 {
		t.Fatalf("every prompt that lost the claim must be rejected with ErrAgentPromptInProgress, got %d of %d", rejectedBusy, prompters-1)
	}
	// The decisive assertion: only one turn was actually started on the agent.
	agentMgr.mu.Lock()
	captured := len(agentMgr.capturedPrompts)
	agentMgr.mu.Unlock()
	if captured != 1 {
		t.Fatalf("overlapping turns reached the agent: %d prompts forwarded, want exactly 1", captured)
	}
}

// A queued dispatch can lose its ownership token after the initial promptability
// check but before it claims the session RUNNING. If that happens after the
// background-idle foreground was claimed, the failed dispatch must reopen the
// foreground gate so a later prompt is not locked out.
func TestPromptTask_SupersededQueuedDispatchReleasesForegroundClaim(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	const (
		taskID    = "task1"
		sessionID = "session-superseded-queued-dispatch"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-1")
	svc.registerBackgroundTask(sessionID, "background-1")
	svc.markForegroundIdle(sessionID)

	_, err := svc.promptTask(
		context.Background(), taskID, sessionID, "queued prompt", "", false, nil, false, "stale-entry",
	)
	if !errors.Is(err, errQueuedDispatchSuperseded) {
		t.Fatalf("a dispatch without the current ownership token must be superseded, got: %v", err)
	}
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("a superseded dispatch must restore the background-idle gate, got %q", got)
	}
	if svc.claimForegroundTurn(sessionID) == nil {
		t.Fatal("a later prompt must be able to claim the foreground after the superseded dispatch")
	}

	agentMgr.mu.Lock()
	forwarded := len(agentMgr.capturedPrompts)
	agentMgr.mu.Unlock()
	if forwarded != 0 {
		t.Fatalf("a superseded queued dispatch must not reach the agent, captured=%d", forwarded)
	}
}

// The claim has to be durable against the background set moving underneath it.
// A background tool_call can land while a prompt is mid-admission. Registration
// must not override the in-flight foreground claim or reopen the gate.
func TestClaimForegroundTurn_BackgroundRegistrationCannotReopenTheAdmissionWindow(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-reopen"
	svc.registerBackgroundTask(sessionID, "tool-subagent-1")
	svc.markForegroundIdle(sessionID)

	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("the first prompt must win the claim")
	}

	// The agent spawns a second background task while the first prompt is still in
	// preflight (reloading the session, ensuring the agent is running, ...).
	svc.registerBackgroundTask(sessionID, "tool-subagent-2")

	if !svc.isForegroundTurnGenerating(sessionID) {
		t.Fatal("a session with a prompt in flight must stay un-promptable")
	}
	if svc.claimForegroundTurn(sessionID) != nil {
		t.Fatal("a new background task must not reopen the admission window under an in-flight prompt")
	}

	// Dispatch alone is not an idle boundary; the foreground keeps precedence.
	if svc.completeForegroundClaim(claim) {
		t.Fatal("dispatch must not expose background work before foreground idle")
	}
	if !svc.isForegroundTurnGenerating(sessionID) {
		t.Fatal("after handoff the foreground must remain generating")
	}
	if !svc.markForegroundIdle(sessionID) {
		t.Fatal("foreground idle must expose the outstanding background work")
	}
}

// A release must not stomp a foreground that started generating for real while the
// failing prompt was in preflight. Handing the turn back to background-idle there
// would let a second prompt overlap a live turn — the very thing the claim prevents.
func TestReleaseForegroundClaim_DoesNotReopenGateOverALiveForeground(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-stale-release"
	svc.registerBackgroundTask(sessionID, "tool-subagent-1")
	svc.markForegroundIdle(sessionID)

	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("the prompt must win the claim")
	}

	// While the prompt is in preflight, the agent's foreground streams real output:
	// the turn is genuinely generating again, whatever happens to this prompt.
	svc.markForegroundGenerating(sessionID)

	// The prompt then fails. Its claim is stale — releasing must not reopen the gate.
	if svc.releaseForegroundClaim(claim) {
		t.Fatal("a stale claim must not hand a live generating foreground back to background-idle")
	}
	if !svc.isForegroundTurnGenerating(sessionID) {
		t.Fatal("the foreground is generating; the gate must stay closed")
	}
	if svc.claimForegroundTurn(sessionID) != nil {
		t.Fatal("no prompt may claim a turn whose foreground is actively generating")
	}
}

// An untracked session has no background work outstanding, so there is nothing to
// claim: the historical reject-while-RUNNING default must stand.
func TestClaimForegroundTurn_UntrackedSessionCannotBeClaimed(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	if svc.claimForegroundTurn("session-never-seen") != nil {
		t.Fatal("a session with no outstanding background work must not be claimable")
	}
	if svc.claimForegroundTurn("") != nil {
		t.Fatal("an empty session ID must not be claimable")
	}
}

// A prompt that claims the turn but never reaches the agent (ensureSessionRunning
// failed, the model switch failed) has to hand the claim back. Otherwise the
// session sits in RUNNING advertising a generating foreground it does not have,
// locking the operator out for the rest of the turn — the exact lockout ADR-0049
// exists to remove.
func TestReleaseForegroundClaim_FailedPromptReopensTheGate(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-release"
	svc.registerBackgroundTask(sessionID, "tool-subagent-1")
	svc.markForegroundIdle(sessionID)

	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("the first prompt must win the claim")
	}
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("a claimed turn reads as generating, got %q", got)
	}

	// The prompt fails before reaching the agent.
	if !svc.releaseForegroundClaim(claim) {
		t.Fatal("releasing a live claim with background work outstanding must reopen the gate")
	}

	// Background work is still outstanding, so the session is background-idle again
	// and the operator can retry.
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("a released claim must return the turn to background-idle, got %q", got)
	}
	if svc.claimForegroundTurn(sessionID) == nil {
		t.Fatal("a retried prompt must be able to claim the released turn")
	}
}

// Releasing must not resurrect a background hold that no longer exists: if the
// last background task finished while the failing prompt was in flight, the turn
// genuinely is not waiting on anything and the generating default is correct.
func TestReleaseForegroundClaim_DoesNotReopenGateWithoutBackgroundWork(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-release-nobg"
	svc.registerBackgroundTask(sessionID, "tool-subagent-1")
	svc.markForegroundIdle(sessionID)
	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("the prompt must win the claim")
	}

	// The background task completes while the prompt is still in flight, then the
	// prompt fails.
	svc.completeBackgroundTask(sessionID, "tool-subagent-1")
	if svc.releaseForegroundClaim(claim) {
		t.Fatal("with no background work outstanding the release must not reopen the gate")
	}

	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("with no background work outstanding the turn must read as generating, got %q", got)
	}
}

func TestForegroundClaim_StaleTokenCannotCompleteOrReleaseNewClaim(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-claim-generation"
	svc.registerBackgroundTask(sessionID, "background-1")
	svc.markForegroundIdle(sessionID)
	first := svc.claimForegroundTurn(sessionID)
	if first == nil {
		t.Fatal("first prompt must win the claim")
	}
	// Work registered during admission becomes visible only after the provider
	// reports that the dispatched foreground yielded.
	svc.registerBackgroundTask(sessionID, "background-2")
	if svc.completeForegroundClaim(first) {
		t.Fatal("claim completion alone must not expose background work")
	}
	if !svc.markForegroundIdle(sessionID) {
		t.Fatal("foreground idle must expose background work")
	}
	second := svc.claimForegroundTurn(sessionID)
	if second == nil {
		t.Fatal("second prompt must claim the newly yielded turn")
	}

	if svc.completeForegroundClaim(first) {
		t.Fatal("a stale completion must not clear a newer admission")
	}
	if svc.releaseForegroundClaim(first) {
		t.Fatal("a stale release must not clear a newer admission")
	}
	if svc.claimForegroundTurn(sessionID) != nil {
		t.Fatal("the newer claim must remain active after stale token operations")
	}
}

func TestForegroundDispatch_FailedDispatchRollsBackForRetry(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-dispatch-rollback"
	svc.registerBackgroundTask(sessionID, "background-1")
	svc.markForegroundIdle(sessionID)
	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("first dispatch must claim the background-idle turn")
	}
	dispatch := svc.beginForegroundDispatch(sessionID, claim)
	if dispatch == nil {
		t.Fatal("first dispatch must establish its cycle before provider entry")
	}

	rollbackNow := make(chan struct{})
	rollbackDone := make(chan bool, 1)
	go func() {
		<-rollbackNow
		rollbackDone <- svc.rollbackForegroundDispatch(dispatch)
	}()
	close(rollbackNow)
	if restored := <-rollbackDone; !restored {
		t.Fatal("an unobserved synchronous dispatch failure must restore background-idle")
	}
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("failed dispatch must reopen immediate input, got %q", got)
	}

	retryClaim := svc.claimForegroundTurn(sessionID)
	if retryClaim == nil {
		t.Fatal("retry must claim the foreground after rollback")
	}
	retryDispatch := svc.beginForegroundDispatch(sessionID, retryClaim)
	if retryDispatch == nil {
		t.Fatal("retry must establish a new dispatch token")
	}
	if retryDispatch.generation <= dispatch.generation {
		t.Fatalf("retry must receive a fresh immutable generation: retry=%d failed=%d",
			retryDispatch.generation, dispatch.generation)
	}
	if svc.acceptForegroundDispatch(retryDispatch) {
		t.Fatal("accepting a retry does not expose background work before foreground idle")
	}
	svc.markForegroundIdle(sessionID)
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("accepted retry must retain its own completion/idle identity, got %q", got)
	}
}

func TestForegroundDispatch_StaleRollbackCannotClobberRetry(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-stale-dispatch-rollback"
	svc.registerBackgroundTask(sessionID, "background-1")
	svc.markForegroundIdle(sessionID)
	firstClaim := svc.claimForegroundTurn(sessionID)
	first := svc.beginForegroundDispatch(sessionID, firstClaim)
	if first == nil || !svc.rollbackForegroundDispatch(first) {
		t.Fatal("first failed dispatch must roll back")
	}
	secondClaim := svc.claimForegroundTurn(sessionID)
	second := svc.beginForegroundDispatch(sessionID, secondClaim)
	if second == nil {
		t.Fatal("retry must establish its cycle")
	}

	retryEstablished := make(chan struct{})
	staleRollbackDone := make(chan bool, 1)
	go func() {
		<-retryEstablished
		staleRollbackDone <- svc.rollbackForegroundDispatch(first)
	}()
	close(retryEstablished)
	if rolledBack := <-staleRollbackDone; rolledBack {
		t.Fatal("stale failure token must not roll back the retry")
	}
	if svc.acceptForegroundDispatch(second) {
		t.Fatal("acceptance alone must not expose background work")
	}
	if !svc.markForegroundIdle(sessionID) {
		t.Fatal("retry's current idle event must expose its background work")
	}
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("stale rollback clobbered retry activity: got %q", got)
	}
}

func TestForegroundDispatch_ClaimlessRollbackRestoresOnlyExactUnobservedStart(t *testing.T) {
	t.Run("synchronous failure restores prior background idle", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		const sessionID = "session-claimless-rollback"
		svc.registerBackgroundTask(sessionID, "background-1")
		svc.markForegroundIdle(sessionID)
		dispatch := svc.beginForegroundDispatch(sessionID, nil)
		if dispatch == nil || svc.ForegroundActivity(sessionID) != v1.ForegroundActivityGenerating {
			t.Fatal("begin must atomically establish claimless successor ownership")
		}
		if !svc.rollbackForegroundDispatch(dispatch) {
			t.Fatal("exact unobserved failure must restore prior background idle")
		}
		if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
			t.Fatalf("rollback restored %q, want background", got)
		}
	})

	t.Run("provider output prevents restoration", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		const sessionID = "session-claimless-observed"
		svc.registerBackgroundTask(sessionID, "background-1")
		svc.markForegroundIdle(sessionID)
		dispatch := svc.beginForegroundDispatch(sessionID, nil)
		svc.markForegroundGenerating(sessionID)
		if svc.rollbackForegroundDispatch(dispatch) {
			t.Fatal("provider-observed cycle must not roll back")
		}
		if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
			t.Fatalf("provider output lost successor ownership: got %q", got)
		}
	})

	t.Run("stale failure cannot restore over retry", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		const sessionID = "session-claimless-retry"
		svc.registerBackgroundTask(sessionID, "background-1")
		svc.markForegroundIdle(sessionID)
		first := svc.beginForegroundDispatch(sessionID, nil)
		if !svc.rollbackForegroundDispatch(first) {
			t.Fatal("first synchronous failure must restore background")
		}
		retry := svc.beginForegroundDispatch(sessionID, nil)
		if retry == nil {
			t.Fatal("retry must establish successor ownership")
		}
		if svc.rollbackForegroundDispatch(first) {
			t.Fatal("stale failure token must not restore over retry")
		}
		if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
			t.Fatalf("stale rollback displaced retry: got %q", got)
		}
	})
}
