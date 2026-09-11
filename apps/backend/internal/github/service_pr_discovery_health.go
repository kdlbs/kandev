package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const (
	PRDiscoveryRetryBase  = time.Minute
	prDiscoveryRetryMax   = 15 * time.Minute
	prDiscoveryMaxScopes  = 256
	prDiscoveryMaxTargets = 4096
)

// Milliseconds remain exactly representable in JavaScript while preserving
// process-start ordering for stale HTTP/WS event rejection.
var prDiscoveryRuntimeEpochSeed = time.Now().UnixMilli()

type PRDiscoveryHealthState string

const (
	PRDiscoveryHealthUnknown  PRDiscoveryHealthState = "unknown"
	PRDiscoveryHealthHealthy  PRDiscoveryHealthState = "healthy"
	PRDiscoveryHealthDegraded PRDiscoveryHealthState = "degraded"
)

type PRDiscoveryHealthCategory string

const (
	PRDiscoveryHealthInvalidQuery PRDiscoveryHealthCategory = "invalid_query"
	PRDiscoveryHealthRateLimited  PRDiscoveryHealthCategory = "rate_limited"
	PRDiscoveryHealthUnavailable  PRDiscoveryHealthCategory = "unavailable"
)

// PRDiscoveryHealth is the bounded, credential-scoped projection of the
// provider's PR discovery path. It intentionally contains categories and
// timestamps only; provider payloads and credentials never cross this boundary.
type PRDiscoveryHealth struct {
	State                PRDiscoveryHealthState    `json:"state"`
	FailedTargetCount    int                       `json:"failed_target_count"`
	LastFailureAt        *time.Time                `json:"last_failure_at,omitempty"`
	Category             PRDiscoveryHealthCategory `json:"category,omitempty"`
	RetryAt              *time.Time                `json:"retry_at,omitempty"`
	Revision             uint64                    `json:"revision"`
	CredentialGeneration int64                     `json:"credential_generation"`
	RuntimeEpoch         int64                     `json:"runtime_epoch"`
}

// PRDiscoveryHealthUpdatedEvent is workspace-scoped and carries the same
// revisioned projection returned by the status endpoint.
type PRDiscoveryHealthUpdatedEvent struct {
	WorkspaceID string             `json:"workspace_id"`
	Health      *PRDiscoveryHealth `json:"health"`
}

func (e *PRDiscoveryHealthUpdatedEvent) GetWorkspaceID() string {
	if e == nil {
		return ""
	}
	return e.WorkspaceID
}

type prDiscoveryHealthTarget struct {
	Owner    string
	Repo     string
	Branch   string
	PRNumber int
}

func (t prDiscoveryHealthTarget) key() string {
	if t.PRNumber > 0 {
		return fmt.Sprintf("pr:%s/%s#%d", strings.ToLower(t.Owner), strings.ToLower(t.Repo), t.PRNumber)
	}
	return fmt.Sprintf("branch:%s/%s@%s", strings.ToLower(t.Owner), strings.ToLower(t.Repo), t.Branch)
}

type prDiscoveryHealthTargetState struct {
	attempt       uint64
	active        bool
	failureCount  int
	category      PRDiscoveryHealthCategory
	lastFailureAt time.Time
	retryAt       *time.Time
	consumers     map[string]struct{}
}

type prDiscoveryHealthScope struct {
	workspaceID          string
	cacheScope           string
	credentialGeneration int64
	nextAttempt          uint64
	revision             uint64
	seen                 bool
	targets              map[string]*prDiscoveryHealthTargetState
	watchTargets         map[string]string
	runtimeEpoch         int64
}

type prDiscoveryHealthStore struct {
	mu           sync.Mutex
	now          func() time.Time
	bus          bus.EventBus
	logger       *logger.Logger
	scopes       map[string]*prDiscoveryHealthScope
	retired      map[string]bool
	runtimeEpoch int64
}

func newPRDiscoveryHealth(eventBus bus.EventBus, log *logger.Logger) *prDiscoveryHealthStore {
	return &prDiscoveryHealthStore{
		now:          time.Now,
		bus:          eventBus,
		logger:       log,
		scopes:       make(map[string]*prDiscoveryHealthScope),
		retired:      make(map[string]bool),
		runtimeEpoch: atomic.AddInt64(&prDiscoveryRuntimeEpochSeed, 1),
	}
}

func (h *prDiscoveryHealthStore) setNow(now func() time.Time) {
	if h == nil || now == nil {
		return
	}
	h.mu.Lock()
	h.now = now
	h.mu.Unlock()
}

