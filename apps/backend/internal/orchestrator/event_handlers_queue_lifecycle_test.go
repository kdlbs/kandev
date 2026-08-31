package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type turnStartSignalService struct {
	TurnService
	started chan<- struct{}
}

func (s *turnStartSignalService) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn, err := s.TurnService.StartTurn(ctx, sessionID)
	if err == nil {
		s.started <- struct{}{}
	}
	return turn, err
}

func TestExecuteQueuedMessage_LifecycleReselectsReplacementSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	first, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get first session: %v", err)
	}
	first.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, first); err != nil {
		t.Fatalf("set first session waiting: %v", err)
	}
	if err := repo.SetSessionPrimary(ctx, "s1"); err != nil {
		t.Fatalf("make first session primary: %v", err)
	}
	now := first.UpdatedAt.Add(time.Second)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s2", TaskID: "t1", State: models.TaskSessionStateWaitingForInput,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create replacement session: %v", err)
	}
	seedExecutorRunning(t, repo, "s2", "t1", "exec-2")

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	queued := &messagequeue.QueuedMessage{
		ID: "lifecycle-reselect", SessionID: "s1", TaskID: "t1", Content: "merged lifecycle prompt",
		Metadata: map[string]interface{}{
			"origin": githubPRAutomationOrigin, messagequeue.MetadataCoalesceKey: "github-pr:repo:1:merged",
		},
	}

	// The event was accepted for s1, then s2 becomes the current primary at
	// the final task-level lifecycle-resolution boundary.
	svc.repo = &lifecycleResolveHookRepository{
		repoStore: repo,
		beforeResolve: func() {
			if err := repo.SetSessionPrimary(ctx, "s2"); err != nil {
				t.Errorf("reselect primary session: %v", err)
			}
		},
	}
	svc.markQueuedDispatchInFlight("s1", queued.ID)
	svc.executeQueuedMessage("s1", queued)

	status := svc.messageQueue.GetStatus(ctx, "s2")
	if status.Count != 1 {
		t.Fatalf("replacement lifecycle retries = %d, want 1", status.Count)
	}
	retry, ok := svc.messageQueue.TakeQueued(ctx, "s2")
	if !ok {
		t.Fatal("replacement lifecycle retry was not durable")
	}
	svc.executeQueuedMessage("s2", retry)
	if got := len(agentMgr.capturedPromptCalls); got != 1 {
		t.Fatalf("replacement lifecycle prompts = %d, want 1", got)
	}
	if got := agentMgr.capturedPromptCalls[0].ExecutionID; got != "exec-2" {
		t.Fatalf("replacement prompt execution = %q, want exec-2", got)
	}
}

// A lifecycle event stays eligible when its original session is temporarily
// non-promptable and no replacement exists. That is a retry condition, not a
// task-inactive outcome.
func TestExecuteQueuedMessage_LifecycleNonPromptableSessionRequeues(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session temporarily non-promptable: %v", err)
	}

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	queued := &messagequeue.QueuedMessage{
		ID: "lifecycle-non-promptable", SessionID: "s1", TaskID: "t1", Content: "closed lifecycle prompt",
		Metadata: map[string]interface{}{
			"origin": githubPRAutomationOrigin, messagequeue.MetadataCoalesceKey: "github-pr:repo:1:closed",
		},
	}

	// The original session becomes non-selectable after prompt preparation but
	// before final lifecycle resolution. There is no replacement session.
	svc.repo = &lifecycleResolveHookRepository{
		repoStore: repo,
		beforeResolve: func() {
			if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateCompleted, "temporarily unavailable"); err != nil {
				t.Errorf("make session temporarily non-promptable: %v", err)
			}
		},
	}
	svc.markQueuedDispatchInFlight("s1", queued.ID)
	svc.executeQueuedMessage("s1", queued)

	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("non-promptable lifecycle retries = %d, want 1", got)
	}
	if got := len(agentMgr.capturedPromptCalls); got != 0 {
		t.Fatalf("non-promptable lifecycle prompts = %d, want 0", got)
	}
}

