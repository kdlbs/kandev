package lifecycle

import (
	"context"
	"sync"
	"time"
)

const (
	// deliveryStartupAuthorityBudget bounds the SQL authority wait after a
	// native generation-one session has been created. It never starts before
	// native initialization completes.
	deliveryStartupAuthorityBudget = 30 * time.Second
	deliveryStartupAuthorityProbe  = 5 * time.Second
	deliveryStartupPollInterval    = 25 * time.Millisecond
)

// deliveryStartupAttempt owns one InitializeAndPromptWithLayers attempt.
// Delivery negotiation and native initialization complete separately because
// setup events can arrive after negotiation but before the native session
// exists. Every connected processor retains the attempt it started with.
type deliveryStartupAttempt struct {
	ctx               context.Context
	startupGeneration uint64

	negotiationOnce sync.Once
	negotiationDone chan struct{}
	negotiationErr  error

	nativeOnce sync.Once
	nativeDone chan struct{}
	nativeErr  error
	nativeAt   time.Time
}

func (e *AgentExecution) beginDeliveryStartupAttempt(ctx context.Context) *deliveryStartupAttempt {
	if ctx == nil {
		ctx = context.Background()
	}
	attempt := &deliveryStartupAttempt{
		ctx:               ctx,
		startupGeneration: e.startupAttemptSnapshot(),
		negotiationDone:   make(chan struct{}),
		nativeDone:        make(chan struct{}),
	}
	e.deliveryStartup.Store(attempt)
	return attempt
}

// releaseDeliveryStartupAttempt clears a failed attempt. A successful attempt
// stays visible so queued or replayed setup events keep its native completion
// anchor until SQL authority clears bootstrap admission. A replacement startup
// still stores its own attempt and late completion cannot disturb it.
func (e *AgentExecution) releaseDeliveryStartupAttempt(attempt *deliveryStartupAttempt) {
	if e == nil || attempt == nil || attempt.nativeSucceeded() {
		return
	}
	e.deliveryStartup.CompareAndSwap(attempt, nil)
}

func (a *deliveryStartupAttempt) nativeSucceeded() bool {
	if a == nil {
		return false
	}
	select {
	case <-a.nativeDone:
		return a.nativeErr == nil
	default:
		return false
	}
}

func (e *AgentExecution) deliveryStartupSnapshot() *deliveryStartupAttempt {
	if e == nil {
		return nil
	}
	return e.deliveryStartup.Load()
}

// deliveryStreamStartupSnapshot returns one consistent startup generation and
// attempt pair for a new connection. A replacement startup cannot split the
// pair across the two reads.
func (e *AgentExecution) deliveryStreamStartupSnapshot() (uint64, *deliveryStartupAttempt) {
	attempt := e.deliveryStartupSnapshot()
	if attempt != nil {
		return attempt.startupGeneration, attempt
	}
	return e.startupAttemptSnapshot(), nil
}

func (a *deliveryStartupAttempt) finishNegotiation(err error) {
	if a == nil {
		return
	}
	a.negotiationOnce.Do(func() {
		a.negotiationErr = err
		close(a.negotiationDone)
	})
}

func (a *deliveryStartupAttempt) finishNative(err error) {
	if a == nil {
		return
	}
	a.nativeOnce.Do(func() {
		a.nativeErr = err
		if err == nil {
			a.nativeAt = time.Now()
		}
		close(a.nativeDone)
	})
	if err != nil {
		a.finishNegotiation(err)
	}
}

// fail completes both results for an attempt that did not reach native
// success. It is idempotent and cannot overwrite a published success.
func (a *deliveryStartupAttempt) fail(err error) {
	if a == nil {
		return
	}
	if err == nil {
		err = ErrDurableAdoptionBlocked
	}
	a.finishNative(err)
	a.finishNegotiation(err)
}

func (a *deliveryStartupAttempt) nativeCompletedAt() time.Time {
	if a == nil {
		return time.Time{}
	}
	return a.nativeAt
}

// applyDeliveryBootstrapReset runs a bootstrap flag mutation under the
// attempt's startup ownership lease. The ownership check and mutation are one
// operation, so a successor cannot advance between them and inherit a cleared
// bootstrap. Direct callers without an attempt keep the unconditional reset.
func (execution *AgentExecution) applyDeliveryBootstrapReset(attempt *deliveryStartupAttempt, reset func()) bool {
	if execution == nil || reset == nil {
		return false
	}
	if attempt == nil {
		reset()
		return true
	}
	return execution.withStartupAttempt(attempt.startupGeneration, func(string) { reset() })
}

// clearDeliveryBootstrapForAttempt closes bootstrap admission only for the
// attempt that owns it.
func (execution *AgentExecution) clearDeliveryBootstrapForAttempt(attempt *deliveryStartupAttempt) {
	execution.applyDeliveryBootstrapReset(attempt, func() {
		execution.deliveryBootstrap.Store(false)
	})
}

// awaitNegotiation waits for the attempt's delivery negotiation under its
// initialization owner. It has no independent event deadline: the existing
// initialize RPC limits and the caller context bound it.
func (a *deliveryStartupAttempt) awaitNegotiation(ctx context.Context) error {
	if a == nil {
		return nil
	}
	select {
	case <-a.negotiationDone:
		return a.negotiationErr
	default:
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var ownerDone <-chan struct{}
	if a.ctx != nil {
		ownerDone = a.ctx.Done()
	}
	select {
	case <-a.negotiationDone:
		return a.negotiationErr
	case <-ownerDone:
	case <-ctx.Done():
	}
	select {
	case <-a.negotiationDone:
		return a.negotiationErr
	default:
	}
	if a.ctx != nil && a.ctx.Err() != nil {
		return a.ctx.Err()
	}
	return ctx.Err()
}

// awaitNative waits for this attempt's native session result. Initialization
// failure releases the wait with that cause; caller and stream cancellation
// release it with their own.
func (a *deliveryStartupAttempt) awaitNative(ctx context.Context) error {
	if a == nil {
		return nil
	}
	select {
	case <-a.nativeDone:
		return a.nativeErr
	default:
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var ownerDone <-chan struct{}
	if a.ctx != nil {
		ownerDone = a.ctx.Done()
	}
	select {
	case <-a.nativeDone:
		return a.nativeErr
	case <-ownerDone:
	case <-ctx.Done():
	}
	select {
	case <-a.nativeDone:
		return a.nativeErr
	default:
	}
	if a.ctx != nil && a.ctx.Err() != nil {
		return a.ctx.Err()
	}
	return ctx.Err()
}
