package clarification

import (
	"context"
	"testing"
	"time"
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
	req := &Request{SessionID: "s1", Question: Question{Prompt: "test?"}}

	id := s.CreateRequest(req)

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

	id := s.CreateRequest(req)

	if id != "custom-id" {
		t.Errorf("expected preserved ID %q, got %q", "custom-id", id)
	}
}

func TestGetRequest_Found(t *testing.T) {
	s := NewStore(time.Minute)
	id := s.CreateRequest(&Request{SessionID: "s1", Question: Question{Prompt: "test?"}})

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
	id := s.CreateRequest(&Request{SessionID: "s1"})

	// Respond in a goroutine
	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = s.Respond(id, &Response{Answer: &Answer{CustomText: "hello"}})
	}()

	resp, err := s.WaitForResponse(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Answer == nil || resp.Answer.CustomText != "hello" {
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
	id := s.CreateRequest(&Request{SessionID: "s1"})

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
	id := s.CreateRequest(&Request{SessionID: "s1"})

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
	id := s.CreateRequest(&Request{SessionID: "s1"})

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
	id := s.CreateRequest(&Request{SessionID: "s1"})

	err := s.Respond(id, &Response{Answer: &Answer{CustomText: "yes"}})
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
	id := s.CreateRequest(&Request{SessionID: "s1"})

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
	id1 := s.CreateRequest(&Request{SessionID: "s1"})
	id2 := s.CreateRequest(&Request{SessionID: "s1"})
	id3 := s.CreateRequest(&Request{SessionID: "s2"})

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

func TestCancelSession_NoMatch(t *testing.T) {
	s := NewStore(time.Minute)
	s.CreateRequest(&Request{SessionID: "s1"})

	cancelled := s.CancelSession("other")

	if len(cancelled) != 0 {
		t.Errorf("expected 0 cancelled, got %d", len(cancelled))
	}
}
