package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/stretchr/testify/require"
)

type retainedRepositoryHTTPFixture struct {
	client           *agentctl.Client
	runner           *process.ProcessRunner
	materializations atomic.Int32
	rescans          atomic.Int32
	stops            atomic.Int32
	cancelOnRead     context.CancelFunc
	failRescan       bool
	failStop         bool
}

func newRetainedRepositoryHTTPFixture(t *testing.T, materializationOK bool) *retainedRepositoryHTTPFixture {
	t.Helper()
	fixture := &retainedRepositoryHTTPFixture{runner: process.NewProcessRunner(nil, dialerTestLogger(t), 4096)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/v1/processes/start":
			var req process.StartProcessRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad process", 400)
				return
			}
			info, err := fixture.runner.Start(context.Background(), req)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"process": info})
		case r.URL.Path == "/api/v1/processes/stop":
			var req process.StopProcessRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad stop", 400)
				return
			}
			fixture.stops.Add(1)
			if fixture.failStop {
				http.Error(w, "process stop failed", http.StatusInternalServerError)
				return
			}
			_ = fixture.runner.Stop(r.Context(), req)
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case r.URL.Path == "/api/v1/workspace/rescan":
			fixture.rescans.Add(1)
			if fixture.failRescan {
				http.Error(w, "workspace rescan failed", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		case strings.HasPrefix(r.URL.Path, "/api/v1/processes/"):
			if fixture.cancelOnRead != nil {
				fixture.cancelOnRead()
			}
			info, ok := fixture.runner.Get(strings.TrimPrefix(r.URL.Path, "/api/v1/processes/"), r.URL.Query().Get("include_output") == "true")
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(info)
		case strings.Contains(r.URL.Path, "materialize"):
			fixture.materializations.Add(1)
			if !materializationOK {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "existing checkout has unpublished commits"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"reused": false})
		default:
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		}
	}))
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	fixture.client = agentctl.NewClient(u.Hostname(), port, dialerTestLogger(t), agentctl.WithAuthToken("fixture-token"))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = fixture.runner.StopAllAndWait(ctx)
		fixture.client.Close()
		server.Close()
	})
	return fixture
}

func retainedRepositoryLaunch(t *testing.T) (*LaunchRequest, string, string) {
	t.Helper()
	root := t.TempDir()
	origin := "https://example.com/secondary.git"
	request := &LaunchRequest{SessionID: "retained-probe-session", Repositories: []RepoLaunchSpec{
		{RepoName: "primary", RepositoryURL: "https://example.com/primary.git", BaseBranch: "main"},
		{RepoName: "secondary", RepositoryURL: origin, BaseBranch: "main", CheckoutBranch: "feature/retained"},
	}}
	projection, err := remoteWorkspaceProjectionFromLaunch(request, false)
	require.NoError(t, err)
	directory := filepath.Join(root, projection[0].Destination)
	require.NoError(t, os.Mkdir(directory, 0700))
	initGitRepoAt(t, directory)
	runGitForPreparerTest(t, directory, "remote", "add", "origin", origin)
	runGitForPreparerTest(t, directory, "update-ref", "refs/remotes/origin/main", "HEAD")
	runGitForPreparerTest(t, directory, "checkout", "-b", "feature/retained")
	runGitForPreparerTest(t, directory, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "unpublished local work")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "untracked.txt"), []byte("keep local work"), 0600))
	return request, root, directory
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestLaunchRepositoryReconciliationPreservesUnpublishedCommit(t *testing.T) {
	request, root, directory := retainedRepositoryLaunch(t)
	fixture := newRetainedRepositoryHTTPFixture(t, false)
	before := runGitForPreparerTest(t, directory, "show-ref") + runGitForPreparerTest(t, directory, "status", "--porcelain")
	instance := &ExecutorInstance{RuntimeName: agentruntime.RuntimeRemoteDocker, ReusedWorkspace: true, Client: fixture.client, WorkspacePath: root}
	err := reconcileLaunchWorkspace(context.Background(), NewRemoteDockerExecutor(dialerTestLogger(t)), request, instance)
	require.NoError(t, err, "a valid retained branch must accept its unpublished commit")
	require.Zero(t, fixture.materializations.Load(), "retained source must not be rematerialized")
	require.EqualValues(t, 1, fixture.rescans.Load(), "verified repositories must be added to workspace trackers")
	require.Equal(t, before, runGitForPreparerTest(t, directory, "show-ref")+runGitForPreparerTest(t, directory, "status", "--porcelain"))
	require.Eventually(t, func() bool { return len(fixture.runner.List(request.SessionID)) == 0 }, 3*time.Second, 10*time.Millisecond)
}

