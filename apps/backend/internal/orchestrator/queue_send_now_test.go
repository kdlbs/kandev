package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestSelectSendNowEntriesUsesExactEntryOrSnapshot(t *testing.T) {
	status := &messagequeue.QueueStatus{Entries: []messagequeue.QueuedMessage{
		{ID: "first", Content: "one"},
		{ID: "second", Content: "two"},
	}}

	entry, ids, err := selectSendNowEntries(status, QueueSendNowScopeEntry, "second")
	if err != nil {
		t.Fatalf("entry selection error = %v", err)
	}
	if len(entry) != 1 || entry[0].ID != "second" || len(ids) != 1 || ids[0] != "second" {
		t.Fatalf("entry selection = %#v, %#v", entry, ids)
	}

	all, ids, err := selectSendNowEntries(status, QueueSendNowScopeAll, "")
	if err != nil {
		t.Fatalf("all selection error = %v", err)
	}
	if len(all) != 2 || ids[0] != "first" || ids[1] != "second" {
		t.Fatalf("all selection = %#v, %#v", all, ids)
	}
	all[0].Content = "mutated copy"
	if status.Entries[0].Content != "one" {
		t.Fatal("all selection mutated the authoritative status snapshot")
	}
}

func TestSelectSendNowEntriesRejectsEmptyAndRacedEntry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  *messagequeue.QueueStatus
		scope   string
		entryID string
		want    error
	}{
		{name: "empty queue", status: &messagequeue.QueueStatus{}, scope: QueueSendNowScopeAll, want: ErrSendNowQueueEmpty},
		{name: "missing entry", status: &messagequeue.QueueStatus{Entries: []messagequeue.QueuedMessage{{ID: "other"}}}, scope: QueueSendNowScopeEntry, entryID: "gone", want: ErrSendNowEntryNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := selectSendNowEntries(tc.status, tc.scope, tc.entryID)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSendNowWorkersCanRestartAfterStop(t *testing.T) {
	svc := &Service{logger: testLogger()}

	svc.stopSendNowWorkers()
	if !svc.sendNowStopped {
		t.Fatal("stopping Send Now workers did not mark the worker owner stopped")
	}

	svc.resetSendNowWorkers()
	if svc.sendNowStopped {
		t.Fatal("resetting Send Now workers left the worker owner stopped")
	}
	if svc.sendNowCtx == nil || svc.sendNowCancel == nil {
		t.Fatal("resetting Send Now workers did not create a fresh cancellable context")
	}
	select {
	case <-svc.sendNowCtx.Done():
		t.Fatal("fresh Send Now worker context is already cancelled")
	default:
	}

	svc.stopSendNowWorkers()
}

func TestExplicitCancellationDoesNotJoinSendNowOperation(t *testing.T) {
	operation := &cancelOperation{
		done:   make(chan struct{}),
		joined: make(chan struct{}),
		kind:   cancellationKindQueueSendNow,
	}
	svc := &Service{
		cancellationOperations: map[string]*cancelOperation{"session": operation},
	}

	_, owner, action := svc.claimExplicitCancellation("session", func(context.Context, *cancelOperation) (bool, error) {
		t.Fatal("explicit cancellation action must not be registered for Send Now")
		return false, nil
	})
	if owner {
		t.Fatal("explicit cancellation unexpectedly claimed the Send Now operation")
	}
	if action != nil {
		t.Fatal("explicit cancellation unexpectedly joined the Send Now operation")
	}
	if len(operation.actions) != 0 {
		t.Fatalf("Send Now operation gained %d explicit actions", len(operation.actions))
	}
	select {
	case <-operation.joined:
		t.Fatal("explicit cancellation should not mark Send Now as joined")
	default:
	}
}

func TestExecuteSendNowClaimRestorePreservesRecordedSources(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		promptErr:              errors.New("replacement prompt rejected"),
		repoForExecutionLookup: repo,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	messageCreator := &mockMessageCreator{}
	svc.messageCreator = messageCreator
	svc.activeTurns.Store("session-1", "turn-1")
	if err := svc.messageQueue.SetAutoRun(ctx, "session-1", false); err != nil {
		t.Fatalf("pause Auto-run: %v", err)
	}

	if _, err := svc.messageQueue.QueueMessageWithMetadata(ctx, "session-1", "task-1", "first", "", messagequeue.QueuedByUser, false, nil, nil); err != nil {
		t.Fatalf("seed ordinary replacement source: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(ctx, "session-1", "task-1", "second", "", messagequeue.QueuedByWorkflow, false, nil,
		map[string]interface{}{messagequeue.MetadataLifecycleDurable: true}); err != nil {
		t.Fatalf("seed durable replacement source: %v", err)
	}
	sources := svc.messageQueue.GetStatus(ctx, "session-1").Entries
	if len(sources) != 2 {
		t.Fatalf("seeded replacement source count = %d, want 2", len(sources))
	}
	claimed, err := svc.messageQueue.ClaimSendNow(ctx, "session-1", []messagequeue.QueuedMessage{sources[0], sources[1]})
	if err != nil {
		t.Fatalf("claim replacement sources: %v", err)
	}
	if !svc.messageQueue.GetStatus(ctx, "session-1").AutoRun {
		t.Fatal("accepted Send Now claim did not resume Auto-run")
	}

	// Production dispatch registers the handoff before starting the worker;
	// preserve that ownership precondition for this direct worker test.
	svc.markQueuedDispatchInFlight("session-1", claimed.Dispatch.ID)
	svc.executeSendNowClaim(claimed)
	if len(messageCreator.userMessages) != 1 {
		t.Fatalf("replacement retry created %d user messages, want 1", len(messageCreator.userMessages))
	}
	entries := svc.messageQueue.GetStatus(ctx, "session-1").Entries
	if len(entries) != 2 {
		t.Fatalf("restored source count = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		if recorded, _ := entry.Metadata[metaKeyUserMessageRecorded].(bool); !recorded {
			t.Fatalf("restored source %q lost user_message_recorded marker: %#v", entry.ID, entry.Metadata)
		}
	}
	if !svc.messageQueue.GetStatus(ctx, "session-1").AutoRun {
		t.Fatal("restoring accepted Send Now claim reverted Auto-run")
	}
}

func TestSendQueuedNowMissingEntryPreservesAutoRunOff(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	if _, err := svc.messageQueue.QueueMessage(
		ctx, "session-1", "task-1", "pending", "", messagequeue.QueuedByUser, false, nil,
	); err != nil {
		t.Fatalf("queue message: %v", err)
	}
	if err := svc.messageQueue.SetAutoRun(ctx, "session-1", false); err != nil {
		t.Fatalf("pause Auto-run: %v", err)
	}

	if _, err := svc.SendQueuedNow(ctx, "session-1", QueueSendNowScopeEntry, "missing"); !errors.Is(err, ErrSendNowEntryNotFound) {
		t.Fatalf("SendQueuedNow error = %v, want %v", err, ErrSendNowEntryNotFound)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.AutoRun {
		t.Fatal("rejected Send Now selection resumed Auto-run")
	}
	if status.Count != 1 {
		t.Fatalf("rejected Send Now changed queue count to %d", status.Count)
	}
}

func TestSendQueuedNowSupersedesPendingFIFOHandoff(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		promptDone:             make(chan struct{}),
		repoForExecutionLookup: repo,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}

	reservationMetadata := map[string]interface{}{
		messagequeue.MetadataLifecycleDurable: true,
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "first queued", "", messagequeue.QueuedByWorkflow, false, nil, reservationMetadata,
	); err != nil {
		t.Fatalf("queue first message: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "second queued", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue second message: %v", err)
	}

	reserved, ok := svc.messageQueue.ReserveQueued(ctx, "session-1")
	if !ok || reserved == nil || reserved.Content != "first queued" {
		t.Fatalf("reserve FIFO head: message=%#v ok=%v", reserved, ok)
	}
	reservation := svc.markQueuedDispatchInFlightWithSource("session-1", reserved.ID, reserved)

	sent, err := svc.SendQueuedNow(ctx, "session-1", QueueSendNowScopeAll, "")
	if err != nil {
		t.Fatalf("send now: %v", err)
	}
	if sent != 2 {
		t.Fatalf("send now sent_count = %d, want 2", sent)
	}
	// The real FIFO worker may already be runnable when Send Now wins. Run its
	// stale handoff synchronously here as well: it must observe the superseded
	// phase and neither requeue the durable source nor create side effects.
	svc.executeQueuedMessageWithReservation("session-1", reserved, reservation)

	select {
	case <-agentMgr.promptDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for aggregate replacement prompt")
	}
	if len(agentMgr.capturedPrompts) != 1 {
		t.Fatalf("replacement prompt count = %d, want 1", len(agentMgr.capturedPrompts))
	}
	if got := agentMgr.capturedPrompts[0]; got != "first queued\n\nsecond queued" {
		t.Fatalf("replacement prompt = %q, want FIFO aggregate", got)
	}
	if !strings.Contains(agentMgr.capturedPrompts[0], "first queued") ||
		!strings.Contains(agentMgr.capturedPrompts[0], "second queued") {
		t.Fatalf("replacement prompt lost a queued body: %q", agentMgr.capturedPrompts[0])
	}
	if got := svc.messageQueue.GetStatus(ctx, "session-1").Count; got != 0 {
		t.Fatalf("queue count after aggregate dispatch = %d, want 0", got)
	}
	if got := len(svc.messageCreator.(*mockMessageCreator).userMessages); got != 1 {
		t.Fatalf("visible replacement message count = %d, want 1", got)
	}
}

func TestSendQueuedNowConflictsAfterFIFOHandoffAccepted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	promptEntered := make(chan struct{})
	allowPrompt := make(chan struct{})
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptAgentFunc: func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
			close(promptEntered)
			<-allowPrompt
			return &executor.PromptResult{}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.messageQueue.SetAutoMergeEnabled(false)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}

	for _, content := range []string{"first queued", "second queued"} {
		if _, err := svc.messageQueue.QueueMessageWithMetadata(
			ctx, "session-1", "task-1", content, "", messagequeue.QueuedByUser, false, nil, nil,
		); err != nil {
			t.Fatalf("queue %q: %v", content, err)
		}
	}
	if !svc.drainQueuedMessageForPromptableSession(ctx, "session-1") {
		t.Fatal("normal FIFO drain did not start")
	}
	<-promptEntered

	if _, err := svc.SendQueuedNow(ctx, "session-1", QueueSendNowScopeAll, ""); !errors.Is(err, ErrSendNowConflict) {
		t.Fatalf("send now error = %v, want %v", err, ErrSendNowConflict)
	}
	if got := agentMgr.cancelAgentCalls.Load(); got != 0 {
		t.Fatalf("send now cancelled accepted FIFO turn %d times, want 0", got)
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 || status.Entries[0].Content != "second queued" {
		t.Fatalf("remaining queue = %#v, want second queued only", status.Entries)
	}

	close(allowPrompt)
}

func TestStreamCompletePreservesAcceptedSendNowDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	reservation := svc.markQueuedDispatchInFlight("session1", "dispatch-1")
	tracked, err := svc.claimQueuedDispatchForExecution("session1", "dispatch-1", reservation)
	if err != nil || !tracked {
		t.Fatalf("claim accepted dispatch: tracked=%v err=%v", tracked, err)
	}
	successor, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("start successor turn: %v", err)
	}
	reservation.bindSuccessorTurn(successor.ID)
	if !svc.isCurrentQueuedDispatch("session1", "dispatch-1") {
		t.Fatal("accepted send-now dispatch was not current before stream complete")
	}

	svc.completeTurnForTaskSession(ctx, "task1", "session1")
	if !svc.isCurrentQueuedDispatch("session1", "dispatch-1") {
		t.Fatal("stream-only predecessor complete wiped accepted send-now dispatch")
	}
	active, err := svc.turnService.GetActiveTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("get active turn: %v", err)
	}
	if active == nil || active.ID != successor.ID {
		t.Fatalf("active successor = %#v, want %s", active, successor.ID)
	}
	svc.completeTurnForSession(ctx, "session1")
	if svc.isCurrentQueuedDispatch("session1", "dispatch-1") {
		t.Fatal("ready-path successor complete left accepted send-now dispatch blocking the next queue action")
	}
}

