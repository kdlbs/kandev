package clarification

import (
	"context"
	"errors"
	"sync"
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

// TestWaitForResponse_Broadcast_MultipleWaiters verifies that close(done) in
// Respond unblocks every parked waiter. The onWaitEntered hook lets the test
// observe each goroutine after it has captured a *PendingClarification pointer
// but before it parks on the select, so Respond can fire only once both
// waiters are guaranteed to see the broadcast. Signalling before the lookup
// (the previous shape) raced with the first-reader-deletes path inside
// WaitForResponse and was flaky under the Linux scheduler in CI.
func TestWaitForResponse_Broadcast_MultipleWaiters(t *testing.T) {
	s := NewStore(time.Minute)
	entered := make(chan string, 2)
	s.onWaitEntered = func(id string) { entered <- id }
	id, _ := s.CreateRequest(&Request{SessionID: "s1", Questions: []Question{{Prompt: "test?", Options: []Option{{ID: "o1", Label: "A"}}}}})

	done := make(chan *Response, 2)
	for i := 0; i < 2; i++ {
		go func() {
			resp, err := s.WaitForResponse(context.Background(), id)
			if err != nil {
				done <- nil
				return
			}
			done <- resp
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case gotID := <-entered:
			if gotID != id {
				t.Fatalf("onWaitEntered id = %q, want %q", gotID, id)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("WaitForResponse did not reach the parked wait state")
		}
	}

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

// TestCancelRequest_AfterRespond_DoesNotCloseCancelCh proves the Review
// Round 3 fix (codex-flagged race): resolver.go's claimAndApply calls
// CancelRequest unconditionally on every cancel outcome (spec X3/X3a), win
// or lose, independently of whether a concurrent winning Respond has
// already recorded the resolution. Before the fix, CancelRequest closed
// CancelCh under only the store-level s.mu, never checking pending.resolved
// under pending.mu the way Respond does -- so a losing cancel arriving
// after Respond had already closed done (but before WaitForResponse's own
// cleanup removed the map entry -- exactly the window X3's "whenever an
// entry exists" targets) could ALSO close CancelCh on the same live entry.
// With both channels closed, WaitForResponse's select picks between them
// non-deterministically, occasionally reporting a spurious "cancelled"
// error to a waiter whose answer had already been delivered successfully.
//
// The fix makes CancelRequest check pending.resolved under pending.mu
// (mirroring the check Respond already performs) and skip closing CancelCh
// once the entry is resolved: closing it serves no purpose once there is no
// wedge left to break (X3a's own rationale), and skipping it is what
// guarantees at most one of done/CancelCh is ever closed per entry.
func TestCancelRequest_AfterRespond_DoesNotCloseCancelCh(t *testing.T) {
	s := NewStore(time.Second)
	id, _ := s.CreateRequest(&Request{
		SessionID: "s1",
		Questions: []Question{{ID: "q1", Prompt: "p?", Options: []Option{{ID: "o1", Label: "A"}}}},
	})
	pending := s.pending[id] // captured before CancelRequest can remove the map entry

	if err := s.Respond(id, &Response{Answers: []Answer{{QuestionID: "q1"}}}); err != nil {
		t.Fatalf("unexpected Respond error: %v", err)
	}

	s.CancelRequest(id)

	select {
	case <-pending.CancelCh:
		t.Fatal("a losing cancel must not close CancelCh once the entry is already resolved: doing so races WaitForResponse's select against the already-closed done channel and can spuriously report cancellation for an answer that was actually delivered")
	default:
	}
	select {
	case <-pending.done:
	default:
		t.Fatal("expected done to remain closed (set by Respond)")
	}
}

// TestRespond_AfterCancelRequest_ReturnsError proves the mirror ordering of
// the same fix: once a cancel has already closed CancelCh (X3, unconditional
// on any cancel outcome), a Respond call racing in afterward must not
// silently record success -- there is no live waiter left to deliver to, so
// treating it as delivered would let the resolver believe resume:"published"
// while the blocked tool call already exited with a cancellation error.
// Respond must instead report failure so the caller falls back to the R7a
// no-live-waiter path.
//
// Review Round 4 found the original, sequential version of this test
// (CancelRequest fully completes, then Respond is called) is phantom-green:
// CancelRequest deletes the map entry from s.pending before Respond's own
// lookup runs, so a purely sequential call always short-circuits on the
// pre-existing ErrNotFound path and never reaches the
// `select { case <-pending.CancelCh: return ErrNotFound }` guard this test
// is named for. Confirmed by mutation testing: deleting that guard entirely
// left the sequential test passing. The guard deliberately reuses
// ErrNotFound rather than a distinct sentinel: to the resolver, "cancel
// closed CancelCh first" and "no entry at all" both mean the durable claim
// has no live in-memory waiter left, so both must fall back to the same
// detached-delivery path.
//
// This version forces the real interleaving instead: two goroutines actually
// racing at the Store level. Respond's own map lookup must complete (finding
// the live entry) before CancelRequest deletes it, but CancelRequest's
// pending.mu section -- which closes CancelCh -- must complete before
// Respond's own pending.mu section runs. The onRespondEntered hook parks
// Respond deterministically between those two points (after the lookup,
// before pending.mu.Lock()) so the ordering is guaranteed rather than left to
// scheduler luck.
func TestRespond_AfterCancelRequest_ReturnsError(t *testing.T) {
	s := NewStore(time.Second)
	id, _ := s.CreateRequest(&Request{
		SessionID: "s1",
		Questions: []Question{{ID: "q1", Prompt: "p?", Options: []Option{{ID: "o1", Label: "A"}}}},
	})
	pending := s.pending[id]

	parked := make(chan struct{})
	release := make(chan struct{})
	s.onRespondEntered = func(string) {
		close(parked)
		<-release
	}

	respondErr := make(chan error, 1)
	go func() {
		respondErr <- s.Respond(id, &Response{Answers: []Answer{{QuestionID: "q1"}}})
	}()
	<-parked // Respond found the live entry; it is parked just before acquiring pending.mu.

	if !s.CancelRequest(id) {
		t.Fatal("expected CancelRequest to report the entry existed and was still unresolved")
	}
	close(release) // Let Respond proceed to pending.mu, now that CancelCh is closed.

	err := <-respondErr
	if err == nil {
		t.Fatal("expected Respond to report an error once a concurrent CancelRequest closed CancelCh first -- silently succeeding would be reported as a delivered answer nobody is waiting for")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	select {
	case <-pending.done:
		t.Fatal("Respond must not close done once a concurrent cancel has already closed CancelCh: there is no live waiter left to deliver to")
	default:
	}
}

// TestCancelSession_ConcurrentWithRespond_DoesNotCloseCancelChOnceDelivered
// proves CancelSession's pending.resolved gate under genuine concurrency,
// independently of TestCancelRequest_AfterRespond_DoesNotCloseCancelCh above.
// CancelSession shares the exact bug shape CancelRequest had before the
// Round 3 fix (it can close CancelCh on an already-resolved entry) and is the
// higher-traffic of the two paths -- it fires on every turn-complete, for
// every pending entry of a session, not just one targeted entry. Review
// Round 4 found it had zero regression coverage: reverting its
// `if !pending.resolved` gate to an unconditional close left the entire
// internal/clarification suite green.
//
// Uses two real goroutines actually racing through the Store -- one running
// Respond, one running CancelSession for the same session -- synchronized via
// the onRespondEntered and onCancelSessionEntered hooks so the interleaving
// is deterministic rather than scheduler luck (an earlier attempt at
// reproducing the sibling CancelRequest race via raw iteration counts did not
// reliably reproduce in this sandbox). Both goroutines are parked
// independently after reaching their own lock-acquisition point; the test
// then releases Respond first and waits for it to fully complete (closing
// done) before releasing CancelSession, forcing CancelSession's pending.mu
// section to observe an already-resolved entry.
func TestCancelSession_ConcurrentWithRespond_DoesNotCloseCancelChOnceDelivered(t *testing.T) {
	s := NewStore(time.Second)
	id, _ := s.CreateRequest(&Request{
		SessionID: "s1",
		Questions: []Question{{ID: "q1", Prompt: "p?", Options: []Option{{ID: "o1", Label: "A"}}}},
	})
	pending := s.pending[id]

	respondParked := make(chan struct{})
	respondRelease := make(chan struct{})
	s.onRespondEntered = func(string) {
		close(respondParked)
		<-respondRelease
	}

	cancelParked := make(chan struct{})
	cancelRelease := make(chan struct{})
	s.onCancelSessionEntered = func(string) {
		close(cancelParked)
		<-cancelRelease
	}

	respondErr := make(chan error, 1)
	go func() {
		respondErr <- s.Respond(id, &Response{Answers: []Answer{{QuestionID: "q1"}}})
	}()
	<-respondParked // Respond found the live entry; parked just before acquiring pending.mu.

	cancelDone := make(chan []string, 1)
	go func() {
		cancelDone <- s.CancelSession("s1")
	}()
	<-cancelParked // CancelSession removed the entry from s.pending; parked just before acquiring pending.mu.

	// Release Respond first and wait for it to fully finish -- pending.resolved
	// is now true and done is closed -- before letting CancelSession proceed.
	close(respondRelease)
	if err := <-respondErr; err != nil {
		t.Fatalf("unexpected Respond error: %v", err)
	}

	close(cancelRelease)
	cancelled := <-cancelDone
	if len(cancelled) != 1 || cancelled[0] != id {
		t.Fatalf("expected CancelSession to report the entry, got %v", cancelled)
	}

	select {
	case <-pending.CancelCh:
		t.Fatal("CancelSession must not close CancelCh once Respond has already resolved and delivered the entry: doing so races WaitForResponse's select against the already-closed done channel and can spuriously report cancellation for an answer that was actually delivered")
	default:
	}
	select {
	case <-pending.done:
	default:
		t.Fatal("expected done to remain closed (set by Respond)")
	}
}

// TestCancelRequest_ConcurrentWithCancelRequest_DoesNotDoubleCloseCancelCh
// covers the double-close race PR review flagged in CancelRequest itself:
// before this fix, CancelRequest only checked pending.resolved before
// closing CancelCh, not pending.cancelled. Two overlapping CancelRequest
// calls for the same pendingID (e.g. a retried client request racing itself)
// would both observe resolved=false and both call close(pending.CancelCh),
// panicking with "close of closed channel" and taking down the process.
//
// Uses the onCancelRequestEntered hook to force the actual race: both
// goroutines must complete their initial (unlocked) pending lookup before
// either acquires pending.mu, so both see a live, uncancelled entry and both
// attempt the guarded section for real -- a purely sequential call would let
// the first CancelRequest delete the map entry before the second's lookup
// runs, short-circuiting on the "not found" path without ever reaching the
// cancelled-guard this test exists to cover.
func TestCancelRequest_ConcurrentWithCancelRequest_DoesNotDoubleCloseCancelCh(t *testing.T) {
	s := NewStore(time.Second)
	id, _ := s.CreateRequest(&Request{
		SessionID: "s1",
		Questions: []Question{{ID: "q1", Prompt: "p?", Options: []Option{{ID: "o1", Label: "A"}}}},
	})
	pending := s.pending[id]

	var (
		mu      sync.Mutex
		parked  int
		release = make(chan struct{})
	)
	allParked := make(chan struct{})
	s.onCancelRequestEntered = func(string) {
		mu.Lock()
		parked++
		n := parked
		mu.Unlock()
		if n == 2 {
			close(allParked)
		}
		<-release
	}

	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		go func() {
			results <- s.CancelRequest(id)
		}()
	}
	<-allParked // Both goroutines found the live entry; both parked before pending.mu.
	close(release)

	// The double close, if the guard were missing, panics inside CancelRequest
	// itself -- the assertion below on trueCount only runs if the process
	// survived, but the race detector and the panic both fire before that.
	first, second := <-results, <-results
	trueCount := 0
	if first {
		trueCount++
	}
	if second {
		trueCount++
	}
	if trueCount != 1 {
		t.Fatalf("expected exactly one of the two concurrent CancelRequest calls to report success, got %d", trueCount)
	}

	select {
	case <-pending.CancelCh:
	default:
		t.Fatal("expected CancelCh to be closed by the winning CancelRequest")
	}
}

// TestCancelSession_ConcurrentWithCancelRequest_DoesNotDoubleCloseCancelCh
// covers the sibling shape of the same race across CancelRequest and
// CancelSession: a client cancelling one specific request while the owning
// session is independently torn down (e.g. task cancellation) races the
// per-entry CancelRequest guard against CancelSession's per-entry loop. Both
// paths must agree on pending.cancelled under the same pending.mu, or both
// can observe resolved=false, cancelled=false and both close(pending.CancelCh).
func TestCancelSession_ConcurrentWithCancelRequest_DoesNotDoubleCloseCancelCh(t *testing.T) {
	s := NewStore(time.Second)
	id, _ := s.CreateRequest(&Request{
		SessionID: "s1",
		Questions: []Question{{ID: "q1", Prompt: "p?", Options: []Option{{ID: "o1", Label: "A"}}}},
	})
	pending := s.pending[id]

	cancelReqParked := make(chan struct{})
	cancelReqRelease := make(chan struct{})
	s.onCancelRequestEntered = func(string) {
		close(cancelReqParked)
		<-cancelReqRelease
	}

	cancelSessParked := make(chan struct{})
	cancelSessRelease := make(chan struct{})
	s.onCancelSessionEntered = func(string) {
		close(cancelSessParked)
		<-cancelSessRelease
	}

	cancelReqResult := make(chan bool, 1)
	go func() {
		cancelReqResult <- s.CancelRequest(id)
	}()
	<-cancelReqParked // CancelRequest found the live entry; parked before pending.mu.

	cancelSessResult := make(chan []string, 1)
	go func() {
		cancelSessResult <- s.CancelSession("s1")
	}()
	<-cancelSessParked // CancelSession removed the entry from s.pending; parked before pending.mu.

	close(cancelReqRelease)
	close(cancelSessRelease)

	// Neither call's return value proves who "won": CancelSession reports
	// every id that was still in s.pending at its own lock-acquisition time
	// unconditionally, independent of whether its pending.mu section actually
	// closed the channel, so both goroutines completing here (rather than one
	// panicking on a double close(pending.CancelCh)) is the property under
	// test -- the race detector and any panic would already have failed the
	// test before reaching these assertions.
	<-cancelReqResult
	sessCancelled := <-cancelSessResult
	if len(sessCancelled) != 1 || sessCancelled[0] != id {
		t.Fatalf("expected CancelSession to report the entry, got %v", sessCancelled)
	}

	select {
	case <-pending.CancelCh:
	default:
		t.Fatal("expected CancelCh to be closed by whichever of CancelRequest/CancelSession won the race")
	}
}
