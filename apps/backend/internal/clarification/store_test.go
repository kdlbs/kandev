package clarification

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/shared"
)

func TestNewStore_DefaultTimeout(t *testing.T) {
	s := NewStore(0)
	if s.timeout != 2*time.Hour {
		t.Errorf("expected default timeout 2h, got %v", s.timeout)
	}
}

func TestNewStore_CustomTimeout(t *testing.T) {
	s := NewStore(5 * time.Minute)
	if s.timeout != 5*time.Minute {
		t.Errorf("expected timeout 5m, got %v", s.timeout)
	}
}

func TestCreateRequest_GeneratesID(t *testing.T) {
	s := NewStore(time.Minute)
	req := &Request{SessionID: "s1", Questions: []Question{{Prompt: "test?"}}}

	id, _ := s.CreateRequest(req)

	if id == "" {
		t.Fatal("expected non-empty pending ID")
	}
	if req.PendingID != id {
		t.Errorf("expected request PendingID to be set to %q, got %q", id, req.PendingID)
	}
}

func TestCreateRequest_PreservesExistingID(t *testing.T) {
	s := NewStore(time.Minute)
	req := &Request{PendingID: "custom-id", SessionID: "s1"}

	id, _ := s.CreateRequest(req)

	if id != "custom-id" {
		t.Errorf("expected preserved ID %q, got %q", "custom-id", id)
	}
}

func TestGetRequest_Found(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1", Questions: []Question{{Prompt: "test?"}}})

	req, ok := s.GetRequest(id)

	if !ok {
		t.Fatal("expected request to be found")
	}
	if req.SessionID != "s1" {
		t.Errorf("expected session ID %q, got %q", "s1", req.SessionID)
	}
}

func TestGetRequest_NotFound(t *testing.T) {
	s := NewStore(time.Minute)

	_, ok := s.GetRequest("nonexistent")

	if ok {
		t.Fatal("expected request not to be found")
	}
}

func TestWaitForResponse_Success(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})

	// Respond in a goroutine
	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = s.Respond(id, &Response{Answers: []Answer{{CustomText: "hello"}}})
	}()

	resp, err := s.WaitForResponse(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Answers) != 1 || resp.Answers[0].CustomText != "hello" {
		t.Errorf("unexpected response: %+v", resp)
	}

	// Entry should be cleaned up
	if _, ok := s.GetRequest(id); ok {
		t.Error("expected entry to be cleaned up after response")
	}
}

func TestWaitForResponse_NotFound(t *testing.T) {
	s := NewStore(time.Minute)

	_, err := s.WaitForResponse(context.Background(), "nonexistent")

	if err == nil {
		t.Fatal("expected error for nonexistent request")
	}
}

func TestWaitForResponse_ContextCancelled(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := s.WaitForResponse(ctx, id)
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
}

func TestWaitForResponse_CancelCh(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})

	// Cancel via CancelSession in a goroutine
	go func() {
		time.Sleep(10 * time.Millisecond)
		s.CancelSession("s1")
	}()

	_, err := s.WaitForResponse(context.Background(), id)
	if err == nil {
		t.Fatal("expected error on cancel")
	}
	if _, ok := s.GetRequest(id); ok {
		t.Error("expected entry to be cleaned up after cancel")
	}
}

func TestWaitForResponse_StoreTimeout(t *testing.T) {
	s := NewStore(50 * time.Millisecond)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})

	_, err := s.WaitForResponse(context.Background(), id)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if _, ok := s.GetRequest(id); ok {
		t.Error("expected entry to be cleaned up after timeout")
	}
}

