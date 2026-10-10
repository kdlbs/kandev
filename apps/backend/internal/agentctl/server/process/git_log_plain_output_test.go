package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.6
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.7
func TestGitComparisonPlainOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	repo, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	base, head, changes := seedPlainOutputChanges(t, repo)
	for _, mode := range []struct{ name, ui, diff string }{
		{"defaults", "", ""},
		{"ui_always", "always", ""},
		{"diff_overrides_disabled_ui", "false", "always"},
		{"both_always", "always", "always"},
		{"disabled_diff_overrides_ui", "always", "false"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			if mode.ui != "" {
				runGit(t, repo, "config", "color.ui", mode.ui)
			}
			if mode.diff != "" {
				runGit(t, repo, "config", "color.diff", mode.diff)
			}
			t.Run("commit", func(t *testing.T) { assertPlainOutputCommit(t, repo, head, changes) })
			t.Run("cumulative", func(t *testing.T) { assertPlainOutputCumulative(t, repo, base, head, changes) })
		})
	}
	runGit(t, repo, "config", "color.diff", "always")
	writeFile(t, repo, "README.md", "new \x1b[31mliteral\x1b[0m\ndirty\n")
	changes[0].additions = 2
	assertPlainOutputCumulative(t, repo, base, head, changes)
}

func seedPlainOutputChanges(t *testing.T, repo string) (base, head string, changes []statusMetadataChange) {
	t.Helper()
	writeFile(t, repo, "README.md", "old \x1b[31mliteral\x1b[0m\n")
	writeFile(t, repo, "binary.bin", "\x00old")
	writeFile(t, repo, "deleted empty.txt", "")
	writeFile(t, repo, "rename old.txt", "unique rename sentinel\n")
	if runtime.GOOS != "windows" {
		// Windows filenames cannot contain control characters.
		writeFile(t, repo, "escape-\x1b[31m.txt", "old\n")
	}
	runGit(t, repo, "config", "diff.renames", "true")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "plain baseline")
	base = strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	writeFile(t, repo, "README.md", "new \x1b[31mliteral\x1b[0m\n")
	writeFile(t, repo, "binary.bin", "\x00new")
	runGit(t, repo, "rm", "deleted empty.txt")
	runGit(t, repo, "mv", "rename old.txt", "rename new.txt")
	changes = []statusMetadataChange{
		{path: "README.md", status: "modified", additions: 1, deletions: 1},
		{path: "binary.bin", status: "modified"},
		{path: "deleted empty.txt", status: "deleted"},
		{path: "rename new.txt", status: "renamed"},
	}
	if runtime.GOOS != "windows" {
		writeFile(t, repo, "escape-\x1b[31m.txt", "new\n")
		changes = append(changes, statusMetadataChange{path: "escape-\x1b[31m.txt", status: "modified", additions: 1, deletions: 1})
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "plain content and addition")
	head = strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	return base, head, changes
}

func newPlainOutputOperator(t *testing.T, repo string) *GitOperator {
	t.Helper()
	op := NewGitOperator(repo, newTestLogger(t), nil)
	env := filterTestGitEnv(os.Environ())
	op.setEnvironmentProvider(func() []string { return append([]string(nil), env...) })
	return op
}

func plainOutputRepositoryState(t *testing.T, repo string) string {
	t.Helper()
	config, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	return string(config) + runGit(t, repo, "rev-parse", "HEAD") +
		runGit(t, repo, "show-ref") + runGit(t, repo, "ls-files", "--stage", "-z") +
		runGit(t, repo, "--no-optional-locks", "status", "--porcelain=v1", "-z") +
		runGit(t, repo, "diff", "--no-color", "--binary", "HEAD")
}

