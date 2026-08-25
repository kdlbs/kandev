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

const maxStartedDeliveryConfirmationWait = 5 * time.Minute

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

	// onRespondEntered, if non-nil, is invoked inside Respond after the
	// initial pending lookup and before pending.mu is acquired. Tests use it
	// to force a specific interleaving against a concurrent CancelRequest or
	// CancelSession; always nil in production.
	onRespondEntered func(pendingID string)

	// onCancelSessionEntered, if non-nil, is invoked inside CancelSession for
	// each matching entry, after it has been removed from s.pending and
	// before that entry's pending.mu is acquired. Tests use it to force a
	// specific interleaving against a concurrent Respond; always nil in
	// production.
	onCancelSessionEntered func(pendingID string)

	// onCancelRequestEntered, if non-nil, is invoked inside CancelRequest
	// after the initial pending lookup and before pending.mu is acquired.
	// Tests use it to force a specific interleaving against a concurrent
	// CancelRequest/CancelSession on the same entry; always nil in
	// production.
	onCancelRequestEntered func(pendingID string)

	// onRespondLoaded coordinates cancellation-after-lookup tests. It is always
	// nil in production.
	onRespondLoaded func(pendingID string)
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

// SetOnRespondLoaded installs a test hook invoked after Respond loads a
// pending clarification and before it claims the entry.
func (s *Store) SetOnRespondLoaded(fn func(pendingID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onRespondLoaded = fn
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
		return s.consumeResponse(pendingID, pending)
	case <-pending.CancelCh:
		// Agent's turn completed — cancel the blocking wait
		s.deletePendingIfCurrent(pendingID, pending)
		return nil, fmt.Errorf("clarification cancelled (agent moved on): %s", pendingID)
	case <-timeoutCtx.Done():
		if ctx.Err() != nil {
			// Parent context cancelled — do not delete the shared entry
			// because another waiter may still be blocked on it.
			return nil, ctx.Err()
		}
		// Store-level timeout — cancel the shared entry unless a response won
		// the race and already woke this waiter.
		if !s.cancelPendingIfCurrent(pendingID, pending) {
			select {
			case <-pending.done:
				return s.consumeResponse(pendingID, pending)
			default:
			}
		}
		return nil, fmt.Errorf("clarification request timed out: %s", pendingID)
	}
}

func (s *Store) consumeResponse(
	pendingID string,
	pending *PendingClarification,
) (*Response, error) {
	pending.mu.Lock()
	confirm := pending.deliveryConfirmation
	abandoned := pending.deliveryAbandoned
	pending.mu.Unlock()
	if abandoned {
		s.deletePendingIfCurrent(pendingID, pending)
		return nil, errors.New("clarification response delivery was abandoned")
	}
	if confirm != nil {
		pending.deliveryConfirmationOnce.Do(func() {
			pending.mu.Lock()
			if pending.deliveryAbandoned {
				pending.deliveryConfirmationErr = errors.New("clarification response delivery was abandoned")
				pending.deliveryConfirmationComplete = true
				confirmationDone := pending.deliveryConfirmationDone
				pending.mu.Unlock()
				close(confirmationDone)
				return
			}
			pending.deliveryConfirmationStarted = true
			pending.mu.Unlock()
			err := confirm()
			pending.mu.Lock()
			pending.deliveryConfirmationErr = err
			pending.deliveryConfirmationComplete = true
			confirmationDone := pending.deliveryConfirmationDone
			pending.mu.Unlock()
			close(confirmationDone)
		})
		pending.mu.Lock()
		confirmationErr := pending.deliveryConfirmationErr
		pending.mu.Unlock()
		if confirmationErr != nil {
			s.deletePendingIfCurrent(pendingID, pending)
			return nil, fmt.Errorf("confirm clarification response delivery: %w", confirmationErr)
		}
	}
	pending.mu.Lock()
	resp := pending.resp
	pending.mu.Unlock()
	s.deletePendingIfCurrent(pendingID, pending)
	return resp, nil
}

