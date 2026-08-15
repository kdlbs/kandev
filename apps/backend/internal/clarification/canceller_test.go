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
	messages          map[string][]*taskmodels.Message
	activeBySession   map[string][]*taskmodels.Message
	activeErr         error
	updated           []*taskmodels.Message
	activeHasDeadline bool
	activeContextErr  error
	detachHasDeadline bool
	detachContextErr  error
	expireHasDeadline bool
	expireContextErr  error
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

func (s *stubMessageStore) FindActiveClarificationMessagesBySessionID(ctx context.Context, sessionID string) ([]*taskmodels.Message, error) {
	_, s.activeHasDeadline = ctx.Deadline()
	s.activeContextErr = ctx.Err()
	if s.activeErr != nil {
		return nil, s.activeErr
	}
	if s.activeBySession != nil {
		return s.activeBySession[sessionID], nil
	}
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

func (s *stubMessageStore) DetachActiveClarificationMessagesBySessionID(
	ctx context.Context,
	sessionID string,
) ([]*taskmodels.Message, error) {
	_, s.detachHasDeadline = ctx.Deadline()
	s.detachContextErr = ctx.Err()
	active, err := s.FindActiveClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var changed []*taskmodels.Message
	for _, message := range active {
		if detached, _ := message.Metadata["agent_disconnected"].(bool); detached {
			continue
		}
		message.Metadata["agent_disconnected"] = true
		s.updated = append(s.updated, message)
		changed = append(changed, message)
	}
	return changed, nil
}

func (s *stubMessageStore) ExpireActiveClarificationBundle(
	ctx context.Context,
	sessionID, pendingID string,
) ([]*taskmodels.Message, error) {
	_, s.expireHasDeadline = ctx.Deadline()
	s.expireContextErr = ctx.Err()
	active, err := s.FindActiveClarificationMessagesBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	changed := make([]*taskmodels.Message, 0, len(active))
	for _, message := range active {
		if stringFromMetadata(message.Metadata, "pending_id") != pendingID {
			continue
		}
		message.Metadata["agent_disconnected"] = true
		message.Metadata["status"] = "expired"
		s.updated = append(s.updated, message)
		changed = append(changed, message)
	}
	return changed, nil
}

func (s *stubMessageStore) UpdateMessage(_ context.Context, m *taskmodels.Message) error {
	s.updated = append(s.updated, m)
	return nil
}

type stubEventBus struct {
	events            []*bus.Event
	publishErr        error
	contextErrs       []error
	resumeRequests    []DetachedClarificationResume
	resumeErr         error
	resumeContextErrs []error
}

func (s *stubEventBus) Publish(ctx context.Context, _ string, ev *bus.Event) error {
	s.contextErrs = append(s.contextErrs, ctx.Err())
	if s.publishErr != nil {
		return s.publishErr
	}
	s.events = append(s.events, ev)
	return nil
}

func (s *stubEventBus) ResumeDetachedClarification(
	ctx context.Context,
	request DetachedClarificationResume,
) error {
	s.resumeContextErrs = append(s.resumeContextErrs, ctx.Err())
	s.resumeRequests = append(s.resumeRequests, request)
	return s.resumeErr
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
			"status":     "pending",
			"pending_id": pendingID,
		},
	}}

	cancelled := c.DetachSessionAndNotify(context.Background(), "s1")
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
			"status":     "pending",
			"pending_id": pendingID,
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

func TestCanceller_PersistenceUsesFreshBoundedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("detach", func(t *testing.T) {
		c, repo, _ := newTestCanceller(t, map[string][]*taskmodels.Message{})
		c.DetachSessionAndNotify(ctx, "s1")
		if !repo.detachHasDeadline || repo.detachContextErr != nil {
			t.Fatalf("detach context deadline=%v err=%v, want fresh bounded context",
				repo.detachHasDeadline, repo.detachContextErr)
		}
	})

	t.Run("expiry", func(t *testing.T) {
		message := &taskmodels.Message{
			ID: "m1", TaskSessionID: "s1",
			Metadata: map[string]any{"status": "pending", "pending_id": "pending-1"},
		}
		c, repo, _ := newTestCanceller(t, map[string][]*taskmodels.Message{
			"pending-1": {message},
		})
		c.ExpireSessionAndNotify(ctx, "s1")
		if !repo.activeHasDeadline || repo.activeContextErr != nil {
			t.Fatalf("expiry lookup context deadline=%v err=%v, want fresh bounded context",
				repo.activeHasDeadline, repo.activeContextErr)
		}
		if !repo.expireHasDeadline || repo.expireContextErr != nil {
			t.Fatalf("expiry update context deadline=%v err=%v, want fresh bounded context",
				repo.expireHasDeadline, repo.expireContextErr)
		}
	})
}

// TestCanceller_NoMessagesToUpdate confirms that a cancel with no pending
// clarifications returns 0 and does not touch the repo.
func TestCanceller_NoMessagesToUpdate(t *testing.T) {
	c, repo, _ := newTestCanceller(t, map[string][]*taskmodels.Message{})

	if got := c.DetachSessionAndNotify(context.Background(), "nonexistent"); got != 0 {
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
	updatedAt := time.Date(2026, time.August, 2, 20, 0, 0, 123456789, time.UTC)

	pendingID, _ := c.store.CreateRequest(&Request{SessionID: "s1"})
	msgs[pendingID] = []*taskmodels.Message{{
		ID:            "m1",
		TaskSessionID: "s1",
		Metadata:      map[string]any{"status": "pending", "pending_id": pendingID},
		UpdatedAt:     updatedAt,
	}}

	c.DetachSessionAndNotify(context.Background(), "s1")

	if len(eventBus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventBus.events))
	}
	data, ok := eventBus.events[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map event data, got %T", eventBus.events[0].Data)
	}
	if got := data["updated_at"]; got != updatedAt.Format(time.RFC3339Nano) {
		t.Errorf("expected updated_at %q, got %#v", updatedAt.Format(time.RFC3339Nano), got)
	}
}