func assertPlainOutputCommit(t *testing.T, repo, head string, changes []statusMetadataChange) {
	t.Helper()
	before := plainOutputRepositoryState(t, repo)
	t.Cleanup(func() {
		if plainOutputRepositoryState(t, repo) != before {
			t.Error("commit read changed repository state")
		}
	})
	result, err := newPlainOutputOperator(t, repo).ShowCommit(context.Background(), head)
	if err != nil || !result.Success {
		t.Fatalf("ShowCommit = %+v, err = %v", result, err)
	}
	metadata := strings.Split(strings.TrimSpace(runGit(t, repo, "show", "--no-patch", "--format=%H%n%s%n%an <%ae>%n%aI", head)), "\n")
	if result.CommitSHA != metadata[0] || result.Message != metadata[1] || result.Author != metadata[2] || result.Date != metadata[3] {
		t.Errorf("commit metadata = %+v, want %v", result, metadata)
	}
	patch := runGit(t, repo, "show", "--no-color", "--first-parent", "--format=", "-p", "--src-prefix=a/", "--dst-prefix=b/", head)
	assertStatusMetadataChanges(t, result.Files, changes, patch)
	additions, deletions := 0, 0
	for _, change := range changes {
		additions += change.additions
		deletions += change.deletions
	}
	if result.FilesChanged != len(changes) || result.Insertions != additions || result.Deletions != deletions {
		t.Errorf("commit totals = %d +%d/-%d", result.FilesChanged, result.Insertions, result.Deletions)
	}
}

func assertPlainOutputCumulative(t *testing.T, repo, base, head string, changes []statusMetadataChange) {
	t.Helper()
	before := plainOutputRepositoryState(t, repo)
	t.Cleanup(func() {
		if plainOutputRepositoryState(t, repo) != before {
			t.Error("cumulative read changed repository state")
		}
	})
	result, err := newPlainOutputOperator(t, repo).GetCumulativeDiff(context.Background(), base)
	if err != nil || !result.Success {
		t.Fatalf("GetCumulativeDiff = %+v, err = %v", result, err)
	}
	if result.BaseCommit != base || result.HeadCommit != head || result.TruncatedFilesCount != 0 || result.TotalCommits != 1 {
		t.Errorf("cumulative metadata = %+v", result)
	}
	assertStatusMetadataChanges(t, result.Files, changes, runGit(t, repo, "diff", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", base))
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.6
func TestGitComparisonPlainOutputRootAndEmpty(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	runGit(t, repo, "config", "color.ui", "always")
	runGit(t, repo, "config", "color.diff", "always")
	root := strings.TrimSpace(runGit(t, repo, "rev-list", "--max-parents=0", "HEAD"))
	t.Run("root", func(t *testing.T) {
		assertPlainOutputCommit(t, repo, root, []statusMetadataChange{{path: "README.md", status: "added", additions: 1}})
	})
	writeFile(t, repo, "added empty.txt", "")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "empty addition")
	addition := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	t.Run("empty_file", func(t *testing.T) {
		assertPlainOutputCommit(t, repo, addition, []statusMetadataChange{{path: "added empty.txt", status: "added"}})
	})
	runGit(t, repo, "commit", "--allow-empty", "-m", "empty")
	head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	t.Run("empty_commit", func(t *testing.T) { assertPlainOutputCommit(t, repo, head, nil) })
	before := plainOutputRepositoryState(t, repo)
	result, err := newPlainOutputOperator(t, repo).GetCumulativeDiff(context.Background(), head)
	if err != nil || !result.Success || len(result.Files) != 0 || result.TotalCommits != 0 || result.BaseCommit != head || result.HeadCommit != head {
		t.Errorf("empty cumulative = %+v, err = %v", result, err)
	}
	if plainOutputRepositoryState(t, repo) != before {
		t.Error("empty cumulative read changed repository state")
	}
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.6
func TestGitComparisonPlainOutputMerge(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	runGit(t, repo, "checkout", "-b", "feature/plain")
	writeFile(t, repo, "feature.txt", "feature\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "feature")
	runGit(t, repo, "checkout", "main")
	writeFile(t, repo, "incoming.txt", "incoming\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "incoming")
	runGit(t, repo, "checkout", "feature/plain")
	runGit(t, repo, "merge", "--no-ff", "-m", "merge", "main")
	runGit(t, repo, "config", "color.ui", "always")
	runGit(t, repo, "config", "color.diff", "always")
	head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	assertPlainOutputCommit(t, repo, head, []statusMetadataChange{{path: "incoming.txt", status: "added", additions: 1}})
}