// A reset can cancel a lifecycle turn while task-state reconciliation is
// blocked. The queue must retry that entry instead of dispatching the stale
// turn or recording a duplicate visible message.
func TestExecuteQueuedMessage_LifecycleResetAfterTurnCreationRequeues(t *testing.T) {
	ctx := context.Background()
	svc, repo, manager, session := newActiveResetTestService(t)
	setupTurnValue, ok := svc.activeTurns.Load(session.ID)
	if !ok {
		t.Fatal("test setup did not create an active turn")
	}
	setupTurnID, ok := setupTurnValue.(string)
	if !ok || setupTurnID == "" {
		t.Fatalf("active turn cache value = %#v, want turn ID", setupTurnValue)
	}
	if err := svc.turnService.CompleteTurn(ctx, setupTurnID); err != nil {
		t.Fatalf("complete setup turn: %v", err)
	}
	svc.activeTurns.Delete(session.ID)
	if err := repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	manager.isAgentRunning = true
	svc.messageCreator = &mockMessageCreator{}
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})

	turnStarted := make(chan struct{}, 1)
	svc.turnService = &turnStartSignalService{
		TurnService: svc.turnService,
		started:     turnStarted,
	}
	_, _, accepted, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, session.ID, session.TaskID, "merged lifecycle prompt", "", messagequeue.QueuedByWorkflow,
		false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin},
		"github-pr:repo:1:merged", true,
	)
	if err != nil || !accepted {
		t.Fatalf("queue lifecycle entry: accepted=%v err=%v", accepted, err)
	}
	reserved, ok := svc.messageQueue.ReserveQueued(ctx, session.ID)
	if !ok {
		t.Fatal("reserve lifecycle entry")
	}
	svc.markQueuedDispatchInFlight(session.ID, reserved.ID)

	svc.taskRuntimeStateMu.Lock()
	locked := true
	defer func() {
		if locked {
			svc.taskRuntimeStateMu.Unlock()
		}
	}()
	dispatchDone := make(chan struct{})
	go func() {
		svc.executeQueuedMessage(session.ID, reserved)
		close(dispatchDone)
	}()
	select {
	case <-turnStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for lifecycle turn creation")
	}

	resetDone := make(chan bool, 1)
	go func() {
		resetDone <- svc.resetAgentContext(ctx, session.TaskID, session, "Successor")
	}()
	requireResetEvents(t, manager.events, "cancel", "reset")
	if resetOK := <-resetDone; !resetOK {
		t.Fatal("resetAgentContext returned false")
	}

	svc.taskRuntimeStateMu.Unlock()
	locked = false
	select {
	case <-dispatchDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for lifecycle dispatch")
	}

	if got := len(manager.capturedPromptCalls); got != 0 {
		t.Fatalf("prompts after reset cancelled lifecycle turn = %d, want 0", got)
	}
	if got := len(svc.messageCreator.(*mockMessageCreator).userMessages); got != 0 {
		t.Fatalf("visible lifecycle messages after reset = %d, want 0", got)
	}
	if got := svc.messageQueue.GetStatus(ctx, session.ID).Count; got != 1 {
		t.Fatalf("lifecycle retries after reset = %d, want 1", got)
	}
}

// A lifecycle prompt cannot be dispatched unless its visible automation chat
// message was persisted. The failed write must restore the pre-claim state,
// complete the just-started turn, and retain exactly one coalesced retry.
func TestExecuteQueuedMessage_LifecycleMessagePersistenceFailureDoesNotDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = &mockMessageCreator{userMessageErr: errors.New("persist lifecycle message")}
	queued := &messagequeue.QueuedMessage{
		ID: "lifecycle-message-persist-failure", SessionID: "s1", TaskID: "t1", Content: "review lifecycle prompt",
		Metadata: map[string]interface{}{
			"origin": githubPRAutomationOrigin, messagequeue.MetadataCoalesceKey: "github-pr:repo:1:review_requested",
		},
	}

	svc.markQueuedDispatchInFlight("s1", queued.ID)
	svc.executeQueuedMessage("s1", queued)

	if got := len(agentMgr.capturedPromptCalls); got != 0 {
		t.Fatalf("prompt calls after visible-message persistence failure = %d, want 0", got)
	}
	persisted, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if persisted.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after visible-message persistence failure = %s, want WAITING_FOR_INPUT", persisted.State)
	}
	task, err := taskRepo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task state: %v", err)
	}
	if task.State != v1.TaskStateReview {
		t.Fatalf("task state after visible-message persistence failure = %s, want REVIEW", task.State)
	}
	activeTurn, err := svc.turnService.GetActiveTurn(ctx, "s1")
	if err != nil {
		t.Fatalf("get active turn: %v", err)
	}
	if activeTurn != nil {
		t.Fatalf("active turn after visible-message persistence failure = %q, want none", activeTurn.ID)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		entries, _, snapshotErr := svc.messageQueue.SnapshotSession(ctx, "s1")
		t.Fatalf(
			"visible-message persistence lifecycle retries = %d, want 1; snapshot=%+v err=%v",
			got, entries, snapshotErr,
		)
	}
}

