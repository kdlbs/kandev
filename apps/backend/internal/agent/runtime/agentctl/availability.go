package client

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

const (
	AvailabilityStatusAvailable   = "available"
	AvailabilityStatusRecovering  = "recovering"
	AvailabilityStatusUnavailable = "unavailable"

	AvailabilityReasonAgentctlExited      = "agentctl_exited"
	AvailabilityReasonOwnershipUnverified = "ownership_unverified"
	AvailabilityReasonStartFailed         = "start_failed"
	AvailabilityReasonRecoveryExhausted   = "recovery_exhausted"
)

var (
	ErrRuntimeUnavailable     = errors.New("local agent runtime is unavailable")
	ErrRuntimeLeaseRetired    = errors.New("local agent runtime lease has been retired")
	ErrRuntimeStopUnconfirmed = errors.New("local agent runtime stop could not be confirmed")
	ErrRuntimeOwnerStopped    = errors.New("local agent runtime owner is stopping")
	ErrRuntimeCandidateState  = errors.New("local agent runtime candidate is no longer preparable")
	ErrRuntimeBindingActive   = errors.New("local agent runtime already has an active binding")
	ErrRecoveryConflict       = errors.New("agent runtime recovery request is stale")
	ErrRecoveryNotRetryable   = errors.New("agent runtime recovery is not retryable")
	ErrRecoveryRequestID      = errors.New("agent runtime recovery request id is invalid")
)

// AvailabilitySnapshot is the sanitized, install-wide agent runtime state
// exposed to authenticated clients. It intentionally contains no process or
// launcher details.
type AvailabilitySnapshot struct {
	Status       string     `json:"status"`
	Reason       string     `json:"reason,omitempty"`
	OccurredAt   *time.Time `json:"occurred_at,omitempty"`
	BootID       string     `json:"boot_id,omitempty"`
	Revision     uint64     `json:"revision"`
	RuntimeEpoch uint64     `json:"runtime_epoch"`
	RecoveryID   string     `json:"recovery_id,omitempty"`
	RetryAllowed bool       `json:"retry_allowed,omitempty"`
}

// RuntimeOwner owns the authenticated connection to local agentctl and its
// sanitized availability state.
type RuntimeOwner struct {
	mu                sync.RWMutex
	eventBus          bus.EventBus
	logger            *logger.Logger
	bootID            string
	nextEpoch         uint64
	active            *runtimeBinding
	candidates        map[uint64]*RuntimeBindingCandidate
	snapshot          AvailabilitySnapshot
	published         bool
	stopping          bool
	stopped           bool
	publications      []AvailabilitySnapshot
	publicationSignal chan struct{}
	publicationStop   chan struct{}
	publicationOnce   sync.Once
	retiredCleanup    sync.WaitGroup
}

// Availability remains as the compatibility name used by existing boot-state
// and gateway wiring. New runtime code should use RuntimeOwner.
type Availability = RuntimeOwner

type runtimeBinding struct {
	leaseMu         sync.Mutex
	leaseNext       uint64
	leases          map[uint64]context.CancelFunc
	bootID          string
	epoch           uint64
	host            string
	port            int
	credential      string
	processID       int
	ctx             context.Context
	cancel          context.CancelFunc
	cleanup         func() error
	processIdentity processidentity.Identity
}

func (binding *runtimeBinding) cancelLeases() {
	binding.cancel()
	binding.leaseMu.Lock()
	for id, cancel := range binding.leases {
		cancel()
		delete(binding.leases, id)
	}
	binding.leaseMu.Unlock()
}

type runtimeCandidateState uint8

const (
	runtimeCandidatePrepared runtimeCandidateState = iota
	runtimeCandidateCommitted
	runtimeCandidateAborted
)

// RuntimeBindingCandidate owns resources prepared for a possible runtime
// publication. Its credential and endpoint are private and never enter a
// snapshot.
type RuntimeBindingCandidate struct {
	owner           *RuntimeOwner
	epoch           uint64
	state           runtimeCandidateState
	host            string
	port            int
	credential      string
	processID       int
	cleanup         func() error
	processIdentity processidentity.Identity
	configured      bool
	exited          bool
	exitReason      string
	binding         *runtimeBinding
}

