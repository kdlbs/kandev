//go:build !windows

package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

const freshMultiStatusPath = "/api/v1/git/status/multi?fresh=true"

func TestGitStatusMultiFreshCoalescesOverlappingHTTPRequests(t *testing.T) {
	server, repoNames := newMultiRepoStatusServer(t)
	gate := newGitStatusCommandGate(t)
	router := server.Router()

	firstResult := make(chan gitStatusHTTPResult, 1)
	go serveFreshMultiStatus(router, httptest.NewRequest(http.MethodGet, freshMultiStatusPath, nil), firstResult)
	started := gate.waitForStarts(t, len(repoNames))
	assertRepoNames(t, started, repoNames)

	peerObserved := make(chan struct{}, len(repoNames))
	peerCtx := &doneObservedContext{Context: context.Background(), observed: peerObserved}
	peerResult := make(chan gitStatusHTTPResult, 1)
	peerReq := httptest.NewRequest(http.MethodGet, freshMultiStatusPath, nil).WithContext(peerCtx)
	go serveFreshMultiStatus(router, peerReq, peerResult)
	waitForContextObservations(t, peerObserved, len(repoNames), "successful peer")

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancelObserved := make(chan struct{}, len(repoNames))
	canceledResult := make(chan gitStatusHTTPResult, 1)
	canceledReq := httptest.NewRequest(http.MethodGet, freshMultiStatusPath, nil).WithContext(
		&doneObservedContext{Context: cancelCtx, observed: cancelObserved},
	)
	go serveFreshMultiStatus(router, canceledReq, canceledResult)
	waitForContextObservations(t, cancelObserved, len(repoNames), "canceled waiter")
	cancel()
	assertCanceledMultiStatus(t, waitForGitStatusHTTPResult(t, canceledResult), repoNames)

	gate.releaseAll(t, 32)
	first := waitForGitStatusHTTPResult(t, firstResult)
	peer := waitForGitStatusHTTPResult(t, peerResult)
	assertSharedFreshStatuses(t, first, peer, repoNames)
	gate.assertInvocationCounts(t, repoNames, 1)
}

func TestGitStatusMultiFreshKeepsInvalidRepoFailureLocal(t *testing.T) {
	taskRoot := t.TempDir()
	newStatusTestRepo(t, taskRoot, "healthy")
	brokenPath := newStatusTestRepo(t, taskRoot, "broken")

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: taskRoot}
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	if err := os.Rename(brokenPath, filepath.Join(taskRoot, ".broken")); err != nil {
		t.Fatalf("invalidate repository: %v", err)
	}

	resultCh := make(chan gitStatusHTTPResult, 1)
	serveFreshMultiStatus(
		server.Router(),
		httptest.NewRequest(http.MethodGet, freshMultiStatusPath, nil),
		resultCh,
	)
	result := waitForGitStatusHTTPResult(t, resultCh)
	if result.code != http.StatusOK || !result.body.Success {
		t.Fatalf("multi status = %d, %+v", result.code, result.body)
	}
	statuses := statusesByRepo(t, result.body, []string{"broken", "healthy"})
	if !statuses["healthy"].Success {
		t.Fatalf("healthy repository failed: %+v", statuses["healthy"])
	}
	if statuses["broken"].Success || !strings.Contains(statuses["broken"].Error, "repo subpath not found") {
		t.Fatalf("broken repository status = %+v, want local not-found failure", statuses["broken"])
	}
}

type doneObservedContext struct {
	context.Context
	observed chan<- struct{}
}

func (c *doneObservedContext) Done() <-chan struct{} {
	c.observed <- struct{}{}
	return c.Context.Done()
}

type gitStatusHTTPResult struct {
	code int
	body MultiRepoGitStatusResult
}

func serveFreshMultiStatus(router http.Handler, req *http.Request, resultCh chan<- gitStatusHTTPResult) {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	var body MultiRepoGitStatusResult
	err := json.Unmarshal(recorder.Body.Bytes(), &body)
	if err != nil {
		body = MultiRepoGitStatusResult{Error: err.Error()}
	}
	resultCh <- gitStatusHTTPResult{code: recorder.Code, body: body}
}

func waitForGitStatusHTTPResult(t *testing.T, resultCh <-chan gitStatusHTTPResult) gitStatusHTTPResult {
	t.Helper()
	select {
	case result := <-resultCh:
		return result
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for git status HTTP response")
		return gitStatusHTTPResult{}
	}
}

func waitForContextObservations(t *testing.T, observed <-chan struct{}, count int, label string) {
	t.Helper()
	for range count {
		select {
		case <-observed:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s to join shared observations", label)
		}
	}
}

func assertCanceledMultiStatus(t *testing.T, result gitStatusHTTPResult, repoNames []string) {
	t.Helper()
	if result.code != http.StatusOK || !result.body.Success {
		t.Fatalf("canceled multi status = %d, %+v", result.code, result.body)
	}
	for repo, status := range statusesByRepo(t, result.body, repoNames) {
		if status.Success || !strings.Contains(status.Error, context.Canceled.Error()) {
			t.Errorf("canceled status for %s = %+v", repo, status)
		}
	}
}

func assertSharedFreshStatuses(t *testing.T, first, peer gitStatusHTTPResult, repoNames []string) {
	t.Helper()
	if first.code != http.StatusOK || peer.code != http.StatusOK || !first.body.Success || !peer.body.Success {
		t.Fatalf("fresh responses: first=%d %+v peer=%d %+v", first.code, first.body, peer.code, peer.body)
	}
	firstByRepo := statusesByRepo(t, first.body, repoNames)
	peerByRepo := statusesByRepo(t, peer.body, repoNames)
	for _, repo := range repoNames {
		if !firstByRepo[repo].Success || !peerByRepo[repo].Success {
			t.Errorf("repository %s failed: first=%+v peer=%+v", repo, firstByRepo[repo], peerByRepo[repo])
			continue
		}
		if firstByRepo[repo].Timestamp == "" || firstByRepo[repo].Timestamp != peerByRepo[repo].Timestamp {
			t.Errorf("repository %s timestamps differ: %q != %q", repo, firstByRepo[repo].Timestamp, peerByRepo[repo].Timestamp)
		}
	}
}

