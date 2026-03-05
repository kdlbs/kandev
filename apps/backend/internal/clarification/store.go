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

// CreateRequest creates a new clarification request and returns its pending ID.
// The request will be stored until a response is received or it times out.
func (s *Store) CreateRequest(req *Request) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.PendingID == "" {
		req.PendingID = uuid.New().String()
	}
	req.CreatedAt = time.Now()

	s.pending[req.PendingID] = &PendingClarification{
		Request:    req,
		ResponseCh: make(chan *Response, 1),
		CancelCh:   make(chan struct{}),
		CreatedAt:  time.Now(),
	}

	return req.PendingID
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
	case resp := <-pending.ResponseCh:
		// Clean up after receiving response
		s.mu.Lock()
		delete(s.pending, pendingID)
		s.mu.Unlock()
		return resp, nil
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

	resp.PendingID = pendingID
	resp.RespondedAt = time.Now()

	// Non-blocking send (channel has buffer of 1)
	select {
	case pending.ResponseCh <- resp:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrAlreadyResponded, pendingID)
	}
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