func TestExecuteQueuedMessage_LifecycleTaskRollbackDoesNotClobberConcurrentTransition(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &hookedMessageCreator{
		mockMessageCreator: &mockMessageCreator{userMessageErr: errors.New("persist lifecycle message")},
		beforeCreate: func() {
			if err := taskRepo.UpdateTaskState(ctx, "t1", v1.TaskStateFailed); err != nil {
				t.Errorf("concurrent task transition: %v", err)
			}
		},
	}
	queued := &messagequeue.QueuedMessage{
		ID: "lifecycle-task-cas-rollback", SessionID: "s1", TaskID: "t1", Content: "review lifecycle prompt",
		Metadata: map[string]interface{}{
			"origin": githubPRAutomationOrigin, messagequeue.MetadataCoalesceKey: "github-pr:repo:1:review_requested",
		},
	}

	svc.markQueuedDispatchInFlight("s1", queued.ID)
	svc.executeQueuedMessage("s1", queued)

	task, err := taskRepo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task state: %v", err)
	}
	if task.State != v1.TaskStateFailed {
		t.Fatalf("task state after concurrent transition = %s, want FAILED", task.State)
	}
}

func TestDrainQueuedMessage_LifecycleEntryRemainsDurableUntilPromptAcceptance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	resolveEntered := make(chan struct{})
	allowResolve := make(chan struct{})
	promptEntered := make(chan struct{})
	acceptPrompt := make(chan struct{})
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptAgentFunc: func(
			context.Context, string, string, []v1.MessageAttachment, bool,
		) (*executor.PromptResult, error) {
			close(promptEntered)
			<-acceptPrompt
			return &executor.PromptResult{}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.repo = &lifecycleResolveBarrierRepository{
		repoStore:      repo,
		resolveEntered: resolveEntered,
		allowResolve:   allowResolve,
	}
	t.Cleanup(func() {
		select {
		case <-allowResolve:
		default:
			close(allowResolve)
		}
		select {
		case <-acceptPrompt:
		default:
			close(acceptPrompt)
		}
	})

	queued, _, accepted, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s1", "t1", "merged lifecycle prompt", "", messagequeue.QueuedByWorkflow,
		false, nil,
		map[string]interface{}{"origin": githubPRAutomationOrigin},
		"github-pr:repo:1:merged", true,
	)
	if err != nil || !accepted {
		t.Fatalf("queue checkpointed lifecycle entry: accepted=%v err=%v", accepted, err)
	}
	drained, err := svc.DrainQueuedMessage(ctx, "s1")
	if err != nil || !drained {
		t.Fatalf("start lifecycle drain: drained=%v err=%v", drained, err)
	}
	<-resolveEntered

	if !svc.messageQueue.IsCurrentLifecycleReservation(ctx, queued) {
		t.Error("durable lifecycle row missing from storage before final claim")
	}
	// The reservation is in flight, not pending: showing it would duplicate the
	// prompt the drain is already delivering.
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Errorf("pending entries before final claim = %d, want 0", got)
	}
	close(allowResolve)
	<-promptEntered
	if !svc.messageQueue.IsCurrentLifecycleReservation(ctx, queued) {
		t.Error("durable lifecycle row missing from storage before PromptAgent acceptance")
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Errorf("pending entries before PromptAgent acceptance = %d, want 0", got)
	}
	close(acceptPrompt)
}

