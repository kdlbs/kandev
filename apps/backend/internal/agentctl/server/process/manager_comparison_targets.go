package process

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// comparisonTargetFor returns a detached target copy for one tracker key.
func (m *Manager) comparisonTargetFor(repositoryName string) *models.ComparisonTarget {
	m.comparisonTargetsMu.RLock()
	defer m.comparisonTargetsMu.RUnlock()
	if m.cfg == nil || m.cfg.ComparisonTargets == nil {
		return nil
	}
	target, ok := m.cfg.ComparisonTargets[repositoryName]
	if !ok {
		return nil
	}
	copy := target
	return &copy
}

func (m *Manager) getComparisonTargets() map[string]models.ComparisonTarget {
	m.comparisonTargetsMu.RLock()
	defer m.comparisonTargetsMu.RUnlock()
	if m.cfg == nil || len(m.cfg.ComparisonTargets) == 0 {
		return nil
	}
	result := make(map[string]models.ComparisonTarget, len(m.cfg.ComparisonTargets))
	for key, target := range m.cfg.ComparisonTargets {
		result[key] = target
	}
	return result
}

// PrepareComparisonTargets schedules materialization for all desired targets
// before tracker polling starts. A failed target becomes explicitly
// unavailable; it never becomes an implicit origin/local comparison.
func (m *Manager) PrepareComparisonTargets(_ context.Context) {
	root, trackers := m.snapshotTrackers()
	if root != nil {
		m.prepareTrackerComparisonTarget(root)
	}
	for _, tracker := range trackers {
		m.prepareTrackerComparisonTarget(tracker)
	}
}

// UpdateComparisonTargets replaces the desired map and refreshes only trackers
// whose target changed. Existing ready siblings keep their state and are not
// refetched when another repository is updated. Target materialization runs in
// the manager lifetime, not in the request context.
func (m *Manager) UpdateComparisonTargets(_ context.Context, targets map[string]models.ComparisonTarget) {
	previous := m.getComparisonTargets()
	m.comparisonTargetsMu.Lock()
	if len(targets) == 0 {
		m.cfg.ComparisonTargets = nil
	} else {
		m.cfg.ComparisonTargets = make(map[string]models.ComparisonTarget, len(targets))
		for key, target := range targets {
			m.cfg.ComparisonTargets[key] = target
		}
	}
	m.comparisonTargetsMu.Unlock()
	current := m.getComparisonTargets()

	root, trackers := m.snapshotTrackers()
	all := append([]*WorkspaceTracker{root}, trackers...)
	for _, tracker := range all {
		if tracker == nil || comparisonTargetMapsEqual(previous, current, tracker.RepositoryName()) {
			continue
		}
		m.prepareTrackerComparisonTarget(tracker)
	}
}

func comparisonTargetMapsEqual(previous, current map[string]models.ComparisonTarget, key string) bool {
	left, leftOK := previous[key]
	right, rightOK := current[key]
	return leftOK == rightOK && (!leftOK || left.Equal(right))
}

func (m *Manager) prepareTrackerComparisonTarget(tracker *WorkspaceTracker) {
	if tracker == nil {
		return
	}
	repositoryName := tracker.RepositoryName()
	target := m.comparisonTargetFor(repositoryName)
	previousResolution := tracker.ComparisonResolution()
	if !m.setComparisonTargetIfCurrent(tracker, repositoryName, target) {
		return
	}
	if target != nil || previousResolution.Explicit {
		m.refreshComparisonTrackerDetached(tracker)
	}
	if target == nil {
		m.cancelComparisonTargetOperation(repositoryName)
		return
	}
	if tracker.gitIndexPath == "" || target.Validate() != nil {
		m.cancelComparisonTargetOperation(repositoryName)
		if m.setComparisonTargetUnavailableIfCurrent(tracker, repositoryName, target, comparisonTargetErrorInvalid) {
			m.refreshComparisonTrackerDetached(tracker)
		}
		return
	}

	m.scheduleComparisonTargetOperation(repositoryName, tracker, *target)
}

