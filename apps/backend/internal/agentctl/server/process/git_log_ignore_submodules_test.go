package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const ignoreGitlinkPath = "vendor/dependency"

type ignoreGitlinkFixture struct {
	parent, child, base, head, old, next string
	env                                  []string
}

func ignoreGitlinkEnvironment(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME"} {
		t.Setenv(key, home)
	}
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
}

func ignoreGitlinkGit(t *testing.T, env []string, repo string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append([]string(nil), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func initIgnoreGitlinkRepo(t *testing.T, env []string, repo string) {
	t.Helper()
	ignoreGitlinkGit(t, env, repo, "init", "--initial-branch=main")
	for key, value := range map[string]string{
		"user.name": "Gitlink Review Test", "user.email": "review@test.invalid",
		"commit.gpgsign": "false", "core.hooksPath": os.DevNull, "core.autocrlf": "false",
	} {
		ignoreGitlinkGit(t, env, repo, "config", key, value)
	}
}

func newIgnoreGitlinkFixture(t *testing.T, backward bool) ignoreGitlinkFixture {
	t.Helper()
	f := ignoreGitlinkFixture{parent: t.TempDir(), env: ignoreGitlinkEnvironment(t)}
	source := t.TempDir()
	initIgnoreGitlinkRepo(t, f.env, source)
	writeFile(t, source, "code.txt", "before\n")
	ignoreGitlinkGit(t, f.env, source, "add", ".")
	ignoreGitlinkGit(t, f.env, source, "commit", "-m", "child before")
	f.old = strings.TrimSpace(ignoreGitlinkGit(t, f.env, source, "rev-parse", "HEAD"))
	writeFile(t, source, "code.txt", "after\n")
	ignoreGitlinkGit(t, f.env, source, "commit", "-am", "child after")
	f.next = strings.TrimSpace(ignoreGitlinkGit(t, f.env, source, "rev-parse", "HEAD"))
	if backward {
		f.old, f.next = f.next, f.old
	}
	initIgnoreGitlinkRepo(t, f.env, f.parent)
	ignoreGitlinkGit(t, f.env, f.parent, "-c", "protocol.file.allow=always", "submodule", "add", filepath.ToSlash(source), ignoreGitlinkPath)
	f.child = filepath.Join(f.parent, filepath.FromSlash(ignoreGitlinkPath))
	ignoreGitlinkGit(t, f.env, f.child, "checkout", "--detach", f.old)
	ignoreGitlinkGit(t, f.env, f.parent, "add", ".")
	ignoreGitlinkGit(t, f.env, f.parent, "commit", "-m", "parent baseline")
	f.base = strings.TrimSpace(ignoreGitlinkGit(t, f.env, f.parent, "rev-parse", "HEAD"))
	ignoreGitlinkGit(t, f.env, f.child, "checkout", "--detach", f.next)
	ignoreGitlinkGit(t, f.env, f.parent, "add", ignoreGitlinkPath)
	ignoreGitlinkGit(t, f.env, f.parent, "commit", "-m", "parent pointer change")
	f.head = strings.TrimSpace(ignoreGitlinkGit(t, f.env, f.parent, "rev-parse", "HEAD"))
	return f
}

func ignoreGitlinkPatch(t *testing.T, f ignoreGitlinkFixture, commit bool) string {
	t.Helper()
	args := []string{"diff"}
	ref := f.base
	if commit {
		args, ref = []string{"show", "--first-parent", "--format=", "-p"}, f.head
	}
	args = append(args, "--no-color", "--no-ext-diff", "--no-textconv", "--submodule=short", "--ignore-submodules=none", "--src-prefix=a/", "--dst-prefix=b/", ref)
	return ignoreGitlinkGit(t, f.env, f.parent, args...)
}

func ignoreGitlinkState(t *testing.T, f ignoreGitlinkFixture) map[string]string {
	t.Helper()
	state := map[string]string{}
	for _, repo := range []string{f.parent, f.child} {
		if _, err := os.Stat(filepath.Join(repo, "code.txt")); repo == f.child && os.IsNotExist(err) {
			entries, readErr := os.ReadDir(repo)
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			state[repo] = "unavailable"
			for _, entry := range entries {
				state[repo] += "\x00" + entry.Name()
			}
			continue
		}
		for _, name := range []string{"config", "index"} {
			p := strings.TrimSpace(ignoreGitlinkGit(t, f.env, repo, "rev-parse", "--git-path", name))
			if !filepath.IsAbs(p) {
				p = filepath.Join(repo, p)
			}
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			state[repo+name] = string(data)
		}
		state[repo+"refs"] = ignoreGitlinkGit(t, f.env, repo, "show-ref", "--head")
		state[repo+"head"] = ignoreGitlinkGit(t, f.env, repo, "rev-parse", "HEAD")
		state[repo+"entries"] = ignoreGitlinkGit(t, f.env, repo, "ls-files", "--stage", "-z")
		state[repo+"status"] = ignoreGitlinkGit(t, f.env, repo, "--no-optional-locks", "status", "--porcelain=v1", "--ignore-submodules=none", "-z")
		state[repo+"patch"] = ignoreGitlinkGit(t, f.env, repo, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--submodule=short", "--ignore-submodules=none", "HEAD")
		for _, name := range []string{".gitmodules", "code.txt", "ordinary.txt", "untracked.txt"} {
			data, err := os.ReadFile(filepath.Join(repo, name))
			if err == nil {
				state[repo+name] = string(data)
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
	}
	for _, name := range []string{"config", "HEAD", "index"} {
		key := "modules/" + ignoreGitlinkPath + "/" + name
		p := strings.TrimSpace(ignoreGitlinkGit(t, f.env, f.parent, "rev-parse", "--git-path", key))
		if !filepath.IsAbs(p) {
			p = filepath.Join(f.parent, p)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		state[key] = string(data)
	}
	return state
}

func assertIgnoreGitlinkRead(t *testing.T, f ignoreGitlinkFixture, commit bool, expectedPath string, commits int) {
	t.Helper()
	before := ignoreGitlinkState(t, f)
	defer func() {
		if after := ignoreGitlinkState(t, f); !reflect.DeepEqual(before, after) {
			t.Error("comparison mutated owned repository state")
		}
	}()
	op := NewGitOperator(f.parent, newTestLogger(t), nil)
	op.setEnvironmentProvider(func() []string { return append([]string(nil), f.env...) })
	var files map[string]interface{}
	if commit {
		result, err := op.ShowCommit(context.Background(), f.head)
		if err != nil || result == nil || !result.Success {
			t.Fatalf("ShowCommit = %+v, %v", result, err)
		}
		assertIgnoreGitlinkCommitMetadata(t, f, result, expectedPath)
		files = result.Files
	} else {
		result, err := op.GetCumulativeDiff(context.Background(), f.base)
		if err != nil || result == nil || !result.Success {
			t.Fatalf("GetCumulativeDiff = %+v, %v", result, err)
		}
		if result.BaseCommit != f.base || result.HeadCommit != f.head || result.TotalCommits != commits || result.TruncatedFilesCount != 0 {
			t.Errorf("cumulative metadata = %+v", result)
		}
		files = result.Files
	}
	assertIgnoreGitlinkFile(t, files, f, expectedPath, ignoreGitlinkPatch(t, f, commit))
}

func assertIgnoreGitlinkCommitMetadata(t *testing.T, f ignoreGitlinkFixture, result *CommitDiffResult, path string) {
	t.Helper()
	meta := strings.Split(strings.TrimSpace(ignoreGitlinkGit(t, f.env, f.parent, "show", "--no-patch", "--format=%H%n%s%n%an <%ae>%n%aI", f.head)), "\n")
	if result.CommitSHA != meta[0] || result.Message != meta[1] || result.Author != meta[2] || result.Date != meta[3] {
		t.Errorf("commit identity = %+v, expected %v", result, meta)
	}
	want := 1
	if path == "" {
		want = 0
	}
	if result.FilesChanged != want || result.Insertions != want || result.Deletions != want {
		t.Errorf("commit totals = %d +%d/-%d, want %d +%d/-%d", result.FilesChanged, result.Insertions, result.Deletions, want, want, want)
	}
}

func assertIgnoreGitlinkFile(t *testing.T, files map[string]interface{}, f ignoreGitlinkFixture, path, patch string) {
	t.Helper()
	if path == "" {
		if len(files) != 0 || patch != "" {
			t.Fatalf("genuinely unchanged comparison = %#v, raw patch %q", files, patch)
		}
		return
	}
	if path == ignoreGitlinkPath {
		for _, id := range []string{f.old, f.next} {
			if !strings.Contains(patch, "Subproject commit "+id) {
				t.Fatalf("independent patch missing child ID %s: %q", id, patch)
			}
		}
	}
	entry, ok := files[path].(map[string]interface{})
	if !ok || len(files) != 1 {
		t.Fatalf("expected exactly %q, got %#v", path, files)
	}
	if entry["path"] != path || entry["status"] != "modified" || entry["diff"] != patch || entry["additions"] != 1 || entry["deletions"] != 1 || entry["staged"] != false {
		t.Errorf("comparison file = %#v, expected raw patch %q and +1/-1", entry, patch)
	}
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.14
func TestGitComparisonIgnoreSubmodules(t *testing.T) {
	for _, mode := range []string{"default", "all", "none", "untracked", "dirty", "backward_all"} {
		t.Run(mode, func(t *testing.T) {
			f := newIgnoreGitlinkFixture(t, mode == "backward_all")
			setting := mode
			if mode == "backward_all" {
				setting = "all"
			}
			if setting != "default" {
				ignoreGitlinkGit(t, f.env, f.parent, "config", "diff.ignoreSubmodules", setting)
			}
			if mode == "default" || mode == "all" || mode == "backward_all" {
				ignoreGitlinkGit(t, f.env, f.parent, "submodule", "deinit", "--force", "--all")
			}
			t.Run("commit", func(t *testing.T) { assertIgnoreGitlinkRead(t, f, true, ignoreGitlinkPath, 1) })
			t.Run("cumulative", func(t *testing.T) { assertIgnoreGitlinkRead(t, f, false, ignoreGitlinkPath, 1) })
		})
	}
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
func TestGitComparisonIgnoreSubmodulesControls(t *testing.T) {
	t.Run("ordinary_all", func(t *testing.T) {
		f := newIgnoreGitlinkFixture(t, false)
		writeFile(t, f.parent, "ordinary.txt", "before\n")
		ignoreGitlinkGit(t, f.env, f.parent, "add", "ordinary.txt")
		ignoreGitlinkGit(t, f.env, f.parent, "commit", "-m", "ordinary baseline")
		f.base = strings.TrimSpace(ignoreGitlinkGit(t, f.env, f.parent, "rev-parse", "HEAD"))
		writeFile(t, f.parent, "ordinary.txt", "after\n")
		ignoreGitlinkGit(t, f.env, f.parent, "commit", "-am", "ordinary change")
		f.head = strings.TrimSpace(ignoreGitlinkGit(t, f.env, f.parent, "rev-parse", "HEAD"))
		ignoreGitlinkGit(t, f.env, f.parent, "config", "diff.ignoreSubmodules", "all")
		assertIgnoreGitlinkRead(t, f, true, "ordinary.txt", 1)
		assertIgnoreGitlinkRead(t, f, false, "ordinary.txt", 1)
	})
	t.Run("unchanged_all", func(t *testing.T) {
		f := newIgnoreGitlinkFixture(t, false)
		ignoreGitlinkGit(t, f.env, f.parent, "commit", "--allow-empty", "-m", "empty")
		f.head = strings.TrimSpace(ignoreGitlinkGit(t, f.env, f.parent, "rev-parse", "HEAD"))
		f.base = f.head
		ignoreGitlinkGit(t, f.env, f.parent, "config", "diff.ignoreSubmodules", "all")
		assertIgnoreGitlinkRead(t, f, true, "", 0)
		assertIgnoreGitlinkRead(t, f, false, "", 0)
	})
	t.Run("uncommitted_pointer_all", func(t *testing.T) {
		f := newIgnoreGitlinkFixture(t, false)
		ignoreGitlinkGit(t, f.env, f.parent, "reset", "--mixed", f.base)
		f.head = f.base
		ignoreGitlinkGit(t, f.env, f.parent, "config", "diff.ignoreSubmodules", "all")
		assertIgnoreGitlinkRead(t, f, false, ignoreGitlinkPath, 0)
	})
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.14
func TestGitComparisonIgnoreSubmodulesChildDirt(t *testing.T) {
	for _, scenario := range []struct {
		name, path string
		pointer    bool
	}{
		{name: "tracked", path: "code.txt"},
		{name: "untracked", path: "untracked.txt"},
		{name: "pointer_and_tracked", path: "code.txt", pointer: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newIgnoreGitlinkFixture(t, false)
			if !scenario.pointer {
				f.base = f.head
			}
			ignoreGitlinkGit(t, f.env, f.parent, "config", "diff.ignoreSubmodules", "all")
			writeFile(t, f.child, scenario.path, "child-only dirt\n")
			if strings.TrimSpace(ignoreGitlinkGit(t, f.env, f.child, "rev-parse", "HEAD")) != f.next {
				t.Fatal("child dirt must not change the child HEAD")
			}
			if !strings.Contains(ignoreGitlinkPatch(t, f, false), "-dirty") {
				t.Fatal("ignore-none positive control did not expose child dirt")
			}
			patch := ignoreGitlinkGit(t, f.env, f.parent, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--submodule=short", "--ignore-submodules=dirty", "--src-prefix=a/", "--dst-prefix=b/", f.base)
			before := ignoreGitlinkState(t, f)
			op := NewGitOperator(f.parent, newTestLogger(t), nil)
			op.setEnvironmentProvider(func() []string { return append([]string(nil), f.env...) })
			result, err := op.GetCumulativeDiff(context.Background(), f.base)
			if err != nil || result == nil || !result.Success {
				t.Fatalf("cumulative child dirt = %+v, %v", result, err)
			}
			if after := ignoreGitlinkState(t, f); !reflect.DeepEqual(before, after) {
				t.Error("child-dirt comparison mutated owned state")
			}
			assertIgnoreGitlinkChildDirtResult(t, f, result, patch, scenario.pointer)
		})
	}
}

func assertIgnoreGitlinkChildDirtResult(t *testing.T, f ignoreGitlinkFixture, result *CumulativeDiffResult, patch string, pointer bool) {
	t.Helper()
	path, commits := "", 0
	if pointer {
		path, commits = ignoreGitlinkPath, 1
	}
	if result.BaseCommit != f.base || result.HeadCommit != f.head || result.TotalCommits != commits || result.TruncatedFilesCount != 0 {
		t.Errorf("child-dirt metadata = %+v", result)
	}
	if strings.Contains(patch, "-dirty") {
		t.Fatalf("pointer-only oracle contains child dirt: %q", patch)
	}
	assertIgnoreGitlinkFile(t, result.Files, f, path, patch)
}