func (h *prDiscoveryHealthStore) nowLocked() time.Time {
	if h.now == nil {
		return time.Now().UTC()
	}
	return h.now().UTC()
}

func prDiscoveryScopeKey(workspaceID, cacheScope string) string {
	return fmt.Sprintf("%d:%s|%d:%s", len(workspaceID), workspaceID, len(cacheScope), cacheScope)
}

func (h *prDiscoveryHealthStore) scopeLocked(workspaceID, cacheScope string, credentialGeneration int64) *prDiscoveryHealthScope {
	key := prDiscoveryScopeKey(workspaceID, cacheScope)
	scope := h.scopes[key]
	if scope == nil {
		if !h.ensureScopeCapacityLocked() {
			return nil
		}
		scope = &prDiscoveryHealthScope{
			workspaceID:  workspaceID,
			cacheScope:   cacheScope,
			targets:      make(map[string]*prDiscoveryHealthTargetState),
			watchTargets: make(map[string]string),
			runtimeEpoch: atomic.AddInt64(&prDiscoveryRuntimeEpochSeed, 1),
		}
		h.scopes[key] = scope
	}
	if credentialGeneration != 0 {
		scope.credentialGeneration = credentialGeneration
	}
	return scope
}

func (h *prDiscoveryHealthStore) ensureScopeCapacityLocked() bool {
	if len(h.scopes) < prDiscoveryMaxScopes {
		return true
	}
	for candidateKey, candidate := range h.scopes {
		if !prDiscoveryScopeHasLiveTargets(candidate) {
			delete(h.scopes, candidateKey)
			return true
		}
	}
	return false
}

func prDiscoveryScopeHasLiveTargets(scope *prDiscoveryHealthScope) bool {
	if scope == nil {
		return false
	}
	for _, state := range scope.targets {
		if state != nil && (state.active || len(state.consumers) > 0) {
			return true
		}
	}
	return false
}

func newPRDiscoveryHealthTargetState() *prDiscoveryHealthTargetState {
	return &prDiscoveryHealthTargetState{consumers: make(map[string]struct{})}
}

func (h *prDiscoveryHealthStore) ensureTargetCapacityLocked(scope *prDiscoveryHealthScope) bool {
	if scope == nil {
		return false
	}
	if len(scope.targets) < prDiscoveryMaxTargets {
		return true
	}
	for key, state := range scope.targets {
		if state == nil || (!state.active && len(state.consumers) == 0) {
			delete(scope.targets, key)
			return true
		}
	}
	return false
}

func prDiscoveryTargetHasFailure(state *prDiscoveryHealthTargetState) bool {
	return state != nil && (state.category != "" || !state.lastFailureAt.IsZero())
}

