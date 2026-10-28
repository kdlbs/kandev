package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestQueueUserPrompt_RuntimeUnavailableSurvivesFastPathRedispatch is the
// regression test for Review round 1's F1 finding: QueueUserPrompt's T2
// fast-path drain (tryFastPathDrainAfterEnqueue) reloads the session with
// only checkSessionPromptable, which cannot tell "promptable" apart from
// "promptable but its runtime has not finished launching". So the fast path
// takes the entry and re-dispatches it through promptTask microseconds
// after it was queued — with the runtime exactly as unavailable as it was
// the first time.
//
// This drives the real Service.QueueUserPrompt -> tryFastPathDrainAfterEnqueue
// -> executeQueuedMessageWithReservation -> promptTask -> handleQueuedMessageExecutionError
// chain end to end (no fakes), the way F3 requires: a fake orchestrator that
// only counts QueueUserPrompt calls cannot see this loop at all, because the
// loop happens entirely inside the real Service after the initial enqueue
// returns.
func TestQueueUserPrompt_RuntimeUnavailableSurvivesFastPathRedispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	// WAITING_FOR_INPUT with no executors_running row and no in-memory
	// execution reproduces the exact production shape: a session promoted
	// by a workflow step move whose runtime has not launched yet.
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.executor = executor.NewExecutor(&mockAgentManager{repoForExecutionLookup: repo}, repo, testLogger(), executor.ExecutorConfig{})

	workerDone := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(workerDone) }

	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "hello", "", false, nil, map[string]interface{}{}, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}

	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the fast-path redispatch to settle")
	}

	entries, _, err := svc.messageQueue.SnapshotSession(ctx, "s1")
	if err != nil {
		t.Fatalf("snapshot queue: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("message lost: fast-path redispatch failed with ErrSessionRuntimeUnavailable and "+
			"was not requeued (queue has %d entries, want 1)", len(entries))
	}
	if entries[0].Content != "hello" {
		t.Fatalf("surviving entry content = %q, want %q", entries[0].Content, "hello")
	}
}

func TestQueueUserPrompt_RetryableRecoveryGuardRetainsPersistedTurnAndMessage(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	const turnID = "turn-accepted-before-recovery"
	const messageID = "message-accepted-before-recovery"
	acceptedAt := time.Now().UTC()
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskSessionID: "s1", TaskID: "t1", StartedAt: acceptedAt,
	}); err != nil {
		t.Fatalf("seed accepted turn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: messageID, TaskSessionID: "s1", TaskID: "t1", TurnID: turnID,
		AuthorType: models.MessageAuthorUser, Content: "accepted prompt", CreatedAt: acceptedAt,
	}); err != nil {
		t.Fatalf("seed accepted message: %v", err)
	}
	assertAcceptedRecords := func(stage string) {
		t.Helper()
		turns, turnErr := repo.ListTurnsBySession(ctx, "s1")
		if turnErr != nil {
			t.Fatalf("%s: list turns: %v", stage, turnErr)
		}
		if len(turns) != 1 || turns[0].ID != turnID || turns[0].CompletedAt != nil {
			t.Fatalf("%s: persisted turns = %+v, want the same one open turn %q", stage, turns, turnID)
		}
		messages, messageErr := repo.ListMessages(ctx, "s1")
		if messageErr != nil {
			t.Fatalf("%s: list messages: %v", stage, messageErr)
		}
		if len(messages) != 1 || messages[0].ID != messageID || messages[0].TurnID != turnID ||
			messages[0].Content != "accepted prompt" {
			t.Fatalf("%s: persisted messages = %+v, want original message %q on turn %q", stage, messages, messageID, turnID)
		}
	}
	assertAcceptedRecords("before queueing")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExecutorRunning(t, repo, "s1", "t1", "exec-retained")

	guardErr := &SessionRecoveryGuardError{
		Cause: errors.New("remote owner is still being reconciled"), SessionID: "s1", Retryable: true,
	}
	var ready atomic.Bool
	agentMgr := &mockAgentManager{
		repoForExecutionLookup:   repo,
		isAgentRunningFn:         func(context.Context, string) bool { return ready.Load() },
		isAgentReadyFn:           func(context.Context, string) bool { return ready.Load() },
		currentPromptExecutionID: "exec-retained",
		promptDone:               make(chan struct{}),
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return nil, guardErr
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &repoTurnService{repo: repo}

	workerDone := make(chan struct{}, 2)
	svc.onQueuedMessageExecutionComplete = func() { workerDone <- struct{}{} }
	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "accepted prompt", "", false, nil, nil, true); err != nil {
		t.Fatalf("QueueUserPrompt: %v", err)
	}
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the accepted prompt to be deferred")
	}

	entries, _, err := svc.messageQueue.SnapshotSession(ctx, "s1")
	if err != nil {
		t.Fatalf("snapshot queue: %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "accepted prompt" {
		t.Fatalf("accepted prompt queue = %+v, want one retained accepted prompt", entries)
	}
	if entries[0].ID == "" {
		t.Fatal("accepted prompt lost its queue identity")
	}
	finalSession, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if finalSession.State == models.TaskSessionStateFailed {
		t.Fatalf("retryable recovery guard terminally failed the session: %q", finalSession.ErrorMessage)
	}
	registry := svc.resumeAttemptStore()
	registry.mu.Lock()
	hasRecoveryHistory := registry.hasRecoveryHistoryLocked("s1")
	tombstoneCount := len(registry.tombstones["s1"])
	registry.mu.Unlock()
	if !hasRecoveryHistory || tombstoneCount == 0 {
		t.Fatalf("retryable cold-resume attempt did not leave a tombstone: history=%t tombstones=%d", hasRecoveryHistory, tombstoneCount)
	}
	assertAcceptedRecords("after retryable recovery refusal")

	ready.Store(true)
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-retained",
		Status: string(v1.AgentStatusReady), RecoveredAvailabilityOnly: true,
	})
	select {
	case <-agentMgr.promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("availability did not dispatch the retained accepted prompt after recovery")
	}
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for retained prompt dispatch to finish")
	}
	if len(agentMgr.capturedPrompts) != 1 || agentMgr.capturedPrompts[0] != "accepted prompt" {
		t.Fatalf("recovered prompt dispatches = %v, want exactly the accepted prompt once", agentMgr.capturedPrompts)
	}
	entries, _, err = svc.messageQueue.SnapshotSession(ctx, "s1")
	if err != nil {
		t.Fatalf("snapshot queue after availability dispatch: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("queue after availability dispatch = %+v, want empty", entries)
	}
	assertAcceptedRecords("after availability dispatch")
}