func (m *Manager) scheduleComparisonTargetOperation(
	repositoryName string,
	tracker *WorkspaceTracker,
	target models.ComparisonTarget,
) {
	m.comparisonTargetOpsMu.Lock()
	if m.comparisonTargetOpsStopping {
		m.comparisonTargetOpsMu.Unlock()
		return
	}
	operationCtx, release, err := m.beginComparisonTargetOperation()
	if err != nil {
		m.comparisonTargetOpsMu.Unlock()
		return
	}
	operationCtx, cancel := context.WithTimeout(operationCtx, gitCommandTimeout)
	operation := &comparisonTargetOperation{target: target, tracker: tracker, cancel: cancel}

	if !m.comparisonTargetIsCurrent(repositoryName, &target) {
		m.comparisonTargetOpsMu.Unlock()
		cancel()
		release()
		return
	}
	if existing := m.comparisonTargetOps[repositoryName]; existing != nil {
		if existing.tracker == tracker && existing.target.Equal(target) {
			m.comparisonTargetOpsMu.Unlock()
			cancel()
			release()
			return
		}
		existing.cancel()
	}
	if m.comparisonTargetOps == nil {
		m.comparisonTargetOps = make(map[string]*comparisonTargetOperation)
	}
	m.comparisonTargetOps[repositoryName] = operation
	m.comparisonTargetOpsWG.Add(1)
	m.comparisonTargetOpsMu.Unlock()

	go m.runComparisonTargetOperation(repositoryName, operation, operationCtx, release)
}

type comparisonTargetOperation struct {
	target  models.ComparisonTarget
	tracker *WorkspaceTracker
	cancel  context.CancelFunc
}

func (m *Manager) beginComparisonTargetOperation() (context.Context, func(), error) {
	if m.lifetimeCtx == nil {
		ctx, cancel := context.WithCancel(context.Background())
		return ctx, cancel, nil
	}
	return m.BeginOwnedOperation(m.lifetimeCtx)
}

func (m *Manager) runComparisonTargetOperation(
	repositoryName string,
	operation *comparisonTargetOperation,
	ctx context.Context,
	release func(),
) {
	defer m.comparisonTargetOpsWG.Done()
	defer release()
	defer operation.cancel()
	defer func() {
		m.comparisonTargetOpsMu.Lock()
		if m.comparisonTargetOps[repositoryName] == operation {
			delete(m.comparisonTargetOps, repositoryName)
		}
		m.comparisonTargetOpsMu.Unlock()
	}()

	runner := func(runCtx context.Context, args ...string) (string, error) {
		output, err := operation.tracker.runGitOutput(runCtx, args...)
		return string(output), err
	}
	materialized, err := materializeComparisonTarget(ctx, runner, operation.target)
	if err != nil {
		code := comparisonTargetErrorCode(err)
		if !m.publishComparisonTargetUnavailable(repositoryName, operation, code) {
			return
		}
		m.refreshComparisonTrackerDetached(operation.tracker)
		m.logger.Warn("comparison target unavailable",
			zap.String("repository", repositoryName),
			zap.String("error_code", code),
			zap.Error(err))
		return
	}
	if !m.publishComparisonTargetReady(repositoryName, operation, materialized.Ref) {
		return
	}
	m.refreshComparisonTrackerDetached(operation.tracker)
}

func (m *Manager) setComparisonTargetUnavailableIfCurrent(
	tracker *WorkspaceTracker,
	repositoryName string,
	target *models.ComparisonTarget,
	code string,
) bool {
	m.comparisonTargetsMu.RLock()
	defer m.comparisonTargetsMu.RUnlock()
	if !comparisonTargetMatches(m.cfg, repositoryName, target) {
		return false
	}
	tracker.SetComparisonTargetUnavailable(target, code)
	return true
}