func (s *Store) deletePendingIfCurrent(pendingID string, pending *PendingClarification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending[pendingID] == pending {
		delete(s.pending, pendingID)
	}
}

// Respond submits a response to a pending clarification request.
// Returns an error if the request is not found.
func (s *Store) Respond(pendingID string, resp *Response) error {
	return s.respond(context.Background(), pendingID, resp, nil)
}

// RespondWithDeliveryConfirmation submits a response and waits for its live
// waiter to durably confirm delivery before either side returns success.
func (s *Store) RespondWithDeliveryConfirmation(
	ctx context.Context,
	pendingID string,
	resp *Response,
	confirm func() error,
) error {
	if ctx == nil {
		return errors.New("clarification delivery confirmation context is required")
	}
	if confirm == nil {
		return errors.New("clarification delivery confirmation is required")
	}
	return s.respond(ctx, pendingID, resp, confirm)
}

func (s *Store) respond(
	ctx context.Context,
	pendingID string,
	resp *Response,
	confirm func() error,
) error {
	s.mu.RLock()
	pending, ok := s.pending[pendingID]
	hook := s.onRespondEntered
	loadedHook := s.onRespondLoaded
	s.mu.RUnlock()
	if hook != nil {
		hook(pendingID)
	}
	if loadedHook != nil {
		loadedHook(pendingID)
	}

	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, pendingID)
	}

	pending.mu.Lock()

	if pending.cancelled {
		pending.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNotFound, pendingID)
	}
	if pending.resolved {
		pending.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrAlreadyResponded, pendingID)
	}

	// A concurrent cancel may have already closed CancelCh (and possibly
	// already unblocked WaitForResponse) while this entry was still
	// unresolved. pending.mu makes this check race-free: CancelRequest
	// only closes CancelCh while holding the same lock, so if it got there
	// first, that close is already visible here. There is no live waiter
	// left to deliver to, so this must not report success. Report the same
	// ErrNotFound a fresh lookup would have hit had CancelRequest won a
	// touch earlier: the caller's dichotomy (ErrNotFound falls back to
	// detached delivery; anything else is a hard failure) is correct here
	// too, since the durable claim already succeeded and there is simply no
	// live in-memory waiter left to hand it to.
	select {
	case <-pending.CancelCh:
		pending.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNotFound, pendingID)
	default:
	}

	resp.PendingID = pendingID
	resp.RespondedAt = time.Now()
	pending.resp = resp
	pending.deliveryConfirmation = confirm
	if confirm != nil {
		pending.deliveryConfirmationDone = make(chan struct{})
	}
	pending.resolved = true
	close(pending.done)
	confirmationDone := pending.deliveryConfirmationDone
	pending.mu.Unlock()
	if confirmationDone == nil {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	select {
	case <-confirmationDone:
		return clarificationDeliveryConfirmationResult(pending)
	case <-waitCtx.Done():
		pending.mu.Lock()
		if pending.deliveryConfirmationComplete {
			confirmationErr := pending.deliveryConfirmationErr
			pending.mu.Unlock()
			return confirmationErr
		}
		if pending.deliveryConfirmationStarted {
			pending.mu.Unlock()
			return s.waitForStartedDeliveryConfirmation(ctx, pending, confirmationDone)
		}
		pending.deliveryAbandoned = true
		pending.mu.Unlock()
		s.deletePendingIfCurrent(pendingID, pending)
		return fmt.Errorf("wait for clarification delivery confirmation: %w", waitCtx.Err())
	}
}

