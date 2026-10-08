package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

const textconvHTTPPath = "content with spaces.txt"
const textconvHTTPMarker = "--kandev-http-textconv-helper"

func TestHTTPTextconvHelperProcess(t *testing.T) {
	if os.Getenv("KANDEV_TEST_HTTP_TEXTCONV") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg != textconvHTTPMarker || i+3 >= len(os.Args) {
			continue
		}
		if err := os.WriteFile(os.Args[i+1], []byte("executed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		switch os.Args[i+2] {
		case "constant":
			fmt.Print("same HTTP converted bytes\n")
		case "changed":
			data, err := os.ReadFile(os.Args[i+3])
			if err != nil {
				t.Fatal(err)
			}
			fmt.Print("HTTP CONVERTED " + strings.ToUpper(string(data)))
		default:
			t.Fatal("unknown HTTP converter mode")
		}
		os.Exit(0)
	}
	t.Fatal("HTTP converter invocation missing private arguments")
}

type textconvHTTPHelper struct{ command, sentinel, mode string }

func newTextconvHTTPHelper(t *testing.T, mode string) textconvHTTPHelper {
	t.Helper()
	t.Setenv("KANDEV_TEST_HTTP_TEXTCONV", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(t.TempDir(), "HTTP converter execution.txt")
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(filepath.ToSlash(value), "'", "'\\''") + "'"
	}
	return textconvHTTPHelper{
		command: quote(executable) + " -test.run=^TestHTTPTextconvHelperProcess$ -- " +
			textconvHTTPMarker + " " + quote(sentinel) + " " + mode,
		sentinel: sentinel,
		mode:     mode,
	}
}

func (h textconvHTTPHelper) configureAndProve(t *testing.T, repo statusMetadataHTTPRepo) {
	t.Helper()
	writeFileAPI(t, repo.dir, ".git/info/attributes", "\""+textconvHTTPPath+"\" diff=probe\n")
	runGitAPI(t, repo.dir, "config", "diff.probe.textconv", h.command)
	runGitAPI(t, repo.dir, "config", "diff.probe.cachetextconv", "false")
	patch := runGitAPI(t, repo.dir, "diff", "--textconv", "--no-ext-diff", "--no-color", repo.base)
	if h.mode == "constant" && patch != "" {
		t.Fatalf("HTTP constant converter did not hide change: %q", patch)
	}
	if h.mode == "changed" && (!strings.Contains(patch, "HTTP CONVERTED ") || !strings.Contains(patch, "diff --git ")) {
		t.Fatalf("HTTP changing converter did not alter patch: %q", patch)
	}
	data, err := os.ReadFile(h.sentinel)
	if err != nil || string(data) != "executed\n" {
		t.Fatalf("HTTP converter execution control = %q, err=%v", data, err)
	}
	if err := os.Remove(h.sentinel); err != nil {
		t.Fatal(err)
	}
}

func (h textconvHTTPHelper) assertAbsent(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(h.sentinel); !os.IsNotExist(err) {
		t.Errorf("HTTP comparison executed converter: %v", err)
	}
}

