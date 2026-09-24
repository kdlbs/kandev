package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestArchiveManifestRejectsForeignRepositoryPath(t *testing.T) {
	foreignRepo := initGitRepoForWorktreeTest(t)
	if err := os.WriteFile(filepath.Join(foreignRepo, "foreign-secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "recorded-worktree", TaskID: "recorded-task", RepositoryID: "recorded-repository",
		Path: foreignRepo, RepositoryPath: foreignRepo,
	}})
	if err == nil {
		t.Fatalf("accepted foreign repository path as task worktree: %+v", manifest)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.4
func TestArchiveManifestRejectsMissingGitMetadata(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	_, err = mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: t.TempDir(), RepositoryPath: repo,
	}})
	if err == nil {
		t.Fatal("captured a worktree path without Git metadata")
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.2
func TestArchiveManifestRejectsForeignLinkedWorktree(t *testing.T) {
	recordedRepo := initGitRepoForWorktreeTest(t)
	foreignRepo := initGitRepoForWorktreeTest(t)
	foreignWorktree := filepath.Join(t.TempDir(), "foreign-worktree")
	runGit(t, foreignRepo, "worktree", "add", foreignWorktree, "feature/pr-branch")
	if err := os.WriteFile(filepath.Join(foreignWorktree, "foreign-secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "recorded-worktree", TaskID: "recorded-task", RepositoryID: "recorded-repository",
		Path: foreignWorktree, RepositoryPath: recordedRepo,
	}})
	if err == nil {
		t.Fatalf("accepted foreign linked worktree as task source: %+v", manifest)
	}
}

func TestArchiveManifestRejectsDisappearedUntrackedPath(t *testing.T) {
	entries, err := archiveSourceManifestEntries(t.TempDir(), "?? disappeared.txt\x00")
	if err == nil {
		t.Fatalf("accepted disappeared untracked path without content identity: %+v", entries)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
func TestArchiveManifestHandlesDeletedRenameDestination(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "next.txt"), []byte("next"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := archiveSourceManifestEntries(root, "RD renamed.txt\x00original.txt\x00?? next.txt\x00")
	if err != nil {
		t.Fatalf("parse staged rename with deleted destination: %v", err)
	}
	if len(entries) != 2 || entries[0].Path != "renamed.txt" || entries[0].Status != "RD" || entries[1].Path != "next.txt" {
		t.Fatalf("rename evidence = %+v, want one deleted destination", entries)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
func TestArchiveManifestCapturesUnmergedIndex(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	if err := os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "conflict.txt")
	runGit(t, repo, "commit", "-m", "base")
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature/task", worktreePath)
	runGit(t, repo, "checkout", "-b", "feature/upstream")
	if err := os.WriteFile(filepath.Join(worktreePath, "conflict.txt"), []byte("task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreePath, "commit", "-am", "task")
	if err := os.WriteFile(filepath.Join(repo, "conflict.txt"), []byte("upstream\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "commit", "-am", "upstream")
	merge := exec.Command("git", "merge", "feature/conflict")
	merge.Dir = repo
	if err := merge.Run(); err == nil {
		t.Fatal("merge succeeded; expected an unmerged index")
	}
	merge = exec.Command("git", "merge", "feature/upstream")
	merge.Dir = worktreePath
	if err := merge.Run(); err == nil {
		t.Fatal("merge succeeded; expected an unmerged index")
	}

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	manifests, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: worktreePath, RepositoryPath: repo,
	}})
	if err != nil {
		t.Fatalf("capture unmerged index: %v", err)
	}
	manifest := manifests["wt"]
	if len(manifest.IndexStateSHA256) != 64 {
		t.Fatalf("unmerged index state digest = %q", manifest.IndexStateSHA256)
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.4
func TestArchiveManifestRejectsCorruptIndex(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature/task", worktreePath)
	indexPath := strings.TrimSpace(runGit(t, worktreePath, "rev-parse", "--path-format=absolute", "--git-path", "index"))
	if err := os.WriteFile(indexPath, []byte("corrupt index"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: worktreePath, RepositoryPath: repo,
	}}); err == nil {
		t.Fatal("captured source state with a corrupt Git index")
	}
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
func TestArchiveManifestInspectionDoesNotWriteGitObjects(t *testing.T) {
	repo := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "task-worktree")
	runGit(t, repo, "worktree", "add", "-b", "feature/task", worktreePath)
	if err := os.WriteFile(filepath.Join(worktreePath, "staged.txt"), []byte("staged"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, worktreePath, "add", "staged.txt")
	before := gitLooseObjectCount(t, repo)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CaptureArchiveSourceManifests(context.Background(), []*Worktree{{
		ID: "wt", TaskID: "task", RepositoryID: "repo", Path: worktreePath, RepositoryPath: repo,
	}}); err != nil {
		t.Fatalf("capture staged source state: %v", err)
	}
	if after := gitLooseObjectCount(t, repo); after != before {
		t.Fatalf("capture wrote Git objects: count before=%d after=%d", before, after)
	}
}

func gitLooseObjectCount(t *testing.T, repo string) int {
	t.Helper()
	output := runGit(t, repo, "count-objects", "-v")
	for _, line := range strings.Split(output, "\n") {
		if value, found := strings.CutPrefix(line, "count: "); found {
			count, err := strconv.Atoi(value)
			if err != nil {
				t.Fatal(err)
			}
			return count
		}
	}
	t.Fatal("git count-objects omitted the loose object count")
	return 0
}

// @covers AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3
func TestArchiveManifestHashesDirtySubmoduleDirectory(t *testing.T) {
	root := t.TempDir()
	submodule := filepath.Join(root, "module")
	if err := os.MkdirAll(submodule, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(submodule, ".git"), []byte("gitdir: ../.git/modules/module\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(submodule, "tracked.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := archiveSourceManifestEntries(root, " M module\x00")
	if err != nil {
		t.Fatalf("hash dirty submodule: %v", err)
	}
	if len(entries) != 1 || entries[0].ContentSHA256 == "" {
		t.Fatalf("submodule evidence = %+v", entries)
	}
}
