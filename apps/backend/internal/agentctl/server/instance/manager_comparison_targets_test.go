package instance

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/agent"
)

const instanceComparisonGitShimModeEnv = "KANDEV_TEST_INSTANCE_COMPARISON_GIT_SHIM_MODE"

func init() {
	if os.Getenv(instanceComparisonGitShimModeEnv) == "instance" {
		runInstanceComparisonGitShim()
	}
}

func TestCreateInstanceDoesNotWaitForComparisonTarget(t *testing.T) {
	isolateInstanceTestGitEnv(t)
	repoDir := createInstanceTestRepo(t)
	startedMarker := filepath.Join(t.TempDir(), "comparison.started")
	shimDir := installInstanceComparisonGitShim(t, startedMarker)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	log := newTestLogger(t)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, log)
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })
	mgr.SetServerFactory(func(_ *config.InstanceConfig, _ *process.Manager, _ *logger.Logger) http.Handler {
		return http.NotFoundHandler()
	})

	resultCh := make(chan struct {
		response *CreateResponse
		err      error
	}, 1)
	started := time.Now()
	go func() {
		response, err := mgr.CreateInstance(context.Background(), &CreateRequest{
			ID:            "comparison-target-startup",
			WorkspacePath: repoDir,
			ComparisonTargets: map[string]models.ComparisonTarget{
				"": instanceComparisonTestTarget(),
			},
		})
		resultCh <- struct {
			response *CreateResponse
			err      error
		}{response: response, err: err}
	}()

	var result struct {
		response *CreateResponse
		err      error
	}
	select {
	case result = <-resultCh:
		if result.err != nil {
			t.Fatalf("CreateInstance: %v", result.err)
		}
		if elapsed := time.Since(started); elapsed >= 5*time.Second {
			t.Fatalf("CreateInstance took %v, want it to return before comparison-target Git work completes", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CreateInstance waited for comparison-target Git work")
	}
	if result.response == nil {
		t.Fatal("CreateInstance returned a nil response")
	}
	t.Cleanup(func() { _ = mgr.StopInstance(context.Background(), result.response.ID) })
	waitForInstanceFile(t, startedMarker)
}

func isolateInstanceTestGitEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE",
		"GIT_AUTHOR_DATE", "GIT_COMMITTER_DATE",
	} {
		value, present := os.LookupEnv(key)
		if !present {
			continue
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() { _ = os.Setenv(key, value) })
	}
}

func createInstanceTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runInstanceTestGit(t, dir, "init", "-b", "main")
	runInstanceTestGit(t, dir, "config", "user.email", "test@example.com")
	runInstanceTestGit(t, dir, "config", "user.name", "Test User")
	runInstanceTestGit(t, dir, "commit", "--allow-empty", "-m", "initial")
	return dir
}

func runInstanceTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func installInstanceComparisonGitShim(t *testing.T, startedMarker string) string {
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
	t.Setenv(instanceComparisonGitShimModeEnv, "instance")
	t.Setenv("KANDEV_TEST_COMPARISON_STARTED", startedMarker)
	return dir
}

func runInstanceComparisonGitShim() {
	args := os.Args[1:]
	if len(args) < 2 {
		os.Exit(0)
	}
	switch args[0] + " " + args[1] {
	case "rev-parse --git-dir":
		_, _ = os.Stdout.WriteString(".git\n")
	case "remote get-url":
		if marker := os.Getenv("KANDEV_TEST_COMPARISON_STARTED"); marker != "" {
			_ = os.WriteFile(marker, nil, 0o600)
		}
		time.Sleep(15 * time.Second)
		os.Exit(1)
	case "rev-parse --verify":
		_, _ = os.Stdout.WriteString("0123456789abcdef0123456789abcdef01234567\n")
	}
	os.Exit(0)
}

func waitForInstanceFile(t *testing.T, path string) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(20 * time.Second)
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

func instanceComparisonTestTarget() models.ComparisonTarget {
	return models.ComparisonTarget{
		Version:      models.ComparisonTargetVersion,
		Provider:     models.ComparisonTargetProviderGitHub,
		Kind:         models.ComparisonTargetKindPullRequest,
		Number:       1154,
		HeadBranch:   "feature/cursor-cost",
		TargetBranch: "main",
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "contributor/widget", ProviderID: "head-42",
			RemoteURL: "https://github.com/contributor/widget.git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "upstream/widget", ProviderID: "base-99",
			RemoteURL: "https://github.com/upstream/widget.git",
		},
	}
}
