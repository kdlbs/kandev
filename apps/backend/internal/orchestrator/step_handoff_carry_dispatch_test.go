package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func seedHandoffCarryToken(t *testing.T, repo interface {
	SetTaskMetadataKey(ctx context.Context, taskID, key string, value interface{}) error
}, taskID, stepID, handoff, stamp string) {
	t.Helper()
	err := repo.SetTaskMetadataKey(context.Background(), taskID, models.MetaKeyStepHandoffCarry, models.StepHandoffCarryToken{
		Handoff: handoff, StepID: stepID, Stamp: stamp,
	})
	if err != nil {
		t.Fatalf("seed carry token: %v", err)
	}
}

// TestAutoStartStepPrompt_DeliversHandoffLastAfterQueuedHandoff covers
// AC-001.6 and AC-001.6a on the ACP dispatch branch: an AutoRun-enabled
// queued hand-off message merges first, then the completion handoff carry
// token is claimed and appended last, under the fixed heading.
func TestAutoStartStepPrompt_DeliversHandoffLastAfterQueuedHandoff(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-handoff-order"
		sessionID = "session-handoff-order"
		stepID    = "step-next"
		queued    = "please also check the migration"
		autoStart = "Run the next workflow step"
		carried   = "watch out for the flaky test"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedHandoffCarryToken(t, repo, taskID, stepID, carried, "stamp-order")
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-handoff-order")

	_, err := svc.messageQueue.QueueMessage(ctx, sessionID, taskID, queued, "", "user", false, nil)
	require.NoError(t, err)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)

	err = svc.autoStartStepPrompt(
		ctx, taskID, session, &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Next"},
		autoStart, false, false, newStepHandoffOnce(),
	)
	require.NoError(t, err)
	require.Len(t, agentMgr.capturedPrompts, 1)
	prompt := agentMgr.capturedPrompts[0]

	autoStartIdx := strings.Index(prompt, autoStart)
	queuedIdx := strings.Index(prompt, queued)
	headingIdx := strings.Index(prompt, stepHandoffPromptHeading)
	carriedIdx := strings.Index(prompt, carried)
	require.NotEqual(t, -1, autoStartIdx, "prompt = %q", prompt)
	require.NotEqual(t, -1, queuedIdx, "prompt = %q", prompt)
	require.NotEqual(t, -1, headingIdx, "prompt = %q", prompt)
	require.NotEqual(t, -1, carriedIdx, "prompt = %q", prompt)
	require.True(t, autoStartIdx < queuedIdx, "auto-start content must precede the queued hand-off")
	require.True(t, queuedIdx < headingIdx, "the completion handoff must land after the queued hand-off")
	require.True(t, headingIdx < carriedIdx)

	if _, present := carryToken(t, repo, taskID); present {
		t.Fatal("the claimed carry token must be removed")
	}
}

// TestAutoStartStepPrompt_NoTokenNoHeading covers the plain case: with no
// carry token, the prompt is unchanged and no heading is injected.
func TestAutoStartStepPrompt_NoTokenNoHeading(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-no-handoff"
		sessionID = "session-no-handoff"
		autoStart = "Run the next workflow step"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-no-handoff")
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)

	err = svc.autoStartStepPrompt(
		ctx, taskID, session, &wfmodels.WorkflowStep{ID: "step-next", WorkflowID: "wf1", Name: "Next"},
		autoStart, false, false, newStepHandoffOnce(),
	)
	require.NoError(t, err)
	require.Len(t, agentMgr.capturedPrompts, 1)
	require.False(t, strings.Contains(agentMgr.capturedPrompts[0], stepHandoffPromptHeading))
}

