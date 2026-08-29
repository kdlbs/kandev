package github

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

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
		if rate.Remaining <= 0 {
			decision.interactiveAllowed = false
			decision.backgroundAllowed = false
			decision.interactiveReason = rateLimitBlockPrimary
			decision.backgroundReason = rateLimitBlockPrimary
			return decision
		}
		if rate.Limit > 0 && rate.Remaining*10 <= rate.Limit {
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

func WithGitHubWorkClass(ctx context.Context, class WorkClass) context.Context {
	return context.WithValue(ctx, workClassContextKey{}, class)
}

func githubWorkClass(ctx context.Context) WorkClass {
	if class, ok := ctx.Value(workClassContextKey{}).(WorkClass); ok && class == WorkClassBackground {
		return class
	}
	return WorkClassInteractive
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
		return a.acquireBackground(ctx, resource)
	}
	return a.acquireInteractive(ctx, resource)
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
	for {
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
		stateChanged := state.changed
		a.principal.mu.Unlock()
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
		reserve := (snap.Limit + 9) / 10
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