// TestPromptTask_SessionAwaitingLaunchClassifiedRuntimeUnavailable pins the
// positive case for the narrowed sentinel (Review round 1's F2 finding): the
// exact "prepared but not launched yet" shape (WAITING_FOR_INPUT, no
// executors_running row) must still classify as ErrSessionRuntimeUnavailable
// so the caller queues instead of surfacing an error.
func TestPromptTask_SessionAwaitingLaunchClassifiedRuntimeUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.executor = executor.NewExecutor(&mockAgentManager{repoForExecutionLookup: repo}, repo, testLogger(), executor.ExecutorConfig{})

	_, err := svc.PromptTask(ctx, "t1", "s1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected an error: no executors_running row exists for this session")
	}
	if !errors.Is(err, ErrSessionRuntimeUnavailable) {
		t.Fatalf("expected ErrSessionRuntimeUnavailable for the not-yet-launched shape, got: %v", err)
	}
}

// TestPromptTask_ExhaustedColdResumeNotClassifiedRuntimeUnavailable pins the
// negative case for Review round 1's F2 finding: a session that has an
// executors_running row (so a resume attempt actually runs) and never
// becomes prompt-ready is a genuine launch failure, not the "hasn't started
// yet" case ErrSessionRuntimeUnavailable exists for. It must keep surfacing
// as a plain, visible error so it isn't silently queued forever with no
// error row and no log — the exact regression F2 flagged.
func TestPromptTask_ExhaustedColdResumeNotClassifiedRuntimeUnavailable(t *testing.T) {
	oldReadyTimeout := agentPromptReadyTimeout
	oldReadyInterval := agentPromptReadyInterval
	agentPromptReadyTimeout = 20 * time.Millisecond
	agentPromptReadyInterval = time.Millisecond
	t.Cleanup(func() {
		agentPromptReadyTimeout = oldReadyTimeout
		agentPromptReadyInterval = oldReadyInterval
	})

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "er1",
		SessionID:        "s1",
		TaskID:           "t1",
		AgentExecutionID: "exec-wedged",
		Status:           "running",
		Resumable:        true,
		ResumeToken:      "resume-token-123",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed executor running: %v", err)
	}

	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunningFn:       func(context.Context, string) bool { return false },
		isAgentReadyFn:         func(context.Context, string) bool { return false },
		stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
			return nil
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			go func(sessID string) {
				sess, gErr := repo.GetTaskSession(ctx, sessID)
				if gErr == nil && sess != nil {
					sess.State = models.TaskSessionStateWaitingForInput
					sess.UpdatedAt = time.Now().UTC()
					_ = repo.UpdateTaskSession(ctx, sess)
				}
			}(req.SessionID)
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-still-wedged"}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	_, err = svc.PromptTask(ctx, "t1", "s1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected the exhausted cold-resume retry to fail")
	}
	if errors.Is(err, ErrSessionRuntimeUnavailable) {
		t.Fatalf("a genuine resume failure must not classify as ErrSessionRuntimeUnavailable "+
			"(it would be silently queued with no visible error): %v", err)
	}
}

