package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// ErrResumeAttemptCancelled is returned when a startup continuation no longer
// owns the session's recovery attempt. It is deliberately distinct from a
// provider error so callers can leave cancellation reconciliation to the
// cancellation owner without creating a second failure projection.
var ErrResumeAttemptCancelled = errors.New("resume attempt cancelled")

// resumeAttempt is process-local ownership for one session startup. Provider
// execution IDs are mutable across retries, so every continuation must also
// prove that its attempt is still the registry's current owner.
type resumeAttempt struct {
	id        uint64
	taskID    string
	sessionID string
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}

	finishOnce  sync.Once
	executionMu sync.Mutex
	executionID string
}

type resumeAttemptRegistry struct {
	mu         sync.Mutex
	nextID     uint64
	attempts   map[string]*resumeAttempt
	tombstones map[string][]resumeAttemptTombstone
}

const maxResumeAttemptTombstones = 16

type resumeAttemptTombstone struct {
	id          uint64
	executionID string
	cancelled   bool
}

func newResumeAttemptRegistry() *resumeAttemptRegistry {
	return &resumeAttemptRegistry{
		attempts:   make(map[string]*resumeAttempt),
		tombstones: make(map[string][]resumeAttemptTombstone),
	}
}

// begin returns the active attempt when another caller already owns startup.
// A cancelled owner is replaced immediately; its identity remains valid only
// for exact cleanup and its late callbacks cannot match the replacement.
func (r *resumeAttemptRegistry) begin(parent context.Context, taskID, sessionID string) (*resumeAttempt, bool) {
	if parent == nil {
		parent = context.Background()
	}
	parent = context.WithoutCancel(parent)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.attempts == nil {
		r.attempts = make(map[string]*resumeAttempt)
	}
	if current := r.attempts[sessionID]; current != nil && current.ctx.Err() == nil {
		return current, false
	}
	if current := r.attempts[sessionID]; current != nil {
		r.retainLocked(current)
		delete(r.attempts, sessionID)
	}

	r.nextID++
	attemptCtx, cancel := context.WithCancel(parent)
	attempt := &resumeAttempt{
		id:        r.nextID,
		taskID:    taskID,
		sessionID: sessionID,
		ctx:       attemptCtx,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
	r.attempts[sessionID] = attempt
	return attempt, true
}

// invalidate cancels the current startup attempt. Callers pair it with the
// session's cancellation guard so registration and invalidation are ordered
// against prompt admission and lifecycle event handling.
func (r *resumeAttemptRegistry) invalidate(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt := r.attempts[sessionID]
	if attempt == nil || attempt.ctx.Err() != nil {
		return false
	}
	attempt.cancel()
	return true
}

func (r *resumeAttemptRegistry) cancelAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, attempt := range r.attempts {
		if attempt != nil {
			attempt.cancel()
		}
	}
}

func (r *resumeAttemptRegistry) current(sessionID string) (*resumeAttempt, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt, ok := r.attempts[sessionID]
	return attempt, ok
}

func (r *resumeAttemptRegistry) isCurrent(attempt *resumeAttempt) bool {
	if attempt == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts[attempt.sessionID] == attempt
}

// canCleanup reports whether an attempt still owns teardown of its execution.
// A managed runtime may reuse one execution ID for a replacement startup, so
// an old cancelled attempt must not stop that replacement after it has taken
// ownership of the same ID.
func (r *resumeAttemptRegistry) canCleanup(attempt *resumeAttempt) bool {
	if attempt == nil {
		return false
	}
	executionID := attempt.execution()
	if executionID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if current := r.attempts[attempt.sessionID]; current != nil && current != attempt {
		return false
	}
	for _, tombstone := range r.tombstones[attempt.sessionID] {
		if tombstone.id > attempt.id && tombstone.executionID == executionID {
			return false
		}
	}
	return true
}

func (r *resumeAttemptRegistry) retainLocked(attempt *resumeAttempt) {
	if attempt == nil {
		return
	}
	if r.tombstones == nil {
		r.tombstones = make(map[string][]resumeAttemptTombstone)
	}
	entries := r.tombstones[attempt.sessionID]
	entries = append(entries, resumeAttemptTombstone{
		id:          attempt.id,
		executionID: attempt.execution(),
		cancelled:   attempt.ctx.Err() != nil,
	})
	if len(entries) > maxResumeAttemptTombstones {
		entries = entries[len(entries)-maxResumeAttemptTombstones:]
	}
	r.tombstones[attempt.sessionID] = entries
}