// RuntimeLease pins operations to one immutable runtime binding. Callers must
// use Context for requests and CheckCurrent before applying asynchronous
// results to durable state.
type RuntimeLease struct {
	owner     *RuntimeOwner
	binding   *runtimeBinding
	id        uint64
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// NewRuntimeOwner creates an unpublished local runtime owner.
func NewRuntimeOwner(eventBus bus.EventBus, log *logger.Logger, bootID string) *RuntimeOwner {
	owner := &RuntimeOwner{
		eventBus:   eventBus,
		logger:     log,
		bootID:     bootID,
		candidates: make(map[uint64]*RuntimeBindingCandidate),
	}
	if eventBus != nil {
		owner.publicationSignal = make(chan struct{}, 1)
		owner.publicationStop = make(chan struct{})
		go owner.runPublisher()
	}
	return owner
}

// NewAvailability creates an unpublished runtime owner for existing callers.
func NewAvailability(eventBus bus.EventBus, log *logger.Logger) *Availability {
	return NewRuntimeOwner(eventBus, log, "")
}

// Snapshot returns a copy of the current published state.
func (o *RuntimeOwner) Snapshot() (AvailabilitySnapshot, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if !o.published {
		return AvailabilitySnapshot{}, false
	}
	return cloneAvailabilitySnapshot(o.snapshot), true
}

func cloneAvailabilitySnapshot(snapshot AvailabilitySnapshot) AvailabilitySnapshot {
	if snapshot.OccurredAt != nil {
		occurredAt := *snapshot.OccurredAt
		snapshot.OccurredAt = &occurredAt
	}
	return snapshot
}

// PrepareBinding reserves a monotonic epoch for a successor runtime. The
// candidate remains private until it is configured and committed.
func (o *RuntimeOwner) PrepareBinding() (*RuntimeBindingCandidate, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stopping {
		return nil, ErrRuntimeOwnerStopped
	}
	o.nextEpoch++
	candidate := &RuntimeBindingCandidate{owner: o, epoch: o.nextEpoch}
	o.candidates[candidate.epoch] = candidate
	return candidate, nil
}

// Epoch is the reserved generation for this candidate.
func (candidate *RuntimeBindingCandidate) Epoch() uint64 {
	if candidate == nil {
		return 0
	}
	return candidate.epoch
}

// SetProcessIdentity records optional OS ownership evidence for cleanup and
// adopted-runtime health checks. Missing evidence never authorizes a kill.
func (candidate *RuntimeBindingCandidate) SetProcessIdentity(identity processidentity.Identity) error {
	if candidate == nil || candidate.owner == nil {
		return ErrRuntimeCandidateState
	}
	o := candidate.owner
	o.mu.Lock()
	defer o.mu.Unlock()
	if candidate.state != runtimeCandidatePrepared || o.stopping {
		return ErrRuntimeCandidateState
	}
	candidate.processIdentity = identity
	return nil
}

// Configure sets the candidate's immutable connection and cleanup details.
func (candidate *RuntimeBindingCandidate) Configure(host string, port int, credential string, processID int, cleanup func() error) error {
	if candidate == nil || candidate.owner == nil {
		return ErrRuntimeCandidateState
	}
	if host == "" || port <= 0 || credential == "" {
		return errors.New("runtime binding requires host, port, and credential")
	}
	o := candidate.owner
	o.mu.Lock()
	if candidate.state != runtimeCandidatePrepared || o.stopping || candidate.configured {
		state := candidate.state
		o.mu.Unlock()
		if state == runtimeCandidateAborted && cleanup != nil {
			_ = cleanup()
		}
		return ErrRuntimeCandidateState
	}
	candidate.host = host
	candidate.port = port
	candidate.credential = credential
	candidate.processID = processID
	candidate.cleanup = cleanup
	candidate.configured = true
	if candidate.exited {
		candidate.state = runtimeCandidateAborted
		delete(o.candidates, candidate.epoch)
		candidate.credential = ""
		candidate.host = ""
		candidate.port = 0
		candidate.processID = 0
		candidate.cleanup = nil
		o.mu.Unlock()
		if cleanup != nil {
			_ = cleanup()
		}
		return ErrRuntimeCandidateState
	}
	o.mu.Unlock()
	return nil
}

// Commit atomically publishes a fully configured candidate. It cannot
// replace a live binding; the current owner must retire that binding first.
func (candidate *RuntimeBindingCandidate) Commit() error {
	if candidate == nil || candidate.owner == nil {
		return ErrRuntimeCandidateState
	}
	o := candidate.owner
	o.mu.Lock()
	if candidate.state == runtimeCandidateCommitted {
		o.mu.Unlock()
		return ErrRuntimeCandidateState
	}
	if candidate.state != runtimeCandidatePrepared || o.stopping || candidate.exited || !candidate.configured {
		cleanup := o.abortCandidateLocked(candidate)
		stopping := o.stopping
		o.mu.Unlock()
		if cleanup != nil {
			_ = cleanup()
		}
		if stopping {
			return ErrRuntimeOwnerStopped
		}
		return ErrRuntimeCandidateState
	}
	if o.active != nil {
		cleanup := o.abortCandidateLocked(candidate)
		o.mu.Unlock()
		if cleanup != nil {
			_ = cleanup()
		}
		return ErrRuntimeBindingActive
	}
	if candidate.epoch <= o.snapshot.RuntimeEpoch {
		cleanup := o.abortCandidateLocked(candidate)
		o.mu.Unlock()
		if cleanup != nil {
			_ = cleanup()
		}
		return ErrRuntimeCandidateState
	}

	bindingCtx, cancel := context.WithCancel(context.Background())
	binding := &runtimeBinding{
		leases:          make(map[uint64]context.CancelFunc),
		bootID:          o.bootID,
		epoch:           candidate.epoch,
		host:            candidate.host,
		port:            candidate.port,
		credential:      candidate.credential,
		processID:       candidate.processID,
		processIdentity: candidate.processIdentity,
		ctx:             bindingCtx,
		cancel:          cancel,
		cleanup:         candidate.cleanup,
	}
	candidate.state = runtimeCandidateCommitted
	candidate.credential = ""
	candidate.cleanup = nil
	candidate.binding = binding
	delete(o.candidates, candidate.epoch)
	o.active = binding
	snapshot := o.setSnapshotLocked(AvailabilityStatusAvailable, "", candidate.epoch)
	o.queuePublicationLocked(snapshot)
	o.mu.Unlock()
	return nil
}

// Abort discards an unpublished candidate and closes only its prepared
// resources. It never restores a retired binding.
func (candidate *RuntimeBindingCandidate) Abort() error {
	if candidate == nil || candidate.owner == nil {
		return ErrRuntimeCandidateState
	}
	o := candidate.owner
	o.mu.Lock()
	if candidate.state == runtimeCandidateCommitted {
		o.mu.Unlock()
		return ErrRuntimeCandidateState
	}
	cleanup := o.abortCandidateLocked(candidate)
	o.mu.Unlock()
	if cleanup != nil {
		return cleanup()
	}
	return nil
}

func (o *RuntimeOwner) abortCandidateLocked(candidate *RuntimeBindingCandidate) func() error {
	if candidate.state == runtimeCandidateAborted {
		return nil
	}
	candidate.state = runtimeCandidateAborted
	delete(o.candidates, candidate.epoch)
	cleanup := candidate.cleanup
	candidate.cleanup = nil
	candidate.credential = ""
	candidate.host = ""
	candidate.port = 0
	candidate.processID = 0
	return cleanup
}

// MarkUnexpectedExit reports exit from the process associated with this
// candidate. A delayed callback from an aborted or retired epoch is ignored.
func (candidate *RuntimeBindingCandidate) MarkUnexpectedExit() bool {
	return candidate.MarkUnexpectedExitWithReason(AvailabilityReasonAgentctlExited)
}

// MarkUnexpectedExitWithReason fences the exact candidate epoch after an
// exit signal. Unverified cleanup never grants the coordinator authority to
// replace or terminate another process.
func (candidate *RuntimeBindingCandidate) MarkUnexpectedExitWithReason(reason string) bool {
	if candidate == nil || candidate.owner == nil {
		return false
	}
	if !knownAvailabilityReason(reason) {
		reason = AvailabilityReasonOwnershipUnverified
	}
	o := candidate.owner
	o.mu.Lock()
	if candidate.state == runtimeCandidatePrepared {
		candidate.exited = true
		candidate.exitReason = reason
		o.mu.Unlock()
		return true
	}
	if candidate.state != runtimeCandidateCommitted || o.active != candidate.binding || o.stopping {
		o.mu.Unlock()
		return false
	}
	binding := o.active
	o.active = nil
	startCleanup := o.retireBindingLocked(binding)
	snapshot := o.setSnapshotLocked(AvailabilityStatusUnavailable, reason, binding.epoch)
	o.queuePublicationLocked(snapshot)
	o.mu.Unlock()
	binding.cancelLeases()
	if startCleanup != nil {
		startCleanup()
	}
	return true
}

// MarkAvailable preserves the legacy initial-state helper used by status
// tests. Production startup publishes a configured binding with Commit.
func (o *RuntimeOwner) MarkAvailable() {
	o.mu.Lock()
	if o.stopping || (o.published && o.snapshot.Status != "") {
		o.mu.Unlock()
		return
	}
	snapshot := o.setSnapshotLocked(AvailabilityStatusAvailable, "", o.snapshot.RuntimeEpoch)
	o.queuePublicationLocked(snapshot)
	o.mu.Unlock()
}

// MarkUnavailable records an unexpected exit for the currently published
// epoch. New code should use the epoch-bound candidate callback.
func (o *RuntimeOwner) MarkUnavailable() {
	o.mu.RLock()
	epoch := o.snapshot.RuntimeEpoch
	o.mu.RUnlock()
	o.MarkUnavailableEpoch(epoch, AvailabilityReasonAgentctlExited)
}

// MarkUnavailableEpoch retires only the named current binding. It returns
// false for stale callbacks and after intentional shutdown.
func (o *RuntimeOwner) MarkUnavailableEpoch(epoch uint64, reason string) bool {
	o.mu.Lock()
	if o.stopping || !o.published || o.snapshot.RuntimeEpoch != epoch || o.snapshot.Status == AvailabilityStatusUnavailable {
		o.mu.Unlock()
		return false
	}
	var binding *runtimeBinding
	var startCleanup func()
	if o.active != nil {
		if o.active.epoch != epoch {
			o.mu.Unlock()
			return false
		}
		binding = o.active
		o.active = nil
		startCleanup = o.retireBindingLocked(binding)
	} else if epoch != 0 {
		o.mu.Unlock()
		return false
	}
	if !knownAvailabilityReason(reason) {
		reason = AvailabilityReasonAgentctlExited
	}
	snapshot := o.setSnapshotLocked(AvailabilityStatusUnavailable, reason, epoch)
	o.queuePublicationLocked(snapshot)
	o.mu.Unlock()
	if binding != nil {
		binding.cancelLeases()
	}
	if startCleanup != nil {
		startCleanup()
	}
	return true
}

func (o *RuntimeOwner) retireBindingLocked(binding *runtimeBinding) func() {
	if binding == nil || binding.cleanup == nil {
		return nil
	}
	o.retiredCleanup.Add(1)
	return func() {
		go func() {
			defer o.retiredCleanup.Done()
			if err := binding.cleanup(); err != nil && o.logger != nil {
				o.logger.Warn("failed to close retired agent runtime binding",
					zap.Uint64("runtime_epoch", binding.epoch), zap.Error(err))
			}
		}()
	}
}

// BeginRecovery publishes the recovering state only for the unavailable
// runtime epoch that triggered the episode. Local admission was already
// fenced when that epoch was retired.
func (o *RuntimeOwner) BeginRecovery(epoch uint64, reason, recoveryID string) bool {
	if recoveryID == "" {
		return false
	}
	o.mu.Lock()
	if o.stopping || !o.published || o.snapshot.Status != AvailabilityStatusUnavailable ||
		o.snapshot.RuntimeEpoch != epoch {
		o.mu.Unlock()
		return false
	}
	if !knownAvailabilityReason(reason) {
		reason = AvailabilityReasonOwnershipUnverified
	}
	previousTime := o.snapshot.OccurredAt
	snapshot := o.setSnapshotLocked(AvailabilityStatusRecovering, reason, epoch)
	snapshot.OccurredAt = previousTime
	snapshot.RecoveryID = recoveryID
	snapshot.RetryAllowed = false
	o.snapshot = snapshot
	o.queuePublicationLocked(snapshot)
	o.mu.Unlock()
	return true
}

// BeginManualRecovery transitions an exhausted outage into a new episode
// after the caller has authenticated an explicit operator request.
func (o *RuntimeOwner) BeginManualRecovery(bootID string, epoch uint64, recoveryID string) (AvailabilitySnapshot, error) {
	if recoveryID == "" {
		return AvailabilitySnapshot{}, ErrRuntimeCandidateState
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stopping {
		return AvailabilitySnapshot{}, ErrRuntimeOwnerStopped
	}
	if !o.published || o.snapshot.BootID != bootID || o.snapshot.RuntimeEpoch != epoch {
		return AvailabilitySnapshot{}, ErrRecoveryConflict
	}
	if o.snapshot.Status == AvailabilityStatusRecovering {
		return cloneAvailabilitySnapshot(o.snapshot), nil
	}
	if o.snapshot.Status != AvailabilityStatusUnavailable || !o.snapshot.RetryAllowed {
		return AvailabilitySnapshot{}, ErrRecoveryNotRetryable
	}
	previousTime := o.snapshot.OccurredAt
	snapshot := o.setSnapshotLocked(AvailabilityStatusRecovering, AvailabilityReasonAgentctlExited, epoch)
	snapshot.OccurredAt = previousTime
	snapshot.RecoveryID = recoveryID
	o.snapshot = snapshot
	o.queuePublicationLocked(snapshot)
	return cloneAvailabilitySnapshot(snapshot), nil
}

// FinishRecovery leaves the exact failed epoch unavailable. retryAllowed is
// true only after a safely replaceable outage exhausts its automatic budget.
func (o *RuntimeOwner) FinishRecovery(epoch uint64, recoveryID, reason string, retryAllowed bool) bool {
	o.mu.Lock()
	if o.stopping || o.active != nil || !o.published || o.snapshot.Status != AvailabilityStatusRecovering ||
		o.snapshot.RuntimeEpoch != epoch || o.snapshot.RecoveryID != recoveryID {
		o.mu.Unlock()
		return false
	}
	if !knownAvailabilityReason(reason) {
		reason = AvailabilityReasonRecoveryExhausted
	}
	previousTime := o.snapshot.OccurredAt
	snapshot := o.setSnapshotLocked(AvailabilityStatusUnavailable, reason, epoch)
	snapshot.OccurredAt = previousTime
	snapshot.RecoveryID = recoveryID
	snapshot.RetryAllowed = retryAllowed
	o.snapshot = snapshot
	o.queuePublicationLocked(snapshot)
	o.mu.Unlock()
	return true
}

func knownAvailabilityReason(reason string) bool {
	switch reason {
	case AvailabilityReasonAgentctlExited, AvailabilityReasonOwnershipUnverified,
		AvailabilityReasonStartFailed, AvailabilityReasonRecoveryExhausted:
		return true
	default:
		return false
	}
}

func (o *RuntimeOwner) setSnapshotLocked(status, reason string, epoch uint64) AvailabilitySnapshot {
	recoveryID := ""
	if status == AvailabilityStatusAvailable && o.snapshot.Status == AvailabilityStatusRecovering {
		recoveryID = o.snapshot.RecoveryID
	}
	o.snapshot = AvailabilitySnapshot{
		Status:       status,
		Reason:       reason,
		BootID:       o.bootID,
		Revision:     o.snapshot.Revision + 1,
		RuntimeEpoch: epoch,
		RecoveryID:   recoveryID,
	}
	if status == AvailabilityStatusUnavailable {
		now := time.Now().UTC()
		o.snapshot.OccurredAt = &now
	}
	o.published = true
	return cloneAvailabilitySnapshot(o.snapshot)
}

// Acquire returns a lease for the current authenticated runtime binding.
func (o *RuntimeOwner) Acquire(ctx context.Context) (*RuntimeLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	o.mu.RLock()
	if o.stopping || o.active == nil || !o.published || o.snapshot.Status != AvailabilityStatusAvailable {
		o.mu.RUnlock()
		return nil, ErrRuntimeUnavailable
	}
	binding := o.active
	binding.leaseMu.Lock()
	if binding.ctx.Err() != nil {
		binding.leaseMu.Unlock()
		o.mu.RUnlock()
		return nil, ErrRuntimeUnavailable
	}
	binding.leaseNext++
	leaseID := binding.leaseNext
	leaseCtx, cancel := context.WithCancel(ctx)
	binding.leases[leaseID] = cancel
	binding.leaseMu.Unlock()
	o.mu.RUnlock()
	return &RuntimeLease{
		owner:   o,
		binding: binding,
		id:      leaseID,
		ctx:     leaseCtx,
		cancel:  cancel,
	}, nil
}

// Context is canceled when either the caller or this binding is retired.
func (lease *RuntimeLease) Context() context.Context {
	if lease == nil || lease.ctx == nil {
		return context.Background()
	}
	return lease.ctx
}

// Epoch identifies the immutable local-runtime generation held by the lease.
func (lease *RuntimeLease) Epoch() uint64 {
	if lease == nil || lease.binding == nil {
		return 0
	}
	return lease.binding.epoch
}

// BootID identifies the backend process that issued this lease.
func (lease *RuntimeLease) BootID() string {
	if lease == nil || lease.binding == nil {
		return ""
	}
	return lease.binding.bootID
}

// BootID returns the identity shared with the rest of this backend process.
func (o *RuntimeOwner) BootID() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.bootID
}

// IsEpochCurrent reports whether epoch still identifies the published local
// runtime binding.
func (o *RuntimeOwner) IsEpochCurrent(epoch uint64) bool {
	if epoch == 0 {
		return false
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return !o.stopping && o.active != nil && o.active.epoch == epoch &&
		o.snapshot.Status == AvailabilityStatusAvailable
}

// ProcessID returns the host process id for internal liveness coordination.
func (lease *RuntimeLease) ProcessID() int {
	if lease == nil || lease.binding == nil {
		return 0
	}
	return lease.binding.processID
}

// ProcessIdentity returns the OS birth and containment identity bound to the
// lease. Empty or legacy identity data is not sufficient for process cleanup.
func (lease *RuntimeLease) ProcessIdentity() processidentity.Identity {
	if lease == nil || lease.binding == nil {
		return processidentity.Identity{}
	}
	return lease.binding.processIdentity
}

// NewBoundInstanceClient constructs an authenticated client for an instance
// served by this runtime generation.
func (lease *RuntimeLease) NewBoundInstanceClient(port int, log *logger.Logger, opts ...ClientOption) *Client {
	if lease == nil || lease.binding == nil {
		return nil
	}
	opts = append(opts, WithAuthToken(lease.binding.credential), WithRuntimeLease(lease))
	return NewClient(lease.binding.host, port, log, opts...)
}

// IsCurrent reports whether the lease still names the published binding.
func (lease *RuntimeLease) IsCurrent() bool {
	if lease == nil || lease.owner == nil || lease.binding == nil || lease.ctx.Err() != nil {
		return false
	}
	lease.owner.mu.RLock()
	defer lease.owner.mu.RUnlock()
	return !lease.owner.stopping && lease.owner.active == lease.binding &&
		lease.owner.snapshot.Status == AvailabilityStatusAvailable
}

// CheckCurrent rejects results from a retired binding before callers apply
// them to durable state or successor-owned caches.
func (lease *RuntimeLease) CheckCurrent() error {
	if !lease.IsCurrent() {
		return ErrRuntimeLeaseRetired
	}
	return nil
}

// NewControlClient constructs a control client without exposing the binding
// credential to its caller.
func (lease *RuntimeLease) NewControlClient(log *logger.Logger) *ControlClient {
	if lease == nil || lease.binding == nil {
		return nil
	}
	return newRuntimeBoundControlClient(lease, log)
}

// NewInstanceClient constructs an authenticated per-instance client without
// exposing the binding credential to its caller.
func (lease *RuntimeLease) NewInstanceClient(host string, port int, log *logger.Logger, opts ...ClientOption) *Client {
	if lease == nil || lease.binding == nil {
		return nil
	}
	opts = append(opts, WithAuthToken(lease.binding.credential), WithRuntimeLease(lease))
	return NewClient(host, port, log, opts...)
}

// Close releases the caller-owned lease context.
func (lease *RuntimeLease) Close() {
	if lease == nil {
		return
	}
	lease.closeOnce.Do(func() {
		if lease.binding != nil {
			lease.binding.leaseMu.Lock()
			delete(lease.binding.leases, lease.id)
			lease.binding.leaseMu.Unlock()
		}
		if lease.cancel != nil {
			lease.cancel()
		}
	})
}

// Stop closes runtime admission and invalidates outstanding leases without
// publishing an outage during intentional backend shutdown.
func (o *RuntimeOwner) Stop() {
	o.mu.Lock()
	if o.stopped {
		o.mu.Unlock()
		return
	}
	o.stopping = true
	o.stopped = true
	cleanups := make([]func() error, 0, len(o.candidates)+1)
	var activeBinding *runtimeBinding
	if o.active != nil {
		binding := o.active
		o.active = nil
		activeBinding = binding
		if binding.cleanup != nil {
			cleanups = append(cleanups, binding.cleanup)
		}
	}
	for _, candidate := range o.candidates {
		if cleanup := o.abortCandidateLocked(candidate); cleanup != nil {
			cleanups = append(cleanups, cleanup)
		}
	}
	o.mu.Unlock()
	if activeBinding != nil {
		activeBinding.cancelLeases()
	}
	if o.publicationStop != nil {
		o.publicationOnce.Do(func() { close(o.publicationStop) })
	}
	for _, cleanup := range cleanups {
		if err := cleanup(); err != nil && o.logger != nil {
			o.logger.Warn("failed to close unpublished agent runtime candidate", zap.Error(err))
		}
	}
	o.retiredCleanup.Wait()
}

// PreventNewBindings closes publication and lease admission before a
// coordinator cancels an in-flight candidate. Stop performs resource cleanup.
func (o *RuntimeOwner) PreventNewBindings() {
	o.mu.Lock()
	o.stopping = true
	o.mu.Unlock()
}

func (o *RuntimeOwner) queuePublicationLocked(snapshot AvailabilitySnapshot) {
	if o.eventBus == nil || o.stopping {
		return
	}
	o.publications = append(o.publications, cloneAvailabilitySnapshot(snapshot))
	select {
	case o.publicationSignal <- struct{}{}:
	default:
	}
}

func (o *RuntimeOwner) runPublisher() {
	for {
		select {
		case <-o.publicationSignal:
		case <-o.publicationStop:
		}
		for {
			o.mu.Lock()
			if o.stopping {
				o.publications = nil
				o.mu.Unlock()
				return
			}
			if len(o.publications) == 0 {
				o.mu.Unlock()
				break
			}
			snapshot := o.publications[0]
			o.publications[0] = AvailabilitySnapshot{}
			o.publications = o.publications[1:]
			o.mu.Unlock()
			o.publishSnapshot(snapshot)
		}
	}
}

func (o *RuntimeOwner) publishSnapshot(snapshot AvailabilitySnapshot) {
	if err := o.eventBus.Publish(
		context.Background(),
		events.AgentRuntimeAvailabilityChanged,
		bus.NewEvent(events.AgentRuntimeAvailabilityChanged, "agentctl", snapshot),
	); err != nil && o.logger != nil {
		o.logger.Warn("failed to publish agent runtime availability", zap.Error(err))
	}
}
