package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

type discardAPIState struct {
	index, worktree string
	indexed, exists bool
}

type discardAPICase struct {
	name, path, decoy, kind string
	multiple                bool
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.44
func TestHandleGitDiscardLiteralSelections(t *testing.T) {
	cases := []discardAPICase{
		{name: "untracked-bracket", path: "new[ab].txt", decoy: "newa.txt", kind: "untracked"},
		{name: "added-bracket", path: "new[ab].txt", decoy: "newa.txt", kind: "added"},
		{name: "tracked-control", path: "new[ab].txt", decoy: "newa.txt", kind: "tracked"},
		{name: "native-magic", path: ":(glob)a*.txt", decoy: "alpha.txt", kind: "tracked"},
		{name: "multiple", path: "new[ab].txt", decoy: "newa.txt", kind: "untracked", multiple: true},
	}
	for _, literal := range []string{"0", "1"} {
		for _, icase := range []string{"0", "1"} {
			for _, tc := range cases {
				t.Run(fmt.Sprintf("%s/literal-%s/icase-%s", tc.name, literal, icase), func(t *testing.T) {
					if runtime.GOOS == "windows" && strings.ContainsAny(tc.path, ":*?") {
						t.Skip("filename cannot be represented by the native Windows filesystem")
					}
					checkDiscardAPIRouting(t, tc, literal, icase)
				})
			}
		}
	}
	t.Run("invalid-requests", testDiscardAPIInvalidRequests)
}

func newDiscardAPIServer(t *testing.T, root, literal, icase string) *Server {
	t.Helper()
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	env = append(env, "GIT_LITERAL_PATHSPECS="+literal, "GIT_ICASE_PATHSPECS="+icase)
	cfg := &config.InstanceConfig{WorkDir: root, AgentEnv: env,
		BaseBranches: map[string]string{"selected": "main", "other": "main"}}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() {
		if err := manager.StopForTeardown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return NewServer(cfg, manager, nil, nil, log)
}

func checkDiscardAPIRouting(t *testing.T, tc discardAPICase, literal, icase string) {
	t.Helper()
	root := t.TempDir()
	want := make(map[string]map[string]discardAPIState)
	var selected []string
	for _, repo := range []string{"selected", "other"} {
		var after map[string]discardAPIState
		selected, after = seedDiscardAPIRepo(t, root, repo, tc)
		if repo == "other" {
			for path := range after {
				after[path] = readDiscardAPIState(t, filepath.Join(root, repo), path)
			}
		}
		want[repo] = after
	}
	t.Setenv("GIT_LITERAL_PATHSPECS", "0")
	t.Setenv("GIT_ICASE_PATHSPECS", "0")
	server := newDiscardAPIServer(t, root, literal, icase)
	beforeEnv := os.Environ()
	beforeInstance := append([]string(nil), server.cfg.AgentEnv...)
	rec := postGitAPI(t, server, "/api/v1/git/discard", GitDiscardRequest{Repo: "selected", Paths: selected})
	if rec.Code != http.StatusOK || !decodeGitOperationResult(t, rec).Success {
		t.Errorf("Discard HTTP %d: %s", rec.Code, rec.Body.String())
	}
	assertDiscardAPIState(t, root, want)
	if !reflect.DeepEqual(os.Environ(), beforeEnv) || !slices.Equal(server.cfg.AgentEnv, beforeInstance) {
		t.Error("Discard changed ambient or instance environment")
	}
}

func seedDiscardAPIRepo(t *testing.T, root, repo string, tc discardAPICase) ([]string, map[string]discardAPIState) {
	t.Helper()
	dir := filepath.Join(root, repo)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitAPI(t, dir, "init", "--initial-branch=main")
	runGitAPI(t, dir, "config", "user.email", "test@test.com")
	runGitAPI(t, dir, "config", "user.name", "Test User")
	runGitAPI(t, dir, "config", "core.autocrlf", "false")
	head := []string{tc.decoy, "tracked.txt", "addeda.txt"}
	if tc.kind == "tracked" {
		head = append(head, tc.path)
	}
	want := make(map[string]discardAPIState)
	for _, path := range head {
		writeFileAPI(t, dir, path, repo+"/"+path+" committed\n")
	}
	runGitAPI(t, dir, "add", "-A")
	runGitAPI(t, dir, "commit", "-m", "independent discard API fixture")
	for _, path := range head {
		want[path] = readDiscardAPIState(t, dir, path)
	}
	selected := []string{tc.path}
	if tc.multiple {
		selected = append(selected, "tracked.txt", "added[ab].txt")
	}
	for _, path := range selected {
		writeFileAPI(t, dir, path, repo+"/"+path+" staged\n")
		if tc.name != "tracked-control" && (path != tc.path || tc.kind != "untracked") {
			runGitAPI(t, dir, "add", "--", ":(literal)"+path)
		}
		writeFileAPI(t, dir, path, repo+"/"+path+" working\n")
		if _, tracked := want[path]; !tracked {
			want[path] = discardAPIState{}
		}
	}
	for _, path := range []string{tc.decoy, "addeda.txt"} {
		writeFileAPI(t, dir, path, repo+"/"+path+" unrelated staged\n")
		if tc.name != "tracked-control" {
			runGitAPI(t, dir, "add", "--", ":(literal)"+path)
		}
		writeFileAPI(t, dir, path, repo+"/"+path+" unrelated working\n")
		want[path] = readDiscardAPIState(t, dir, path)
	}
	return selected, want
}

func readDiscardAPIState(t *testing.T, dir, path string) discardAPIState {
	t.Helper()
	indexed := slices.Contains(strings.Split(runGitAPI(t, dir, "ls-files", "-z"), "\x00"), path)
	got := discardAPIState{indexed: indexed}
	if indexed {
		got.index = runGitAPI(t, dir, "show", ":"+path)
	}
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	got.exists, got.worktree = err == nil, string(data)
	return got
}

func assertDiscardAPIState(t *testing.T, root string, want map[string]map[string]discardAPIState) {
	t.Helper()
	for repo, files := range want {
		for path, expected := range files {
			if got := readDiscardAPIState(t, filepath.Join(root, repo), path); got != expected {
				t.Errorf("repository %q file %q = %+v, want %+v", repo, path, got, expected)
			}
		}
	}
}

func testDiscardAPIInvalidRequests(t *testing.T) {
	root := t.TempDir()
	tc := discardAPICase{path: "new[ab].txt", decoy: "newa.txt", kind: "tracked"}
	before := make(map[string]map[string]discardAPIState)
	for _, repo := range []string{"selected", "other"} {
		_, files := seedDiscardAPIRepo(t, root, repo, tc)
		for path := range files {
			files[path] = readDiscardAPIState(t, filepath.Join(root, repo), path)
		}
		before[repo] = files
	}
	server := newDiscardAPIServer(t, root, "0", "0")
	for _, req := range []GitDiscardRequest{
		{Repo: "../other", Paths: []string{tc.path}}, {Repo: "missing", Paths: []string{tc.path}},
		{Repo: "selected"}, {Repo: "selected", Paths: []string{""}},
	} {
		rec := postGitAPI(t, server, "/api/v1/git/discard", req)
		result := decodeGitOperationResult(t, rec)
		if result.Success || result.Error == "" {
			t.Errorf("invalid request %+v HTTP %d: %+v", req, rec.Code, result)
		}
		assertDiscardAPIState(t, root, before)
	}
}