// TestCanceller_MultiQuestion_MarksAllMessagesDetached confirms a multi-question
// bundle has every message marked agent_disconnected while staying pending.
func TestCanceller_MultiQuestion_MarksAllMessagesDetached(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{}
	c, _, eventBus := newTestCanceller(t, msgs)

	pendingID, _ := c.store.CreateRequest(&Request{SessionID: "s1"})
	msgs[pendingID] = []*taskmodels.Message{
		{ID: "m1", TaskSessionID: "s1", Metadata: map[string]any{"status": "pending", "pending_id": pendingID, "question_id": "q1"}},
		{ID: "m2", TaskSessionID: "s1", Metadata: map[string]any{"status": "pending", "pending_id": pendingID, "question_id": "q2"}},
		{ID: "m3", TaskSessionID: "s1", Metadata: map[string]any{"status": "pending", "pending_id": pendingID, "question_id": "q3"}},
	}

	cancelled := c.DetachSessionAndNotify(context.Background(), "s1")
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

func TestCanceller_SupersededStoreBundleDoesNotCount(t *testing.T) {
	pendingID := "superseded-store-bundle"
	msgs := map[string][]*taskmodels.Message{
		pendingID: {{
			ID:            "message-superseded-store",
			TaskSessionID: "s1",
			Metadata: map[string]any{
				"status":     "pending",
				"pending_id": pendingID,
			},
		}},
	}
	c, repo, eventBus := newTestCanceller(t, msgs)
	repo.activeBySession = map[string][]*taskmodels.Message{"s1": {}}
	if _, created := c.store.CreateRequest(&Request{
		PendingID: pendingID,
		SessionID: "s1",
	}); !created {
		t.Fatal("expected superseded in-memory request to be created")
	}

	if got := c.DetachSessionAndNotify(context.Background(), "s1"); got != 0 {
		t.Fatalf("superseded store bundle count = %d, want 0", got)
	}
	if _, exists := c.store.GetRequest(pendingID); exists {
		t.Fatal("superseded in-memory waiter was not drained")
	}
	if len(repo.updated) != 0 || len(eventBus.events) != 0 {
		t.Fatalf("superseded store bundle produced writes=%d events=%d", len(repo.updated), len(eventBus.events))
	}
}

func TestCanceller_RepeatedDetachIsNoOp(t *testing.T) {
	pendingID := "already-detached"
	msgs := map[string][]*taskmodels.Message{
		pendingID: {{
			ID:            "m-detached",
			TaskSessionID: "s1",
			Metadata: map[string]any{
				"status":             "pending",
				"pending_id":         pendingID,
				"agent_disconnected": true,
			},
		}},
	}
	c, repo, eventBus := newTestCanceller(t, msgs)
	if _, created := c.store.CreateRequest(&Request{
		PendingID: pendingID,
		SessionID: "s1",
	}); !created {
		t.Fatal("expected in-memory request to be created")
	}

	if got := c.DetachSessionAndNotify(context.Background(), "s1"); got != 0 {
		t.Fatalf("repeated detach count = %d, want 0", got)
	}
	if len(repo.updated) != 0 {
		t.Fatalf("repeated detach wrote %d messages", len(repo.updated))
	}
	if len(eventBus.events) != 0 {
		t.Fatalf("repeated detach published %d events", len(eventBus.events))
	}
}

func TestCanceller_RepeatedExpiryIsNoOp(t *testing.T) {
	pendingID := "already-expired"
	msgs := map[string][]*taskmodels.Message{
		pendingID: {{
			ID:            "m-expired",
			TaskSessionID: "s1",
			Metadata: map[string]any{
				"status":     "expired",
				"pending_id": pendingID,
			},
		}},
	}
	c, repo, eventBus := newTestCanceller(t, msgs)
	if _, created := c.store.CreateRequest(&Request{
		PendingID: pendingID,
		SessionID: "s1",
	}); !created {
		t.Fatal("expected in-memory request to be created")
	}

	if got := c.ExpireSessionAndNotify(context.Background(), "s1"); got != 0 {
		t.Fatalf("repeated expiry count = %d, want 0", got)
	}
	if len(repo.updated) != 0 {
		t.Fatalf("repeated expiry wrote %d messages", len(repo.updated))
	}
	if len(eventBus.events) != 0 {
		t.Fatalf("repeated expiry published %d events", len(eventBus.events))
	}
}

// TestCanceller_Idempotent_SkipsAnsweredMessages verifies that an already-answered
// message is not clobbered back to expired.
func TestCanceller_Idempotent_SkipsAnsweredMessages(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{}
	c, repo, _ := newTestCanceller(t, msgs)

	pendingID, _ := c.store.CreateRequest(&Request{SessionID: "s1"})
	msgs[pendingID] = []*taskmodels.Message{
		{ID: "m1", TaskSessionID: "s1", Metadata: map[string]any{"status": "answered", "pending_id": pendingID}},
		{ID: "m2", TaskSessionID: "s1", Metadata: map[string]any{"status": "pending", "pending_id": pendingID}},
	}

	c.DetachSessionAndNotify(context.Background(), "s1")

	if len(repo.updated) != 1 {
		t.Fatalf("expected 1 message update (only the pending one), got %d", len(repo.updated))
	}
	if repo.updated[0].ID != "m2" {
		t.Errorf("expected m2 to be updated, got %s", repo.updated[0].ID)
	}
}
