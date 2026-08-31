//go:build !windows

package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/task/models"
)

func TestComparisonTargetPreparationNonBlocking(t *testing.T) {
	previousTimeout := gitCommandTimeout
	gitCommandTimeout = 200 * time.Millisecond
	t.Cleanup(func() { gitCommandTimeout = previousTimeout })

	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	target := comparisonTargetProcessTestTarget()
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:           repoDir,
		ComparisonTargets: map[string]models.ComparisonTarget{"": target},
	}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })

	shimDir := installSleepGitShim(t)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	started := time.Now()
	mgr.PrepareComparisonTargets(context.Background())
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("PrepareComparisonTargets took %v, want it to return before Git work completes", elapsed)
	}

	resolution := mgr.GetWorkspaceTracker().ComparisonResolution()
	if !resolution.Explicit || resolution.Status != comparisonTargetStatusUnavailable ||
		resolution.ErrorCode != comparisonTargetErrorPending {
		t.Fatalf("comparison resolution = %#v, want explicit pending state", resolution)
	}
}

func TestComparisonTargetMaterializationPublishesReadyAndRefreshes(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	target := comparisonTargetProcessTestTarget()
	mgr, markers := newComparisonTargetTestManager(t, repoDir, target, nil)

	mgr.PrepareComparisonTargets(context.Background())
	resolution := waitForComparisonResolution(t, mgr.GetWorkspaceTracker(), comparisonTargetStatusReady, "")
	if resolution.Ref != target.ComparisonRef() {
		t.Fatalf("comparison ref = %q, want %q", resolution.Ref, target.ComparisonRef())
	}
	waitForComparisonFile(t, markers.statusStarted)
}

func TestComparisonTargetMaterializationPublishesBoundedFetchFailureNoTransportFallback(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	target := comparisonTargetProcessTestTarget()
	mgr, markers := newComparisonTargetTestManager(t, repoDir, target, map[string]string{
		"KANDEV_TEST_FETCH_ERROR": "1",
	})

	mgr.PrepareComparisonTargets(context.Background())
	resolution := waitForComparisonResolution(t, mgr.GetWorkspaceTracker(), comparisonTargetStatusUnavailable, comparisonTargetErrorFetch)
	logBytes, err := os.ReadFile(markers.commandLog)
	if err != nil {
		t.Fatalf("read Git command log: %v", err)
	}
	log := string(logBytes)
	if strings.Contains(log, "ssh://") || strings.Contains(log, "git@") || strings.Contains(log, " ssh ") {
		t.Fatalf("comparison target attempted a transport fallback; commands:\n%s", log)
	}
	if got := strings.Count(log, "fetch --no-tags"); got != 1 {
		t.Fatalf("comparison fetch count = %d, want one; commands:\n%s", got, log)
	}
	if resolution.Ref != "" {
		t.Fatalf("failed comparison ref = %q, want empty", resolution.Ref)
	}
}

func TestComparisonTargetUpdateSupersedesStaleOperation(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	first := comparisonTargetProcessTestTarget()
	second := first
	second.TargetBranch = "develop"
	gate := filepath.Join(t.TempDir(), "fetch.gate")
	mgr, markers := newComparisonTargetTestManager(t, repoDir, first, map[string]string{
		"KANDEV_TEST_FETCH_GATE": gate,
	})

	mgr.PrepareComparisonTargets(context.Background())
	waitForComparisonFile(t, markers.fetchStarted)
	mgr.UpdateComparisonTargets(context.Background(), map[string]models.ComparisonTarget{"": second})

	resolution := waitForComparisonResolution(t, mgr.GetWorkspaceTracker(), comparisonTargetStatusReady, "")
	if resolution.Ref != second.ComparisonRef() {
		t.Fatalf("comparison ref = %q, want latest target ref %q", resolution.Ref, second.ComparisonRef())
	}
	if snapshot := mgr.GetWorkspaceTracker().ComparisonTargetSnapshot(); snapshot == nil || !snapshot.Equal(second) {
		t.Fatalf("comparison target snapshot = %#v, want latest target %#v", snapshot, second)
	}
}

func TestComparisonTargetMaterializationCancellationOnShutdown(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	target := comparisonTargetProcessTestTarget()
	gate := filepath.Join(t.TempDir(), "fetch.gate")
	mgr, markers := newComparisonTargetTestManager(t, repoDir, target, map[string]string{
		"KANDEV_TEST_FETCH_GATE": gate,
	})
	mgr.PrepareComparisonTargets(context.Background())
	waitForComparisonFile(t, markers.fetchStarted)

	started := time.Now()
	if err := mgr.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("StopForTeardown: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("StopForTeardown took %v, want canceled materialization to stop promptly", elapsed)
	}
}