func (m *Manager) setComparisonTargetIfCurrent(
	tracker *WorkspaceTracker,
	repositoryName string,
	target *models.ComparisonTarget,
) bool {
	m.comparisonTargetsMu.RLock()
	defer m.comparisonTargetsMu.RUnlock()
	if !comparisonTargetMatches(m.cfg, repositoryName, target) {
		return false
	}
	tracker.SetComparisonTarget(target)
	return true
}

func (m *Manager) publishComparisonTargetReady(
	repositoryName string,
	operation *comparisonTargetOperation,
	ref string,
) bool {
	m.comparisonTargetOpsMu.Lock()
	defer m.comparisonTargetOpsMu.Unlock()
	m.comparisonTargetsMu.RLock()
	defer m.comparisonTargetsMu.RUnlock()
	if m.comparisonTargetOps[repositoryName] != operation ||
		!comparisonTargetMatches(m.cfg, repositoryName, &operation.target) {
		return false
	}
	operation.tracker.SetComparisonTargetReady(&operation.target, ref)
	return true
}

func (m *Manager) publishComparisonTargetUnavailable(
	repositoryName string,
	operation *comparisonTargetOperation,
	code string,
) bool {
	m.comparisonTargetOpsMu.Lock()
	defer m.comparisonTargetOpsMu.Unlock()
	m.comparisonTargetsMu.RLock()
	defer m.comparisonTargetsMu.RUnlock()
	if m.comparisonTargetOps[repositoryName] != operation ||
		!comparisonTargetMatches(m.cfg, repositoryName, &operation.target) {
		return false
	}
	operation.tracker.SetComparisonTargetUnavailable(&operation.target, code)
	return true
}

func comparisonTargetMatches(
	cfg *config.InstanceConfig,
	repositoryName string,
	target *models.ComparisonTarget,
) bool {
	if cfg == nil || cfg.ComparisonTargets == nil {
		return target == nil
	}
	current, ok := cfg.ComparisonTargets[repositoryName]
	if target == nil {
		return !ok
	}
	return ok && current.Equal(*target)
}

func (m *Manager) comparisonTargetIsCurrent(repositoryName string, target *models.ComparisonTarget) bool {
	current := m.comparisonTargetFor(repositoryName)
	if current == nil || target == nil {
		return current == nil && target == nil
	}
	return current.Equal(*target)
}

func comparisonTargetErrorCode(err error) string {
	var targetErr *comparisonTargetMaterializationError
	if errors.As(err, &targetErr) && targetErr.code != "" {
		return targetErr.code
	}
	return comparisonTargetErrorRemoteSetup
}

func (m *Manager) refreshComparisonTrackerDetached(tracker *WorkspaceTracker) {
	if tracker == nil {
		return
	}
	go func() {
		ctx := tracker.cancelCtx
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		tracker.RefreshGitStatus(ctx)
	}()
}

func (m *Manager) cancelComparisonTargetOperation(repositoryName string) {
	m.comparisonTargetOpsMu.Lock()
	if operation := m.comparisonTargetOps[repositoryName]; operation != nil {
		operation.cancel()
	}
	m.comparisonTargetOpsMu.Unlock()
}

func (m *Manager) stopComparisonTargetOperations(ctx context.Context) (error, bool) {
	m.comparisonTargetOpsMu.Lock()
	m.comparisonTargetOpsStopping = true
	for _, operation := range m.comparisonTargetOps {
		operation.cancel()
	}
	m.comparisonTargetOpsMu.Unlock()

	done := make(chan struct{})
	go func() {
		m.comparisonTargetOpsWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil, true
	case <-ctx.Done():
		return ctx.Err(), false
	}
}

func (m *Manager) reopenComparisonTargetOperations() {
	m.comparisonTargetOpsMu.Lock()
	if !m.comparisonTargetOpsPermanent {
		m.comparisonTargetOpsStopping = false
	}
	m.comparisonTargetOpsMu.Unlock()
}