func (h *prDiscoveryHealthStore) begin(
	workspaceID, cacheScope string, credentialGeneration int64, target prDiscoveryHealthTarget,
) (uint64, bool) {
	if h == nil || workspaceID == "" || cacheScope == "" {
		return 0, true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	scopeKey := prDiscoveryScopeKey(workspaceID, cacheScope)
	delete(h.retired, scopeKey)
	scope := h.scopeLocked(workspaceID, cacheScope, credentialGeneration)
	if scope == nil {
		return 0, false
	}
	key := target.key()
	state := scope.targets[key]
	now := h.nowLocked()
	if state != nil {
		if state.active {
			// A concurrent entry point may share the in-flight provider
			// attempt. Its own watch still needs the result fan-out, while the
			// immutable attempt number makes completion idempotent.
			return state.attempt, true
		}
		if state.retryAt != nil && now.Before(*state.retryAt) {
			return 0, false
		}
	}
	if state == nil {
		if !h.ensureTargetCapacityLocked(scope) {
			return 0, false
		}
		state = newPRDiscoveryHealthTargetState()
		scope.targets[key] = state
	}
	scope.nextAttempt++
	state.attempt = scope.nextAttempt
	state.active = true
	return state.attempt, true
}

// trackConsumer records the live watch identity for a target. It moves a
// watch between target keys when a searching branch changes and invalidates
// any active attempt that no longer has a live consumer.
func (h *prDiscoveryHealthStore) trackConsumer(
	workspaceID, cacheScope string, credentialGeneration int64, watchID string, target prDiscoveryHealthTarget,
) {
	if h == nil || workspaceID == "" || cacheScope == "" || watchID == "" {
		return
	}
	key := target.key()
	var eventHealth *PRDiscoveryHealth
	h.mu.Lock()
	scopeKey := prDiscoveryScopeKey(workspaceID, cacheScope)
	delete(h.retired, scopeKey)
	scope := h.scopeLocked(workspaceID, cacheScope, credentialGeneration)
	if scope == nil {
		h.mu.Unlock()
		return
	}
	oldKey := scope.watchTargets[watchID]
	changed := detachPRDiscoveryConsumerLocked(scope, watchID, oldKey, key)
	state, ok := h.targetStateLocked(scope, key)
	if !ok {
		h.mu.Unlock()
		if changed {
			h.publish(workspaceID, h.snapshot(workspaceID, cacheScope, credentialGeneration))
		}
		return
	}
	if state.consumers == nil {
		state.consumers = make(map[string]struct{})
	}
	if _, exists := state.consumers[watchID]; !exists {
		state.consumers[watchID] = struct{}{}
		scope.watchTargets[watchID] = key
	}
	if changed {
		scope.revision++
		snapshot := h.snapshotLocked(scope)
		eventHealth = &snapshot
	}
	h.mu.Unlock()
	if eventHealth != nil {
		h.publish(workspaceID, eventHealth)
	}
}

func detachPRDiscoveryConsumerLocked(scope *prDiscoveryHealthScope, watchID, oldKey, newKey string) bool {
	if oldKey == "" || oldKey == newKey {
		return false
	}
	state := scope.targets[oldKey]
	if state == nil {
		delete(scope.watchTargets, watchID)
		return false
	}
	wasFailed := prDiscoveryTargetHasFailure(state)
	delete(state.consumers, watchID)
	if len(state.consumers) == 0 {
		delete(scope.targets, oldKey)
	}
	delete(scope.watchTargets, watchID)
	return wasFailed
}

func (h *prDiscoveryHealthStore) targetStateLocked(scope *prDiscoveryHealthScope, key string) (*prDiscoveryHealthTargetState, bool) {
	state := scope.targets[key]
	if state != nil {
		return state, true
	}
	if !h.ensureTargetCapacityLocked(scope) {
		return nil, false
	}
	state = newPRDiscoveryHealthTargetState()
	scope.targets[key] = state
	return state, true
}

// removeWatch removes a watch from every credential scope for its workspace.
// A deleted or archived watch must not keep a failed target degraded forever.
func (h *prDiscoveryHealthStore) removeWatch(workspaceID, watchID string) {
	if h == nil || workspaceID == "" || watchID == "" {
		return
	}
	type publication struct {
		workspaceID string
		health      *PRDiscoveryHealth
	}
	publications := make([]publication, 0)
	h.mu.Lock()
	for _, scope := range h.scopes {
		if scope.workspaceID != workspaceID {
			continue
		}
		key, ok := scope.watchTargets[watchID]
		if !ok {
			continue
		}
		delete(scope.watchTargets, watchID)
		state := scope.targets[key]
		if state == nil {
			continue
		}
		wasFailed := prDiscoveryTargetHasFailure(state)
		delete(state.consumers, watchID)
		if len(state.consumers) == 0 {
			delete(scope.targets, key)
		}
		if wasFailed {
			scope.revision++
			snapshot := h.snapshotLocked(scope)
			publications = append(publications, publication{workspaceID: workspaceID, health: &snapshot})
		}
	}
	h.mu.Unlock()
	for _, item := range publications {
		h.publish(item.workspaceID, item.health)
	}
}

func (h *prDiscoveryHealthStore) finishSuccess(
	workspaceID, cacheScope string, credentialGeneration int64, target prDiscoveryHealthTarget, attempt uint64,
) {
	if h == nil || workspaceID == "" || cacheScope == "" {
		return
	}
	var eventHealth *PRDiscoveryHealth
	h.mu.Lock()
	if h.retired[prDiscoveryScopeKey(workspaceID, cacheScope)] {
		h.mu.Unlock()
		return
	}
	scope := h.scopes[prDiscoveryScopeKey(workspaceID, cacheScope)]
	if scope == nil {
		h.mu.Unlock()
		return
	}
	state, ok := scope.targets[target.key()]
	if attempt != 0 && (!ok || !state.active || state.attempt != attempt) {
		h.mu.Unlock()
		return
	}
	if ok {
		wasFailed := prDiscoveryTargetHasFailure(state)
		state.active = false
		state.failureCount = 0
		state.category = ""
		state.lastFailureAt = time.Time{}
		state.retryAt = nil
		if len(state.consumers) == 0 {
			delete(scope.targets, target.key())
		}
		ok = wasFailed
	}
	changed := !scope.seen || ok
	scope.seen = true
	if changed {
		scope.revision++
		snapshot := h.snapshotLocked(scope)
		eventHealth = &snapshot
	}
	h.mu.Unlock()
	if eventHealth != nil {
		h.publish(workspaceID, eventHealth)
	}
}

func (h *prDiscoveryHealthStore) release(
	workspaceID, cacheScope string, credentialGeneration int64, target prDiscoveryHealthTarget, attempt uint64,
) {
	if h == nil || workspaceID == "" || cacheScope == "" || attempt == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.retired[prDiscoveryScopeKey(workspaceID, cacheScope)] {
		return
	}
	scope := h.scopes[prDiscoveryScopeKey(workspaceID, cacheScope)]
	if scope == nil {
		return
	}
	state := scope.targets[target.key()]
	if state != nil && state.active && state.attempt == attempt {
		state.active = false
		if len(state.consumers) == 0 {
			delete(scope.targets, target.key())
		}
	}
}

func (h *prDiscoveryHealthStore) finishFailure(
	workspaceID, cacheScope string, credentialGeneration int64, target prDiscoveryHealthTarget,
	attempt uint64, category PRDiscoveryHealthCategory, retryAt *time.Time,
) {
	if h == nil || workspaceID == "" || cacheScope == "" {
		return
	}
	if category == "" {
		category = PRDiscoveryHealthUnavailable
	}
	var eventHealth *PRDiscoveryHealth
	h.mu.Lock()
	if h.retired[prDiscoveryScopeKey(workspaceID, cacheScope)] {
		h.mu.Unlock()
		return
	}
	scope := h.scopes[prDiscoveryScopeKey(workspaceID, cacheScope)]
	if scope == nil {
		h.mu.Unlock()
		return
	}
	state := scope.targets[target.key()]
	if state == nil {
		// A non-zero attempt with no state means the watch was removed or
		// retargeted while the provider request was in flight. Do not recreate
		// a degraded target for a dead watch.
		if attempt != 0 {
			h.mu.Unlock()
			return
		}
		state = newPRDiscoveryHealthTargetState()
		scope.targets[target.key()] = state
	}
	if attempt != 0 && (!state.active || state.attempt != attempt) {
		h.mu.Unlock()
		return
	}
	if stalePRDiscoveryFailure(state, attempt) {
		h.mu.Unlock()
		return
	}
	now := h.nowLocked()
	state.attempt = attempt
	state.active = false
	state.failureCount++
	state.category = category
	state.lastFailureAt = now
	state.retryAt = prDiscoveryRetryDeadline(now, category, state.failureCount, retryAt)
	scope.seen = true
	scope.revision++
	snapshot := h.snapshotLocked(scope)
	eventHealth = &snapshot
	h.mu.Unlock()
	h.publish(workspaceID, eventHealth)
}

func stalePRDiscoveryFailure(state *prDiscoveryHealthTargetState, attempt uint64) bool {
	if state == nil || attempt == 0 {
		return false
	}
	return (state.active && state.attempt != attempt) || (!state.active && state.attempt > attempt)
}

func prDiscoveryRetryDeadline(
	now time.Time, category PRDiscoveryHealthCategory, failureCount int, retryAt *time.Time,
) *time.Time {
	if retryAt != nil && retryAt.After(now) {
		return copyTime(retryAt)
	}
	delay := PRDiscoveryRetryBase
	if category == PRDiscoveryHealthRateLimited {
		delay = prDiscoveryRateLimitDelay(failureCount)
	}
	deadline := now.Add(delay)
	return &deadline
}

func prDiscoveryRateLimitDelay(failureCount int) time.Duration {
	delay := PRDiscoveryRetryBase
	for i := 1; i < failureCount; i++ {
		if delay >= prDiscoveryRetryMax/2 {
			return prDiscoveryRetryMax
		}
		delay *= 2
	}
	return delay
}

func (h *prDiscoveryHealthStore) snapshot(
	workspaceID, cacheScope string, credentialGeneration int64,
) *PRDiscoveryHealth {
	if h == nil {
		return &PRDiscoveryHealth{State: PRDiscoveryHealthUnknown, CredentialGeneration: credentialGeneration}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.retired[prDiscoveryScopeKey(workspaceID, cacheScope)] {
		return &PRDiscoveryHealth{
			State: PRDiscoveryHealthUnknown, CredentialGeneration: credentialGeneration, RuntimeEpoch: h.runtimeEpoch,
		}
	}
	scope := h.scopes[prDiscoveryScopeKey(workspaceID, cacheScope)]
	if scope == nil {
		return &PRDiscoveryHealth{
			State: PRDiscoveryHealthUnknown, CredentialGeneration: credentialGeneration, RuntimeEpoch: h.runtimeEpoch,
		}
	}
	if credentialGeneration != 0 {
		scope.credentialGeneration = credentialGeneration
	}
	snapshot := h.snapshotLocked(scope)
	return &snapshot
}

func (h *prDiscoveryHealthStore) snapshotLocked(scope *prDiscoveryHealthScope) PRDiscoveryHealth {
	health := PRDiscoveryHealth{
		State:                PRDiscoveryHealthUnknown,
		Revision:             scope.revision,
		CredentialGeneration: scope.credentialGeneration,
		RuntimeEpoch:         scope.runtimeEpoch,
	}
	var latestAttempt uint64
	for _, state := range scope.targets {
		if state.category == "" || state.lastFailureAt.IsZero() {
			continue
		}
		health.FailedTargetCount++
		if health.LastFailureAt == nil || state.lastFailureAt.After(*health.LastFailureAt) ||
			(state.lastFailureAt.Equal(*health.LastFailureAt) && state.attempt > latestAttempt) {
			lastFailure := state.lastFailureAt
			health.LastFailureAt = &lastFailure
			health.Category = state.category
			latestAttempt = state.attempt
		}
		if state.retryAt != nil && (health.RetryAt == nil || state.retryAt.Before(*health.RetryAt)) {
			health.RetryAt = copyTime(state.retryAt)
		}
	}
	if health.FailedTargetCount > 0 {
		health.State = PRDiscoveryHealthDegraded
	} else if scope.seen {
		health.State = PRDiscoveryHealthHealthy
	}
	return health
}

func (h *prDiscoveryHealthStore) clearWorkspace(workspaceID string) {
	if h == nil || workspaceID == "" {
		return
	}
	h.mu.Lock()
	for key, scope := range h.scopes {
		if scope.workspaceID == workspaceID {
			delete(h.scopes, key)
			h.retired[key] = true
		}
	}
	h.mu.Unlock()
}

func (h *prDiscoveryHealthStore) clearAll() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.scopes = make(map[string]*prDiscoveryHealthScope)
	h.retired = make(map[string]bool)
	h.mu.Unlock()
}

func (h *prDiscoveryHealthStore) publish(workspaceID string, health *PRDiscoveryHealth) {
	if h == nil || h.bus == nil || health == nil {
		return
	}
	eventData := &PRDiscoveryHealthUpdatedEvent{WorkspaceID: workspaceID, Health: health}
	event := bus.NewEvent(events.GitHubPRDiscoveryHealthUpdated, "github", eventData)
	if err := h.bus.Publish(context.Background(), events.GitHubPRDiscoveryHealthUpdated, event); err != nil && h.logger != nil {
		h.logger.Debug("publish PR discovery health event failed", zap.Error(err))
	}
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func classifyPRDiscoveryError(err error) PRDiscoveryHealthCategory {
	if err == nil {
		return ""
	}
	var apiErr *GitHubAPIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 429 || containsRateLimitMarker(apiErr.Body)) {
		return PRDiscoveryHealthRateLimited
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "graphql") && (strings.Contains(lower, "validation") ||
		strings.Contains(lower, "doesn't exist") || strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "cannot query field") || strings.Contains(lower, "unknown argument")) {
		return PRDiscoveryHealthInvalidQuery
	}
	if containsRateLimitMarker(lower) {
		return PRDiscoveryHealthRateLimited
	}
	return PRDiscoveryHealthUnavailable
}

