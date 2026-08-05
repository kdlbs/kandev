package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
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