func TestExecuteQueuedMessage_LifecycleReservationPurgedAcrossArchiveUnarchiveDoesNotDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	resolveEntered := make(chan struct{})
	allowResolve := make(chan struct{})
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	svc.repo = &lifecycleResolveBarrierRepository{
		repoStore:      repo,
		resolveEntered: resolveEntered,
		allowResolve:   allowResolve,
	}
	t.Cleanup(func() {
		select {
		case <-allowResolve:
		default:
			close(allowResolve)
		}
	})

	_, _, accepted, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s1", "t1", "merged lifecycle prompt", "", messagequeue.QueuedByWorkflow,
		false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin},
		"github-pr:repo:1:merged", true,
	)
	if err != nil || !accepted {
		t.Fatalf("queue lifecycle entry: accepted=%v err=%v", accepted, err)
	}
	queued, ok := svc.messageQueue.ReserveQueued(ctx, "s1")
	if !ok {
		t.Fatal("reserve lifecycle entry")
	}
	svc.markQueuedDispatchInFlight("s1", queued.ID)
	dispatchDone := make(chan struct{})
	go func() {
		svc.executeQueuedMessage("s1", queued)
		close(dispatchDone)
	}()
	<-resolveEntered

	if err := repo.ArchiveTask(ctx, "t1"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if _, err := repo.UnarchiveTask(ctx, "t1"); err != nil {
		t.Fatalf("unarchive task: %v", err)
	}
	close(allowResolve)
	select {
	case <-dispatchDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stale lifecycle dispatch")
	}

	if got := len(agentMgr.capturedPromptCalls); got != 0 {
		t.Fatalf("stale lifecycle prompts = %d, want 0", got)
	}
	if got := len(svc.messageCreator.(*mockMessageCreator).userMessages); got != 0 {
		t.Fatalf("stale visible lifecycle messages = %d, want 0", got)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("stale lifecycle retries = %d, want 0", got)
	}
}

func TestExecuteQueuedMessage_LifecycleReservationPurgedBeforeWorkflowSideEffects(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	sessionReadEntered := make(chan struct{})
	allowSessionRead := make(chan struct{})
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
	steps := &countingLifecycleStepGetter{mockStepGetter: newMockStepGetter()}
	svc := createTestServiceWithAgent(repo, steps.mockStepGetter, taskRepo, agentMgr)
	svc.workflowStepGetter = steps
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	svc.repo = &lifecycleWorkflowBarrierRepository{
		repoStore:          repo,
		sessionReadEntered: sessionReadEntered,
		allowSessionRead:   allowSessionRead,
	}
	t.Cleanup(func() {
		select {
		case <-allowSessionRead:
		default:
			close(allowSessionRead)
		}
	})

	_, _, accepted, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s1", "t1", "review lifecycle prompt", "", messagequeue.QueuedByWorkflow,
		false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin},
		"github-pr:repo:1:review_requested", true,
	)
	if err != nil || !accepted {
		t.Fatalf("queue lifecycle entry: accepted=%v err=%v", accepted, err)
	}
	queued, ok := svc.messageQueue.ReserveQueued(ctx, "s1")
	if !ok {
		t.Fatal("reserve lifecycle entry")
	}
	svc.markQueuedDispatchInFlight("s1", queued.ID)
	dispatchDone := make(chan struct{})
	go func() {
		svc.executeQueuedMessage("s1", queued)
		close(dispatchDone)
	}()
	<-sessionReadEntered

	if err := repo.ArchiveTask(ctx, "t1"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if _, err := repo.UnarchiveTask(ctx, "t1"); err != nil {
		t.Fatalf("unarchive task: %v", err)
	}
	close(allowSessionRead)
	select {
	case <-dispatchDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stale lifecycle dispatch")
	}

	if steps.getStepCalls != 0 {
		t.Fatalf("stale lifecycle workflow step lookups = %d, want 0", steps.getStepCalls)
	}
	if got := len(agentMgr.capturedPromptCalls); got != 0 {
		t.Fatalf("stale lifecycle prompts = %d, want 0", got)
	}
	if got := len(svc.messageCreator.(*mockMessageCreator).userMessages); got != 0 {
		t.Fatalf("stale visible lifecycle messages = %d, want 0", got)
	}
}
