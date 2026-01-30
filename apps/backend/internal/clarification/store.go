// Package clarification provides types and services for agent clarification requests.
package clarification

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store manages pending clarification requests.
// It provides thread-safe storage and notification when responses arrive.
type Store struct {
	mu       sync.RWMutex
	pending  map[string]*PendingClarification
	timeout  time.Duration
}

// NewStore creates a new clarification store.
func NewStore(timeout time.Duration) *Store {
	if timeout == 0 {
		timeout = 10 * time.Minute // Default timeout
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
		return fmt.Errorf("clarification request not found: %s", pendingID)
	}

	resp.PendingID = pendingID
	resp.RespondedAt = time.Now()

	// Non-blocking send (channel has buffer of 1)
	select {
	case pending.ResponseCh <- resp:
		return nil
	default:
		return fmt.Errorf("response already submitted for: %s", pendingID)
	}
}

// Cancel cancels a pending clarification request.
func (s *Store) Cancel(pendingID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending, ok := s.pending[pendingID]
	if !ok {
		return fmt.Errorf("clarification request not found: %s", pendingID)
	}

	// Send a cancelled response
	select {
	case pending.ResponseCh <- &Response{
		PendingID:   pendingID,
		Rejected:    true,
		RejectReason: "cancelled",
		RespondedAt: time.Now(),
	}:
	default:
	}

	delete(s.pending, pendingID)
	return nil
}

// CleanupExpired removes expired pending requests.
// Returns the number of requests cleaned up.
func (s *Store) CleanupExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	now := time.Now()
	for id, pending := range s.pending {
		if now.Sub(pending.CreatedAt) > s.timeout {
			// Send timeout response
			select {
			case pending.ResponseCh <- &Response{
				PendingID:    id,
				Rejected:     true,
				RejectReason: "expired",
				RespondedAt:  now,
			}:
			default:
			}
			delete(s.pending, id)
			count++
		}
	}
	return count
}

// ListPending returns all pending clarification requests.
func (s *Store) ListPending() []*Request {
	s.mu.RLock()
	defer s.mu.RUnlock()

	requests := make([]*Request, 0, len(s.pending))
	for _, pending := range s.pending {
		requests = append(requests, pending.Request)
	}
	return requests
}

