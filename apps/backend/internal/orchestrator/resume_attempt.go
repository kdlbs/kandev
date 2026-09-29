package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
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
	// interruptedMarker is the marker value observed when this recovery attempt
	// was admitted. A delayed boot callback may clear only that exact marker;
	// a later restart must remain visible until its own attempt succeeds.
	interruptedMarkerMu       sync.RWMutex
	interruptedMarker         string
	interruptedMarkerCaptured bool
	// retained is protected by resumeAttemptRegistry.mu. A cancelled attempt
	// can be retained when a replacement is admitted and retained again when
	// its owner eventually returns; both paths must describe one tombstone.
	retained bool
	// accepted is protected by resumeAttemptRegistry.mu. Once provider
	// acceptance is recorded, explicit session cancellation no longer owns
	// startup teardown for this attempt.
	accepted bool
	// awaitingInitialPrompt keeps startup ownership active after the model
	// switch path returns. Its initial prompt is dispatched asynchronously by
	// lifecycle, so promptTask's deferred finish must wait for the real provider
	// acceptance callback.
	awaitingInitialPrompt bool
	// finishRequested is set when the owning operation returns while the
	// initial prompt is still pending. The acceptance callback completes the
	// attempt after it transfers ownership.
	finishRequested bool
}

type resumeAttemptRegistry struct {
	mu              sync.Mutex
	nextID          uint64
	attempts        map[string]*resumeAttempt
	tombstones      map[string][]resumeAttemptTombstone
	recoveryHistory map[string]struct{}
	latestExecution map[string]map[string]uint64
}

const maxResumeAttemptTombstones = 16

const resumeAttemptIdentityPrefix = "resume-"

type resumeAttemptTombstone struct {
	id                        uint64
	executionID               string
	cancelled                 bool
	accepted                  bool
	interruptedMarker         string
	interruptedMarkerCaptured bool
}

type interruptedMarkerSnapshot struct {
	value    string
	captured bool
}

