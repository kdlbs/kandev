package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const submoduleFormatPath = "vendor/lib"

type submoduleFormatFixture struct {
	repo, child, base, head, old, next string
	status                             string
	additions, deletions               int
}

func initSubmoduleFormatRepo(t *testing.T, repo string) {
	t.Helper()
	runGit(t, repo, "init", "--initial-branch=main")
	runGit(t, repo, "config", "user.name", "Submodule Format Test")
	runGit(t, repo, "config", "user.email", "format@test.invalid")
	runGit(t, repo, "config", "core.hooksPath", os.DevNull)
	runGit(t, repo, "config", "core.autocrlf", "false")
}

func newSubmoduleFormatFixture(t *testing.T, operation string) submoduleFormatFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	isolateTestGitEnv(t)
	repo := t.TempDir()
	child := filepath.Join(repo, filepath.FromSlash(submoduleFormatPath))
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	initSubmoduleFormatRepo(t, repo)
	initSubmoduleFormatRepo(t, child)
	writeFile(t, child, "README.md", "child before\n")
	runGit(t, child, "add", ".")
	runGit(t, child, "commit", "-m", "child baseline")
	old := strings.TrimSpace(runGit(t, child, "rev-parse", "HEAD"))
	writeFile(t, child, "README.md", "child after\n")
	runGit(t, child, "add", ".")
	runGit(t, child, "commit", "-m", "child advance")
	next := strings.TrimSpace(runGit(t, child, "rev-parse", "HEAD"))
	f := submoduleFormatFixture{repo: repo, child: child, old: old, next: next, status: "modified", additions: 1, deletions: 1}
	switch operation {
	case "backward":
		f.old, f.next = next, old
	case "add", "root":
		f.old, f.status, f.deletions = "", "added", 0
	case "delete":
		f.next, f.status, f.additions = "", "deleted", 0
	}
	if f.old != "" {
		runGit(t, child, "checkout", "--detach", f.old)
		runGit(t, repo, "update-index", "--add", "--cacheinfo", "160000,"+f.old+","+submoduleFormatPath)
	}
	if operation != "root" {
		runGit(t, repo, "commit", "--allow-empty", "-m", "parent baseline")
		f.base = strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	}
	if f.next == "" {
		runGit(t, repo, "update-index", "--force-remove", submoduleFormatPath)
	} else {
		runGit(t, child, "checkout", "--detach", f.next)
		runGit(t, repo, "update-index", "--add", "--cacheinfo", "160000,"+f.next+","+submoduleFormatPath)
	}
	runGit(t, repo, "commit", "-m", "parent gitlink change")
	f.head = strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	return f
}

