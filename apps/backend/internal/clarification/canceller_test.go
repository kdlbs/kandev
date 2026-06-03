package clarification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type stubMessageStore struct {
	messages map[string][]*taskmodels.Message
	updated  []*taskmodels.Message
}

func (s *stubMessageStore) GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error) {
	return nil, errors.New("not implemented")
}

func (s *stubMessageStore) FindMessageByPendingID(_ context.Context, pendingID string) (*taskmodels.Message, error) {
	msgs, ok := s.messages[pendingID]
	if !ok || len(msgs) == 0 {
		return nil, errors.New("not found")
	}
	return msgs[0], nil
}

func (s *stubMessageStore) FindMessagesByPendingID(_ context.Context, pendingID string) ([]*taskmodels.Message, error) {
	msgs, ok := s.messages[pendingID]
	if !ok {
		return nil, nil
	}
	return msgs, nil
}

func (s *stubMessageStore) FindPendingClarificationMessagesBySessionID(_ context.Context, sessionID string) ([]*taskmodels.Message, error) {
	var out []*taskmodels.Message
	for _, msgs := range s.messages {
		for _, m := range msgs {
			if m.TaskSessionID == sessionID {
				if status, _ := m.Metadata["status"].(string); status == "pending" {
					out = append(out, m)
				}
			}
		}
	}
	return out, nil
}

func (s *stubMessageStore) UpdateMessage(_ context.Context, m *taskmodels.Message) error {
	s.updated = append(s.updated, m)
	return nil
}

type stubEventBus struct {
	events []*bus.Event
}

func (s *stubEventBus) Publish(_ context.Context, _ string, ev *bus.Event) error {
	s.events = append(s.events, ev)
	return nil
}

func newTestCanceller(t *testing.T, msgs map[string][]*taskmodels.Message) (*Canceller, *stubMessageStore, *stubEventBus) {
	t.Helper()
	store := NewStore(time.Minute)
	repo := &stubMessageStore{messages: msgs}
	eventBus := &stubEventBus{}
	return NewCanceller(store, repo, eventBus, logger.Default()), repo, eventBus
}

// TestCanceller_MarksDetachedOnDisconnect verifies that when the agent's turn
// ends with a pending clarification, the message stays pending with
// agent_disconnected so the overlay remains interactive for a deferred answer.
func TestCanceller_MarksDetachedOnDisconnect(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{}
	c, repo, _ := newTestCanceller(t, msgs)

	pendingID, _ := c.store.CreateRequest(&Request{SessionID: "s1"})
	msgs[pendingID] = []*taskmodels.Message{{
		ID:            "m1",
		TaskSessionID: "s1",
		Metadata: map[string]any{
			"status": "pending",
		},
	}}

	cancelled := c.CancelSessionAndNotify(context.Background(), "s1")
	if cancelled != 1 {
		t.Fatalf("expected 1 cancelled, got %d", cancelled)
	}

	if len(repo.updated) != 1 {
		t.Fatalf("expected 1 message update, got %d", len(repo.updated))
	}
	updated := repo.updated[0]
	if got, _ := updated.Metadata["agent_disconnected"].(bool); !got {
		t.Errorf("expected agent_disconnected=true, got %v", updated.Metadata["agent_disconnected"])
	}
	if got, _ := updated.Metadata["status"].(string); got != "pending" {
		t.Errorf("expected status=pending, got %q", got)
	}
}

// TestCanceller_ExpireSessionAndNotify_MarksExpired verifies the explicit expiry
// path closes the overlay and records a timed-out history entry.
func TestCanceller_ExpireSessionAndNotify_MarksExpired(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{}
	c, repo, _ := newTestCanceller(t, msgs)

	pendingID, _ := c.store.CreateRequest(&Request{SessionID: "s1"})
	msgs[pendingID] = []*taskmodels.Message{{
		ID:            "m1",
		TaskSessionID: "s1",
		Metadata: map[string]any{
			"status": "pending",
		},
	}}

	cancelled := c.ExpireSessionAndNotify(context.Background(), "s1")
	if cancelled != 1 {
		t.Fatalf("expected 1 cancelled, got %d", cancelled)
	}
	if len(repo.updated) != 1 {
		t.Fatalf("expected 1 message update, got %d", len(repo.updated))
	}
	updated := repo.updated[0]
	if got, _ := updated.Metadata["status"].(string); got != "expired" {
		t.Errorf("expected status=expired, got %q", got)
	}
}