func statusesByRepo(t *testing.T, result MultiRepoGitStatusResult, wantRepos []string) map[string]GitStatusResult {
	t.Helper()
	if len(result.Repos) != len(wantRepos) {
		t.Fatalf("repository entries = %d, want %d: %+v", len(result.Repos), len(wantRepos), result)
	}
	statuses := make(map[string]GitStatusResult, len(result.Repos))
	for _, repo := range result.Repos {
		statuses[repo.RepositoryName] = repo.Status
	}
	assertRepoNames(t, mapKeys(statuses), wantRepos)
	return statuses
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func assertRepoNames(t *testing.T, got, want []string) {
	t.Helper()
	gotSet := make(map[string]bool, len(got))
	for _, repo := range got {
		gotSet[repo] = true
	}
	for _, repo := range want {
		if !gotSet[repo] {
			t.Errorf("repositories %v missing %q", got, repo)
		}
	}
}

func newMultiRepoStatusServer(t *testing.T) (*Server, []string) {
	t.Helper()
	taskRoot := t.TempDir()
	repoNames := []string{"alpha", "beta"}
	for _, repo := range repoNames {
		newStatusTestRepo(t, taskRoot, repo)
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	cfg := &config.InstanceConfig{WorkDir: taskRoot}
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log), repoNames
}

func newStatusTestRepo(t *testing.T, taskRoot, name string) string {
	t.Helper()
	repoDir := filepath.Join(taskRoot, name)
	if err := os.Mkdir(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	runGitAPI(t, repoDir, "init", "--initial-branch=main")
	runGitAPI(t, repoDir, "config", "user.email", "test@test.com")
	runGitAPI(t, repoDir, "config", "user.name", "Test User")
	writeFileAPI(t, repoDir, "README.md", name+"\n")
	runGitAPI(t, repoDir, "add", ".")
	runGitAPI(t, repoDir, "commit", "-m", "initial")
	return repoDir
}

type gitStatusCommandGate struct {
	events  *os.File
	release *os.File
	logPath string
}

func newGitStatusCommandGate(t *testing.T) *gitStatusCommandGate {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	root := t.TempDir()
	eventPath := filepath.Join(root, "events")
	releasePath := filepath.Join(root, "release")
	for _, path := range []string{eventPath, releasePath} {
		if output, err := exec.Command("mkfifo", path).CombinedOutput(); err != nil {
			t.Fatalf("mkfifo %s: %v: %s", path, err, output)
		}
	}
	events := openStatusFIFO(t, eventPath)
	release := openStatusFIFO(t, releasePath)
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatalf("mkdir git shim bin: %v", err)
	}
	wrapperPath := filepath.Join(binDir, "git")
	if err := os.WriteFile(wrapperPath, []byte(gitStatusWrapperScript), 0o755); err != nil {
		t.Fatalf("write git shim: %v", err)
	}
	logPath := filepath.Join(root, "invocations.log")
	t.Setenv("KANDEV_TEST_REAL_GIT", realGit)
	t.Setenv("KANDEV_TEST_GIT_EVENTS", eventPath)
	t.Setenv("KANDEV_TEST_GIT_RELEASE", releasePath)
	t.Setenv("KANDEV_TEST_GIT_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &gitStatusCommandGate{events: events, release: release, logPath: logPath}
}

func openStatusFIFO(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open fifo %s: %v", path, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func (g *gitStatusCommandGate) waitForStarts(t *testing.T, count int) []string {
	t.Helper()
	resultCh := make(chan []string, 1)
	go func() {
		scanner := bufio.NewScanner(g.events)
		started := make([]string, 0, count)
		for len(started) < count && scanner.Scan() {
			started = append(started, scanner.Text())
		}
		resultCh <- started
	}()
	select {
	case started := <-resultCh:
		return started
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for gated git status commands")
		return nil
	}
}

func (g *gitStatusCommandGate) releaseAll(t *testing.T, count int) {
	t.Helper()
	if _, err := g.release.Write(bytes.Repeat([]byte("go\n"), count)); err != nil {
		t.Fatalf("release git status commands: %v", err)
	}
}

func (g *gitStatusCommandGate) assertInvocationCounts(t *testing.T, repos []string, want int) {
	t.Helper()
	content, err := os.ReadFile(g.logPath)
	if err != nil {
		t.Fatalf("read git shim log: %v", err)
	}
	counts := make(map[string]int)
	for _, repo := range strings.Fields(string(content)) {
		counts[repo]++
	}
	for _, repo := range repos {
		if counts[repo] != want {
			t.Errorf("primary observations for %s = %d, want %d (all counts: %v)", repo, counts[repo], want, counts)
		}
	}
}

const gitStatusWrapperScript = `#!/bin/sh
if [ "$#" -ge 3 ] && [ "$1" = "rev-parse" ] && [ "$2" = "--abbrev-ref" ] && [ "$3" = "HEAD" ]; then
	repo=${PWD##*/}
	printf '%s\n' "$repo" >> "$KANDEV_TEST_GIT_LOG"
	printf '%s\n' "$repo" > "$KANDEV_TEST_GIT_EVENTS"
	IFS= read -r ignored < "$KANDEV_TEST_GIT_RELEASE"
fi
exec "$KANDEV_TEST_REAL_GIT" "$@"
`