func TestVerifyRetainedLaunchWorkspaceAcceptsBranchWhenTagSharesName(t *testing.T) {
	request, root, directory := retainedRepositoryLaunch(t)
	runGitForPreparerTest(t, directory, "tag", "feature/retained")
	fixture := newRetainedRepositoryHTTPFixture(t, false)
	repositories, err := remoteWorkspaceProjectionFromLaunch(request, false)
	require.NoError(t, err)
	instance := &ExecutorInstance{Client: fixture.client, WorkspacePath: root}

	err = verifyRetainedLaunchWorkspace(context.Background(), request.SessionID, instance, repositories)

	require.NoError(t, err, "a matching branch must remain valid when a tag has the same name")
}

func TestVerifyRetainedLaunchWorkspaceReportsProbeStopFailure(t *testing.T) {
	request, root, _ := retainedRepositoryLaunch(t)
	fixture := newRetainedRepositoryHTTPFixture(t, false)
	fixture.failStop = true
	repositories, err := remoteWorkspaceProjectionFromLaunch(request, false)
	require.NoError(t, err)
	instance := &ExecutorInstance{Client: fixture.client, WorkspacePath: root}

	err = verifyRetainedLaunchWorkspace(context.Background(), request.SessionID, instance, repositories)

	require.ErrorContains(t, err, "stop retained repository verification")
}

func TestLaunchRepositoryReconciliationPreservesTrackedUncommittedChanges(t *testing.T) {
	request, root, directory := retainedRepositoryLaunch(t)
	trackedPath := filepath.Join(directory, "tracked.txt")
	require.NoError(t, os.WriteFile(trackedPath, []byte("committed\n"), 0600))
	runGitForPreparerTest(t, directory, "add", "tracked.txt")
	runGitForPreparerTest(t, directory, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "track file")
	require.NoError(t, os.WriteFile(trackedPath, []byte("local edit\n"), 0600))
	fixture := newRetainedRepositoryHTTPFixture(t, false)
	before := runGitForPreparerTest(t, directory, "show-ref") + runGitForPreparerTest(t, directory, "status", "--porcelain")
	instance := &ExecutorInstance{RuntimeName: agentruntime.RuntimeRemoteDocker, ReusedWorkspace: true, Client: fixture.client, WorkspacePath: root}

	err := reconcileLaunchWorkspace(context.Background(), NewRemoteDockerExecutor(dialerTestLogger(t)), request, instance)

	require.NoError(t, err)
	content, err := os.ReadFile(trackedPath)
	require.NoError(t, err)
	require.Equal(t, "local edit\n", string(content))
	require.Equal(t, before, runGitForPreparerTest(t, directory, "show-ref")+runGitForPreparerTest(t, directory, "status", "--porcelain"))
	require.Zero(t, fixture.materializations.Load())
}

func TestVerifyRetainedLaunchWorkspaceWaitsForGitAdmission(t *testing.T) {
	restore := subproc.Git().SetCapForTest(1)
	t.Cleanup(restore)
	fixture := newRetainedRepositoryHTTPFixture(t, false)
	request, root, _ := retainedRepositoryLaunch(t)
	repositories, err := remoteWorkspaceProjectionFromLaunch(request, false)
	require.NoError(t, err)

	hold, err := subproc.AcquireGit(context.Background(), subproc.GitLifecycle)
	require.NoError(t, err)
	defer hold()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	instance := &ExecutorInstance{Client: fixture.client, WorkspacePath: root}
	done := make(chan error, 1)
	go func() {
		done <- verifyRetainedLaunchWorkspace(ctx, request.SessionID, instance, repositories)
	}()

	require.Eventually(t, func() bool {
		return subproc.AdmissionSnapshot().Classes[string(subproc.GitLifecycle)].Waiters == 1
	}, time.Second, 10*time.Millisecond, "retained Git verification must wait for lifecycle admission")
	require.Empty(t, fixture.runner.List(request.SessionID), "verification must not start Git before admission")

	hold()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("retained Git verification did not finish after admission")
	}
}

