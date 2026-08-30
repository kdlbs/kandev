package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

// ErrBackgroundAdmissionDeferred marks a background request that should be
// requeued instead of waiting inside an execution worker.
var ErrBackgroundAdmissionDeferred = errors.New("background provider admission deferred")

// AdmissionDeferredError describes the local admission signal that will wake
// a requeued background request.
type AdmissionDeferredError struct {
	Resource       Resource
	Delay          time.Duration
	Changed        <-chan struct{}
	TrackerChanged <-chan struct{}
	Reason         string
}

func (e *AdmissionDeferredError) Error() string {
	return fmt.Sprintf("%s: %s", ErrBackgroundAdmissionDeferred, e.Reason)
}

// Unwrap supports errors.Is and errors.As through provider-specific wrappers.
func (e *AdmissionDeferredError) Unwrap() error { return ErrBackgroundAdmissionDeferred }

// Wait blocks until admission changes, the retry boundary elapses, or ctx is
// cancelled. It owns no execution-pool capacity while waiting.
func (e *AdmissionDeferredError) Wait(ctx context.Context) error {
	return waitForAdmissionChange(ctx, e.Delay, e.TrackerChanged, e.Changed)
}

// RateCoordinator owns rate observations by GitHub's upstream quota identity,
// rather than by Kandev workspace or credential generation.
type RateCoordinator struct {
	mu         sync.Mutex
	principals map[string]*ratePrincipalState
	eventBus   bus.EventBus
	logger     *logger.Logger
}

type ratePrincipalState struct {
	tracker   *RateTracker
	logger    *logger.Logger
	mu        sync.Mutex
	resources map[Resource]*rateAdmissionState
}

type rateAdmissionState struct {
	backgroundBusy     bool
	waitingInteractive int
	nextBackgroundAt   time.Time
	changed            chan struct{}
}

func (a *RateAdmission) snapshot(resource Resource, now time.Time) rateAdmissionDecision {
	if a == nil || a.principal == nil || a.principal.tracker == nil {
		return rateAdmissionDecision{interactiveAllowed: true, backgroundAllowed: true}
	}
	decision := rateAdmissionDecision{interactiveAllowed: true, backgroundAllowed: true}
	if secondary := a.principal.tracker.Secondary(resource); secondary.RetryAt.After(now) {
		decision.interactiveAllowed = false
		decision.backgroundAllowed = false
		decision.interactiveReason = rateLimitBlockSecondary
		decision.backgroundReason = rateLimitBlockSecondary
		return decision
	}
	if rate, known := a.principal.tracker.Snapshot(resource); known && rate.ResetAt.After(now) {
		if primaryRateExhausted(rate) {
			decision.interactiveAllowed = false
			decision.backgroundAllowed = false
			decision.interactiveReason = rateLimitBlockPrimary
			decision.backgroundReason = rateLimitBlockPrimary
			return decision
		}
		if interactiveReserveReached(rate) {
			decision.backgroundAllowed = false
			decision.backgroundReason = rateLimitBlockPrimaryReserve
		}
	}

	a.principal.mu.Lock()
	state := a.principal.resources[resource]
	if state != nil && decision.backgroundAllowed {
		switch {
		case state.waitingInteractive > 0:
			decision.backgroundAllowed = false
			decision.backgroundReason = rateLimitBlockInteractiveWaiting
		case state.backgroundBusy:
			decision.backgroundAllowed = false
			decision.backgroundReason = rateLimitBlockBackgroundBusy
		case state.nextBackgroundAt.After(now):
			decision.backgroundAllowed = false
			decision.backgroundReason = rateLimitBlockBackgroundPacing
		}
	}
	a.principal.mu.Unlock()
	return decision
}

func primaryRateExhausted(rate RateSnapshot) bool {
	return rate.Remaining <= 0 && (rate.RemainingObserved || !rate.ParsedFromHeaders)
}

const defaultBackgroundPace = time.Second

type RateAdmission struct {
	principal *ratePrincipalState
}

type WorkClass string

const (
	WorkClassInteractive WorkClass = "interactive"
	WorkClassBackground  WorkClass = "background"
)

type workClassContextKey struct{}

type nonBlockingAdmissionContextKey struct{}

func WithGitHubWorkClass(ctx context.Context, class WorkClass) context.Context {
	return context.WithValue(ctx, workClassContextKey{}, class)
}