func textconvHTTPState(t *testing.T, repo string) map[string]string {
	t.Helper()
	state := make(map[string]string)
	for _, path := range []string{".git/config", ".git/index", ".git/info/attributes", textconvHTTPPath} {
		data, err := os.ReadFile(filepath.Join(repo, path))
		if os.IsNotExist(err) {
			state[path] = "absent"
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		state[path] = string(data)
	}
	state["HEAD"] = runGitAPI(t, repo, "rev-parse", "HEAD")
	state["refs"] = runGitAPI(t, repo, "show-ref")
	state["entries"] = runGitAPI(t, repo, "ls-files", "--stage", "-z")
	state["status"] = runGitAPI(t, repo, "--no-optional-locks", "status", "--porcelain=v1", "-z")
	state["dirty"] = runGitAPI(t, repo, "diff", "--no-textconv", "--no-ext-diff", "--no-color", "--binary", "HEAD")
	return state
}

func guardTextconvHTTPRead(t *testing.T, repos []statusMetadataHTTPRepo, helpers []textconvHTTPHelper) {
	t.Helper()
	for _, repo := range repos {
		before := textconvHTTPState(t, repo.dir)
		t.Cleanup(func() {
			if !reflect.DeepEqual(before, textconvHTTPState(t, repo.dir)) {
				t.Errorf("HTTP comparison changed repository %s", repo.name)
			}
		})
	}
	t.Cleanup(func() {
		for _, helper := range helpers {
			helper.assertAbsent(t)
		}
	})
}

func textconvHTTPPatch(t *testing.T, repo statusMetadataHTTPRepo, commit bool) string {
	t.Helper()
	args := []string{"diff", "--no-textconv", "--no-ext-diff", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", repo.base}
	if commit {
		args = []string{"show", "--first-parent", "--no-textconv", "--no-ext-diff", "--no-color", "--format=", "-p", "--src-prefix=a/", "--dst-prefix=b/", repo.head}
	}
	patch := runGitAPI(t, repo.dir, args...)
	if !strings.Contains(patch, "-"+repo.name+" baseline\n+"+repo.name+" changed\n") {
		t.Fatalf("HTTP actual-byte oracle lacks repository's expected bytes: %q", patch)
	}
	return patch
}

func assertTextconvHTTPCommit(t *testing.T, server *Server, repo statusMetadataHTTPRepo) {
	t.Helper()
	var result process.CommitDiffResult
	readStatusMetadataHTTP(t, server, "/api/v1/git/commit/"+repo.head+"?repo="+repo.name, &result)
	if !result.Success || result.CommitSHA != repo.head || len(result.Files) != 1 || result.FilesChanged != 1 || result.Insertions != 2 || result.Deletions != 1 {
		t.Fatalf("selected HTTP commit = %+v", result)
	}
	metadata := strings.Split(strings.TrimSpace(runGitAPI(t, repo.dir, "show", "--no-patch", "--format=%H%n%s%n%an <%ae>%n%aI", repo.head)), "\n")
	if len(metadata) != 4 || result.Message != metadata[1] || result.Author != metadata[2] || result.Date != metadata[3] {
		t.Errorf("HTTP commit metadata = %+v, want %v", result, metadata)
	}
	assertStatusMetadataHTTPFile(t, result.Files, textconvHTTPPath, textconvHTTPPath, "modified", 2, 1, textconvHTTPPatch(t, repo, true))
}

func assertTextconvHTTPSelected(t *testing.T, server *Server, repo statusMetadataHTTPRepo) {
	t.Helper()
	var result process.CumulativeDiffResult
	readStatusMetadataHTTP(t, server, "/api/v1/git/cumulative-diff?base="+repo.base+"&repo="+repo.name, &result)
	if !result.Success || len(result.Files) != 1 || result.BaseCommit != repo.base || result.HeadCommit != repo.head || result.TotalCommits != 1 || result.TruncatedFilesCount != 0 {
		t.Fatalf("selected HTTP cumulative = %+v", result)
	}
	assertStatusMetadataHTTPFile(t, result.Files, textconvHTTPPath, textconvHTTPPath, "modified", repo.additions, 1, textconvHTTPPatch(t, repo, false))
}

func assertTextconvHTTPReads(t *testing.T, server *Server, repos []statusMetadataHTTPRepo, helpers []textconvHTTPHelper) {
	t.Helper()
	for _, repo := range repos {
		t.Run(repo.name+"_commit", func(t *testing.T) {
			guardTextconvHTTPRead(t, repos, helpers)
			assertTextconvHTTPCommit(t, server, repo)
		})
		t.Run(repo.name+"_cumulative", func(t *testing.T) {
			guardTextconvHTTPRead(t, repos, helpers)
			assertTextconvHTTPSelected(t, server, repo)
		})
	}
	t.Run("aggregate", func(t *testing.T) {
		guardTextconvHTTPRead(t, repos, helpers)
		var result process.CumulativeDiffResult
		readStatusMetadataHTTP(t, server, "/api/v1/git/cumulative-diff?base="+repos[0].base, &result)
		if !result.Success || len(result.Files) != 2 || result.TotalCommits != 2 || result.TruncatedFilesCount != 0 {
			t.Fatalf("aggregate HTTP cumulative = %+v", result)
		}
		for _, repo := range repos {
			entry := assertStatusMetadataHTTPFile(t, result.Files, repo.name+"\x00"+textconvHTTPPath, textconvHTTPPath, "modified", repo.additions, 1, textconvHTTPPatch(t, repo, false))
			if entry["repository_name"] != repo.name || entry["base_ref"] != repo.base {
				t.Errorf("aggregate repository identity = %#v", entry)
			}
			if _, present := entry["is_submodule"]; present {
				t.Errorf("ordinary repository marked as submodule: %#v", entry)
			}
		}
	})
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.10
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.11
func TestGitComparisonTextconvHTTP(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	root := t.TempDir()
	alpha := seedStatusMetadataHTTPRepo(t, root, "alpha", textconvHTTPPath, false)
	beta := seedStatusMetadataHTTPRepo(t, root, "beta", textconvHTTPPath, false)
	if alpha.base == beta.base || alpha.head == beta.head {
		t.Fatal("HTTP repositories must have independent commit identity")
	}
	helpers := []textconvHTTPHelper{newTextconvHTTPHelper(t, "constant"), newTextconvHTTPHelper(t, "changed")}
	env := externalHTTPEnvironment()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.InstanceConfig{WorkDir: root, AgentEnv: append([]string(nil), env...), BaseBranches: map[string]string{"alpha": "main", "beta": "main"}}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() {
		if err := manager.StopForTeardown(context.Background()); err != nil {
			t.Errorf("stop HTTP fixture manager: %v", err)
		}
	})
	server := NewServer(cfg, manager, nil, nil, log)
	repos := []statusMetadataHTTPRepo{alpha, beta}
	t.Run("no_driver", func(t *testing.T) { assertTextconvHTTPReads(t, server, repos, helpers) })
	for i, repo := range repos {
		helpers[i].configureAndProve(t, repo)
	}
	writeFileAPI(t, alpha.dir, textconvHTTPPath, "alpha changed\nrename from marker\ndirty alpha bytes\n")
	repos[0].additions = 3
	t.Run("configured_dirty", func(t *testing.T) { assertTextconvHTTPReads(t, server, repos, helpers) })
	if !reflect.DeepEqual(cfg.AgentEnv, env) {
		t.Error("HTTP comparison changed captured helper environment")
	}
}