func containsRateLimitMarker(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"rate limit", "too many requests", "429", "abuse detection", "secondary limit"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

type prDiscoveryWatchAttempt struct {
	target prDiscoveryHealthTarget
	number uint64
}

type prDiscoveryRetryAtProvider interface {
	ProviderRetryAt() *time.Time
}

type prDiscoveryRetryAtError struct {
	err     error
	retryAt *time.Time
}

func (e *prDiscoveryRetryAtError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *prDiscoveryRetryAtError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *prDiscoveryRetryAtError) ProviderRetryAt() *time.Time {
	if e == nil {
		return nil
	}
	return copyTime(e.retryAt)
}

func withPRDiscoveryRetryAt(err error, retryAt *time.Time) error {
	if err == nil || retryAt == nil {
		return err
	}
	return &prDiscoveryRetryAtError{err: err, retryAt: copyTime(retryAt)}
}

func prDiscoveryRetryAtFromError(err error) *time.Time {
	if err == nil {
		return nil
	}
	var providerErr prDiscoveryRetryAtProvider
	if errors.As(err, &providerErr) {
		return providerErr.ProviderRetryAt()
	}
	return nil
}

func (s *Service) ensurePRDiscoveryHealth() *prDiscoveryHealthStore {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prDiscoveryHealth == nil {
		s.prDiscoveryHealth = newPRDiscoveryHealth(s.eventBus, s.logger)
	}
	return s.prDiscoveryHealth
}

func prDiscoveryTargetForWatch(watch *PRWatch) prDiscoveryHealthTarget {
	if watch == nil {
		return prDiscoveryHealthTarget{}
	}
	return prDiscoveryHealthTarget{
		Owner: watch.Owner, Repo: watch.Repo, Branch: watch.Branch, PRNumber: watch.PRNumber,
	}
}

func (s *Service) beginPRDiscoveryWatch(
	workspaceID, cacheScope string, credentialGeneration int64, watch *PRWatch,
) (prDiscoveryWatchAttempt, bool) {
	target := prDiscoveryTargetForWatch(watch)
	attempt := prDiscoveryWatchAttempt{target: target}
	health := s.ensurePRDiscoveryHealth()
	if health == nil {
		return attempt, true
	}
	health.trackConsumer(workspaceID, cacheScope, credentialGeneration, prDiscoveryWatchID(watch), target)
	number, admitted := health.begin(workspaceID, cacheScope, credentialGeneration, target)
	attempt.number = number
	return attempt, admitted
}

func (s *Service) trackPRDiscoveryWatchConsumer(
	workspaceID, cacheScope string, credentialGeneration int64, watch *PRWatch,
) {
	s.trackPRDiscoveryWatchTarget(
		workspaceID, cacheScope, credentialGeneration, watch, prDiscoveryTargetForWatch(watch),
	)
}

func (s *Service) trackPRDiscoveryWatchTarget(
	workspaceID, cacheScope string, credentialGeneration int64, watch *PRWatch,
	target prDiscoveryHealthTarget,
) {
	if health := s.ensurePRDiscoveryHealth(); health != nil {
		health.trackConsumer(workspaceID, cacheScope, credentialGeneration, prDiscoveryWatchID(watch), target)
	}
}

func (s *Service) finishPRDiscoveryWatchSuccess(
	workspaceID, cacheScope string, credentialGeneration int64, attempt prDiscoveryWatchAttempt,
) {
	if health := s.ensurePRDiscoveryHealth(); health != nil {
		health.finishSuccess(workspaceID, cacheScope, credentialGeneration, attempt.target, attempt.number)
	}
}

func (s *Service) finishPRDiscoveryWatchFailure(
	workspaceID, cacheScope string, credentialGeneration int64, attempt prDiscoveryWatchAttempt, err error,
) {
	if health := s.ensurePRDiscoveryHealth(); health != nil {
		health.finishFailure(
			workspaceID, cacheScope, credentialGeneration, attempt.target, attempt.number,
			classifyPRDiscoveryError(err), prDiscoveryRetryAtFromError(err),
		)
	}
}

func (s *Service) finishPRDiscoveryWatchFailureCategory(
	workspaceID, cacheScope string, credentialGeneration int64, attempt prDiscoveryWatchAttempt,
	category PRDiscoveryHealthCategory,
) {
	if health := s.ensurePRDiscoveryHealth(); health != nil {
		health.finishFailure(
			workspaceID, cacheScope, credentialGeneration, attempt.target, attempt.number, category, nil,
		)
	}
}

func (s *Service) releasePRDiscoveryWatch(
	workspaceID, cacheScope string, credentialGeneration int64, attempt prDiscoveryWatchAttempt,
) {
	if health := s.ensurePRDiscoveryHealth(); health != nil {
		health.release(workspaceID, cacheScope, credentialGeneration, attempt.target, attempt.number)
	}
}

func prDiscoveryWatchID(watch *PRWatch) string {
	if watch == nil {
		return ""
	}
	return watch.ID
}

func (s *Service) removePRDiscoveryWatchConsumer(watch *PRWatch) {
	if s == nil || watch == nil {
		return
	}
	if health := s.ensurePRDiscoveryHealth(); health != nil {
		health.removeWatch(watch.WorkspaceID, watch.ID)
	}
}