// TestCanceller_NoMessagesToUpdate confirms that a cancel with no pending
// clarifications returns 0 and does not touch the repo.
func TestCanceller_NoMessagesToUpdate(t *testing.T) {
	c, repo, _ := newTestCanceller(t, map[string][]*taskmodels.Message{})

	if got := c.CancelSessionAndNotify(context.Background(), "nonexistent"); got != 0 {
		t.Errorf("expected 0 cancelled, got %d", got)
	}
	if len(repo.updated) != 0 {
		t.Errorf("expected no message updates, got %d", len(repo.updated))
	}
}

// TestCanceller_PublishesMessageUpdatedEvent confirms the canceller publishes
// a message.updated event so the frontend refreshes the message in place.
func TestCanceller_PublishesMessageUpdatedEvent(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{}
	c, _, eventBus := newTestCanceller(t, msgs)

	pendingID, _ := c.store.CreateRequest(&Request{SessionID: "s1"})
	msgs[pendingID] = []*taskmodels.Message{{
		ID:            "m1",
		TaskSessionID: "s1",
		Metadata:      map[string]any{"status": "pending"},
	}}

	c.CancelSessionAndNotify(context.Background(), "s1")

	if len(eventBus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventBus.events))
	}
}

// TestCanceller_MultiQuestion_MarksAllMessagesDetached confirms a multi-question
// bundle has every message marked agent_disconnected while staying pending.
func TestCanceller_MultiQuestion_MarksAllMessagesDetached(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{}
	c, _, eventBus := newTestCanceller(t, msgs)

	pendingID, _ := c.store.CreateRequest(&Request{SessionID: "s1"})
	msgs[pendingID] = []*taskmodels.Message{
		{ID: "m1", TaskSessionID: "s1", Metadata: map[string]any{"status": "pending", "question_id": "q1"}},
		{ID: "m2", TaskSessionID: "s1", Metadata: map[string]any{"status": "pending", "question_id": "q2"}},
		{ID: "m3", TaskSessionID: "s1", Metadata: map[string]any{"status": "pending", "question_id": "q3"}},
	}

	cancelled := c.CancelSessionAndNotify(context.Background(), "s1")
	if cancelled != 1 {
		t.Fatalf("expected 1 cancelled bundle, got %d", cancelled)
	}
	if len(eventBus.events) != 3 {
		t.Fatalf("expected 3 message.updated events, got %d", len(eventBus.events))
	}
}

// TestCanceller_MarksDetachedWhenStoreAlreadyDrained verifies that even when
// the in-memory store entry has already been removed (racing MCP-timeout path),
// DetachSessionAndNotify still finds and marks the DB messages detached.
func TestCanceller_MarksDetachedWhenStoreAlreadyDrained(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{}
	c, repo, _ := newTestCanceller(t, msgs)

	pendingID := "orphan-pending-id"
	msgs[pendingID] = []*taskmodels.Message{{
		ID:            "m1",
		TaskSessionID: "s1",
		Metadata: map[string]any{
			"status":     "pending",
			"pending_id": pendingID,
		},
	}}

	cancelled := c.DetachSessionAndNotify(context.Background(), "s1")
	if cancelled != 1 {
		t.Fatalf("expected 1 cancelled bundle from DB fallback, got %d", cancelled)
	}
	if len(repo.updated) != 1 {
		t.Fatalf("expected 1 message update, got %d", len(repo.updated))
	}
	updated := repo.updated[0]
	if got, _ := updated.Metadata["status"].(string); got != "pending" {
		t.Errorf("expected status=pending, got %q", got)
	}
	if got, _ := updated.Metadata["agent_disconnected"].(bool); !got {
		t.Errorf("expected agent_disconnected=true")
	}
}

// TestCanceller_Idempotent_SkipsAnsweredMessages verifies that an already-answered
// message is not clobbered back to expired.
func TestCanceller_Idempotent_SkipsAnsweredMessages(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{}
	c, repo, _ := newTestCanceller(t, msgs)

	pendingID, _ := c.store.CreateRequest(&Request{SessionID: "s1"})
	msgs[pendingID] = []*taskmodels.Message{
		{ID: "m1", TaskSessionID: "s1", Metadata: map[string]any{"status": "answered"}},
		{ID: "m2", TaskSessionID: "s1", Metadata: map[string]any{"status": "pending"}},
	}

	c.CancelSessionAndNotify(context.Background(), "s1")

	if len(repo.updated) != 1 {
		t.Fatalf("expected 1 message update (only the pending one), got %d", len(repo.updated))
	}
	if repo.updated[0].ID != "m2" {
		t.Errorf("expected m2 to be updated, got %s", repo.updated[0].ID)
	}
}