func TestLiveSendNowSuccessorDoesNotConflictAndStaysProtected(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	reservation := svc.markQueuedDispatchInFlight("session1", "dispatch-1")
	if _, err := svc.claimQueuedDispatchForExecution("session1", "dispatch-1", reservation); err != nil {
		t.Fatalf("claim accepted dispatch: %v", err)
	}
	successor, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("start successor turn: %v", err)
	}
	reservation.bindSuccessorTurn(successor.ID)
	if !svc.isQueuedDispatchAccepted("session1") {
		t.Fatal("handoff reservation should conflict before it is live")
	}

	svc.markAcceptedDispatchLive("session1", reservation)
	if svc.isQueuedDispatchAccepted("session1") {
		t.Fatal("live send-now successor still reported as a handoff conflict")
	}
	if _, err := svc.pendingQueuedDispatchForSendNow("session1"); err != nil {
		t.Fatalf("live successor blocked send-now restore: %v", err)
	}

	svc.completeTurnForTaskSession(ctx, "task1", "session1")
	if !svc.isCurrentQueuedDispatch("session1", "dispatch-1") {
		t.Fatal("stream-only predecessor complete wiped live send-now dispatch")
	}
	active, err := svc.turnService.GetActiveTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("get active successor turn: %v", err)
	}
	if active == nil || active.ID != successor.ID {
		t.Fatalf("active successor = %#v, want %s", active, successor.ID)
	}
}