func (s *Store) waitForStartedDeliveryConfirmation(
	ctx context.Context,
	pending *PendingClarification,
	confirmationDone <-chan struct{},
) error {
	// A started callback owns its durable operation through completion, even if
	// the responder stops waiting. WithoutCancel protects that operation from
	// caller cancellation; the second bound extends total latency by at most
	// min(s.timeout, five minutes). If it expires, callback inputs must remain
	// immutable and callback results must remain callback-owned.
	finishTimeout := min(s.timeout, maxStartedDeliveryConfirmationWait)
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()
	select {
	case <-confirmationDone:
		return clarificationDeliveryConfirmationResult(pending)
	case <-finishCtx.Done():
		return fmt.Errorf("wait for started clarification delivery confirmation: %w", finishCtx.Err())
	}
}

func clarificationDeliveryConfirmationResult(pending *PendingClarification) error {
	pending.mu.Lock()
	defer pending.mu.Unlock()
	return pending.deliveryConfirmationErr
}

// CancelRequest cancels a single pending clarification by id, unblocking any
// WaitForResponse caller that is currently parked on it. Returns true only if
// the entry existed and was still unresolved, so the cancel actually took
// effect; returns false if the entry was never found, or a concurrent
// Respond has already recorded a resolution and there is nothing left to
// cancel. Used by callers that need to surface a creation-side failure
// immediately rather than wait for the 2-hour MCP timeout, and by
// httpCancelRequest to distinguish a real cancel from cancelling an entry
// that already has an answer in flight.
//
// A losing resolution claim (see clarification.Resolver) calls this on every
// cancel outcome, independently of whichever concurrent request reaches the
// entry first. CancelCh is only closed while an entry is still unresolved:
// once Respond has recorded a resolution, closing CancelCh would serve no
// purpose (there is no wedge left to break) and would race Respond's own
// close of done, letting WaitForResponse's select nondeterministically
// report a spurious cancellation for an answer that was actually delivered.
// Gating on pending.resolved under the same pending.mu Respond uses is what
// makes the two mutually exclusive.
//
// Gating additionally on pending.cancelled (set here, under the same lock,
// before the close) makes CancelRequest idempotent against a concurrent
// second close of the same entry -- two overlapping /:id/cancel requests, or
// a request racing CancelSession -- which would otherwise both observe
// resolved=false and both call close(pending.CancelCh), panicking with
// "close of closed channel" and taking down the backend process.
func (s *Store) CancelRequest(pendingID string) bool {
	s.mu.RLock()
	pending, ok := s.pending[pendingID]
	hook := s.onCancelRequestEntered
	s.mu.RUnlock()

	if hook != nil {
		hook(pendingID)
	}

	if !ok {
		return false
	}

	pending.mu.Lock()
	if pending.resolved || pending.cancelled {
		pending.mu.Unlock()
		return false
	}
	pending.cancelled = true
	close(pending.CancelCh)
	pending.mu.Unlock()

	s.deletePendingIfCurrent(pendingID, pending)
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
// unresolved and not already cancelled, checked and set under that entry's
// own pending.mu -- guarding against a concurrent CancelRequest (or a second
// CancelSession call) closing the same channel twice and panicking.
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
	hook := s.onCancelSessionEntered
	s.mu.Unlock()

	for i, pending := range toCancel {
		if hook != nil {
			hook(cancelled[i])
		}
		pending.mu.Lock()
		if !pending.resolved && !pending.cancelled {
			pending.cancelled = true
			close(pending.CancelCh)
		}
		pending.mu.Unlock()
	}
	return cancelled
}

func (s *Store) cancelPendingIfCurrent(
	pendingID string,
	pending *PendingClarification,
) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending[pendingID] != pending {
		return false
	}
	return s.cancelPendingLocked(pendingID, pending)
}

// cancelPendingLocked cancels an unresolved entry while s.mu is write-locked.
func (s *Store) cancelPendingLocked(
	pendingID string,
	pending *PendingClarification,
) bool {
	pending.mu.Lock()
	defer pending.mu.Unlock()
	if pending.resolved || pending.cancelled {
		return false
	}
	pending.cancelled = true
	close(pending.CancelCh)
	delete(s.pending, pendingID)
	return true
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
