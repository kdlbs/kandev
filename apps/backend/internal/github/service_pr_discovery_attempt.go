package github

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type prDiscoveryWatchAttempt struct {
	target prDiscoveryHealthTarget
	number uint64
	joined bool
	result *prDiscoveryAttemptResult
}

type prDiscoveryAttemptResult struct {
	done             chan struct{}
	once             sync.Once
	workspaceID      string
	targetKey        string
	status           *PRStatus
	resolved         bool
	syncFailed       bool
	err              error
	suppressFallback bool
}

func newPRDiscoveryAttemptResult(workspaceID, targetKey string) *prDiscoveryAttemptResult {
	return &prDiscoveryAttemptResult{done: make(chan struct{}), workspaceID: workspaceID, targetKey: targetKey}
}

func (r *prDiscoveryAttemptResult) complete(
	status *PRStatus, resolved, syncFailed bool, err error, suppressFallback bool,
) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		r.status = status
		r.resolved = resolved
		r.syncFailed = syncFailed
		r.err = err
		r.suppressFallback = suppressFallback
		close(r.done)
	})
}

func (r *prDiscoveryAttemptResult) wait(ctx context.Context) (*PRStatus, bool, bool, error, bool) {
	if r == nil {
		return nil, false, false, nil, false
	}
	select {
	case <-r.done:
		return r.status, r.resolved, r.syncFailed, r.err, r.suppressFallback
	case <-ctx.Done():
		return nil, false, false, ctx.Err(), false
	}
}

func (r *prDiscoveryAttemptResult) invalidated() bool {
	if r == nil {
		return false
	}
	select {
	case <-r.done:
		return r.err == nil && r.syncFailed && r.suppressFallback
	default:
		return false
	}
}

func sharedPRDiscoveryResult(ctx context.Context, attempt prDiscoveryWatchAttempt) (*PR, error) {
	status, _, _, err, suppressFallback := attempt.result.wait(ctx)
	if err != nil {
		if suppressFallback {
			return nil, nil
		}
		return nil, err
	}
	if status == nil {
		return nil, nil
	}
	return status.PR, nil
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
	invalidatedTargets := health.trackConsumer(
		workspaceID, cacheScope, credentialGeneration, prDiscoveryWatchID(watch), target,
	)
	s.invalidatePRDiscoveryAttempts(workspaceID, invalidatedTargets)
	s.prDiscoveryAttemptsMu.Lock()
	defer s.prDiscoveryAttemptsMu.Unlock()
	if s.prDiscoveryAttempts == nil {
		s.prDiscoveryAttempts = make(map[string]*prDiscoveryAttemptResult)
	}
	number, admitted, joined := health.beginWithJoin(workspaceID, cacheScope, credentialGeneration, target)
	attempt.number = number
	if admitted && workspaceID != "" && cacheScope != "" {
		key := prDiscoveryAttemptKey(workspaceID, cacheScope, credentialGeneration, target)
		result := s.prDiscoveryAttempts[key]
		if joined && result != nil {
			attempt.joined = true
		} else {
			result = newPRDiscoveryAttemptResult(workspaceID, target.key())
			s.prDiscoveryAttempts[key] = result
		}
		attempt.result = result
	}
	return attempt, admitted
}

func (h *prDiscoveryHealthStore) beginWithJoin(
	workspaceID, cacheScope string, credentialGeneration int64, target prDiscoveryHealthTarget,
) (uint64, bool, bool) {
	if h == nil || workspaceID == "" || cacheScope == "" {
		return 0, true, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	scopeKey := prDiscoveryScopeKey(workspaceID, cacheScope)
	delete(h.retired, scopeKey)
	scope := h.scopeLocked(workspaceID, cacheScope, credentialGeneration)
	if scope == nil {
		return 0, false, false
	}
	key := target.key()
	state := scope.targets[key]
	now := h.nowLocked()
	if state != nil {
		if state.active {
			return state.attempt, true, true
		}
		if state.retryAt != nil && now.Before(*state.retryAt) {
			return 0, false, false
		}
	}
	if state == nil {
		if !h.ensureTargetCapacityLocked(scope) {
			return 0, false, false
		}
		state = newPRDiscoveryHealthTargetState()
		scope.targets[key] = state
	}
	scope.nextAttempt++
	state.attempt = scope.nextAttempt
	state.active = true
	return state.attempt, true, false
}

func prDiscoveryAttemptKey(
	workspaceID, cacheScope string, credentialGeneration int64, target prDiscoveryHealthTarget,
) string {
	return fmt.Sprintf("%d:%s|%d:%s|%d|%s", len(workspaceID), workspaceID, len(cacheScope), cacheScope, credentialGeneration, target.key())
}

func (s *Service) completePRDiscoveryWatchAttempt(
	attempt prDiscoveryWatchAttempt, status *PRStatus, resolved, syncFailed bool, err error, suppressFallback bool,
) {
	if attempt.result == nil {
		return
	}
	attempt.result.complete(status, resolved, syncFailed, err, suppressFallback)
}

func (s *Service) forgetPRDiscoveryWatchAttempt(
	workspaceID, cacheScope string, credentialGeneration int64, attempt prDiscoveryWatchAttempt,
) {
	if s == nil || attempt.result == nil || workspaceID == "" || cacheScope == "" {
		return
	}
	key := prDiscoveryAttemptKey(workspaceID, cacheScope, credentialGeneration, attempt.target)
	s.prDiscoveryAttemptsMu.Lock()
	if s.prDiscoveryAttempts[key] == attempt.result {
		delete(s.prDiscoveryAttempts, key)
	}
	s.prDiscoveryAttemptsMu.Unlock()
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
		invalidatedTargets := health.trackConsumer(
			workspaceID, cacheScope, credentialGeneration, prDiscoveryWatchID(watch), target,
		)
		s.invalidatePRDiscoveryAttempts(workspaceID, invalidatedTargets)
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
		invalidatedTargets := health.removeWatch(watch.WorkspaceID, watch.ID)
		s.invalidatePRDiscoveryAttempts(watch.WorkspaceID, invalidatedTargets)
	}
}

func (s *Service) invalidatePRDiscoveryAttempts(workspaceID string, targetKeys []string) {
	if s == nil || workspaceID == "" || len(targetKeys) == 0 {
		return
	}
	wanted := make(map[string]struct{}, len(targetKeys))
	for _, targetKey := range targetKeys {
		if targetKey != "" {
			wanted[targetKey] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return
	}
	s.prDiscoveryAttemptsMu.Lock()
	for key, result := range s.prDiscoveryAttempts {
		if result.workspaceID != workspaceID {
			continue
		}
		if _, ok := wanted[result.targetKey]; !ok {
			continue
		}
		result.complete(nil, false, true, nil, true)
		delete(s.prDiscoveryAttempts, key)
	}
	s.prDiscoveryAttemptsMu.Unlock()
}

func (s *Service) invalidateAllPRDiscoveryAttempts(workspaceID string) {
	if s == nil {
		return
	}
	s.prDiscoveryAttemptsMu.Lock()
	for key, result := range s.prDiscoveryAttempts {
		if workspaceID != "" && result.workspaceID != workspaceID {
			continue
		}
		result.complete(nil, false, true, nil, true)
		delete(s.prDiscoveryAttempts, key)
	}
	s.prDiscoveryAttemptsMu.Unlock()
}
