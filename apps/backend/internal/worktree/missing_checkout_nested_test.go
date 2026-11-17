package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addNestedAdoptedSlot persists an agent-created worktree that lives below a
// checkout inside the task root, the way adoption records it.
func (f *missingCheckoutFixture) addNestedAdoptedSlot(t *testing.T, materialize bool) *Worktree {
	t.Helper()
	const branch = "feature/agent-nested"
	path := filepath.Join(f.worktreePath, ".tmp", "agent-nested")
	runGit(t, f.repositoryPath, "branch", branch, f.branchHead)
	if materialize {
		runGit(t, f.repositoryPath, "worktree", "add", path, branch)
	}
	relative, err := filepath.Rel(filepath.Dir(f.worktreePath), path)
	if err != nil {
		t.Fatalf("relative nested path: %v", err)
	}
	worktree := &Worktree{
		ID: "wt-nested-adopted", SessionID: f.sessionID, TaskID: f.taskID, TaskDirName: f.taskDirName,
		TaskEnvironmentID: f.environmentID, RepositoryID: f.repositoryID, BranchSlug: "agent-nested",
		WorkspaceRelativePath: relative, Position: 1,
		RepositoryPath: f.repositoryPath, Path: path, Branch: branch, BaseBranch: "main",
		BranchOwner: BranchOwnerExternal, Status: StatusActive,
	}
	if err := f.store.CreateWorktree(context.Background(), worktree); err != nil {
		t.Fatalf("persist nested adopted worktree: %v", err)
	}
	f.refreshSelectionSnapshot(t)
	f.request.Slots = append(f.request.Slots, RecoverySlot{
		WorktreeID: worktree.ID, RepositoryID: f.repositoryID, BranchSlug: worktree.BranchSlug,
		RepositoryPath: f.repositoryPath, Worktree: worktree,
	})
	return worktree
}

// @covers AC-TASKS-AGENT-WORKTREE-ADOPTION-003.6
func TestRecoveryAdmissionAcceptsPresentNestedAdoptedWorktree(t *testing.T) {
	ctx := context.Background()
	fixture := newMissingCheckoutFixture(t)
	nested := fixture.addNestedAdoptedSlot(t, true)

	admission, err := fixture.manager.AdmitRecovery(ctx, fixture.request)
	if err != nil {
		t.Fatalf("AdmitRecovery with a present nested adopted worktree: %v", err)
	}
	if admission != nil {
		if err := admission.Release(ctx); err != nil {
			t.Errorf("release recovery admission: %v", err)
		}
	}

	got, err := fixture.manager.Create(ctx, CreateRequest{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, TaskEnvironmentID: fixture.environmentID,
		RepositoryID: fixture.repositoryID, RepositoryPath: fixture.repositoryPath, BaseBranch: "main",
		WorktreeID: nested.ID, BranchSlug: nested.BranchSlug, ReuseRequired: true,
	})
	if err != nil {
		t.Fatalf("attach nested adopted worktree on resume: %v", err)
	}
	if got == nil || filepath.Clean(got.Path) != filepath.Clean(nested.Path) || got.Branch != nested.Branch {
		t.Fatalf("attached worktree = %+v, want %q on %q", got, nested.Path, nested.Branch)
	}
}

// @covers AC-TASKS-AGENT-WORKTREE-ADOPTION-003.6
func TestRecoveryAdmissionNeverRecreatesMissingNestedWorktree(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	nested := fixture.addNestedAdoptedSlot(t, false)

	_, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request)
	const wantErr = "automatic recovery requires a persisted managed task-root identity"
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("AdmitRecovery error = %v, want %q", err, wantErr)
	}
	if _, statErr := os.Lstat(nested.Path); !os.IsNotExist(statErr) {
		t.Fatalf("missing nested checkout was recreated at %q (stat err %v)", nested.Path, statErr)
	}
}

func TestMissingCheckoutLocationClassifiesTaskRootDescendants(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	taskRoot := filepath.Dir(fixture.worktreePath)
	tasksBase := filepath.Dir(taskRoot)
	tests := []struct {
		name        string
		path        string
		wantManaged bool
		wantErr     string
	}{
		{name: "direct child", path: fixture.worktreePath, wantManaged: true},
		{name: "nested inside own task root", path: filepath.Join(fixture.worktreePath, ".tmp", "agent"), wantManaged: false},
		{name: "outside tasks base", path: filepath.Join(t.TempDir(), "repo"), wantManaged: false},
		{name: "another task root", path: filepath.Join(tasksBase, "other-task", "repo"), wantManaged: true,
			wantErr: "does not match its managed task-root identity"},
		{name: "nested inside another task root", path: filepath.Join(tasksBase, "other-task", "repo", ".tmp", "agent"), wantManaged: true,
			wantErr: "does not match its managed task-root identity"},
		{name: "task root itself", path: taskRoot, wantManaged: true,
			wantErr: "does not match its managed task-root identity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wt := *fixture.request.Slots[0].Worktree
			wt.Path = tt.path
			location, managed, err := fixture.manager.missingCheckoutLocation(&wt)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("missingCheckoutLocation(%q) error = %v, want %q", tt.path, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("missingCheckoutLocation(%q): %v", tt.path, err)
			}
			if managed != tt.wantManaged {
				t.Fatalf("missingCheckoutLocation(%q) managed = %v, want %v (location %+v)", tt.path, managed, tt.wantManaged, location)
			}
		})
	}
}