func TestUpdateComparisonTargetsDoesNotWaitForMaterialization(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	target := comparisonTargetProcessTestTarget()
	gate := filepath.Join(t.TempDir(), "fetch.gate")
	mgr, markers := newComparisonTargetTestManager(t, repoDir, target, map[string]string{
		"KANDEV_TEST_FETCH_GATE": gate,
	})
	mgr.UpdateComparisonTargets(context.Background(), nil)

	shimDir := installComparisonTargetGitShim(t)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	started := time.Now()
	mgr.UpdateComparisonTargets(context.Background(), map[string]models.ComparisonTarget{"": target})
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("UpdateComparisonTargets took %v, want it to return before Git work completes", elapsed)
	}
	waitForComparisonFile(t, markers.fetchStarted)
}

func TestGetWorkspaceTrackerForDoesNotWaitForMaterialization(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	lazyDir := filepath.Join(repoDir, "lazy")
	initGitRepoAt(t, lazyDir)
	target := comparisonTargetProcessTestTarget()
	gate := filepath.Join(t.TempDir(), "fetch.gate")
	mgr, markers := newComparisonTargetTestManager(t, repoDir, target, map[string]string{
		"KANDEV_TEST_FETCH_GATE": gate,
	})
	mgr.UpdateComparisonTargets(context.Background(), map[string]models.ComparisonTarget{"lazy": target})

	shimDir := installComparisonTargetGitShim(t)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	started := time.Now()
	tracker, err := mgr.GetWorkspaceTrackerFor("lazy")
	if err != nil {
		t.Fatalf("GetWorkspaceTrackerFor: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("GetWorkspaceTrackerFor took %v, want it to return before Git work completes", elapsed)
	}
	waitForComparisonFile(t, markers.fetchStarted)
	resolution := tracker.ComparisonResolution()
	if !resolution.Explicit || resolution.ErrorCode != comparisonTargetErrorPending {
		t.Fatalf("lazy comparison resolution = %#v, want pending", resolution)
	}
}

type comparisonTargetTestMarkers struct {
	commandLog    string
	fetchStarted  string
	statusStarted string
}

func newComparisonTargetTestManager(
	t *testing.T,
	repoDir string,
	target models.ComparisonTarget,
	overrides map[string]string,
) (*Manager, comparisonTargetTestMarkers) {
	t.Helper()
	markers := comparisonTargetTestMarkers{
		commandLog:    filepath.Join(t.TempDir(), "commands.log"),
		fetchStarted:  filepath.Join(t.TempDir(), "fetch.started"),
		statusStarted: filepath.Join(t.TempDir(), "status.started"),
	}
	instanceEnv := []string{
		"KANDEV_TEST_GIT_LOG=" + markers.commandLog,
		"KANDEV_TEST_FETCH_STARTED=" + markers.fetchStarted,
		"KANDEV_TEST_STATUS_STARTED=" + markers.statusStarted,
	}
	for key, value := range overrides {
		instanceEnv = append(instanceEnv, key+"="+value)
	}
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:           repoDir,
		AgentEnv:          instanceEnv,
		ComparisonTargets: map[string]models.ComparisonTarget{"": target},
	}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
	shimDir := installComparisonTargetGitShim(t)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return mgr, markers
}

func installComparisonTargetGitShim(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	shim := filepath.Join(dir, "git")
	const script = `#!/bin/sh
printf '%s\n' "$*" >> "$KANDEV_TEST_GIT_LOG"
case "$1 $2" in
  "remote get-url")
    exit 1
    ;;
  "remote add"|"config remote."*)
    exit 0
    ;;
  "fetch --no-tags")
    if [ "${KANDEV_TEST_FETCH_ERROR:-0}" = "1" ]; then
      exit 1
    fi
    case "$*" in
      *"refs/heads/main:"*)
        : > "$KANDEV_TEST_FETCH_STARTED"
        if [ -n "${KANDEV_TEST_FETCH_GATE:-}" ]; then
          while [ ! -e "$KANDEV_TEST_FETCH_GATE" ]; do
            /bin/sleep 0.01
          done
        fi
        ;;
    esac
    ;;
  "rev-parse --git-dir")
    printf '.git\n'
    ;;
  "rev-parse --verify")
    printf '0123456789abcdef0123456789abcdef01234567\n'
    ;;
  "status --porcelain")
    : > "$KANDEV_TEST_STATUS_STARTED"
    ;;
esac
exit 0
`
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatalf("write Git shim: %v", err)
	}
	return dir
}

func waitForComparisonFile(t *testing.T, path string) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-ticker.C:
		case <-timeout.C:
			t.Fatalf("timed out waiting for %s", path)
		}
	}
}

func waitForComparisonResolution(
	t *testing.T,
	tracker *WorkspaceTracker,
	wantStatus string,
	wantErrorCode string,
) ComparisonResolution {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		resolution := tracker.ComparisonResolution()
		if resolution.Status == wantStatus && (wantErrorCode == "" || resolution.ErrorCode == wantErrorCode) {
			return resolution
		}
		select {
		case <-ticker.C:
		case <-timeout.C:
			t.Fatalf("timed out waiting for comparison resolution %s/%s, last %#v", wantStatus, wantErrorCode, resolution)
		}
	}
}
