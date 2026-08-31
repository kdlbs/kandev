package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	comparisonTargetGitShimModeEnv = "KANDEV_TEST_COMPARISON_GIT_SHIM_MODE"
	comparisonTargetCallTimeout    = 15 * time.Second
)

func init() {
	if os.Getenv(comparisonTargetGitShimModeEnv) == "comparison" {
		runComparisonTargetGitShim()
	}
}

func TestComparisonTargetPreparationNonBlocking(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	target := comparisonTargetProcessTestTarget()
	gate := filepath.Join(t.TempDir(), "fetch.gate")
	mgr, markers := newComparisonTargetTestManager(t, repoDir, target, map[string]string{
		"KANDEV_TEST_FETCH_GATE": gate,
	})
	t.Cleanup(func() { _ = os.WriteFile(gate, nil, 0o600) })

	done := make(chan struct{}, 1)
	go func() {
		mgr.PrepareComparisonTargets(context.Background())
		close(done)
	}()
	waitForComparisonCall(t, done, "PrepareComparisonTargets")

	resolution := mgr.GetWorkspaceTracker().ComparisonResolution()
	if !resolution.Explicit || resolution.Status != comparisonTargetStatusUnavailable ||
		resolution.ErrorCode != comparisonTargetErrorPending {
		t.Fatalf("comparison resolution = %#v, want explicit pending state", resolution)
	}
	waitForComparisonFile(t, markers.fetchStarted)
	if _, err := os.Stat(gate); err == nil {
		t.Fatal("comparison fetch gate opened before PrepareComparisonTargets returned")
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatalf("release comparison fetch: %v", err)
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
	waitForComparisonFile(t, markers.statusStarted)
	if resolution.Ref != "" {
		t.Fatalf("failed comparison ref = %q, want empty", resolution.Ref)
	}
}

func TestComparisonTargetPublicationRequiresActiveOperation(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	first := comparisonTargetProcessTestTarget()
	second := first
	second.TargetBranch = "develop"
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:           repoDir,
		ComparisonTargets: map[string]models.ComparisonTarget{"": first},
	}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })

	tracker := mgr.GetWorkspaceTracker()
	tracker.SetComparisonTarget(&first)
	_, cancel := context.WithCancel(context.Background())
	operation := &comparisonTargetOperation{target: first, tracker: tracker, cancel: cancel}
	mgr.comparisonTargetOpsMu.Lock()
	mgr.comparisonTargetOps = map[string]*comparisonTargetOperation{"": operation}
	mgr.comparisonTargetOpsMu.Unlock()
	mgr.comparisonTargetsMu.Lock()
	mgr.cfg.ComparisonTargets[""] = second
	mgr.comparisonTargetsMu.Unlock()

	if mgr.publishComparisonTargetReady("", operation, first.ComparisonRef()) {
		t.Fatal("superseded comparison operation published ready state")
	}
	resolution := tracker.ComparisonResolution()
	if resolution.Status != comparisonTargetStatusUnavailable || resolution.ErrorCode != comparisonTargetErrorPending {
		t.Fatalf("comparison resolution = %#v, want pending state", resolution)
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

	done := make(chan struct{}, 1)
	go func() {
		mgr.UpdateComparisonTargets(context.Background(), map[string]models.ComparisonTarget{"": target})
		close(done)
	}()
	waitForComparisonCall(t, done, "UpdateComparisonTargets")
	waitForComparisonFile(t, markers.fetchStarted)
	if resolution := mgr.GetWorkspaceTracker().ComparisonResolution(); resolution.ErrorCode != comparisonTargetErrorPending {
		t.Fatalf("comparison resolution = %#v, want pending state", resolution)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatalf("release comparison fetch: %v", err)
	}
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

	resultCh := make(chan struct {
		tracker *WorkspaceTracker
		err     error
	}, 1)
	go func() {
		tracker, err := mgr.GetWorkspaceTrackerFor("lazy")
		resultCh <- struct {
			tracker *WorkspaceTracker
			err     error
		}{tracker: tracker, err: err}
	}()
	var result struct {
		tracker *WorkspaceTracker
		err     error
	}
	select {
	case result = <-resultCh:
	case <-time.After(comparisonTargetCallTimeout):
		t.Fatal("timed out waiting for GetWorkspaceTrackerFor")
	}
	if result.err != nil {
		t.Fatalf("GetWorkspaceTrackerFor: %v", result.err)
	}
	tracker := result.tracker
	waitForComparisonFile(t, markers.fetchStarted)
	resolution := tracker.ComparisonResolution()
	if !resolution.Explicit || resolution.ErrorCode != comparisonTargetErrorPending {
		t.Fatalf("lazy comparison resolution = %#v, want pending", resolution)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatalf("release comparison fetch: %v", err)
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
		comparisonTargetGitShimModeEnv + "=comparison",
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
	return installComparisonTargetGitShimMode(t, "comparison")
}

func installComparisonTargetGitShimMode(t *testing.T, mode string) string {
	t.Helper()
	dir := t.TempDir()
	shimName := "git"
	if runtime.GOOS == "windows" {
		shimName += ".exe"
	}
	shim := filepath.Join(dir, shimName)
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("find test executable: %v", err)
	}
	contents, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read test executable: %v", err)
	}
	if err := os.WriteFile(shim, contents, 0o755); err != nil {
		t.Fatalf("write Git shim: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(shim, 0o755); err != nil {
			t.Fatalf("make Git shim executable: %v", err)
		}
	}
	t.Setenv(comparisonTargetGitShimModeEnv, mode)
	return dir
}

func runComparisonTargetGitShim() {
	args := os.Args[1:]
	if logPath := os.Getenv("KANDEV_TEST_GIT_LOG"); logPath != "" {
		if file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666); err == nil {
			_, _ = fmt.Fprintln(file, strings.Join(args, " "))
			_ = file.Close()
		}
	}
	if len(args) < 2 {
		os.Exit(0)
	}
	if args[0] == "config" && strings.HasPrefix(args[1], "remote.") {
		os.Exit(0)
	}
	switch args[0] + " " + args[1] {
	case "remote get-url":
		os.Exit(1)
	case "remote add":
		os.Exit(0)
	case "fetch --no-tags":
		if os.Getenv("KANDEV_TEST_FETCH_ERROR") == "1" {
			os.Exit(1)
		}
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "refs/heads/main:") {
			touchComparisonTargetShimMarker("KANDEV_TEST_FETCH_STARTED")
			waitForComparisonTargetShimGate()
		}
	case "rev-parse --git-dir":
		_, _ = os.Stdout.WriteString(".git\n")
	case "rev-parse --verify":
		_, _ = os.Stdout.WriteString("0123456789abcdef0123456789abcdef01234567\n")
	case "status --porcelain":
		touchComparisonTargetShimMarker("KANDEV_TEST_STATUS_STARTED")
	}
	os.Exit(0)
}

func touchComparisonTargetShimMarker(envKey string) {
	if path := os.Getenv(envKey); path != "" {
		_ = os.WriteFile(path, nil, 0o600)
	}
}

func waitForComparisonTargetShimGate() {
	gate := os.Getenv("KANDEV_TEST_FETCH_GATE")
	if gate == "" {
		return
	}
	for {
		if _, err := os.Stat(gate); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForComparisonFile(t *testing.T, path string) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(comparisonTargetCallTimeout)
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

func waitForComparisonCall(t *testing.T, done <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(comparisonTargetCallTimeout):
		t.Fatalf("timed out waiting for %s to return", operation)
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
	timeout := time.NewTimer(comparisonTargetCallTimeout)
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
