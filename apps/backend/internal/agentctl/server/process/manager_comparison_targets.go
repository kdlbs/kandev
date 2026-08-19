package process

import (
	"context"
	"strings"

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

// PrepareComparisonTargets materializes all desired targets before tracker
// polling starts. A failed target becomes explicitly unavailable; it never
// becomes an implicit origin/local comparison.
func (m *Manager) PrepareComparisonTargets(ctx context.Context) {
	root, trackers := m.snapshotTrackers()
	if root != nil {
		m.prepareTrackerComparisonTarget(ctx, root)
	}
	for _, tracker := range trackers {
		m.prepareTrackerComparisonTarget(ctx, tracker)
	}
}

// UpdateComparisonTargets replaces the desired map and refreshes only trackers
// whose target changed. Existing ready siblings keep their state and are not
// refetched when another repository is updated.
func (m *Manager) UpdateComparisonTargets(ctx context.Context, targets map[string]models.ComparisonTarget) {
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
		m.prepareTrackerComparisonTarget(ctx, tracker)
	}
	go m.refreshComparisonTrackersDetached(all)
}

func comparisonTargetMapsEqual(previous, current map[string]models.ComparisonTarget, key string) bool {
	left, leftOK := previous[key]
	right, rightOK := current[key]
	return leftOK == rightOK && (!leftOK || left.Equal(right))
}

func (m *Manager) prepareTrackerComparisonTarget(ctx context.Context, tracker *WorkspaceTracker) {
	if tracker == nil {
		return
	}
	target := m.comparisonTargetFor(tracker.RepositoryName())
	tracker.SetComparisonTarget(target)
	if target == nil {
		return
	}
	if tracker.gitIndexPath == "" {
		tracker.SetComparisonTargetUnavailable(target, comparisonTargetErrorInvalid)
		return
	}
	runner := func(runCtx context.Context, args ...string) (string, error) {
		output, err := tracker.runGitOutput(runCtx, args...)
		return string(output), err
	}
	materialized, err := materializeComparisonTarget(ctx, runner, *target)
	if err != nil {
		code := comparisonTargetErrorCode(err)
		tracker.SetComparisonTargetUnavailable(target, code)
		m.logger.Warn("comparison target unavailable",
			zap.String("repository", tracker.RepositoryName()),
			zap.String("error_code", code),
			zap.Error(err))
		return
	}
	tracker.SetComparisonTargetReady(target, materialized.Ref)
}

func comparisonTargetErrorCode(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "collision"):
		return comparisonTargetErrorRemoteCollision
	case strings.Contains(message, "invalid"):
		return comparisonTargetErrorInvalid
	case strings.Contains(message, "ref unavailable"):
		return comparisonTargetErrorRefUnavailable
	case strings.Contains(message, "fetch"):
		return comparisonTargetErrorFetch
	default:
		return comparisonTargetErrorRemoteSetup
	}
}

func (m *Manager) refreshComparisonTrackersDetached(trackers []*WorkspaceTracker) {
	ctx := context.Background()
	for _, tracker := range trackers {
		if tracker != nil {
			tracker.RefreshGitStatus(ctx)
		}
	}
}