func TestSendQueuedNowCancelsLiveReplacementTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	firstPromptEntered := make(chan struct{})
	allowFirstPrompt := make(chan struct{})
	secondPromptEntered := make(chan struct{})
	allowSecondPrompt := make(chan struct{})
	var releaseFirstPrompt, releaseSecondPrompt, markSecondPrompt sync.Once
	var stopSendNowWorkers func()
	t.Cleanup(func() {
		releaseFirstPrompt.Do(func() { close(allowFirstPrompt) })
		releaseSecondPrompt.Do(func() { close(allowSecondPrompt) })
		if stopSendNowWorkers != nil {
			stopSendNowWorkers()
		}
	})
	var promptCount atomic.Int32
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptAgentFunc: func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
			n := promptCount.Add(1)
			if n == 1 {
				close(firstPromptEntered)
				<-allowFirstPrompt
				return &executor.PromptResult{}, nil
			}
			markSecondPrompt.Do(func() { close(secondPromptEntered) })
			<-allowSecondPrompt
			return &executor.PromptResult{}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.messageQueue.SetAutoMergeEnabled(false)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	stopSendNowWorkers = svc.stopSendNowWorkers

	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "first send now", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue first message: %v", err)
	}
	if _, err := svc.SendQueuedNow(ctx, "session-1", QueueSendNowScopeAll, ""); err != nil {
		t.Fatalf("first send now: %v", err)
	}
	select {
	case <-firstPromptEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first replacement prompt")
	}

	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "second send now", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue second message: %v", err)
	}
	if _, err := svc.SendQueuedNow(ctx, "session-1", QueueSendNowScopeAll, ""); err != nil {
		t.Fatalf("later send now: %v", err)
	}
	if got := agentMgr.cancelAgentCalls.Load(); got == 0 {
		t.Fatal("later send now did not cancel the live replacement turn")
	}

	releaseFirstPrompt.Do(func() { close(allowFirstPrompt) })
	select {
	case <-secondPromptEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for later replacement prompt")
	}
	releaseSecondPrompt.Do(func() { close(allowSecondPrompt) })
}

