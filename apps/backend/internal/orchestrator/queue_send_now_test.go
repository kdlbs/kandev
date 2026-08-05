package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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
