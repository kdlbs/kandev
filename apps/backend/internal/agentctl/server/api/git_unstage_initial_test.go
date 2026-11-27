package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.37, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.38
func TestHandleGitUnstageBeforeFirstCommit(t *testing.T) {
	for _, scenario := range []string{"root-all", "multi-all", "multi-selected", "multi-invalid"} {
		t.Run(scenario, func(t *testing.T) {
			isolateAPIExactGit(t)
			root := t.TempDir()
			t.Setenv("HOME", t.TempDir())
			repo, scope := root, ""
			if scenario != "root-all" {
				scope = "selected"
				repo = filepath.Join(root, scope)
			}
			initUnstageHTTPRepo(t, repo, "selected")
			other := filepath.Join(root, "independent")
			if scope != "" {
				initUnstageHTTPRepo(t, other, "independent")
			}
			server := newUnstageHTTPServer(t, root)
			callUnstageHTTP(t, server, "/api/v1/git/stage", GitStageRequest{Repo: scope}, true)
			if scope != "" {
				callUnstageHTTP(t, server, "/api/v1/git/stage", GitStageRequest{Repo: "independent"}, true)
			}
			runGitAPIExpectFailure(t, repo, "rev-parse", "--verify", "HEAD")
			writeFileAPI(t, repo, "initial[ab].txt", "selected later work\x00\n")
			before := readUnstageHTTPState(t, repo)
			var otherBefore unstageHTTPState
			if scope != "" {
				otherBefore = readUnstageHTTPState(t, other)
			}
			paths := []string{}
			switch scenario {
			case "multi-selected":
				paths = []string{"initial[ab].txt"}
			case "multi-invalid":
				paths = []string{""}
			}
			callUnstageHTTP(t, server, "/api/v1/git/unstage",
				GitUnstageRequest{Repo: scope, Paths: paths}, scenario != "multi-invalid")
			after := readUnstageHTTPState(t, repo)
			checkUnstageHTTPContent(t, before, after)
			checkUnstageHTTPInitialIndex(t, repo, scenario, before, after)
			runGitAPIExpectFailure(t, repo, "rev-parse", "--verify", "HEAD")
			if scope != "" && !reflect.DeepEqual(readUnstageHTTPState(t, other), otherBefore) {
				t.Error("selected-repository request changed independent index or working bytes")
			}
		})
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.38
func TestHandleGitUnstageAllCommitted(t *testing.T) {
	isolateAPIExactGit(t)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	initUnstageHTTPRepo(t, dir, "committed")
	runGitAPI(t, dir, "add", "-A")
	runGitAPI(t, dir, "commit", "-m", "HTTP unstage base")
	runGitAPI(t, dir, "tag", "keep")
	base := runGitAPI(t, dir, "ls-files", "--stage", "-z")
	head := runGitAPI(t, dir, "rev-parse", "HEAD")
	writeFileAPI(t, dir, "initial[ab].txt", "HTTP staged edit\n")
	writeFileAPI(t, dir, "added.txt", "HTTP staged addition\n")
	if err := os.Remove(filepath.Join(dir, "initiala.txt")); err != nil {
		t.Fatal(err)
	}
	server := newUnstageHTTPServer(t, dir)
	callUnstageHTTP(t, server, "/api/v1/git/stage", GitStageRequest{}, true)
	writeFileAPI(t, dir, "initial[ab].txt", "HTTP mixed work\x00\n")
	writeFileAPI(t, dir, "added.txt", "HTTP added work\n")
	before := readUnstageHTTPState(t, dir)
	if before.index == base {
		t.Fatal("HTTP fixture has no staged changes")
	}
	callUnstageHTTP(t, server, "/api/v1/git/unstage", GitUnstageRequest{}, true)
	after := readUnstageHTTPState(t, dir)
	checkUnstageHTTPContent(t, before, after)
	if after.index != base {
		t.Errorf("HTTP Unstage all index=%q, want base %q", after.index, base)
	}
	if got := runGitAPI(t, dir, "rev-parse", "HEAD"); got != head {
		t.Errorf("HTTP Unstage moved HEAD: %q", got)
	}
	if got := runGitAPI(t, dir, "rev-parse", "ORIG_HEAD"); got != head {
		t.Errorf("HTTP whole-reset bookkeeping=%q, want %q", got, head)
	}
}

func initUnstageHTTPRepo(t *testing.T, dir, marker string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitAPI(t, dir, "init", "--initial-branch=main")
	runGitAPI(t, dir, "config", "user.name", "HTTP Unstage Test")
	runGitAPI(t, dir, "config", "user.email", "http-unstage@example.test")
	runGitAPI(t, dir, "config", "core.autocrlf", "false")
	runGitAPI(t, dir, "config", "core.hooksPath", filepath.Join(t.TempDir(), "no-hooks"))
	for _, name := range []string{"initial[ab].txt", "initiala.txt", "sub/third.bin"} {
		writeFileAPI(t, dir, name, marker+" staged: "+name+"\n")
	}
}

func newUnstageHTTPServer(t *testing.T, root string) *Server {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.InstanceConfig{WorkDir: root, AgentEnv: os.Environ()}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() {
		if err := manager.StopForTeardown(context.Background()); err != nil {
			t.Errorf("HTTP manager cleanup: %v", err)
		}
	})
	return NewServer(cfg, manager, nil, nil, log)
}

func callUnstageHTTP(t *testing.T, server *Server, endpoint string, body any, wantSuccess bool) {
	t.Helper()
	rec := postGitAPI(t, server, endpoint, body)
	result := decodeGitOperationResult(t, rec)
	if rec.Code != http.StatusOK || result.Success != wantSuccess {
		t.Errorf("%s HTTP %d: %+v, want success=%v", endpoint, rec.Code, result, wantSuccess)
	}
	if !wantSuccess && result.Error == "" {
		t.Error("invalid selection returned no operation error")
	}
}

type unstageHTTPState struct {
	index    string
	metadata map[string]string
	content  map[string]string
	modes    map[string]os.FileMode
}

func readUnstageHTTPState(t *testing.T, dir string) unstageHTTPState {
	t.Helper()
	state := unstageHTTPState{
		index:    runGitAPI(t, dir, "ls-files", "--stage", "-z"),
		metadata: map[string]string{"refs": runGitAPI(t, dir, "for-each-ref")},
		content:  make(map[string]string),
		modes:    make(map[string]os.FileMode),
	}
	for _, name := range []string{"HEAD", "config"} {
		data, err := os.ReadFile(filepath.Join(dir, ".git", name))
		if err != nil {
			t.Fatal(err)
		}
		state.metadata[name] = string(data)
	}
	for _, path := range []string{"initial[ab].txt", "initiala.txt", "sub/third.bin", "added.txt"} {
		raw, err := os.ReadFile(filepath.Join(dir, path))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		state.content[path], state.modes[path] = string(raw), info.Mode().Perm()
	}
	return state
}

func checkUnstageHTTPContent(t *testing.T, before, after unstageHTTPState) {
	t.Helper()
	if !reflect.DeepEqual(before.metadata, after.metadata) ||
		!reflect.DeepEqual(before.content, after.content) || !reflect.DeepEqual(before.modes, after.modes) {
		t.Errorf("HTTP Unstage changed HEAD/refs/config or working bytes/permissions: before=%+v after=%+v", before, after)
	}
}

func checkUnstageHTTPInitialIndex(t *testing.T, repo, scenario string, before, after unstageHTTPState) {
	t.Helper()
	if before.index == "" || before.metadata["refs"] != "" {
		t.Fatal("HTTP initial fixture must contain staged files and no refs")
	}
	switch scenario {
	case "multi-invalid":
		if after.index != before.index {
			t.Error("invalid HTTP selection changed index")
		}
	case "multi-selected":
		if got := runGitAPI(t, repo, "ls-files", "-z"); got != "initiala.txt\x00sub/third.bin\x00" {
			t.Errorf("HTTP selected membership=%q", got)
		}
		for _, path := range []string{"initiala.txt", "sub/third.bin"} {
			if got := runGitAPI(t, repo, "show", ":"+path); got != before.content[path] {
				t.Errorf("HTTP selected request changed sibling %q index bytes=%q", path, got)
			}
		}
	default:
		if after.index != "" {
			t.Errorf("HTTP initial Unstage all left actual index entries: %q", after.index)
		}
	}
}