func TestRespond_Success(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})

	err := s.Respond(id, &Response{Answers: []Answer{{CustomText: "yes"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRespond_NotFound(t *testing.T) {
	s := NewStore(time.Minute)

	err := s.Respond("nonexistent", &Response{})
	if err == nil {
		t.Fatal("expected error for nonexistent request")
	}
}

func TestRespond_Duplicate(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})

	// First respond succeeds
	if err := s.Respond(id, &Response{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Second respond fails (buffer full)
	if err := s.Respond(id, &Response{}); err == nil {
		t.Fatal("expected error for duplicate response")
	}
}

func TestCancelSession_CancelsMatchingRequests(t *testing.T) {
	s := NewStore(time.Minute)
	id1, _ := s.CreateRequest(&Request{SessionID: "s1", Questions: []Question{{Prompt: "q1?", Options: []Option{{ID: "o1", Label: "A"}}}}})
	id2, _ := s.CreateRequest(&Request{SessionID: "s1", Questions: []Question{{Prompt: "q2?", Options: []Option{{ID: "o1", Label: "B"}}}}})
	id3, _ := s.CreateRequest(&Request{SessionID: "s2", Questions: []Question{{Prompt: "q3?", Options: []Option{{ID: "o1", Label: "C"}}}}})

	cancelled := s.CancelSession("s1")

	if len(cancelled) != 2 {
		t.Fatalf("expected 2 cancelled, got %d", len(cancelled))
	}

	// s1 entries should be gone
	if _, ok := s.GetRequest(id1); ok {
		t.Error("expected id1 to be removed")
	}
	if _, ok := s.GetRequest(id2); ok {
		t.Error("expected id2 to be removed")
	}
	// s2 entry should remain
	if _, ok := s.GetRequest(id3); !ok {
		t.Error("expected id3 to remain")
	}
}

// TestCancelRequest unblocks WaitForResponse for a single pending entry.
// Used by the create-message-failure recovery path so the agent doesn't
// have to wait for the full MCP timeout when the bundle could not be
// persisted.
//
// Synchronisation: the goroutine signals it has started before invoking
// WaitForResponse so we don't rely on a time.Sleep. CancelRequest may run
// either before or after the goroutine reads from the pending map, and both
// paths return an error from WaitForResponse — that's the contract under test.
func TestCancelRequest(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := s.WaitForResponse(context.Background(), id)
		done <- err
	}()
	<-started

	if !s.CancelRequest(id) {
		t.Fatalf("CancelRequest returned false for known id")
	}
	if s.CancelRequest(id) {
		t.Errorf("CancelRequest should return false the second time")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Errorf("expected error from cancelled WaitForResponse")
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForResponse did not return after CancelRequest")
	}
}

func TestCancelSession_NoMatch(t *testing.T) {
	s := NewStore(time.Minute)
	_, _ = s.CreateRequest(&Request{SessionID: "s1"})

	cancelled := s.CancelSession("other")

	if len(cancelled) != 0 {
		t.Errorf("expected 0 cancelled, got %d", len(cancelled))
	}
}

func TestListPendingPermissions_Empty(t *testing.T) {
	s := NewStore(time.Minute)
	perms := s.ListPendingPermissions()
	if perms == nil {
		t.Error("expected non-nil slice from ListPendingPermissions")
	}
	if len(perms) != 0 {
		t.Errorf("expected 0 pending permissions, got %d", len(perms))
	}
}

func TestListPendingPermissions_ReturnsPendingRequests(t *testing.T) {
	s := NewStore(time.Minute)

	_, _ = s.CreateRequest(&Request{
		SessionID: "session-1",
		TaskID:    "task-1",
		Questions: []Question{{Prompt: "Allow bash execution?"}},
		Context:   "tool permission",
	})
	_, _ = s.CreateRequest(&Request{
		SessionID: "session-2",
		TaskID:    "task-2",
		Questions: []Question{{Prompt: "Write to /tmp?"}},
	})

	perms := s.ListPendingPermissions()

	if len(perms) != 2 {
		t.Fatalf("expected 2 pending permissions, got %d", len(perms))
	}

	bySession := make(map[string]shared.PendingPermission)
	for _, p := range perms {
		bySession[p.SessionID] = p
	}

	p1, ok := bySession["session-1"]
	if !ok {
		t.Fatal("expected permission for session-1")
	}
	if p1.TaskID != "task-1" {
		t.Errorf("task_id = %q, want task-1", p1.TaskID)
	}
	if p1.Prompt != "Allow bash execution?" {
		t.Errorf("prompt = %q, want 'Allow bash execution?'", p1.Prompt)
	}
	if p1.Context != "tool permission" {
		t.Errorf("context = %q, want 'tool permission'", p1.Context)
	}
	if p1.PendingID == "" {
		t.Error("expected non-empty pending_id")
	}
	if p1.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestListPendingPermissions_ExcludesCancelled(t *testing.T) {
	s := NewStore(time.Minute)

	_, _ = s.CreateRequest(&Request{SessionID: "s1"})
	_, _ = s.CreateRequest(&Request{SessionID: "s2"})

	// Cancel session s1
	s.CancelSession("s1")

	perms := s.ListPendingPermissions()

	if len(perms) != 1 {
		t.Fatalf("expected 1 pending permission after cancel, got %d", len(perms))
	}
	if perms[0].SessionID != "s2" {
		t.Errorf("expected session-2 to remain, got %q", perms[0].SessionID)
	}
}

func TestListPendingPermissions_ImplementsInterface(t *testing.T) {
	s := NewStore(time.Minute)
	// Verify Store satisfies shared.PermissionLister at compile time.
	var _ interface {
		ListPendingPermissions() []shared.PendingPermission
	} = s
}

func TestCreateRequest_Dedup_SameSessionAndQuestions(t *testing.T) {
	s := NewStore(time.Minute)
	q := []Question{{Prompt: "What colour?", Options: []Option{{ID: "o1", Label: "Red"}, {ID: "o2", Label: "Blue"}}}}

	id1, _ := s.CreateRequest(&Request{SessionID: "s1", Questions: q})
	id2, _ := s.CreateRequest(&Request{SessionID: "s1", Questions: q})

	if id1 != id2 {
		t.Fatalf("expected duplicate request to reuse pending ID %q, got %q", id1, id2)
	}

	pending := s.ListPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request after dedup, got %d", len(pending))
	}
}

func TestCreateRequest_NoDedup_DifferentQuestions(t *testing.T) {
	s := NewStore(time.Minute)
	id1, _ := s.CreateRequest(&Request{SessionID: "s1", Questions: []Question{{Prompt: "Q1?", Options: []Option{{ID: "o1", Label: "A"}}}}})
	id2, _ := s.CreateRequest(&Request{SessionID: "s1", Questions: []Question{{Prompt: "Q2?", Options: []Option{{ID: "o1", Label: "A"}}}}})

	if id1 == id2 {
		t.Fatal("expected different pending IDs for different questions")
	}
}

func TestWaitForResponse_Broadcast_MultipleWaiters(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1", Questions: []Question{{Prompt: "test?", Options: []Option{{ID: "o1", Label: "A"}}}}})

	started := make(chan struct{}, 2)
	done := make(chan *Response, 2)
	for i := 0; i < 2; i++ {
		go func() {
			started <- struct{}{}
			resp, err := s.WaitForResponse(context.Background(), id)
			if err != nil {
				done <- nil
				return
			}
			done <- resp
		}()
	}
	<-started
	<-started

	if err := s.Respond(id, &Response{Answers: []Answer{{CustomText: "hello"}}}); err != nil {
		t.Fatalf("unexpected respond error: %v", err)
	}

	var got int
	for i := 0; i < 2; i++ {
		select {
		case resp := <-done:
			if resp != nil && len(resp.Answers) == 1 && resp.Answers[0].CustomText == "hello" {
				got++
			}
		case <-time.After(time.Second):
			t.Fatal("WaitForResponse did not return")
		}
	}
	if got != 2 {
		t.Fatalf("expected both waiters to receive response, got %d", got)
	}
}