// TestAutoStartStepPrompt_ReplacementLaunchReusesClaimedHandoff covers
// AC-001.8: a replacement launch within the same step entry, sharing the same
// stepHandoffOnce, receives the same handoff text even though the DB-level
// claim can only succeed once.
func TestAutoStartStepPrompt_ReplacementLaunchReusesClaimedHandoff(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-replacement-handoff"
		sessionID = "session-replacement-handoff"
		stepID    = "step-next"
		autoStart = "Run the next workflow step"
		carried   = "carried across the replacement"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedHandoffCarryToken(t, repo, taskID, stepID, carried, "stamp-replacement")
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-replacement-handoff")
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	step := &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Next"}
	once := newStepHandoffOnce()

	// First attempt: claims and delivers the token, removing it from the DB.
	err = svc.autoStartStepPrompt(ctx, taskID, session, step, autoStart, false, false, once)
	require.NoError(t, err)
	require.Len(t, agentMgr.capturedPrompts, 1)
	require.Contains(t, agentMgr.capturedPrompts[0], carried)
	if _, present := carryToken(t, repo, taskID); present {
		t.Fatal("the token must be removed after the first claim")
	}

	// A replacement launch in the SAME step entry reuses the shared once — the
	// DB has nothing left to claim, but the memoized text must still appear.
	// Reset the session back to promptable, standing in for a real replacement
	// launch (created after the first session terminalized).
	require.NoError(t, repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""))
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-replacement-handoff-2")
	session, err = repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	err = svc.autoStartStepPrompt(ctx, taskID, session, step, autoStart, false, false, once)
	require.NoError(t, err)
	require.Len(t, agentMgr.capturedPrompts, 2)
	require.Contains(t, agentMgr.capturedPrompts[1], carried, "the replacement launch must reuse the already-claimed handoff text")
}

// TestLaunchAfterOnEnterDispatch_PassthroughEmptyPromptDeliversViaDrain covers
// AC-001.6c: the passthrough branch that finds its composed prompt empty
// still dispatches a drained queued message, and that dispatch carries the
// completion handoff.
func TestLaunchAfterOnEnterDispatch_PassthroughEmptyPromptDeliversViaDrain(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-passthrough-drain"
		sessionID = "session-passthrough-drain"
		stepID    = "step1"
		queued    = "drain me please"
		carried   = "passthrough carried text"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedHandoffCarryToken(t, repo, taskID, stepID, carried, "stamp-passthrough")
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo, isPassthrough: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-passthrough-drain")

	_, err := svc.messageQueue.QueueMessage(ctx, sessionID, taskID, queued, "", "user", false, nil)
	require.NoError(t, err)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)

	step := &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Step 1", Prompt: ""}
	svc.launchAfterOnEnterDispatch(ctx, taskID, session, step, "task description", false, true, false)

	// The claim happens synchronously inside the guarded reserve, before the
	// dispatch's async execution — no need to wait for that goroutine here.
	if _, present := carryToken(t, repo, taskID); present {
		t.Fatal("the drain-only passthrough branch must claim the token when it dispatches")
	}
}

// TestProcessOnEnter_NoOnEnterActionsDrainDeliversHandoff covers F22's first
// gap: processOnEnter's own early return (no on_enter actions, no profile
// switch) never reaches launchAfterOnEnterDispatch, yet still drains a queued
// message and must still carry the handoff.
func TestProcessOnEnter_NoOnEnterActionsDrainDeliversHandoff(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-no-onenter-drain"
		sessionID = "session-no-onenter-drain"
		stepID    = "step1"
		queued    = "drain me too"
		carried   = "no on_enter carried text"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedHandoffCarryToken(t, repo, taskID, stepID, carried, "stamp-no-onenter")
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-no-onenter-drain")

	_, err := svc.messageQueue.QueueMessage(ctx, sessionID, taskID, queued, "", "user", false, nil)
	require.NoError(t, err)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)

	step := &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Step 1"}
	svc.processOnEnter(ctx, taskID, session, step, "task description", 0, nil)

	if _, present := carryToken(t, repo, taskID); present {
		t.Fatal("processOnEnter's own drain-only early return must claim the token when it dispatches")
	}
}

// TestLaunchAfterOnEnterDispatch_EmptyEntrySendsNothingClaimsNothing covers
// AC-001.6a: when a step entry decides — over content excluding the handoff
// text — that it will send the agent nothing, the handoff must never be
// claimed. Modeled on TestWorkflowAutoStartEmptyPrompt's "prompted session
// suppresses task description" case, which reaches autoStartStepPrompt's own
// early return with no queued hand-off to fall back to.
func TestLaunchAfterOnEnterDispatch_EmptyEntrySendsNothingClaimsNothing(t *testing.T) {
	fixture := newInitialPromptDedupFixture(t, true, false)
	seedHandoffCarryToken(t, fixture.repo, fixture.taskID, fixture.step.ID, "must not be claimed", "stamp-unsent")

	fixture.svc.launchAfterOnEnterDispatch(
		context.Background(), fixture.taskID, fixture.session, fixture.step,
		fixture.taskDescription, false, true, false,
	)

	if len(fixture.agent.capturedPrompts) != 0 {
		t.Fatalf("captured prompts = %#v, want none", fixture.agent.capturedPrompts)
	}
	token, present := carryToken(t, fixture.repo, fixture.taskID)
	if !present || token.Handoff != "must not be claimed" {
		t.Fatalf("a dispatch that sends nothing must not claim the token, got present=%v token=%+v", present, token)
	}
}