func (r *resumeAttemptRegistry) tombstoneLocked(sessionID string, attemptID uint64) (resumeAttemptTombstone, bool) {
	for _, tombstone := range r.tombstones[sessionID] {
		if tombstone.id == attemptID {
			return tombstone, true
		}
	}
	return resumeAttemptTombstone{}, false
}

func (r *resumeAttemptRegistry) cancelledExecutionLocked(sessionID, executionID string) bool {
	if executionID == "" {
		return false
	}
	for _, tombstone := range r.tombstones[sessionID] {
		if tombstone.cancelled && tombstone.executionID == executionID {
			return true
		}
	}
	return false
}

func (attempt *resumeAttempt) context() context.Context {
	if attempt == nil || attempt.ctx == nil {
		return context.Background()
	}
	return attempt.ctx
}

func (attempt *resumeAttempt) identity() string {
	if attempt == nil {
		return ""
	}
	return strconv.FormatUint(attempt.id, 10)
}

func (attempt *resumeAttempt) validate(registry *resumeAttemptRegistry) error {
	if attempt == nil || registry == nil || !registry.isCurrent(attempt) || attempt.ctx.Err() != nil {
		if attempt == nil {
			return ErrResumeAttemptCancelled
		}
		return fmt.Errorf("%w: %d", ErrResumeAttemptCancelled, attempt.id)
	}
	return nil
}

func (attempt *resumeAttempt) setExecutionID(executionID string) {
	if attempt == nil || executionID == "" {
		return
	}
	attempt.executionMu.Lock()
	attempt.executionID = executionID
	attempt.executionMu.Unlock()
}

func (attempt *resumeAttempt) execution() string {
	if attempt == nil {
		return ""
	}
	attempt.executionMu.Lock()
	defer attempt.executionMu.Unlock()
	return attempt.executionID
}

func (attempt *resumeAttempt) finish(registry *resumeAttemptRegistry) {
	if attempt == nil {
		return
	}
	attempt.finishOnce.Do(func() {
		if registry != nil {
			registry.mu.Lock()
			if registry.attempts[attempt.sessionID] == attempt {
				delete(registry.attempts, attempt.sessionID)
			}
			registry.retainLocked(attempt)
			registry.mu.Unlock()
		}
		close(attempt.done)
	})
}

func (attempt *resumeAttempt) wait(ctx context.Context) error {
	if attempt == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-attempt.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) resumeAttemptStore() *resumeAttemptRegistry {
	s.resumeAttemptsMu.Lock()
	defer s.resumeAttemptsMu.Unlock()
	if s.resumeAttempts == nil {
		s.resumeAttempts = newResumeAttemptRegistry()
	}
	return s.resumeAttempts
}

// beginResumeAttempt registers startup while holding the same per-session
// guard used by cancellation and prompt admission. If cancellation already
// owns the session, wait for its projection to settle before admitting a new
// attempt, then repeat the guarded check to close the handoff race.
func (s *Service) beginResumeAttempt(
	ctx context.Context,
	taskID, sessionID string,
) (*resumeAttempt, bool, error) {
	for {
		lock, release := s.acquireCancelInFlightGuard(sessionID)
		lock.Lock()
		operation := s.currentCancellation(sessionID)
		if operation == nil {
			attempt, owner := s.resumeAttemptStore().begin(ctx, taskID, sessionID)
			lock.Unlock()
			release()
			return attempt, owner, nil
		}
		lock.Unlock()
		release()

		waitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancellationOperationTTL)
		err := operation.wait(waitCtx)
		cancel()
		if err != nil {
			return nil, false, fmt.Errorf("wait for session cancellation before resume: %w", err)
		}
	}
}

func (s *Service) validateResumeAttempt(attempt *resumeAttempt) error {
	if attempt == nil {
		return nil
	}
	return attempt.validate(s.resumeAttemptStore())
}

func cancellableResumeContext(attempt *resumeAttempt) context.Context {
	if attempt == nil {
		return context.Background()
	}
	ctx := executor.WithCancellableResumeContext(attempt.context())
	return executor.WithResumeAttemptID(ctx, attempt.identity())
}

