package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type statusMetadataChange struct {
	path, before, after, status string
	additions, deletions        int
}

func statusMetadataTrackedChanges() []statusMetadataChange {
	changes := []statusMetadataChange{{"ordinary.txt", "baseline\nold\n", "baseline\nnew\n", "modified", 1, 1}}
	for i, marker := range []string{"new file mode 100644", "deleted file mode 100644", "rename from unrelated.txt"} {
		changes = append(changes,
			statusMetadataChange{marker + ".txt", "baseline\nold\n", "baseline\nnew\n", "modified", 1, 1},
			statusMetadataChange{fmt.Sprintf("added-content-%d.txt", i), "baseline\n", "baseline\n" + marker + "\n", "modified", 1, 0},
			statusMetadataChange{fmt.Sprintf("removed-content-%d.txt", i), "baseline\n" + marker + "\n", "baseline\n", "modified", 0, 1},
			statusMetadataChange{fmt.Sprintf("context-content-%d.txt", i), marker + "\nold\n", marker + "\nnew\n", "modified", 1, 1},
		)
	}
	return append(changes, statusMetadataChange{"new file mode binary.bin", "\x00old\n", "\x00new\n", "modified", 0, 0})
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.1
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.2
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
func TestGitDiffStatusMetadataCallers(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	runGit(t, repo, "config", "diff.renames", "true")
	changes := statusMetadataTrackedChanges()
	for _, change := range changes {
		writeFile(t, repo, change.path, change.before)
	}
	writeFile(t, repo, "new file mode deleted.txt", "unique deletion sentinel\n")
	writeFile(t, repo, "new file mode old.txt", "unique rename sentinel\n")
	writeFile(t, repo, "edited-old.txt", strings.Repeat("edited rename unchanged\n", 20)+"old\n")
	writeFile(t, repo, "mode-only.txt", "mode sentinel\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "tracked baseline")
	base := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	for _, change := range changes {
		writeFile(t, repo, change.path, change.after)
	}
	writeFile(t, repo, "rename from added.txt", "deleted file mode 100644\n")
	runGit(t, repo, "rm", "new file mode deleted.txt")
	runGit(t, repo, "mv", "new file mode old.txt", "deleted file mode moved.txt")
	runGit(t, repo, "mv", "edited-old.txt", "new file mode edited.txt")
	writeFile(t, repo, "new file mode edited.txt", strings.Repeat("edited rename unchanged\n", 20)+"rename from data\n")
	runGit(t, repo, "add", ".")
	if runtime.GOOS != "windows" {
		runGit(t, repo, "config", "core.filemode", "true")
		if err := os.Chmod(filepath.Join(repo, "mode-only.txt"), 0o755); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, "add", "mode-only.txt")
		changes = append(changes, statusMetadataChange{path: "mode-only.txt", status: "modified"})
	}
	runGit(t, repo, "commit", "-m", "metadata changes")
	head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	changes = append(changes,
		statusMetadataChange{path: "rename from added.txt", status: "added", additions: 1},
		statusMetadataChange{path: "new file mode deleted.txt", status: "deleted", deletions: 1},
		statusMetadataChange{path: "deleted file mode moved.txt", status: "renamed"},
		statusMetadataChange{path: "new file mode edited.txt", status: "renamed", additions: 1, deletions: 1},
	)
	assertStatusMetadataCallers(t, repo, base, head, changes)
	// A dirty tracked file adds content independently of the historical commit.
	writeFile(t, repo, "ordinary.txt", "baseline\nnew\nrename from unrelated.txt\n")
	changes[0].additions = 2
	op := NewGitOperator(repo, newTestLogger(t), nil)
	before := statusMetadataRepositoryState(t, repo)
	result, err := op.GetCumulativeDiff(context.Background(), base)
	if err != nil || !result.Success {
		t.Fatalf("dirty cumulative result = %+v, err = %v", result, err)
	}
	patch := runGit(t, repo, "diff", "--src-prefix=a/", "--dst-prefix=b/", base)
	assertStatusMetadataChanges(t, result.Files, changes, patch)
	if after := statusMetadataRepositoryState(t, repo); after != before {
		t.Fatal("cumulative read changed index or worktree")
	}
}

func assertStatusMetadataCallers(t *testing.T, repo, base, head string, changes []statusMetadataChange) {
	t.Helper()
	op := NewGitOperator(repo, newTestLogger(t), nil)
	before := statusMetadataRepositoryState(t, repo)
	commit, err := op.ShowCommit(context.Background(), head)
	if err != nil || !commit.Success {
		t.Fatalf("commit result = %+v, err = %v", commit, err)
	}
	patch := runGit(t, repo, "show", "--first-parent", "--format=", "-p", "--src-prefix=a/", "--dst-prefix=b/", head)
	if len(changes) > 1 && (!strings.Contains(patch, "rename from new file mode old.txt\n") || !strings.Contains(patch, "rename from edited-old.txt\n")) {
		t.Fatal("fixture did not emit genuine pure and edited rename metadata")
	}
	assertStatusMetadataChanges(t, commit.Files, changes, patch)
	additions, deletions := 0, 0
	for _, change := range changes {
		additions += change.additions
		deletions += change.deletions
	}
	if commit.CommitSHA != head || commit.FilesChanged != len(changes) || commit.Insertions != additions || commit.Deletions != deletions {
		t.Errorf("commit identity/totals = %s %d +%d/-%d", commit.CommitSHA, commit.FilesChanged, commit.Insertions, commit.Deletions)
	}
	cumulative, err := op.GetCumulativeDiff(context.Background(), base)
	if err != nil || !cumulative.Success {
		t.Fatalf("cumulative result = %+v, err = %v", cumulative, err)
	}
	if cumulative.BaseCommit != base || cumulative.HeadCommit != head || cumulative.TotalCommits != 1 || cumulative.TruncatedFilesCount != 0 {
		t.Errorf("cumulative identity/totals = %+v", cumulative)
	}
	assertStatusMetadataChanges(t, cumulative.Files, changes, runGit(t, repo, "diff", "--src-prefix=a/", "--dst-prefix=b/", base))
	if after := statusMetadataRepositoryState(t, repo); after != before {
		t.Fatal("comparison reads changed index or worktree")
	}
}

func assertStatusMetadataChanges(t *testing.T, files map[string]interface{}, changes []statusMetadataChange, rawPatch string) {
	t.Helper()
	if len(files) != len(changes) {
		t.Fatalf("files = %d, want %d: %v", len(files), len(changes), fileKeys(files))
	}
	bytes := 0
	for _, change := range changes {
		entry, ok := files[change.path].(map[string]interface{})
		if !ok {
			t.Fatalf("missing path %q", change.path)
		}
		if entry["path"] != change.path || entry["status"] != change.status || entry["additions"] != change.additions || entry["deletions"] != change.deletions || entry["staged"] != false {
			t.Errorf("%q entry = %#v; want %s +%d/-%d", change.path, entry, change.status, change.additions, change.deletions)
		}
		patch, ok := entry["diff"].(string)
		if !ok || patch == "" || !strings.Contains(rawPatch, patch) {
			t.Errorf("%q patch does not match raw Git bytes", change.path)
		}
		if len(entry) != 6 {
			t.Errorf("unexpected entry fields for %q: %#v", change.path, entry)
		}
		bytes += len(patch)
	}
	if bytes != len(rawPatch) {
		t.Errorf("patch bytes = %d, want raw Git %d", bytes, len(rawPatch))
	}
}

func statusMetadataRepositoryState(t *testing.T, repo string) string {
	t.Helper()
	return runGit(t, repo, "ls-files", "--stage", "-z") + runGit(t, repo, "diff", "--binary", "HEAD") + runGit(t, repo, "status", "--porcelain=v1", "-z")
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.2
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
func TestShowCommit_StatusMetadataRootAndEmpty(t *testing.T) {
	isolateTestGitEnv(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "--initial-branch=main")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "user.email", "test@test.com")
	runGit(t, repo, "config", "core.hooksPath", os.DevNull)
	writeFile(t, repo, "deleted file mode.txt", "rename from data\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "root")
	op := NewGitOperator(repo, newTestLogger(t), nil)
	root := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	result, err := op.ShowCommit(context.Background(), root)
	if err != nil || !result.Success {
		t.Fatalf("root = %+v, err = %v", result, err)
	}
	assertStatusMetadataChanges(t, result.Files, []statusMetadataChange{{path: "deleted file mode.txt", status: "added", additions: 1}}, runGit(t, repo, "show", "--format=", "-p", root))
	for _, status := range []string{"added", "deleted"} {
		base := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
		if status == "added" {
			writeFile(t, repo, "new file mode empty.txt", "")
			runGit(t, repo, "add", ".")
		} else {
			runGit(t, repo, "rm", "new file mode empty.txt")
		}
		runGit(t, repo, "commit", "-m", "empty file "+status)
		head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
		assertStatusMetadataCallers(t, repo, base, head, []statusMetadataChange{{path: "new file mode empty.txt", status: status}})
	}
	runGit(t, repo, "commit", "--allow-empty", "-m", "empty commit")
	head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	result, err = op.ShowCommit(context.Background(), head)
	if err != nil || !result.Success || len(result.Files) != 0 || result.FilesChanged != 0 || result.Insertions != 0 || result.Deletions != 0 {
		t.Fatalf("empty commit = %+v, err = %v", result, err)
	}
}
