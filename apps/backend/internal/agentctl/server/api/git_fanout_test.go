package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestFanOutReposRunsConcurrently proves parallelism without measuring how long
// anything takes. Every collect call blocks on a gate that opens only once
// maxGitFanout of them have arrived, so serial execution cannot pass: the first
// would wait on the gate forever and the rest would never start. The timeout is
// a failure detector rather than a threshold, so machine speed does not enter
// into it.
//
// The gate is plain channels rather than the git shim in
// git_status_fresh_concurrency_test.go, which is //go:build !windows because it
// needs mkfifo. This one runs everywhere and covers the shared helper both
// multi-repo handlers use.
func TestFanOutReposRunsConcurrently(t *testing.T) {
	subpaths := []string{"frontend", "backend", "shared"}

	gate := make(chan struct{})
	var arrived sync.WaitGroup
	arrived.Add(len(subpaths))

	done := make(chan []string, 1)
	go func() {
		done <- fanOutRepos(context.Background(), subpaths, func(_ context.Context, sub string) string {
			arrived.Done()
			<-gate
			return sub
		})
	}()

	waitOrFail(t, &arrived, "not every repository entered the fan-out; the work is running serially")
	close(gate)

	select {
	case got := <-done:
		if len(got) != len(subpaths) {
			t.Fatalf("got %d results, want %d", len(got), len(subpaths))
		}
		// Order follows subpaths, not completion order, or the merged output
		// would depend on goroutine scheduling.
		for i, want := range subpaths {
			if got[i] != want {
				t.Errorf("result[%d] = %q, want %q — results must be ordered by subpath", i, got[i], want)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("fan-out did not return after the gate opened")
	}
}

// TestFanOutReposRespectsLimit pins the bound. With more repositories than
// maxGitFanout, the group must admit exactly maxGitFanout at once — enough to
// parallelise, never enough for one request to take the whole shared git
// budget from the background pollers.
func TestFanOutReposRespectsLimit(t *testing.T) {
	subpaths := make([]string, maxGitFanout+4)
	for i := range subpaths {
		subpaths[i] = fmt.Sprintf("repo%d", i)
	}

	var inFlight, peak int64
	admitted := make(chan struct{}, len(subpaths))
	release := make(chan struct{})

	done := make(chan []string, 1)
	go func() {
		done <- fanOutRepos(context.Background(), subpaths, func(_ context.Context, sub string) string {
			cur := atomic.AddInt64(&inFlight, 1)
			for {
				old := atomic.LoadInt64(&peak)
				if cur <= old || atomic.CompareAndSwapInt64(&peak, old, cur) {
					break
				}
			}
			admitted <- struct{}{}
			<-release
			atomic.AddInt64(&inFlight, -1)
			return sub
		})
	}()

	// Reading exactly maxGitFanout admissions blocks until the group is full,
	// which is the parallelism half. It cannot over-read: the rest are held
	// behind SetLimit until we release these.
	for i := 0; i < maxGitFanout; i++ {
		select {
		case <-admitted:
		case <-time.After(10 * time.Second):
			t.Fatalf("only %d of %d workers were admitted; the group is not parallel", i, maxGitFanout)
		}
	}
	if got := atomic.LoadInt64(&peak); got > maxGitFanout {
		t.Fatalf("peak concurrency = %d, want at most %d", got, maxGitFanout)
	}

	close(release)
	select {
	case got := <-done:
		if len(got) != len(subpaths) {
			t.Fatalf("got %d results, want %d", len(got), len(subpaths))
		}
	case <-time.After(20 * time.Second):
		t.Fatal("fan-out did not drain after the gate opened")
	}
	if got := atomic.LoadInt64(&peak); got != maxGitFanout {
		t.Errorf("peak concurrency = %d, want exactly %d", got, maxGitFanout)
	}
}

// TestFanOutReposPropagatesCancellation proves the context handed to each
// collect call is derived from the caller's, so a client that disconnects stops
// the git work in every repository rather than only the ones not yet started.
func TestFanOutReposPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	subpaths := []string{"frontend", "backend"}

	var arrived sync.WaitGroup
	arrived.Add(len(subpaths))
	observed := make(chan error, len(subpaths))

	done := make(chan []string, 1)
	go func() {
		done <- fanOutRepos(ctx, subpaths, func(callCtx context.Context, sub string) string {
			arrived.Done()
			<-callCtx.Done()
			observed <- callCtx.Err()
			return sub
		})
	}()

	waitOrFail(t, &arrived, "workers did not start")
	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("fan-out did not return after cancellation; the context is not reaching the workers")
	}
	for i := 0; i < len(subpaths); i++ {
		if err := <-observed; !errors.Is(err, context.Canceled) {
			t.Errorf("worker %d saw %v, want context.Canceled", i, err)
		}
	}
}