// executorLookupErrorRepo wraps the real repo but forces
// GetExecutorRunningBySessionID to fail with a non-not-found error, standing
// in for a transient DB/context/deserialization failure.
type executorLookupErrorRepo struct {
	*sqliterepo.Repository
	err error
}

func (r *executorLookupErrorRepo) GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error) {
	return nil, r.err
}

// TestPromptTask_ExecutorLookupErrorNotClassifiedRuntimeUnavailable pins a
// second negative case alongside TestPromptTask_ExhaustedColdResumeNotClassifiedRuntimeUnavailable:
// a genuine executor-lookup failure (DB error, context cancellation, metadata
// deserialization) is not the "no row yet" shape errSessionAwaitingRuntimeLaunch
// exists for. It must stay visible instead of being classified as
// ErrSessionRuntimeUnavailable and silently queued with no future
// agent.boot_ready to drain it.
func TestPromptTask_ExecutorLookupErrorNotClassifiedRuntimeUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.executor = executor.NewExecutor(&mockAgentManager{repoForExecutionLookup: repo}, repo, testLogger(), executor.ExecutorConfig{})
	svc.repo = &executorLookupErrorRepo{Repository: repo, err: errors.New("db unavailable")}

	_, err := svc.PromptTask(ctx, "t1", "s1", "hello", "", false, nil, false)
	if err == nil {
		t.Fatal("expected a visible error for a genuine executor lookup failure")
	}
	if errors.Is(err, ErrSessionRuntimeUnavailable) {
		t.Fatalf("a genuine executor lookup error must not classify as ErrSessionRuntimeUnavailable "+
			"(it would be silently queued with no future drain trigger): %v", err)
	}
}

func TestPromptTask_NonRetryableRecoveryGuardRemainsVisible(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExecutorRunning(t, repo, "s1", "t1", "exec-retained")
	guardErr := fmt.Errorf("resume admission: %w", &SessionRecoveryGuardError{
		Cause: errors.New("recovery owner was superseded"), SessionID: "s1", Retryable: false,
	})
	agentMgr := &mockAgentManager{
		repoForExecutionLookup:   repo,
		isAgentRunningFn:         func(context.Context, string) bool { return false },
		isAgentReadyFn:           func(context.Context, string) bool { return false },
		currentPromptExecutionID: "exec-retained",
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return nil, guardErr
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	_, err = svc.PromptTask(ctx, "t1", "s1", "prompt", "", false, nil, false)
	if err == nil {
		t.Fatal("expected the superseded recovery guard to remain a visible error")
	}
	if errors.Is(err, ErrSessionRuntimeUnavailable) {
		t.Fatalf("non-retryable recovery guard was reclassified as runtime-unavailable: %v", err)
	}
	finalSession, loadErr := repo.GetTaskSession(ctx, "s1")
	if loadErr != nil {
		t.Fatalf("reload session: %v", loadErr)
	}
	if finalSession.State != models.TaskSessionStateFailed {
		t.Fatalf("non-retryable recovery guard session state = %q, want visible failed state", finalSession.State)
	}
}

func TestQueueUserPrompt_RecoveryWinsResumeRace(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "t1", "s1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "s1", "t1", "exec-retained")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	session.AgentProfileID = "profile1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	var ready atomic.Bool
	agentMgr := &mockAgentManager{
		repoForExecutionLookup:   repo,
		isAgentRunningFn:         func(context.Context, string) bool { return ready.Load() },
		isAgentReadyFn:           func(context.Context, string) bool { return ready.Load() },
		currentPromptExecutionID: "exec-retained",
		promptDone:               make(chan struct{}),
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			// Remote recovery attaches the idle runtime after cold resume has claimed STARTING.
			ready.Store(true)
			return nil, lifecycle.ErrAgentAlreadyRunning
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.executor.SetOnResumeFailureRollback(svc.rollbackResumeFailureIfCurrentAttempt)
	workerDone := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(workerDone) }
	if err := svc.QueueUserPrompt(ctx, "t1", "s1", "after restart", "", false, nil, nil, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("queued prompt dispatch did not settle")
	}
	if len(agentMgr.capturedPrompts) != 1 || agentMgr.capturedPrompts[0] != "after restart" {
		t.Fatalf("recovered prompt dispatches = %v, want original prompt exactly once", agentMgr.capturedPrompts)
	}
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("queue after recovery = %+v, want empty", entries)
	}
}
