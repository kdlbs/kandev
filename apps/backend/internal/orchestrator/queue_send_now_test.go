package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
