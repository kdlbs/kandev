package worktree

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type completedRelocationFixture struct {
	config       Config
	managedRoot  string
	source       string
	destination  string
	replacement  *Worktree
	store        *managedCloneRelocationStore
	manager      *Manager
	request      RecoveryAdmissionRequest
	recordPath   string
	historicalID string
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.7
func TestCompletedRelocationAdmitsCurrentWork(t *testing.T) {
	for _, provider := range []struct {
		name, host string
	}{
		{name: "github", host: "github.com"},
		{name: "gitlab", host: "gitlab.com"},
	} {
		for _, change := range []string{"commit", "amend", "non-descendant-rebase", "local-edits", "missing-historical-object"} {
			t.Run(provider.name+"/"+change, func(t *testing.T) {
				fixture := newCompletedRelocationFixture(t, provider.name, provider.host)
				beforeRecord, err := os.ReadFile(fixture.recordPath)
				if err != nil {
					t.Fatalf("read complete relocation journal: %v", err)
				}
				applyCompletedRelocationChange(t, fixture.replacement.Path, fixture.replacement.Branch, change)
				if change == "missing-historical-object" {
					record, readErr := readManagedCloneRelocationRecord(fixture.recordPath)
					if readErr != nil {
						t.Fatalf("read relocation journal: %v", readErr)
					}
					record.Head = strings.Repeat("0", 40)
					if err := writeManagedCloneRelocationRecord(fixture.recordPath, record, false); err != nil {
						t.Fatalf("write historical journal fixture: %v", err)
					}
					beforeRecord, err = os.ReadFile(fixture.recordPath)
					if err != nil {
						t.Fatalf("read historical journal fixture: %v", err)
					}
				}
				beforeHead := strings.TrimSpace(runGit(t, fixture.replacement.Path, "rev-parse", "HEAD"))
				if change != "local-edits" && change != "missing-historical-object" && beforeHead == fixture.historicalID {
					t.Fatal("test change did not move HEAD beyond the historical transfer commit")
				}
				beforeStatus := runGit(t, fixture.replacement.Path, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional")

				manager, err := NewManager(fixture.config, fixture.store, newTestLogger())
				if err != nil {
					t.Fatalf("reconstruct manager: %v", err)
				}
				admission, err := manager.AdmitRecovery(context.Background(), fixture.request)
				if err != nil {
					t.Fatalf("AdmitRecovery after completed relocation and %s: %v", change, err)
				}
				if admission != nil {
					_ = admission.Release(context.Background())
					t.Fatal("completed relocation unexpectedly requested another recovery operation")
				}

				afterHead := strings.TrimSpace(runGit(t, fixture.replacement.Path, "rev-parse", "HEAD"))
				afterStatus := runGit(t, fixture.replacement.Path, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional")
				if afterHead != beforeHead || afterStatus != beforeStatus {
					t.Fatalf("admission changed current work: HEAD %q -> %q, status %q -> %q", beforeHead, afterHead, beforeStatus, afterStatus)
				}
				afterRecord, err := os.ReadFile(fixture.recordPath)
				if err != nil {
					t.Fatalf("read relocation journal after admission: %v", err)
				}
				if !bytes.Equal(afterRecord, beforeRecord) {
					t.Fatal("ordinary reuse rewrote the completed relocation journal")
				}
			})
		}
	}
}

func TestCompletedRelocationVerificationPreservesCancellation(t *testing.T) {
	fixture := newCompletedRelocationFixture(t, "github", "github.com")
	record, err := readManagedCloneRelocationRecord(fixture.recordPath)
	if err != nil {
		t.Fatalf("read completed relocation journal: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = verifyCompletedManagedCloneRelocation(
		ctx, fixture.manager, fixture.replacement, fixture.request.Slots[0].CloneRelocation, record,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("completed relocation verification error = %v, want context.Canceled", err)
	}
}

func newCompletedRelocationFixture(t *testing.T, provider, host string) completedRelocationFixture {
	t.Helper()
	managedRoot := filepath.Join(t.TempDir(), "repos")
	source := filepath.Join(managedRoot, "_providers", provider, host, "acme", "widget")
	destination := filepath.Join(managedRoot, "workspaces", "workspace-1", provider, "acme", "widget")
	for _, path := range []string{source, destination} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create clone parent: %v", err)
		}
	}
	seed := initGitRepoForWorktreeTest(t)
	runGit(t, seed, "clone", "--no-hardlinks", seed, source)
	runGit(t, seed, "clone", "--no-hardlinks", seed, destination)
	for _, clone := range []string{source, destination} {
		configureManagedCloneRelocationGitIdentity(t, clone)
		runGit(t, clone, "remote", "set-url", "origin", "https://"+host+"/acme/widget.git")
	}

	branch := "feature/completed-relocation"
	runGit(t, source, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(source, "task-change.txt"), []byte("task commit\n"), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}
	runGit(t, source, "add", "task-change.txt")
	runGit(t, source, "commit", "-m", "task commit")
	runGit(t, source, "checkout", "main")

	config := newTestConfig(t)
	taskID := "completed-" + provider
	original := filepath.Join(config.TasksBasePath, taskID, "widget")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	runGit(t, source, "worktree", "add", original, branch)
	worktree := &Worktree{
		ID: "wt-" + provider, TaskID: taskID, TaskEnvironmentID: "env-" + provider,
		RepositoryID: "repo-" + provider, Path: original, RepositoryPath: destination,
		Branch: branch, BranchSlug: "branch-" + provider, Status: StatusActive,
	}
	store := &managedCloneRelocationStore{recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}}
	store.worktrees[worktree.ID] = worktree
	manager, err := NewManager(config, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	request := RecoveryAdmissionRequest{
		TaskID: taskID, SessionID: "session-" + provider, TaskEnvironmentID: worktree.TaskEnvironmentID,
		OwnerTaskID: taskID, OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree),
		Slots: []RecoverySlot{{
			WorktreeID: worktree.ID, RepositoryID: worktree.RepositoryID, BranchSlug: worktree.BranchSlug,
			RepositoryPath: destination,
			CloneRelocation: &ManagedCloneRelocationProof{
				ManagedRoot: managedRoot, ExpectedSourcePath: source, ExpectedDestinationPath: destination,
				Identity: ManagedRepositoryIdentity{Provider: provider, Host: host, Owner: "acme", Name: "widget"},
			},
		}},
	}
	admission, err := manager.AdmitRecovery(context.Background(), request)
	if err != nil {
		t.Fatalf("initial managed-clone relocation: %v", err)
	}
	if admission == nil {
		t.Fatal("initial managed-clone relocation returned no admission")
	}
	if err := admission.Release(context.Background()); err != nil {
		t.Fatalf("release initial relocation admission: %v", err)
	}
	var replacement *Worktree
	for _, candidate := range store.worktrees {
		if candidate.ID != worktree.ID {
			replacement = candidate
			break
		}
	}
	if replacement == nil {
		t.Fatal("initial relocation did not publish a replacement worktree")
	}
	request.Slots[0].WorktreeID = replacement.ID
	request.Slots[0].RepositoryID = replacement.RepositoryID
	request.Slots[0].BranchSlug = replacement.BranchSlug
	recordPath := replacement.Path + ".kandev-clone-relocation.json"
	record, err := readManagedCloneRelocationRecord(recordPath)
	if err != nil || record.State != string(RecoveryStateComplete) {
		t.Fatalf("published relocation journal = %+v, %v", record, err)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatalf("remove obsolete source clone: %v", err)
	}
	return completedRelocationFixture{
		config: config, managedRoot: managedRoot, source: source, destination: destination,
		replacement: replacement, store: store, manager: manager, request: request,
		recordPath: recordPath, historicalID: record.Head,
	}
}

func applyCompletedRelocationChange(t *testing.T, path, branch, change string) {
	t.Helper()
	switch change {
	case "commit":
		if err := os.WriteFile(filepath.Join(path, "later.txt"), []byte("later commit\n"), 0o644); err != nil {
			t.Fatalf("write later commit file: %v", err)
		}
		runGit(t, path, "add", "later.txt")
		runGit(t, path, "commit", "-m", "later commit")
	case "amend":
		if err := os.WriteFile(filepath.Join(path, "task-change.txt"), []byte("amended task commit\n"), 0o644); err != nil {
			t.Fatalf("write amended file: %v", err)
		}
		runGit(t, path, "add", "task-change.txt")
		runGit(t, path, "commit", "--amend", "--no-edit")
	case "non-descendant-rebase":
		runGit(t, path, "checkout", "--orphan", "rewritten-history")
		runGit(t, path, "add", "-A")
		runGit(t, path, "commit", "-m", "rewritten root")
		runGit(t, path, "branch", "-D", branch)
		runGit(t, path, "branch", "-m", branch)
	case "local-edits":
		if err := os.WriteFile(filepath.Join(path, "task-change.txt"), []byte("unstaged edit\n"), 0o644); err != nil {
			t.Fatalf("write unstaged edit: %v", err)
		}
		runGit(t, path, "add", "task-change.txt")
		if err := os.WriteFile(filepath.Join(path, "task-change.txt"), []byte("staged and unstaged edit\n"), 0o644); err != nil {
			t.Fatalf("write staged edit: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "untracked.txt"), []byte("untracked\n"), 0o644); err != nil {
			t.Fatalf("write untracked file: %v", err)
		}
		commonDir := strings.TrimSpace(runGit(t, path, "rev-parse", "--path-format=absolute", "--git-common-dir"))
		if err := os.WriteFile(filepath.Join(commonDir, "info", "exclude"), []byte("ignored.txt\n"), 0o644); err != nil {
			t.Fatalf("ignore local test file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "ignored.txt"), []byte("ignored\n"), 0o644); err != nil {
			t.Fatalf("write ignored file: %v", err)
		}
	case "missing-historical-object":
		return
	default:
		t.Fatalf("unknown completed relocation change %q", change)
	}
}
