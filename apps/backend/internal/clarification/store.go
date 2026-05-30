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
)

// Store manages pending clarification requests.
// It provides thread-safe storage and notification when responses arrive.
type Store struct {
	mu      sync.RWMutex
	pending map[string]*PendingClarification
	timeout time.Duration
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
	s.mu.RUnlock()

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
		// Clean up on timeout
		s.mu.Lock()
		delete(s.pending, pendingID)
		s.mu.Unlock()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
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
func (s *Store) CancelRequest(pendingID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pending[pendingID]
	if !ok {
		return false
	}
	close(pending.CancelCh)
	delete(s.pending, pendingID)
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
// Returns the list of cancelled pending IDs.
func (s *Store) CancelSession(sessionID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var cancelled []string
	for id, pending := range s.pending {
		if pending.Request.SessionID == sessionID {
			close(pending.CancelCh)
			delete(s.pending, id)
			cancelled = append(cancelled, id)
		}
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