// WithNonBlockingGitHubAdmission asks background GitHub clients to return a
// deferred-admission error instead of blocking the caller.
func WithNonBlockingGitHubAdmission(ctx context.Context) context.Context {
	return context.WithValue(ctx, nonBlockingAdmissionContextKey{}, true)
}

func githubWorkClass(ctx context.Context) WorkClass {
	if class, ok := ctx.Value(workClassContextKey{}).(WorkClass); ok && class == WorkClassBackground {
		return class
	}
	return WorkClassInteractive
}

func nonBlockingGitHubAdmission(ctx context.Context) bool {
	value, _ := ctx.Value(nonBlockingAdmissionContextKey{}).(bool)
	return value
}

func NewRateCoordinator(eventBus bus.EventBus, log *logger.Logger) *RateCoordinator {
	return &RateCoordinator{
		principals: make(map[string]*ratePrincipalState),
		eventBus:   eventBus,
		logger:     log,
	}
}

func (c *RateCoordinator) coordinate(
	host string,
	principal AuthPrincipal,
	preferred *RateTracker,
) (*RateTracker, *RateAdmission) {
	key := ratePrincipalKey(host, principal)
	c.mu.Lock()
	defer c.mu.Unlock()
	if state := c.principals[key]; state != nil {
		return state.tracker, &RateAdmission{principal: state}
	}
	if preferred == nil {
		preferred = NewRateTracker(c.eventBus, c.logger)
	}
	state := &ratePrincipalState{
		tracker:   preferred,
		logger:    c.logger,
		resources: make(map[Resource]*rateAdmissionState),
	}
	c.principals[key] = state
	return preferred, &RateAdmission{principal: state}
}

func (a *RateAdmission) acquire(ctx context.Context, resource Resource) (func(), error) {
	if a == nil || a.principal == nil || a.principal.tracker == nil {
		return func() {}, nil
	}
	if githubWorkClass(ctx) == WorkClassBackground {
		if nonBlockingGitHubAdmission(ctx) {
			return a.tryAcquireBackground(ctx, resource)
		}
		return a.acquireBackground(ctx, resource)
	}
	return a.acquireInteractive(ctx, resource)
}

func (a *RateAdmission) tryAcquireBackground(_ context.Context, resource Resource) (func(), error) {
	decision := a.snapshot(resource, time.Now())
	state := a.resourceState(resource)
	wait := a.principal.tracker.BackgroundWaitDuration(resource)
	a.principal.mu.Lock()
	if paceWait := time.Until(state.nextBackgroundAt); paceWait > wait {
		wait = paceWait
	}
	if wait <= 0 && !state.backgroundBusy && state.waitingInteractive == 0 {
		state.backgroundBusy = true
		a.principal.mu.Unlock()
		return func() { a.releaseBackground(resource, state) }, nil
	}
	changed := state.changed
	waitingInteractive := state.waitingInteractive
	backgroundBusy := state.backgroundBusy
	nextBackgroundAt := state.nextBackgroundAt
	a.principal.mu.Unlock()
	reason := decision.backgroundReason
	if reason == "" {
		switch {
		case waitingInteractive > 0:
			reason = rateLimitBlockInteractiveWaiting
		case backgroundBusy:
			reason = rateLimitBlockBackgroundBusy
		case nextBackgroundAt.After(time.Now()):
			reason = rateLimitBlockBackgroundPacing
		default:
			reason = "provider_retry"
		}
	}
	incGitHubBackgroundDeferral(resource, reason)
	return nil, &AdmissionDeferredError{
		Resource: resource, Delay: wait, Changed: changed,
		TrackerChanged: a.principal.tracker.Changed(), Reason: reason,
	}
}

func (a *RateAdmission) acquireInteractive(ctx context.Context, resource Resource) (func(), error) {
	state := a.resourceState(resource)
	a.principal.mu.Lock()
	state.waitingInteractive++
	a.signalStateLocked(state)
	a.principal.mu.Unlock()
	_, err := a.waitForProviderWindow(ctx, resource)
	if err != nil {
		a.releaseInteractive(state)
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { a.releaseInteractive(state) }) }, nil
}

