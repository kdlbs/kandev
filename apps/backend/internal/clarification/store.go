// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors for Respond.
var (
	ErrNotFound         = errors.New("clarification request not found")
	ErrAlreadyResponded = errors.New("response already submitted")
	ErrAlreadyCancelled = errors.New("clarification request already cancelled")
)

// Store manages pending clarification requests.
// It provides thread-safe storage and notification when responses arrive.
type Store struct {
	mu      sync.RWMutex
	pending map[string]*PendingClarification
	timeout time.Duration

	// onWaitEntered, if non-nil, is invoked inside WaitForResponse after the
	// initial pending lookup and before the select blocks. Tests use it to
	// coordinate multi-waiter scenarios deterministically; always nil in
	// production.
	onWaitEntered func(pendingID string)
}

// NewStore creates a new clarification store.
func NewStore(timeout time.Duration) *Store {
	if timeout == 0 {
		timeout = 2 * time.Hour // Default timeout — long enough for user to respond to clarification
	}
	return &Store{
		pending: make(map[string]*PendingClarification),
		timeout: timeout,
	}
}

// SetOnWaitEntered installs a test hook invoked when WaitForResponse starts
// waiting on a pending clarification.
func (s *Store) SetOnWaitEntered(fn func(pendingID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onWaitEntered = fn
}

// CreateRequest creates a new clarification request and returns its pending ID
// plus a boolean indicating whether a new entry was created (true) or an
// existing one was reused (false). If a pending entry for the same session
// with identical normalised questions already exists, the existing pending ID
// is returned and isNew is false.
func (s *Store) CreateRequest(req *Request) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Normalise in-place so dedup keys are stable even when the caller
	// hasn't assigned IDs yet.
	_ = NormalizeAndValidateQuestions(req.Questions)

	// Deduplicate: if a pending entry for the same session with identical
	// normalised questions already exists, return the existing pending ID.
	for _, existing := range s.pending {
		if existing.Request.SessionID == req.SessionID && questionsEqual(existing.Request.Questions, req.Questions) {
			return existing.Request.PendingID, false
		}
	}

	if req.PendingID == "" {
		req.PendingID = uuid.New().String()
	}
	req.CreatedAt = time.Now()

	s.pending[req.PendingID] = &PendingClarification{
		Request:   req,
		done:      make(chan struct{}),
		CancelCh:  make(chan struct{}),
		CreatedAt: time.Now(),
	}

	return req.PendingID, true
}

// GetRequest returns a pending clarification request by ID.
func (s *Store) GetRequest(pendingID string) (*Request, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pending, ok := s.pending[pendingID]
	if !ok {
		return nil, false
	}
	return pending.Request, true
}

// WaitForResponse blocks until a response is received or the context is cancelled.
// Returns the response or an error if cancelled/timed out.
func (s *Store) WaitForResponse(ctx context.Context, pendingID string) (*Response, error) {
	s.mu.RLock()
	pending, ok := s.pending[pendingID]
	hook := s.onWaitEntered
	s.mu.RUnlock()

	if hook != nil {
		hook(pendingID)
	}

	if !ok {
		return nil, fmt.Errorf("clarification request not found: %s", pendingID)
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	select {
	case <-pending.done:
		// Clean up after receiving response
		s.mu.Lock()
		delete(s.pending, pendingID)
		s.mu.Unlock()
		return pending.resp, nil
	case <-pending.CancelCh:
		// Agent's turn completed — cancel the blocking wait
		s.mu.Lock()
		delete(s.pending, pendingID)
		s.mu.Unlock()
		return nil, fmt.Errorf("clarification cancelled (agent moved on): %s", pendingID)
	case <-timeoutCtx.Done():
		if ctx.Err() != nil {
			// Parent context cancelled — do not delete the shared entry
			// because another waiter may still be blocked on it.
			return nil, ctx.Err()
		}
		// Store-level timeout — safe to clean up.
		s.mu.Lock()
		delete(s.pending, pendingID)
		s.mu.Unlock()
		return nil, fmt.Errorf("clarification request timed out: %s", pendingID)
	}
}

// Respond submits a response to a pending clarification request.
// Returns an error if the request is not found.
func (s *Store) Respond(pendingID string, resp *Response) error {
	s.mu.RLock()
	pending, ok := s.pending[pendingID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, pendingID)
	}

	pending.mu.Lock()
	defer pending.mu.Unlock()

	if pending.resolved {
		return fmt.Errorf("%w: %s", ErrAlreadyResponded, pendingID)
	}

	// A concurrent cancel may have already closed CancelCh (and possibly
	// already unblocked WaitForResponse) while this entry was still
	// unresolved. pending.mu makes this check race-free: CancelRequest
	// only closes CancelCh while holding the same lock, so if it got there
	// first, that close is already visible here. There is no live waiter
	// left to deliver to, so this must not report success.
	select {
	case <-pending.CancelCh:
		return fmt.Errorf("%w: %s", ErrAlreadyCancelled, pendingID)
	default:
	}

	resp.PendingID = pendingID
	resp.RespondedAt = time.Now()
	pending.resp = resp
	pending.resolved = true
	close(pending.done)
	return nil
}