func TestSendQueuedNowConflictsBeforeReplacementClaimsPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	preClaimEntered := make(chan struct{})
	releasePreClaim := make(chan struct{})
	var releasePreClaimOnce sync.Once
	var executionLookupCalls atomic.Int32
	promptDone := make(chan struct{})
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptDone:             promptDone,
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			if executionLookupCalls.Add(1) == 1 {
				close(preClaimEntered)
				<-releasePreClaim
			}
			return "exec-1", nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.messageQueue.SetAutoMergeEnabled(false)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	t.Cleanup(func() {
		releasePreClaimOnce.Do(func() { close(releasePreClaim) })
		svc.stopSendNowWorkers()
	})

	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "first send now", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue first message: %v", err)
	}
	if _, err := svc.SendQueuedNow(ctx, "session-1", QueueSendNowScopeAll, ""); err != nil {
		t.Fatalf("first send now: %v", err)
	}
	select {
	case <-preClaimEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for replacement prompt claim barrier")
	}

	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "second send now", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue second message: %v", err)
	}
	if _, err := svc.SendQueuedNow(ctx, "session-1", QueueSendNowScopeAll, ""); !errors.Is(err, ErrSendNowConflict) {
		t.Fatalf("send now while replacement is pre-claim error = %v, want %v", err, ErrSendNowConflict)
	}

	releasePreClaimOnce.Do(func() { close(releasePreClaim) })
	select {
	case <-promptDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first replacement prompt")
	}
}