// resumeAttemptAllowsExecution rejects callbacks from a cancelled attempt or
// from an older execution after a retry has installed a successor. Callbacks
// produced by a recovery startup must carry the originating attempt ID. The
// registry retains finished ownership records so a delayed callback cannot be
// accepted merely because the active map entry was removed.
//
// The optional form preserves compatibility with older event producers while
// they are not inside an active recovery attempt. Once a recovery attempt is
// active, an untagged startup callback is rejected closed.
func (s *Service) resumeAttemptAllowsExecution(sessionID, executionID string, origin ...string) bool {
	originID := resumeAttemptOrigin(origin)
	registry := s.resumeAttemptStore()
	current, hasCurrent := registry.current(sessionID)
	if hasCurrent && current != nil {
		return s.activeResumeAttemptAllowsExecution(current, executionID, originID)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	return resumeAttemptTombstoneAllowsExecution(registry, sessionID, executionID, originID)
}

func resumeAttemptOrigin(origin []string) string {
	if len(origin) == 0 {
		return ""
	}
	return origin[0]
}

func (s *Service) activeResumeAttemptAllowsExecution(
	current *resumeAttempt,
	executionID, originID string,
) bool {
	if originID == "" {
		return false
	}
	id, err := strconv.ParseUint(originID, 10, 64)
	if err != nil || id != current.id || s.validateResumeAttempt(current) != nil {
		return false
	}
	knownExecutionID := current.execution()
	return knownExecutionID == "" || executionID == "" || knownExecutionID == executionID
}

func resumeAttemptTombstoneAllowsExecution(
	registry *resumeAttemptRegistry,
	sessionID, executionID, originID string,
) bool {
	if originID == "" {
		return !registry.cancelledExecutionLocked(sessionID, executionID)
	}
	id, err := strconv.ParseUint(originID, 10, 64)
	if err != nil {
		// Ordinary execution callbacks may use a provider execution ID in
		// the legacy attempt field. Only decimal registry identities can
		// refer to a retained recovery attempt.
		return true
	}
	tombstone, found := registry.tombstoneLocked(sessionID, id)
	if !found {
		// An attempt ID is optional on callbacks from ordinary (non-resume)
		// starts. Those callers historically used the execution ID in the
		// same field, so an unknown origin is legacy/unmanaged identity. A
		// retained tombstone remains authoritative: only an origin that was
		// actually owned by a finished attempt is fenced here.
		return true
	}
	if tombstone.cancelled {
		return false
	}
	return tombstone.executionID == "" || executionID == "" || tombstone.executionID == executionID
}

// lockResumeAttemptAdmission serializes the final pre-dispatch check with
// cancellation. The returned guard stays locked until the caller reaches the
// provider acceptance boundary. Waiting uses a bounded detached context so a
// browser disconnect cannot abandon the session half-claimed.
func (s *Service) lockResumeAttemptAdmission(
	ctx context.Context,
	sessionID string,
	attempt *resumeAttempt,
) (*lockedCancelInFlightGuard, error) {
	if attempt == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	guard := s.lockCancelInFlightGuard(sessionID)
	waitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancellationOperationTTL)
	err := s.waitForCancellationWithGuard(waitCtx, sessionID, guard.unlock, guard.relock)
	cancel()
	if err != nil {
		guard.release()
		return nil, err
	}
	if err := s.validateResumeAttempt(attempt); err != nil {
		guard.release()
		return nil, err
	}
	return guard, nil
}

func (s *Service) invalidateResumeAttempt(sessionID string) {
	if sessionID == "" {
		return
	}
	s.resumeAttemptStore().invalidate(sessionID)
}

func (s *Service) cancelResumeAttempts() {
	s.resumeAttemptStore().cancelAll()
}

// cleanupCancelledResumeAttempt tears down only the execution captured by the
// cancelled attempt. The cancellation guard and execution teardown claim make
// this safe when a retry has already installed a replacement execution.
func (s *Service) cleanupCancelledResumeAttempt(attempt *resumeAttempt) {
	if attempt == nil || s.executor == nil {
		return
	}
	executionID := attempt.execution()
	if executionID == "" || !s.resumeAttemptStore().canCleanup(attempt) ||
		!s.claimForcedExecutionCleanup(attempt.sessionID, executionID) {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), cancellationOperationTTL)
	defer cancel()
	if err := s.executor.StopExecution(cleanupCtx, executionID, "cancelled resume startup", true); err != nil && s.logger != nil {
		s.logger.Debug("failed to clean up cancelled resume execution",
			zap.String("task_id", attempt.taskID),
			zap.String("session_id", attempt.sessionID),
			zap.String("agent_execution_id", executionID),
			zap.Error(err))
	}
}