// waitOrFail blocks on wg and fails with msg if it does not complete, so the
// barrier reads the same way in every test above.
func waitOrFail(t *testing.T, wg *sync.WaitGroup, msg string) {
	t.Helper()
	ready := make(chan struct{})
	go func() {
		wg.Wait()
		close(ready)
	}()
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal(msg)
	}
}

// TestGitLogCancellationReachesGitOperation guards the single-repo path.
// runGitLogForRepo takes a context.Context and *gin.Context satisfies that
// interface, so handing it the gin context compiles — but this server is built
// with gin.New() and does not enable ContextWithFallback, so that context's
// Done() is nil and cancellation never reaches the subprocesses. With the bug
// the handler runs to completion and returns commits for a request the caller
// had already abandoned.
func TestGitLogCancellationReachesGitOperation(t *testing.T) {
	repoDir := t.TempDir()
	seedCancellationRepo(t, repoDir)
	base := strings.TrimSpace(runGitAPI(t, repoDir, "rev-parse", "main"))

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: repoDir}
	srv := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	path := "/api/v1/git/log?limit=100&since=" + base
	assertCancellationStopsGitLog(t, srv, path)
}

// TestGitLogMultiRepoCancellationReachesGitOperation covers the same guarantee
// on the fan-out path, where every repository runs on its own goroutine and has
// to inherit the request context rather than the gin one.
func TestGitLogMultiRepoCancellationReachesGitOperation(t *testing.T) {
	taskRoot := t.TempDir()
	bases := map[string]string{}
	for _, repo := range []string{"frontend", "backend"} {
		repoDir := filepath.Join(taskRoot, repo)
		if err := os.Mkdir(repoDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", repo, err)
		}
		seedCancellationRepo(t, repoDir)
		bases[repo] = "main"
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: taskRoot, BaseBranches: bases}
	srv := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)

	assertCancellationStopsGitLog(t, srv, "/api/v1/git/log?limit=100")
}

// seedCancellationRepo builds a repository with one commit on main and one on a
// feature branch, so a git log over the divergence has something to return.
func seedCancellationRepo(t *testing.T, repoDir string) {
	t.Helper()
	runGitAPI(t, repoDir, "init", "--initial-branch=main")
	runGitAPI(t, repoDir, "config", "user.email", "test@test.com")
	runGitAPI(t, repoDir, "config", "user.name", "Test User")
	writeFileAPI(t, repoDir, "README.md", "base\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "initial")
	runGitAPI(t, repoDir, "checkout", "-b", "feature/cancel")
	writeFileAPI(t, repoDir, "changed.txt", "change\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "feature change")
}

// assertCancellationStopsGitLog checks the same request twice: once live, to
// prove the fixture actually produces commits, and once with an already
// cancelled context, which must not. Without the live half a broken fixture
// would look like a passing cancellation test.
func assertCancellationStopsGitLog(t *testing.T, srv *Server, path string) {
	t.Helper()

	live := httptest.NewRecorder()
	srv.Router().ServeHTTP(live, httptest.NewRequest(http.MethodGet, path, nil))
	var liveResult process.GitLogResult
	if err := json.Unmarshal(live.Body.Bytes(), &liveResult); err != nil {
		t.Fatalf("decode live git log: %v", err)
	}
	if len(liveResult.Commits) == 0 {
		t.Fatalf("fixture returned no commits with a live context: %s", live.Body.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := httptest.NewRecorder()
	srv.Router().ServeHTTP(cancelled, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))

	var cancelledResult process.GitLogResult
	if err := json.Unmarshal(cancelled.Body.Bytes(), &cancelledResult); err != nil {
		// A non-JSON body means the handler errored out, which is also a
		// legitimate way for cancellation to surface.
		return
	}
	if len(cancelledResult.Commits) > 0 {
		t.Errorf("returned %d commits for a cancelled request; cancellation is not reaching the git operation: %s",
			len(cancelledResult.Commits), cancelled.Body.String())
	}
}