func TestStreamCompleteSettlesCurrentSendNowSuccessor(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")
	agentMgr := &mockAgentManager{currentPromptExecutionID: "exec-1"}
	agentMgr.currentPromptGeneration.Store(2)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	reservation := svc.markQueuedDispatchInFlight("session1", "dispatch-1")
	if tracked, err := svc.claimQueuedDispatchForExecution("session1", "dispatch-1", reservation); err != nil || !tracked {
		t.Fatalf("claim accepted dispatch: tracked=%v err=%v", tracked, err)
	}
	successor, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("start successor turn: %v", err)
	}
	reservation.bindSuccessorTurn(successor.ID)

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "task1",
		SessionID:   "session1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:             agentEventComplete,
			PromptGeneration: 2,
		},
	})

	active, err := svc.turnService.GetActiveTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("get active successor turn: %v", err)
	}
	if active != nil {
		t.Fatalf("current successor complete left turn %q active", active.ID)
	}
	if svc.acceptedDispatchInFlight("session1") {
		t.Fatal("current successor complete left accepted dispatch marker active")
	}
}

func TestStreamCompletePreservesSuccessorForStalePromptGeneration(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")
	agentMgr := &mockAgentManager{currentPromptExecutionID: "exec-1"}
	agentMgr.currentPromptGeneration.Store(2)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	reservation := svc.markQueuedDispatchInFlight("session1", "dispatch-1")
	if tracked, err := svc.claimQueuedDispatchForExecution("session1", "dispatch-1", reservation); err != nil || !tracked {
		t.Fatalf("claim accepted dispatch: tracked=%v err=%v", tracked, err)
	}
	successor, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("start successor turn: %v", err)
	}
	reservation.bindSuccessorTurn(successor.ID)

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:      "task1",
		SessionID:   "session1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:             agentEventComplete,
			PromptGeneration: 1,
		},
	})

	active, err := svc.turnService.GetActiveTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("get active successor turn: %v", err)
	}
	if active == nil || active.ID != successor.ID {
		t.Fatalf("stale predecessor complete changed active turn to %#v, want %s", active, successor.ID)
	}
	if !svc.acceptedDispatchInFlight("session1") {
		t.Fatal("stale predecessor complete cleared accepted successor dispatch")
	}
}

func TestCompleteTurnsExceptReportsIterationExhaustion(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	keep, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("start keep turn: %v", err)
	}
	for i := 0; i < 17; i++ {
		if _, err := svc.turnService.StartTurn(ctx, "session1"); err != nil {
			t.Fatalf("start predecessor turn %d: %v", i, err)
		}
	}

	if err := svc.completeTurnsExcept(ctx, "session1", keep.ID); err == nil {
		t.Fatal("completeTurnsExcept returned nil after hitting its iteration limit")
	}
	active, err := svc.turnService.GetActiveTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("get active turn after exhaustion: %v", err)
	}
	if active == nil || active.ID == keep.ID {
		t.Fatalf("iteration exhaustion incorrectly reported successor as settled: %#v", active)
	}
}