// TestAutoStartPassthroughPrompt_ClaimedTokenNotRestoredOnDispatchFailure
// covers AC-001.9: a claimed token is never restored, even when the entry
// that claimed it then fails to dispatch — and a later entry into that same
// step gets no handoff either, since REQ-005 accepts the handoff as lost
// rather than risk a double-delivery on retry.
func TestAutoStartPassthroughPrompt_ClaimedTokenNotRestoredOnDispatchFailure(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-unrestored-handoff"
		sessionID = "session-unrestored-handoff"
		stepID    = "step-next"
		carried   = "lost on dispatch failure"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedHandoffCarryToken(t, repo, taskID, stepID, carried, "stamp-unrestored")
	agentMgr := &mockAgentManager{
		isAgentRunning: true, repoForExecutionLookup: repo, isPassthrough: true,
		passthroughStdinErr: errors.New("simulated stdin write failure"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-unrestored-handoff")
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)

	step := &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Next", Prompt: "go"}
	svc.launchAfterOnEnterDispatch(ctx, taskID, session, step, "task description", false, true, false)

	if _, present := carryToken(t, repo, taskID); present {
		t.Fatal("the token must be claimed (removed) even though dispatch failed")
	}
	text, claimed := svc.claimStepHandoffCarryText(ctx, taskID, stepID)
	if claimed || text != "" {
		t.Fatalf("a later entry into the same step must get nothing back, got claimed=%v text=%q", claimed, text)
	}
}

// TestStartSessionForWorkflowStep_DeliversHandoff covers AC-001.11: the queue
// promotion / manual auto-start dispatch path also claims and appends the
// completion handoff.
func TestStartSessionForWorkflowStep_DeliversHandoff(t *testing.T) {
	fixture := newInitialPromptDedupFixture(t, true, false)
	fixture.step.Prompt = "Continue with validation."
	seedHandoffCarryToken(t, fixture.repo, fixture.taskID, fixture.step.ID, "queue promotion carried text", "stamp-queue-promo")

	if err := fixture.svc.StartSessionForWorkflowStep(
		context.Background(), fixture.taskID, fixture.sessionID, fixture.step.ID,
	); err != nil {
		t.Fatalf("StartSessionForWorkflowStep returned error: %v", err)
	}

	got := fixture.agent.capturedPrompts
	if len(got) != 1 || !strings.Contains(got[0], fixture.step.Prompt) || !strings.Contains(got[0], "queue promotion carried text") {
		t.Fatalf("captured prompts = %#v, want step prompt plus carried handoff", got)
	}
	if _, present := carryToken(t, fixture.repo, fixture.taskID); present {
		t.Fatal("the claimed token must be removed")
	}
}

// failOnceHandoffClaimRepo wraps a real repo's TakeTaskMetadataKeyIfDestinationStep
// so its first invocation fails, standing in for a transient claim error. Per
// AC-005.2, a failed claim must not spend the step entry's one-claim budget, so
// a later attempt within the same entry (e.g. a replacement launch) must retry
// and succeed against the still-present token.
type failOnceHandoffClaimRepo struct {
	sessionExecutorStore
	failed bool
}

func (r *failOnceHandoffClaimRepo) TakeTaskMetadataKeyIfDestinationStep(
	ctx context.Context, taskID, key, expectedStepID, expectedStamp string,
) (json.RawMessage, bool, error) {
	if !r.failed {
		r.failed = true
		return nil, false, errors.New("simulated transient claim failure")
	}
	return r.sessionExecutorStore.TakeTaskMetadataKeyIfDestinationStep(ctx, taskID, key, expectedStepID, expectedStamp)
}

// TestClaimStepHandoffCarryText_FailedClaimDoesNotSpendBudget covers AC-005.2:
// a failed claim attempt must not memoize an empty result, so a later attempt
// in the same step entry (sharing the same stepHandoffOnce) retries and can
// still succeed.
func TestClaimStepHandoffCarryText_FailedClaimDoesNotSpendBudget(t *testing.T) {
	ctx := context.Background()
	const (
		taskID  = "task-failed-claim-retry"
		stepID  = "step2"
		carried = "carried after retry"
	)
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, "session-failed-claim-retry", "step1")
	seedHandoffCarryToken(t, repo, taskID, stepID, carried, "stamp-retry")
	svc := createTestService(repo, twoStepGetter(), newMockTaskRepo())
	wrapped := &failOnceHandoffClaimRepo{sessionExecutorStore: svc.repo}
	svc.repo = wrapped

	once := newStepHandoffOnce()
	text := svc.resolveStepHandoffText(ctx, once, taskID, stepID, true)
	require.Empty(t, text, "the first, failing attempt must not deliver a handoff")
	require.True(t, wrapped.failed, "expected the wrapped claim to have been invoked")

	text = svc.resolveStepHandoffText(ctx, once, taskID, stepID, true)
	require.Equal(t, carried, text, "a retried attempt must claim the still-present token")
}