func (a *RateAdmission) acquireBackground(ctx context.Context, resource Resource) (func(), error) {
	state := a.resourceState(resource)
	deferralRecorded := false
	for {
		decision := a.snapshot(resource, time.Now())
		wait := a.principal.tracker.BackgroundWaitDuration(resource)
		a.principal.mu.Lock()
		if paceWait := time.Until(state.nextBackgroundAt); paceWait > wait {
			wait = paceWait
		}
		eligible := wait <= 0 && !state.backgroundBusy && state.waitingInteractive == 0
		if eligible {
			state.backgroundBusy = true
			a.principal.mu.Unlock()
			return func() { a.releaseBackground(resource, state) }, nil
		}
		var deferralReason string
		if !deferralRecorded {
			deferralReason = decision.backgroundReason
			switch {
			case deferralReason != "":
			case state.waitingInteractive > 0:
				deferralReason = rateLimitBlockInteractiveWaiting
			case state.backgroundBusy:
				deferralReason = rateLimitBlockBackgroundBusy
			case state.nextBackgroundAt.After(time.Now()):
				deferralReason = rateLimitBlockBackgroundPacing
			default:
				deferralReason = "provider_retry"
			}
			deferralRecorded = true
		}
		stateChanged := state.changed
		a.principal.mu.Unlock()
		if deferralReason != "" {
			incGitHubBackgroundDeferral(resource, deferralReason)
			if a.principal.logger != nil {
				a.principal.logger.Debug("github background request deferred",
					zap.String("resource", string(resource)),
					zap.String("reason", deferralReason),
					zap.Duration("wait", wait))
			}
		}
		if err := waitForAdmissionChange(ctx, wait, a.principal.tracker.Changed(), stateChanged); err != nil {
			return nil, err
		}
	}
}

func (a *RateAdmission) waitForProviderWindow(ctx context.Context, resource Resource) (func(), error) {
	for {
		wait := a.principal.tracker.WaitDuration(resource)
		if wait <= 0 {
			return func() {}, nil
		}
		if err := waitForAdmissionChange(ctx, wait, a.principal.tracker.Changed(), nil); err != nil {
			return nil, err
		}
	}
}

func (a *RateAdmission) resourceState(resource Resource) *rateAdmissionState {
	a.principal.mu.Lock()
	defer a.principal.mu.Unlock()
	state := a.principal.resources[resource]
	if state == nil {
		state = &rateAdmissionState{changed: make(chan struct{})}
		a.principal.resources[resource] = state
	}
	return state
}

func (a *RateAdmission) releaseBackground(resource Resource, state *rateAdmissionState) {
	a.principal.mu.Lock()
	state.backgroundBusy = false
	state.nextBackgroundAt = time.Now().Add(a.backgroundPace(resource))
	a.signalStateLocked(state)
	a.principal.mu.Unlock()
}

func (a *RateAdmission) releaseInteractive(state *rateAdmissionState) {
	a.principal.mu.Lock()
	state.waitingInteractive--
	a.signalStateLocked(state)
	a.principal.mu.Unlock()
}

func (a *RateAdmission) backgroundPace(resource Resource) time.Duration {
	pace := defaultBackgroundPace
	if snap, ok := a.principal.tracker.Snapshot(resource); ok {
		reserve := snap.BackgroundReserve()
		spendable := snap.Remaining - reserve
		if window := time.Until(snap.ResetAt); spendable > 0 && window > 0 {
			if derived := window / time.Duration(spendable); derived > pace {
				pace = derived
			}
		}
	}
	return pace
}

func (a *RateAdmission) signalStateLocked(state *rateAdmissionState) {
	close(state.changed)
	state.changed = make(chan struct{})
}

func waitForAdmissionChange(
	ctx context.Context,
	wait time.Duration,
	trackerChanged <-chan struct{},
	stateChanged <-chan struct{},
) error {
	if wait <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-trackerChanged:
			return nil
		case <-stateChanged:
			return nil
		}
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-trackerChanged:
		return nil
	case <-stateChanged:
		return nil
	case <-timer.C:
		return nil
	}
}

func ratePrincipalKey(host string, principal AuthPrincipal) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		host = defaultGitHubHost
	}
	if principal.Kind == AuthPrincipalApp {
		return fmt.Sprintf(
			"app:%s:%s:%d",
			host,
			strings.ToLower(strings.TrimSpace(principal.AppRegistrationID)),
			principal.InstallationID,
		)
	}
	login := strings.ToLower(strings.TrimSpace(principal.Login))
	if login == "" {
		login = "workspace:" + principal.WorkspaceID
	}
	return "human:" + host + ":" + login
}