func submoduleFormatState(t *testing.T, repo string) string {
	t.Helper()
	configPath := strings.TrimSpace(runGit(t, repo, "rev-parse", "--git-path", "config"))
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(repo, configPath)
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(config) + runGit(t, repo, "rev-parse", "HEAD") +
		runGit(t, repo, "show-ref", "--head") + runGit(t, repo, "ls-files", "--stage", "-z") +
		runGit(t, repo, "--no-optional-locks", "status", "--porcelain=v1", "-z") +
		runGit(t, repo, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--submodule=short", "HEAD")
}

func guardSubmoduleFormatRead(t *testing.T, f submoduleFormatFixture) {
	t.Helper()
	for _, repo := range []string{f.repo, f.child} {
		before := submoduleFormatState(t, repo)
		t.Cleanup(func() {
			if after := submoduleFormatState(t, repo); after != before {
				t.Errorf("comparison changed repository state in %s", repo)
			}
		})
	}
}

func assertSubmoduleFormatFile(t *testing.T, files map[string]interface{}, f submoduleFormatFixture, patch string) {
	t.Helper()
	entry, ok := files[submoduleFormatPath].(map[string]interface{})
	if !ok || len(files) != 1 {
		t.Fatalf("expected exactly parent gitlink %q, got %#v", submoduleFormatPath, files)
	}
	if entry["path"] != submoduleFormatPath || entry["status"] != f.status ||
		entry["additions"] != f.additions || entry["deletions"] != f.deletions || entry["diff"] != patch || entry["staged"] != false {
		t.Errorf("gitlink entry = %#v, want %s +%d/-%d and raw short patch", entry, f.status, f.additions, f.deletions)
	}
	for _, id := range []string{f.old, f.next} {
		if id != "" && !strings.Contains(patch, "Subproject commit "+id) {
			t.Fatalf("raw patch does not contain expected child commit %s: %q", id, patch)
		}
	}
	if _, present := entry["diff_skip_reason"]; present {
		t.Errorf("small gitlink patch skipped: %#v", entry)
	}
}

func assertSubmoduleFormatCommit(t *testing.T, f submoduleFormatFixture) {
	t.Helper()
	guardSubmoduleFormatRead(t, f)
	patch := runGit(t, f.repo, "show", "--first-parent", "--format=", "-p", "--no-color", "--no-ext-diff", "--no-textconv", "--submodule=short", "--src-prefix=a/", "--dst-prefix=b/", f.head)
	result, err := newPlainOutputOperator(t, f.repo).ShowCommit(context.Background(), f.head)
	if err != nil || !result.Success || result.CommitSHA != f.head || result.FilesChanged != 1 || result.Insertions != f.additions || result.Deletions != f.deletions || result.Message != "parent gitlink change" || result.Author != "Submodule Format Test <format@test.invalid>" || result.Date == "" {
		t.Errorf("commit identity/totals = %+v, err = %v", result, err)
	}
	if result == nil {
		t.Fatal("nil commit result")
	}
	assertSubmoduleFormatFile(t, result.Files, f, patch)
}

func assertSubmoduleFormatCumulative(t *testing.T, f submoduleFormatFixture) {
	t.Helper()
	guardSubmoduleFormatRead(t, f)
	patch := runGit(t, f.repo, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--submodule=short", "--src-prefix=a/", "--dst-prefix=b/", f.base)
	result, err := newPlainOutputOperator(t, f.repo).GetCumulativeDiff(context.Background(), f.base)
	if err != nil || !result.Success || result.BaseCommit != f.base || result.HeadCommit != f.head || result.TotalCommits != 1 || result.TruncatedFilesCount != 0 {
		t.Errorf("cumulative metadata = %+v, err = %v", result, err)
	}
	if result == nil {
		t.Fatal("nil cumulative result")
	}
	assertSubmoduleFormatFile(t, result.Files, f, patch)
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.12
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.13
func TestGitComparisonSubmoduleFormat(t *testing.T) {
	for _, operation := range []string{"forward", "backward", "add", "delete"} {
		t.Run(operation, func(t *testing.T) {
			f := newSubmoduleFormatFixture(t, operation)
			modes := []string{"short", "log"}
			if operation == "forward" {
				modes = []string{"", "short", "log", "diff"}
			}
			for _, mode := range modes {
				t.Run("display_"+mode, func(t *testing.T) {
					if mode != "" {
						runGit(t, f.repo, "config", "diff.submodule", mode)
					}
					t.Run("commit", func(t *testing.T) { assertSubmoduleFormatCommit(t, f) })
					t.Run("cumulative", func(t *testing.T) { assertSubmoduleFormatCumulative(t, f) })
				})
			}
		})
	}
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.12
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.13
func TestGitComparisonSubmoduleFormatControls(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		f := newSubmoduleFormatFixture(t, "root")
		runGit(t, f.repo, "config", "diff.submodule", "log")
		assertSubmoduleFormatCommit(t, f)
	})
	t.Run("dirty", func(t *testing.T) {
		f := newSubmoduleFormatFixture(t, "forward")
		runGit(t, f.repo, "config", "diff.submodule", "log")
		writeFile(t, f.child, "README.md", "child dirty advance\n")
		runGit(t, f.child, "add", ".")
		runGit(t, f.child, "commit", "-m", "dirty parent pointer")
		f.next = strings.TrimSpace(runGit(t, f.child, "rev-parse", "HEAD"))
		assertSubmoduleFormatCumulative(t, f)
	})
	t.Run("ordinary_and_empty", func(t *testing.T) {
		f := newSubmoduleFormatFixture(t, "forward")
		runGit(t, f.repo, "config", "diff.submodule", "log")
		writeFile(t, f.repo, "ordinary.txt", "ordinary control\n")
		runGit(t, f.repo, "add", "ordinary.txt")
		runGit(t, f.repo, "commit", "-m", "ordinary change")
		head := strings.TrimSpace(runGit(t, f.repo, "rev-parse", "HEAD"))
		op := newPlainOutputOperator(t, f.repo)
		t.Run("ordinary", func(t *testing.T) {
			guardSubmoduleFormatRead(t, f)
			result, err := op.ShowCommit(context.Background(), head)
			if err != nil || !result.Success || len(result.Files) != 1 || result.Files["ordinary.txt"] == nil || result.Insertions != 1 || result.Deletions != 0 {
				t.Fatalf("ordinary under log = %+v, err = %v", result, err)
			}
			cumulative, err := op.GetCumulativeDiff(context.Background(), f.head)
			if err != nil || !cumulative.Success || len(cumulative.Files) != 1 || cumulative.Files["ordinary.txt"] == nil {
				t.Fatalf("ordinary cumulative under log = %+v, err = %v", cumulative, err)
			}
		})
		runGit(t, f.repo, "commit", "--allow-empty", "-m", "empty control")
		emptyHead := strings.TrimSpace(runGit(t, f.repo, "rev-parse", "HEAD"))
		t.Run("empty", func(t *testing.T) {
			guardSubmoduleFormatRead(t, f)
			commit, commitErr := op.ShowCommit(context.Background(), emptyHead)
			cumulative, cumulativeErr := op.GetCumulativeDiff(context.Background(), emptyHead)
			if commitErr != nil || !commit.Success || len(commit.Files) != 0 || commit.FilesChanged != 0 || commit.Insertions != 0 || commit.Deletions != 0 {
				t.Errorf("empty commit = %+v, err = %v", commit, commitErr)
			}
			if cumulativeErr != nil || !cumulative.Success || len(cumulative.Files) != 0 || cumulative.TotalCommits != 0 {
				t.Errorf("empty cumulative = %+v, err = %v", cumulative, cumulativeErr)
			}
		})
	})
}
