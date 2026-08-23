package process

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

// TestWorkspaceTracker_GitPollTickStats_AccumulatesAcrossTicks pins the
// accumulation contract behind the diagnostic agentctl_git_poll_ms metric:
// GitPollTickStats must report zero before any tick has completed, and must
// report a growing count with a non-zero total after ticks run, regardless
// of whether individual ticks succeed.
func TestWorkspaceTracker_GitPollTickStats_AccumulatesAcrossTicks(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	log := newTestLogger(t)
	wt := NewWorkspaceTracker(repoDir, log)
	ctx := context.Background()

	if count, total := wt.GitPollTickStats(); count != 0 || total != 0 {
		t.Fatalf("GitPollTickStats before any tick = (%d, %d), want (0, 0)", count, total)
	}

	var consecutiveFailures int
	if stop := wt.gitPollTick(ctx, &consecutiveFailures); stop {
		t.Fatal("gitPollTick unexpectedly signalled stop")
	}

	firstCount, firstTotal := wt.GitPollTickStats()
	if firstCount != 1 {
		t.Fatalf("count after one tick = %d, want 1", firstCount)
	}
	if firstTotal <= 0 {
		t.Fatalf("totalNanos after one tick = %d, want > 0", firstTotal)
	}

	if stop := wt.gitPollTick(ctx, &consecutiveFailures); stop {
		t.Fatal("gitPollTick unexpectedly signalled stop")
	}

	secondCount, secondTotal := wt.GitPollTickStats()
	if secondCount != 2 {
		t.Fatalf("count after two ticks = %d, want 2", secondCount)
	}
	if secondTotal <= firstTotal {
		t.Fatalf("totalNanos after two ticks = %d, want > %d", secondTotal, firstTotal)
	}
}

// TestManager_GitPollStats_AggregatesRootTracker exercises the
// process.Manager-level aggregator that api.handleSystemMetrics reads. It
// must report unavailable (count 0) before any tick, and a plausible mean
// once the root tracker has recorded ticks.
func TestManager_GitPollStats_AggregatesRootTracker(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	log := newTestLogger(t)
	cfg := &config.InstanceConfig{WorkDir: repoDir}
	m := NewManager(cfg, log)

	if count, mean := m.GitPollStats(); count != 0 || mean != 0 {
		t.Fatalf("GitPollStats before any tick = (%d, %f), want (0, 0)", count, mean)
	}

	root, _ := m.snapshotTrackers()
	if root == nil {
		t.Fatal("expected a root workspace tracker")
	}
	var consecutiveFailures int
	root.gitPollTick(context.Background(), &consecutiveFailures)
	root.gitPollTick(context.Background(), &consecutiveFailures)

	count, mean := m.GitPollStats()
	if count != 2 {
		t.Fatalf("count after two ticks = %d, want 2", count)
	}
	if mean <= 0 {
		t.Fatalf("mean after two ticks = %f, want > 0", mean)
	}
}

// TestManager_GitPollStats_AggregatesAcrossRepoTrackers exercises the
// multi-repo half of GitPollStats: with a second WorkspaceTracker added to
// m.repoTrackers (the shape a multi-repo task root produces), the aggregate
// count and mean must include both the root tracker's and the extra
// tracker's ticks, not just the root's.
func TestManager_GitPollStats_AggregatesAcrossRepoTrackers(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	repoDir2, cleanup2 := setupTestRepo(t)
	t.Cleanup(cleanup2)

	log := newTestLogger(t)
	cfg := &config.InstanceConfig{WorkDir: repoDir}
	m := NewManager(cfg, log)

	root, _ := m.snapshotTrackers()
	if root == nil {
		t.Fatal("expected a root workspace tracker")
	}
	var consecutiveFailures int
	root.gitPollTick(context.Background(), &consecutiveFailures)
	root.gitPollTick(context.Background(), &consecutiveFailures)

	extra := NewWorkspaceTrackerForRepo(repoDir2, "extra-repo", log)
	extra.gitPollTick(context.Background(), &consecutiveFailures)

	m.repoTrackersMu.Lock()
	m.repoTrackers = append(m.repoTrackers, extra)
	m.repoTrackersMu.Unlock()

	count, mean := m.GitPollStats()
	if count != 3 {
		t.Fatalf("count across root + extra tracker = %d, want 3 (2 from root + 1 from extra)", count)
	}
	if mean <= 0 {
		t.Fatalf("mean across root + extra tracker = %f, want > 0", mean)
	}
}

func TestManager_MonitorPollStats_AggregatesWorkspaceMonitorTicks(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	log := newTestLogger(t)
	m := NewManager(&config.InstanceConfig{WorkDir: repoDir}, log)
	root, _ := m.snapshotTrackers()
	if root == nil {
		t.Fatal("expected a root workspace tracker")
	}

	lastState := workspaceState{}
	var consecutiveFailures int
	if stop := root.monitorTick(context.Background(), &lastState, &consecutiveFailures); stop {
		t.Fatal("monitorTick unexpectedly signalled stop")
	}

	count, mean := m.MonitorPollStats()
	if count != 1 {
		t.Fatalf("MonitorPollStats count = %d, want 1", count)
	}
	if mean <= 0 {
		t.Fatalf("MonitorPollStats mean = %f, want > 0", mean)
	}
}