// CancelRequest cancels a single pending clarification by id, unblocking any
// WaitForResponse caller that is currently parked on it. Returns true if the
// entry existed (and was removed), false otherwise. Used by callers that need
// to surface a creation-side failure immediately rather than wait for the
// 2-hour MCP timeout.
//
// A losing resolution claim (see clarification.Resolver) calls this
// unconditionally on every cancel outcome, independently of whichever
// concurrent request reaches the entry first. CancelCh is only closed while
// an entry is still unresolved: once Respond has recorded a resolution,
// closing CancelCh would serve no purpose (there is no wedge left to break)
// and would race Respond's own close of done, letting WaitForResponse's
// select nondeterministically report a spurious cancellation for an answer
// that was actually delivered. Gating on pending.resolved under the same
// pending.mu Respond uses is what makes the two mutually exclusive.
func (s *Store) CancelRequest(pendingID string) bool {
	s.mu.Lock()
	pending, ok := s.pending[pendingID]
	if !ok {
		s.mu.Unlock()
		return false
	}
	delete(s.pending, pendingID)
	s.mu.Unlock()

	pending.mu.Lock()
	defer pending.mu.Unlock()
	if !pending.resolved {
		close(pending.CancelCh)
	}
	return true
}

// ListPending returns a snapshot of all pending clarification requests.
// The caller should not modify the returned requests.
func (s *Store) ListPending() []*Request {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Request, 0, len(s.pending))
	for _, p := range s.pending {
		out = append(out, p.Request)
	}
	return out
}

// CancelSession cancels all pending clarification requests for a given session.
// It closes the CancelCh to unblock any WaitForResponse callers and removes entries.
// Returns the list of cancelled pending IDs. Same race protection as
// CancelRequest: CancelCh is only closed for an entry that is still
// unresolved, checked under that entry's own pending.mu.
func (s *Store) CancelSession(sessionID string) []string {
	s.mu.Lock()
	var toCancel []*PendingClarification
	var cancelled []string
	for id, pending := range s.pending {
		if pending.Request.SessionID == sessionID {
			toCancel = append(toCancel, pending)
			delete(s.pending, id)
			cancelled = append(cancelled, id)
		}
	}
	s.mu.Unlock()

	for _, pending := range toCancel {
		pending.mu.Lock()
		if !pending.resolved {
			close(pending.CancelCh)
		}
		pending.mu.Unlock()
	}
	return cancelled
}

func questionsEqual(a, b []Question) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Prompt != b[i].Prompt {
			return false
		}
		if !optionsEqual(a[i].Options, b[i].Options) {
			return false
		}
	}
	return true
}

func optionsEqual(a, b []Option) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Label != b[i].Label || a[i].Description != b[i].Description {
			return false
		}
	}
	return true
}