func TestLaunchRepositoryReconciliationPreservesWorkspaceWhenRescanFails(t *testing.T) {
	request, root, directory := retainedRepositoryLaunch(t)
	fixture := newRetainedRepositoryHTTPFixture(t, false)
	fixture.failRescan = true
	before := runGitForPreparerTest(t, directory, "show-ref") + runGitForPreparerTest(t, directory, "status", "--porcelain")
	instance := &ExecutorInstance{RuntimeName: agentruntime.RuntimeRemoteDocker, ReusedWorkspace: true, Client: fixture.client, WorkspacePath: root}

	err := reconcileLaunchWorkspace(context.Background(), NewRemoteDockerExecutor(dialerTestLogger(t)), request, instance)

	require.Error(t, err, "launch must fail when workspace trackers cannot discover the retained repositories")
	require.EqualValues(t, 1, fixture.rescans.Load())
	require.Zero(t, fixture.materializations.Load())
	require.Equal(t, before, runGitForPreparerTest(t, directory, "show-ref")+runGitForPreparerTest(t, directory, "status", "--porcelain"))
	require.Eventually(t, func() bool { return len(fixture.runner.List(request.SessionID)) == 0 }, 3*time.Second, 10*time.Millisecond)
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestLaunchRepositoryReconciliationKeepsFreshAndOtherProvidersMaterializing(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(strconv.FormatBool(retained), func(t *testing.T) {
			request, root, _ := retainedRepositoryLaunch(t)
			fixture := newRetainedRepositoryHTTPFixture(t, true)
			runtime := agentruntime.RuntimeRemoteDocker
			if retained {
				runtime = agentruntime.RuntimeStandalone
			}
			instance := &ExecutorInstance{RuntimeName: runtime, ReusedWorkspace: retained, Client: fixture.client, WorkspacePath: root}
			require.NoError(t, reconcileLaunchWorkspace(context.Background(), NewRemoteDockerExecutor(dialerTestLogger(t)), request, instance))
			require.EqualValues(t, 1, fixture.materializations.Load())
			require.Zero(t, fixture.stops.Load())
		})
	}
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestLaunchRepositoryReconciliationRejectsWrongRetainedIdentity(t *testing.T) {
	request, root, directory := retainedRepositoryLaunch(t)
	runGitForPreparerTest(t, directory, "remote", "set-url", "origin", "https://example.com/wrong.git")
	before := runGitForPreparerTest(t, directory, "show-ref") + runGitForPreparerTest(t, directory, "status", "--porcelain")
	fixture := newRetainedRepositoryHTTPFixture(t, false)
	instance := &ExecutorInstance{RuntimeName: agentruntime.RuntimeRemoteDocker, ReusedWorkspace: true, Client: fixture.client, WorkspacePath: root}
	err := reconcileLaunchWorkspace(context.Background(), NewRemoteDockerExecutor(dialerTestLogger(t)), request, instance)
	require.ErrorContains(t, err, "workspace preserved")
	require.Zero(t, fixture.materializations.Load())
	require.Equal(t, before, runGitForPreparerTest(t, directory, "show-ref")+runGitForPreparerTest(t, directory, "status", "--porcelain"))
	require.Eventually(t, func() bool { return len(fixture.runner.List(request.SessionID)) == 0 }, 3*time.Second, 10*time.Millisecond)
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestLaunchRepositoryReconciliationCancellationStopsOnlyOwnedProbe(t *testing.T) {
	request, root, _ := retainedRepositoryLaunch(t)
	fixture := newRetainedRepositoryHTTPFixture(t, false)
	other, err := fixture.runner.Start(context.Background(), process.StartProcessRequest{SessionID: request.SessionID, Kind: "custom", Command: "sleep 30", WorkingDir: root})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fixture.cancelOnRead = cancel
	instance := &ExecutorInstance{RuntimeName: agentruntime.RuntimeRemoteDocker, ReusedWorkspace: true, Client: fixture.client, WorkspacePath: root}
	err = reconcileLaunchWorkspace(ctx, NewRemoteDockerExecutor(dialerTestLogger(t)), request, instance)
	require.Error(t, err)
	require.Eventually(t, func() bool { return len(fixture.runner.List(request.SessionID)) == 1 }, 3*time.Second, 10*time.Millisecond)
	_, exists := fixture.runner.Get(other.ID, false)
	require.True(t, exists, "unrelated session processes must survive")
	require.EqualValues(t, 1, fixture.stops.Load())
	require.Zero(t, fixture.materializations.Load())
}
