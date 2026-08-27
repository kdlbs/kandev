package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ADR-0049 introduced tracker-side admission claims. The coarse policy no
// longer reaches them from operator prompt admission, but the dormant mechanism
// remains race-safe for a future protocol-backed design.

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

// A prompt admitted while the foreground is idle can resume an agent process
// before dispatch. AgentBootReady then advances the durable session from
// RUNNING through STARTING to WAITING_FOR_INPUT. The admission claim still owns
// the foreground turn, so that expected state transition must not reject the
// already accepted prompt.
func TestRecheckPromptableWithForegroundClaim_AcceptsWaitingAfterResume(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const (
		taskID    = "task-resumed"
		sessionID = "session-resumed"
	)
	svc.registerBackgroundTask(sessionID, "tool-subagent-1")
	svc.markForegroundIdle(sessionID)

	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("the prompt must claim the background-idle foreground")
	}

	if err := svc.recheckPromptableWithForegroundClaim(
		taskID,
		sessionID,
		models.TaskSessionStateWaitingForInput,
		claim,
	); err != nil {
		t.Fatalf("a current claim must survive the expected resume transition to WAITING_FOR_INPUT: %v", err)
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
			Normalized: attestedBackgroundShellPayload("npm run dev"),
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

	if accepted != 0 {
		t.Fatalf("no concurrent prompt may enter a RUNNING turn, got %d accepted", accepted)
	}
	if rejectedBusy != prompters {
		t.Fatalf("every prompt must be rejected with ErrAgentPromptInProgress, got %d of %d", rejectedBusy, prompters)
	}
	// The decisive assertion: only one turn was actually started on the agent.
	agentMgr.mu.Lock()
	captured := len(agentMgr.capturedPrompts)
	agentMgr.mu.Unlock()
	if captured != 0 {
		t.Fatalf("RUNNING-session prompts reached the agent: %d forwarded, want 0", captured)
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
		context.Background(), taskID, sessionID, "queued prompt", "", false, nil, false,
		promptTaskOptions{claimEntryID: "stale-entry"},
	)
	if !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("a queued dispatch into a RUNNING session must be rejected, got: %v", err)
	}
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
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
	if svc.dispatchAndAcceptForegroundClaim(sessionID, claim) {
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
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("a claimed turn reads as generating, got %q", got)
	}

	// The prompt fails before reaching the agent.
	if !svc.releaseForegroundClaim(claim) {
		t.Fatal("releasing a live claim with background work outstanding must reopen the gate")
	}

	// Background work is still outstanding, so the session is background-idle again
	// and the operator can retry.
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
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

	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityGenerating {
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
	if svc.dispatchAndAcceptForegroundClaim(sessionID, first) {
		t.Fatal("claim completion alone must not expose background work")
	}
	if !svc.markForegroundIdle(sessionID) {
		t.Fatal("foreground idle must expose background work")
	}
	second := svc.claimForegroundTurn(sessionID)
	if second == nil {
		t.Fatal("second prompt must claim the newly yielded turn")
	}

	if svc.dispatchAndAcceptForegroundClaim(sessionID, first) {
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
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
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
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
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
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
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
		if dispatch == nil || svc.foregroundActivityValue(sessionID) != v1.ForegroundActivityGenerating {
			t.Fatal("begin must atomically establish claimless successor ownership")
		}
		if !svc.rollbackForegroundDispatch(dispatch) {
			t.Fatal("exact unobserved failure must restore prior background idle")
		}
		if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
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
		if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityGenerating {
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
		if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityGenerating {
			t.Fatalf("stale rollback displaced retry: got %q", got)
		}
	})
}

// TestBeginForegroundDispatch_ClaimlessFailsClosedWhileClaimInFlight covers the
// resume race that used to strand promptInFlight forever: resume can advance
// durable state to WAITING_FOR_INPUT while a claimed prompt is still
// mid-dispatch (see recheckPromptableWithForegroundClaim's resume comment in
// task_operations.go), letting a second, claimless prompt reach
// beginForegroundDispatch with a live admission still outstanding. A claimless
// begin must fail closed in that window instead of bumping
// promptCycleGeneration out from under the claimed dispatch.
func TestBeginForegroundDispatch_ClaimlessFailsClosedWhileClaimInFlight(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-claimless-fail-closed"

	svc.registerBackgroundTask(sessionID, "background-1")
	svc.markForegroundIdle(sessionID)
	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("claim must win the background-idle turn")
	}

	if dispatch := svc.beginForegroundDispatch(sessionID, nil); dispatch != nil {
		t.Fatal("claimless begin must fail closed while a claimed admission is in flight")
	}
}

// TestAcceptForegroundDispatch_StaleGenerationStillReleasesStrandedClaim is the
// regression test for the promptInFlight-stranding bug: before the fix,
// acceptForegroundDispatch returned early on a cycle-generation mismatch
// without ever releasing the admission claim, so generatingLocked() reported
// busy for the rest of the session's life and claimForegroundTurn could never
// win again. The stale-generation interleaving is produced directly here
// (rather than via a second beginForegroundDispatch call) because
// beginForegroundDispatch itself now fails closed for that case — see
// TestBeginForegroundDispatch_ClaimlessFailsClosedWhileClaimInFlight — so this
// isolates acceptForegroundDispatch's own release ordering.
func TestAcceptForegroundDispatch_StaleGenerationStillReleasesStrandedClaim(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-accept-stranded"

	svc.registerBackgroundTask(sessionID, "background-1")
	svc.markForegroundIdle(sessionID)
	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("claim must win the background-idle turn")
	}
	dispatch := svc.beginForegroundDispatch(sessionID, claim)
	if dispatch == nil {
		t.Fatal("begin must establish the dispatch cycle")
	}

	// Simulate a successor cycle advancing past this dispatch's generation
	// while dispatch's own claimGeneration is still current -- the exact
	// desync a claimless resume admission would otherwise produce.
	claim.activity.mu.Lock()
	claim.activity.promptCycleGeneration++
	claim.activity.mu.Unlock()

	if svc.acceptForegroundDispatch(dispatch) {
		t.Fatal("accept for a superseded generation must not itself expose background work")
	}

	claim.activity.mu.Lock()
	stillInFlight := claim.activity.promptInFlight
	claim.activity.mu.Unlock()
	if stillInFlight {
		t.Fatal("accept must release the stranded admission claim even when its cycle generation is stale")
	}

	if !svc.markForegroundIdle(sessionID) {
		t.Fatal("expected background work to become visible once yielded")
	}
	if svc.claimForegroundTurn(sessionID) == nil {
		t.Fatal("claimForegroundTurn must succeed again after the stranded claim is released")
	}
}