func newResumeAttemptRegistry() *resumeAttemptRegistry {
	return &resumeAttemptRegistry{
		attempts:        make(map[string]*resumeAttempt),
		tombstones:      make(map[string][]resumeAttemptTombstone),
		recoveryHistory: make(map[string]struct{}),
		latestExecution: make(map[string]map[string]uint64),
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
	current := r.attempts[sessionID]
	if current != nil && current.ctx.Err() == nil {
		return current, false
	}
	if current != nil {
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
	attempt := r.attempts[sessionID]
	if attempt == nil || attempt.ctx.Err() != nil || attempt.accepted {
		r.mu.Unlock()
		return false
	}
	attempt.cancel()
	finishPending := attempt.awaitingInitialPrompt
	r.mu.Unlock()
	if finishPending {
		// The operation that owns this attempt may already have returned while
		// lifecycle is still dispatching its initial prompt. Finish the pending
		// ownership record now so a cancelled callback cannot leave it active.
		attempt.finish(r)
	}
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
	if attempt.accepted {
		return false
	}
	for _, tombstone := range r.tombstones[attempt.sessionID] {
		if tombstone.id > attempt.id && tombstone.executionID == executionID {
			return false
		}
	}
	if latest := r.latestExecution[attempt.sessionID][executionID]; latest > attempt.id {
		return false
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
	if r.recoveryHistory == nil {
		r.recoveryHistory = make(map[string]struct{})
	}
	r.recoveryHistory[attempt.sessionID] = struct{}{}
	executionID := attempt.execution()
	if executionID != "" {
		if r.latestExecution == nil {
			r.latestExecution = make(map[string]map[string]uint64)
		}
		if r.latestExecution[attempt.sessionID] == nil {
			r.latestExecution[attempt.sessionID] = make(map[string]uint64)
		}
		if attempt.id > r.latestExecution[attempt.sessionID][executionID] {
			r.latestExecution[attempt.sessionID][executionID] = attempt.id
		}
	}
	// begin() may retain a cancelled attempt before its asynchronous launch
	// returns an execution ID. Complete the existing record when that ID
	// becomes known, but never append a second record for the same attempt.
	if attempt.retained {
		entries := r.tombstones[attempt.sessionID]
		for index := range entries {
			if entries[index].id == attempt.id && entries[index].executionID == "" && executionID != "" {
				entries[index].executionID = executionID
			}
			if entries[index].id == attempt.id {
				entries[index].accepted = attempt.accepted
				entries[index].interruptedMarker = attempt.interruptedMarkerValue()
				entries[index].interruptedMarkerCaptured = attempt.interruptedMarkerSnapshotCaptured()
				break
			}
		}
		r.tombstones[attempt.sessionID] = entries
		return
	}
	attempt.retained = true
	entries := r.tombstones[attempt.sessionID]
	entries = append(entries, resumeAttemptTombstone{
		id:                        attempt.id,
		executionID:               executionID,
		cancelled:                 attempt.ctx.Err() != nil,
		accepted:                  attempt.accepted,
		interruptedMarker:         attempt.interruptedMarkerValue(),
		interruptedMarkerCaptured: attempt.interruptedMarkerSnapshotCaptured(),
	})
	if len(entries) > maxResumeAttemptTombstones {
		entries = entries[len(entries)-maxResumeAttemptTombstones:]
	}
	r.tombstones[attempt.sessionID] = entries
}

func (r *resumeAttemptRegistry) hasRecoveryHistoryLocked(sessionID string) bool {
	_, ok := r.recoveryHistory[sessionID]
	return ok
}

func (r *resumeAttemptRegistry) tombstoneLocked(sessionID string, attemptID uint64) (resumeAttemptTombstone, bool) {
	for _, tombstone := range r.tombstones[sessionID] {
		if tombstone.id == attemptID {
			return tombstone, true
		}
	}
	return resumeAttemptTombstone{}, false
}

// canCleanupIdentity reports whether a stale callback still owns cleanup for
// its exact retained attempt. Unknown or compacted identities fail closed so
// a delayed failure cannot stop a later attempt that reused the execution ID.
func (r *resumeAttemptRegistry) canCleanupIdentity(sessionID, executionID, originID string) bool {
	if sessionID == "" || executionID == "" || originID == "" {
		return false
	}
	id, ok := parseResumeAttemptIdentity(originID)
	if !ok {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tombstone, found := r.tombstoneLocked(sessionID, id)
	if !found || tombstone.executionID != executionID {
		return false
	}
	if tombstone.accepted {
		return false
	}
	if current := r.attempts[sessionID]; current != nil && current.id > id {
		return false
	}
	return r.latestExecution[sessionID][executionID] <= id
}

// accept transfers startup authority to the provider turn while retaining the
// attempt identity for later execution events. The registry lock orders this
// transition with explicit cancellation, so an invalidated attempt cannot be
// revived by a late provider callback.
func (r *resumeAttemptRegistry) accept(attempt *resumeAttempt, executionID string) bool {
	if attempt == nil || executionID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.attempts[attempt.sessionID] != attempt || attempt.ctx.Err() != nil {
		return false
	}
	attempt.executionMu.Lock()
	knownExecutionID := attempt.executionID
	if knownExecutionID == "" {
		attempt.executionID = executionID
		knownExecutionID = executionID
	}
	attempt.executionMu.Unlock()
	if knownExecutionID != executionID {
		return false
	}
	attempt.accepted = true
	return true
}

// holdForInitialPrompt keeps an admitted model-switch attempt active until
// lifecycle reports acceptance of the replacement's asynchronously dispatched
// initial prompt. The caller must set this before starting the replacement.
func (r *resumeAttemptRegistry) holdForInitialPrompt(attempt *resumeAttempt) bool {
	if attempt == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.attempts[attempt.sessionID] != attempt || attempt.ctx.Err() != nil || attempt.accepted {
		return false
	}
	attempt.awaitingInitialPrompt = true
	return true
}

// releaseInitialPromptHold releases the model-switch-specific deferred finish
// when the model switch stayed in place and ordinary prompt dispatch will own
// the acceptance boundary instead.
func (r *resumeAttemptRegistry) releaseInitialPromptHold(attempt *resumeAttempt) {
	if attempt == nil {
		return
	}
	r.mu.Lock()
	if r.attempts[attempt.sessionID] == attempt {
		attempt.awaitingInitialPrompt = false
	}
	r.mu.Unlock()
}

// abortInitialPromptHold closes a model-switch attempt when lifecycle reports
// a delivery failure before provider acceptance. There is no later owner that
// can safely finish the attempt after the asynchronous failure callback.
func (r *resumeAttemptRegistry) abortInitialPromptHold(attempt *resumeAttempt) {
	if attempt == nil {
		return
	}
	r.mu.Lock()
	if r.attempts[attempt.sessionID] == attempt {
		attempt.awaitingInitialPrompt = false
	}
	r.mu.Unlock()
	attempt.finish(r)
}

// finishAfterInitialPromptAcceptance completes a deferred owner only after
// the callback has transferred startup authority. If the outer operation is
// still active, its normal finish call remains responsible for completion.
func (r *resumeAttemptRegistry) finishAfterInitialPromptAcceptance(attempt *resumeAttempt) {
	if attempt == nil {
		return
	}
	r.mu.Lock()
	shouldFinish := r.attempts[attempt.sessionID] == attempt &&
		attempt.accepted && attempt.finishRequested
	r.mu.Unlock()
	if shouldFinish {
		attempt.finish(r)
	}
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
	return resumeAttemptIdentityPrefix + strconv.FormatUint(attempt.id, 10)
}

func (attempt *resumeAttempt) validate(registry *resumeAttemptRegistry) error {
	if attempt == nil || registry == nil || !registry.isCurrent(attempt) || attempt.ctx.Err() != nil {
		if attempt == nil {
			return ErrResumeAttemptCancelled
		}
		return fmt.Errorf("%w: %s", ErrResumeAttemptCancelled, attempt.identity())
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

func (attempt *resumeAttempt) setInterruptedMarkerSnapshot(marker string, captured bool) {
	if attempt == nil {
		return
	}
	attempt.interruptedMarkerMu.Lock()
	attempt.interruptedMarker = marker
	attempt.interruptedMarkerCaptured = captured
	attempt.interruptedMarkerMu.Unlock()
}

func (attempt *resumeAttempt) interruptedMarkerValue() string {
	if attempt == nil {
		return ""
	}
	attempt.interruptedMarkerMu.RLock()
	defer attempt.interruptedMarkerMu.RUnlock()
	return attempt.interruptedMarker
}

func (attempt *resumeAttempt) interruptedMarkerSnapshotCaptured() bool {
	if attempt == nil {
		return false
	}
	attempt.interruptedMarkerMu.RLock()
	defer attempt.interruptedMarkerMu.RUnlock()
	return attempt.interruptedMarkerCaptured
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
	if registry != nil {
		registry.mu.Lock()
		if registry.attempts[attempt.sessionID] == attempt &&
			attempt.awaitingInitialPrompt && !attempt.accepted && attempt.ctx.Err() == nil {
			attempt.finishRequested = true
			registry.mu.Unlock()
			return
		}
		if registry.attempts[attempt.sessionID] == attempt {
			delete(registry.attempts, attempt.sessionID)
		}
		registry.retainLocked(attempt)
		registry.mu.Unlock()
	}
	attempt.finishOnce.Do(func() { close(attempt.done) })
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
			if owner {
				s.captureInterruptedMarkerForResumeAttempt(ctx, attempt)
			}
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

func (s *Service) captureInterruptedMarkerForResumeAttempt(ctx context.Context, attempt *resumeAttempt) {
	if s == nil || s.repo == nil || attempt == nil || attempt.taskID == "" {
		return
	}
	task, err := s.repo.GetTask(ctx, attempt.taskID)
	if err != nil || task == nil {
		return
	}
	if task.Metadata == nil {
		attempt.setInterruptedMarkerSnapshot("", true)
		return
	}
	marker, ok := task.Metadata[models.MetaKeyInterruptedAt].(string)
	// A successful task read with no marker is still an immutable snapshot.
	// Preserve that fact through tombstones, while the empty value remains a
	// fail-closed generation for the recovery callback.
	attempt.setInterruptedMarkerSnapshot(marker, ok)
}

func (s *Service) interruptedMarkerSnapshotForResumeAttempt(
	sessionID, attemptID string,
) (interruptedMarkerSnapshot, bool) {
	if s == nil || sessionID == "" || attemptID == "" {
		return interruptedMarkerSnapshot{}, false
	}
	id, isRecovery := parseResumeAttemptIdentity(attemptID)
	if !isRecovery {
		return interruptedMarkerSnapshot{}, false
	}
	registry := s.resumeAttemptStore()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if attempt := registry.attempts[sessionID]; attempt != nil && attempt.id == id {
		return interruptedMarkerSnapshot{
			value:    attempt.interruptedMarkerValue(),
			captured: attempt.interruptedMarkerSnapshotCaptured(),
		}, true
	}
	if tombstone, ok := registry.tombstoneLocked(sessionID, id); ok {
		return interruptedMarkerSnapshot{
			value:    tombstone.interruptedMarker,
			captured: tombstone.interruptedMarkerCaptured,
		}, true
	}
	return interruptedMarkerSnapshot{}, false
}

func (s *Service) interruptedMarkerForResumeAttempt(sessionID, attemptID string) string {
	snapshot, known := s.interruptedMarkerSnapshotForResumeAttempt(sessionID, attemptID)
	if !known || !snapshot.captured {
		return ""
	}
	return snapshot.value
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
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if current := registry.attempts[sessionID]; current != nil {
		return activeResumeAttemptAllowsExecutionLocked(registry, current, executionID, originID)
	}
	return resumeAttemptTombstoneAllowsExecution(registry, sessionID, executionID, originID)
}

func resumeAttemptOrigin(origin []string) string {
	if len(origin) == 0 {
		return ""
	}
	return origin[0]
}

func activeResumeAttemptAllowsExecutionLocked(
	registry *resumeAttemptRegistry,
	current *resumeAttempt,
	executionID, originID string,
) bool {
	if registry == nil || current == nil || originID == "" {
		return false
	}
	id, ok := parseResumeAttemptIdentity(originID)
	if !ok || id != current.id || current.ctx.Err() != nil {
		return false
	}
	current.executionMu.Lock()
	knownExecutionID := current.executionID
	if knownExecutionID == "" && executionID != "" {
		// The first callback may arrive before ResumeSessionWithOptions returns
		// its execution. Bind that callback's execution while the registry lock
		// still proves that this attempt is current; later callbacks must match
		// this captured execution exactly.
		current.executionID = executionID
		knownExecutionID = executionID
	}
	current.executionMu.Unlock()
	return knownExecutionID == "" || executionID == "" || knownExecutionID == executionID
}

func parseResumeAttemptIdentity(identity string) (uint64, bool) {
	if !strings.HasPrefix(identity, resumeAttemptIdentityPrefix) {
		return 0, false
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(identity, resumeAttemptIdentityPrefix), 10, 64)
	return id, err == nil
}

func resumeAttemptTombstoneAllowsExecution(
	registry *resumeAttemptRegistry,
	sessionID, executionID, originID string,
) bool {
	if originID == "" {
		// Once this session has entered recovery, every recovery callback must
		// carry its immutable identity. An untagged callback after the active
		// entry is removed cannot be attributed safely, even if the completed
		// attempt was successful rather than cancelled.
		if registry.hasRecoveryHistoryLocked(sessionID) {
			return false
		}
		return !registry.cancelledExecutionLocked(sessionID, executionID)
	}
	id, ok := parseResumeAttemptIdentity(originID)
	if !ok {
		// Ordinary execution callbacks may use a provider execution ID in
		// the legacy attempt field. The resume- prefix makes registry identities
		// unambiguous even when a provider execution ID is numeric.
		return true
	}
	tombstone, found := registry.tombstoneLocked(sessionID, id)
	if !found {
		// Provider execution identifiers are opaque and may remain in this
		// legacy field, while decimal values are reserved for registry attempt
		// identities. Once recovery has started for this session, fail closed
		// for an unknown decimal identity even after its tombstone was compacted.
		return !registry.hasRecoveryHistoryLocked(sessionID)
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
	if s.lspLeases != nil {
		s.lspLeases.StopLSPLeasesForExecution(executionID)
	}
	if err := s.executor.StopExecution(cleanupCtx, executionID, "cancelled resume startup", true); err != nil && s.logger != nil {
		s.logger.Debug("failed to clean up cancelled resume execution",
			zap.String("task_id", attempt.taskID),
			zap.String("session_id", attempt.sessionID),
			zap.String("agent_execution_id", executionID),
			zap.Error(err))
	}
}

// cleanupStaleResumeExecution tears down an exact execution named by a late
// resume failure. The retained attempt check is required before the generic
// teardown claim: without it, an unknown or compacted callback could stop a
// successor that reused the provider execution ID.
func (s *Service) cleanupStaleResumeExecution(executionID, taskID, sessionID, attemptID string) {
	if s == nil || s.executor == nil || executionID == "" ||
		!s.resumeAttemptStore().canCleanupIdentity(sessionID, executionID, attemptID) {
		return
	}
	go s.cleanupAgentExecution(executionID, taskID, sessionID)
}