// TestLaunchAfterOnEnterDispatch_FallbackDrainDeliversHandoff covers F22's
// fourth gap: the final fallback drain for a step with no auto_start_agent and
// no profile switch (e.g. a Review-shaped step) still carries the handoff.
func TestLaunchAfterOnEnterDispatch_FallbackDrainDeliversHandoff(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-fallback-drain"
		sessionID = "session-fallback-drain"
		stepID    = "step-review"
		queued    = "drain via fallback"
		carried   = "fallback carried text"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedHandoffCarryToken(t, repo, taskID, stepID, carried, "stamp-fallback")
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-fallback-drain")

	_, err := svc.messageQueue.QueueMessage(ctx, sessionID, taskID, queued, "", "user", false, nil)
	require.NoError(t, err)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)

	// hasAutoStart=false, sessionSwitched=false: neither the ACP branch nor the
	// implicit profile-switch branch applies, landing on the final fallback
	// drain — the shape a step without auto_start_agent (e.g. Review) takes.
	step := &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Review"}
	svc.launchAfterOnEnterDispatch(ctx, taskID, session, step, "task description", false, false, false)

	if _, present := carryToken(t, repo, taskID); present {
		t.Fatal("the final fallback drain must claim the token when it dispatches")
	}
}

// TestLaunchAfterOnEnterDispatch_AsyncProfileSwitchFailureDrainDeliversHandoff
// covers F22's third gap: when the implicit profile-switch async launch fails
// to dispatch, the failure drain reuses the already-claimed handoff text (the
// shared stepHandoffOnce, not a fresh claim) and delivers it via the drained
// queued message.
func TestLaunchAfterOnEnterDispatch_AsyncProfileSwitchFailureDrainDeliversHandoff(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-profile-switch-drain"
		sessionID = "session-profile-switch-drain"
		stepID    = "step-next"
		queued    = "queued after failed switch"
		carried   = "profile switch carried text"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedHandoffCarryToken(t, repo, taskID, stepID, carried, "stamp-profile-switch")
	agentMgr := &mockAgentManager{
		isAgentRunning: true, repoForExecutionLookup: repo,
		promptErr: errors.New("simulated fatal profile switch prompt error"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-profile-switch-drain")

	_, err := svc.messageQueue.QueueMessage(ctx, sessionID, taskID, queued, "", "user", false, nil)
	require.NoError(t, err)
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)

	done := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(done) }

	// sessionSwitched=true with a non-empty step.Prompt and hasAutoStart=false
	// selects the implicit profile-switch async goroutine (the design's
	// AC-005-adjacent "profile switch failure" branch): the launch fails, then
	// the failure drain must deliver the queued message with the handoff the
	// launch attempt already claimed.
	step := &wfmodels.WorkflowStep{ID: stepID, WorkflowID: "wf1", Name: "Next", Prompt: "please continue"}
	svc.launchAfterOnEnterDispatch(ctx, taskID, session, step, "task description", false, false, true)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the queued message to execute after the failed profile-switch dispatch")
	}

	require.GreaterOrEqual(t, len(agentMgr.capturedPrompts), 2,
		"expected the failed profile-switch attempt plus the drained queued dispatch")
	last := agentMgr.capturedPrompts[len(agentMgr.capturedPrompts)-1]
	require.Contains(t, last, queued)
	require.Contains(t, last, carried, "the drain must reuse the already-claimed handoff text")
	if _, present := carryToken(t, repo, taskID); present {
		t.Fatal("the token must have been claimed by the failed attempt")
	}
}
