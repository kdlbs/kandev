package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newWorkspaceGroupTestRepo provides a repo with a tasks table so the
// active-member join works. Mirrors newTreeHoldTestRepo.
func newWorkspaceGroupTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo := newTestRepo(t)
	_, err := repo.ExecRaw(context.Background(), `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			parent_id TEXT DEFAULT '',
			archived_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	return repo
}

func insertWGTask(t *testing.T, repo *sqlite.Repository, id string, archived bool) {
	t.Helper()
	q := `INSERT INTO tasks (id, workspace_id) VALUES (?, 'ws-1')`
	if archived {
		q = `INSERT INTO tasks (id, workspace_id, archived_at) VALUES (?, 'ws-1', datetime('now'))`
	}
	if _, err := repo.ExecRaw(context.Background(), q, id); err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

// TestWorkspaceGroup_CreateDefaultsAreSafe is the critical safety test:
// a freshly-inserted group MUST default to owned_by_kandev=false and
// cleanup_policy=never_delete. Anything else risks deleting user data.
func TestWorkspaceGroup_CreateDefaultsAreSafe(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	ctx := context.Background()

	g := &models.WorkspaceGroup{
		WorkspaceID:      "ws-1",
		OwnerTaskID:      "owner-task",
		MaterializedKind: models.WorkspaceGroupKindSingleRepo,
	}
	if err := repo.CreateWorkspaceGroup(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.ID == "" {
		t.Fatal("ID should be auto-assigned")
	}

	got, err := repo.GetWorkspaceGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("group not found after create")
	}
	if got.OwnedByKandev {
		t.Error("OwnedByKandev should default to false (cleanup-safety invariant)")
	}
	if got.CleanupPolicy != models.WorkspaceCleanupPolicyNeverDelete {
		t.Errorf("CleanupPolicy = %q, want never_delete", got.CleanupPolicy)
	}
	if got.CleanupStatus != models.WorkspaceCleanupStatusActive {
		t.Errorf("CleanupStatus = %q, want active", got.CleanupStatus)
	}
	if got.RestoreStatus != models.WorkspaceRestoreStatusNotNeeded {
		t.Errorf("RestoreStatus = %q, want not_needed", got.RestoreStatus)
	}
}

// TestWorkspaceGroup_RawInsertDefaultsAreSafe asserts the SQL DEFAULTs
// are correct even when a writer bypasses the Go method and inserts
// the minimum required columns directly. Belt-and-braces against any
// future call site that builds its own INSERT.
func TestWorkspaceGroup_RawInsertDefaultsAreSafe(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO task_workspace_groups (
			id, workspace_id, owner_task_id, materialized_kind, created_at, updated_at
		) VALUES (?, 'ws-1', 'owner-task', 'plain_folder', datetime('now'), datetime('now'))
	`, "raw-1"); err != nil {
		t.Fatalf("raw insert: %v", err)
	}
	got, err := repo.GetWorkspaceGroup(ctx, "raw-1")
	if err != nil || got == nil {
		t.Fatalf("get raw: err=%v got=%v", err, got)
	}
	if got.OwnedByKandev {
		t.Error("raw insert: owned_by_kandev defaulted to true (UNSAFE)")
	}
	if got.CleanupPolicy != models.WorkspaceCleanupPolicyNeverDelete {
		t.Errorf("raw insert cleanup_policy = %q, want never_delete", got.CleanupPolicy)
	}
}

// TestMarkWorkspaceMaterialized_FlipsOwnershipAtomically: setting
// OwnedByKandev=true via the materializer is the only way the cleanup
// policy switches to delete_when_last_member. Setting OwnedByKandev=false
// must keep it at never_delete.
func TestMarkWorkspaceMaterialized_FlipsOwnershipAtomically(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	ctx := context.Background()
	g := &models.WorkspaceGroup{
		WorkspaceID:      "ws-1",
		OwnerTaskID:      "owner-task",
		MaterializedKind: models.WorkspaceGroupKindSingleRepo,
	}
	if err := repo.CreateWorkspaceGroup(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}
	err := repo.MarkWorkspaceMaterialized(ctx, g.ID, models.MaterializedWorkspace{
		Path:          "/tmp/wt/abc",
		Kind:          models.WorkspaceGroupKindSingleRepo,
		OwnedByKandev: true,
		RestoreConfig: `{"kind":"single_repo"}`,
	})
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	got, _ := repo.GetWorkspaceGroup(ctx, g.ID)
	if !got.OwnedByKandev {
		t.Error("OwnedByKandev should be true after materialization")
	}
	if got.CleanupPolicy != models.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel {
		t.Errorf("CleanupPolicy = %q, want delete_when_last_member_*", got.CleanupPolicy)
	}
	if got.MaterializedPath != "/tmp/wt/abc" {
		t.Errorf("MaterializedPath = %q", got.MaterializedPath)
	}
	if got.RestoreConfigJSON != `{"kind":"single_repo"}` {
		t.Errorf("RestoreConfigJSON not round-tripped: %q", got.RestoreConfigJSON)
	}

	// Now mark again with OwnedByKandev=false → cleanup policy must
	// flip back to never_delete (defensive: a reuser of an existing
	// path should not silently inherit deletable state).
	g2 := &models.WorkspaceGroup{
		WorkspaceID:      "ws-1",
		OwnerTaskID:      "owner-task-2",
		MaterializedKind: models.WorkspaceGroupKindPlainFolder,
	}
	if err := repo.CreateWorkspaceGroup(ctx, g2); err != nil {
		t.Fatalf("create g2: %v", err)
	}
	if err := repo.MarkWorkspaceMaterialized(ctx, g2.ID, models.MaterializedWorkspace{
		Path:          "/Users/dev/my-checkout",
		Kind:          models.WorkspaceGroupKindPlainFolder,
		OwnedByKandev: false,
	}); err != nil {
		t.Fatalf("mark g2: %v", err)
	}
	got2, _ := repo.GetWorkspaceGroup(ctx, g2.ID)
	if got2.OwnedByKandev {
		t.Error("OwnedByKandev should remain false")
	}
	if got2.CleanupPolicy != models.WorkspaceCleanupPolicyNeverDelete {
		t.Errorf("CleanupPolicy = %q, want never_delete (user-owned path)", got2.CleanupPolicy)
	}
}

func TestMarkWorkspaceMaterialized_GroupNotFound(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	err := repo.MarkWorkspaceMaterialized(context.Background(), "missing", models.MaterializedWorkspace{
		Kind: models.WorkspaceGroupKindPlainFolder,
	})
	if err == nil {
		t.Fatal("expected error for missing group")
	}
}

func TestWorkspaceGroupMembers_AddListRelease(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	ctx := context.Background()
	insertWGTask(t, repo, "task-a", false)
	insertWGTask(t, repo, "task-b", false)
	insertWGTask(t, repo, "task-c", false)

	g := &models.WorkspaceGroup{
		WorkspaceID:      "ws-1",
		OwnerTaskID:      "task-a",
		MaterializedKind: models.WorkspaceGroupKindSingleRepo,
	}
	if err := repo.CreateWorkspaceGroup(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.AddWorkspaceGroupMember(ctx, g.ID, "task-a", models.WorkspaceMemberRoleOwner); err != nil {
		t.Fatalf("add owner: %v", err)
	}
	if err := repo.AddWorkspaceGroupMember(ctx, g.ID, "task-b", ""); err != nil {
		t.Fatalf("add b: %v", err)
	}
	if err := repo.AddWorkspaceGroupMember(ctx, g.ID, "task-c", ""); err != nil {
		t.Fatalf("add c: %v", err)
	}

	active, err := repo.ListActiveWorkspaceGroupMembers(ctx, g.ID)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 3 {
		t.Fatalf("active count = %d, want 3", len(active))
	}

	// Default role for an empty role param should be "member".
	for _, m := range active {
		if m.TaskID == "task-b" && m.Role != models.WorkspaceMemberRoleMember {
			t.Errorf("task-b role = %q, want member", m.Role)
		}
		if m.TaskID == "task-a" && m.Role != models.WorkspaceMemberRoleOwner {
			t.Errorf("task-a role = %q, want owner", m.Role)
		}
	}

	// Release b with a cascade id.
	if err := repo.ReleaseWorkspaceGroupMember(ctx, g.ID, "task-b",
		models.WorkspaceReleaseReasonArchived, "cascade-1"); err != nil {
		t.Fatalf("release b: %v", err)
	}
	active, _ = repo.ListActiveWorkspaceGroupMembers(ctx, g.ID)
	if len(active) != 2 {
		t.Fatalf("active count after release = %d, want 2", len(active))
	}
	for _, m := range active {
		if m.TaskID == "task-b" {
			t.Error("task-b should not appear in active list after release")
		}
	}

	// All members (incl. released) must still expose history.
	all, err := repo.ListWorkspaceGroupMembers(ctx, g.ID)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("all count = %d, want 3", len(all))
	}
	for _, m := range all {
		if m.TaskID == "task-b" {
			if m.ReleasedAt == nil {
				t.Error("task-b ReleasedAt should be non-nil")
			}
			if m.ReleaseReason != models.WorkspaceReleaseReasonArchived {
				t.Errorf("task-b release_reason = %q", m.ReleaseReason)
			}
			if m.ReleasedByCascadeID != "cascade-1" {
				t.Errorf("task-b cascade id = %q", m.ReleasedByCascadeID)
			}
		}
	}

	// Re-releasing is idempotent: stamps from the second cascade are ignored
	// because released_at IS NULL no longer holds.
	if err := repo.ReleaseWorkspaceGroupMember(ctx, g.ID, "task-b",
		models.WorkspaceReleaseReasonDeleted, "cascade-2"); err != nil {
		t.Fatalf("re-release: %v", err)
	}
	all, _ = repo.ListWorkspaceGroupMembers(ctx, g.ID)
	for _, m := range all {
		if m.TaskID == "task-b" && m.ReleasedByCascadeID != "cascade-1" {
			t.Errorf("task-b cascade id changed on re-release: %q", m.ReleasedByCascadeID)
		}
	}
}

// TestListActiveWorkspaceGroupMembers_ExcludesArchivedTasks: a member task
// archived directly on the tasks table (without a release row update) is
// still excluded from the active list. This is the join contract.
func TestListActiveWorkspaceGroupMembers_ExcludesArchivedTasks(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	ctx := context.Background()
	insertWGTask(t, repo, "live", false)
	insertWGTask(t, repo, "archived", true)

	g := &models.WorkspaceGroup{
		WorkspaceID:      "ws-1",
		OwnerTaskID:      "live",
		MaterializedKind: models.WorkspaceGroupKindMultiRepo,
	}
	if err := repo.CreateWorkspaceGroup(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = repo.AddWorkspaceGroupMember(ctx, g.ID, "live", "")
	_ = repo.AddWorkspaceGroupMember(ctx, g.ID, "archived", "")

	active, err := repo.ListActiveWorkspaceGroupMembers(ctx, g.ID)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 || active[0].TaskID != "live" {
		t.Errorf("expected only live task in active members, got %+v", active)
	}
}

func TestRestoreWorkspaceGroupMemberByCascade_ScopedToCascade(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	ctx := context.Background()
	insertWGTask(t, repo, "task-a", false)
	insertWGTask(t, repo, "task-b", false)

	g := &models.WorkspaceGroup{
		WorkspaceID:      "ws-1",
		OwnerTaskID:      "task-a",
		MaterializedKind: models.WorkspaceGroupKindSingleRepo,
	}
	_ = repo.CreateWorkspaceGroup(ctx, g)
	_ = repo.AddWorkspaceGroupMember(ctx, g.ID, "task-a", "")
	_ = repo.AddWorkspaceGroupMember(ctx, g.ID, "task-b", "")

	// task-a released by cascade-1; task-b released by cascade-2.
	_ = repo.ReleaseWorkspaceGroupMember(ctx, g.ID, "task-a",
		models.WorkspaceReleaseReasonArchived, "cascade-1")
	_ = repo.ReleaseWorkspaceGroupMember(ctx, g.ID, "task-b",
		models.WorkspaceReleaseReasonArchived, "cascade-2")

	// Restoring cascade-1 should ONLY un-release task-a.
	if err := repo.RestoreWorkspaceGroupMemberByCascade(ctx, "task-a", "cascade-1"); err != nil {
		t.Fatalf("restore a: %v", err)
	}
	if err := repo.RestoreWorkspaceGroupMemberByCascade(ctx, "task-b", "cascade-1"); err != nil {
		t.Fatalf("restore b (no-op): %v", err)
	}

	all, _ := repo.ListWorkspaceGroupMembers(ctx, g.ID)
	for _, m := range all {
		switch m.TaskID {
		case "task-a":
			if m.ReleasedAt != nil {
				t.Error("task-a should be restored")
			}
			if m.ReleasedByCascadeID != "" {
				t.Errorf("task-a cascade id should be cleared, got %q", m.ReleasedByCascadeID)
			}
		case "task-b":
			if m.ReleasedAt == nil {
				t.Error("task-b should still be released (cascade-2 mismatch)")
			}
			if m.ReleasedByCascadeID != "cascade-2" {
				t.Errorf("task-b cascade id corrupted: %q", m.ReleasedByCascadeID)
			}
		}
	}
}

func TestGetWorkspaceGroupForTask(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	ctx := context.Background()
	insertWGTask(t, repo, "owner", false)
	insertWGTask(t, repo, "member", false)

	g := &models.WorkspaceGroup{
		WorkspaceID:      "ws-1",
		OwnerTaskID:      "owner",
		MaterializedKind: models.WorkspaceGroupKindSingleRepo,
	}
	_ = repo.CreateWorkspaceGroup(ctx, g)
	_ = repo.AddWorkspaceGroupMember(ctx, g.ID, "member", "")

	got, err := repo.GetWorkspaceGroupForTask(ctx, "member")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.ID != g.ID {
		t.Fatalf("expected group %q, got %+v", g.ID, got)
	}

	// Unknown task → (nil, nil).
	got, err = repo.GetWorkspaceGroupForTask(ctx, "stranger")
	if err != nil || got != nil {
		t.Errorf("expected nil for stranger; err=%v got=%v", err, got)
	}

	// Released member → not found.
	_ = repo.ReleaseWorkspaceGroupMember(ctx, g.ID, "member",
		models.WorkspaceReleaseReasonArchived, "cascade-x")
	got, _ = repo.GetWorkspaceGroupForTask(ctx, "member")
	if got != nil {
		t.Error("released member should not return a group")
	}
}

func TestWorkspaceGroup_StatusUpdates(t *testing.T) {
	repo := newWorkspaceGroupTestRepo(t)
	ctx := context.Background()
	g := &models.WorkspaceGroup{
		WorkspaceID:      "ws-1",
		OwnerTaskID:      "owner",
		MaterializedKind: models.WorkspaceGroupKindPlainFolder,
	}
	_ = repo.CreateWorkspaceGroup(ctx, g)

	if err := repo.UpdateWorkspaceGroupCleanupStatus(ctx, g.ID,
		models.WorkspaceCleanupStatusFailed, "rm: permission denied", nil); err != nil {
		t.Fatalf("update cleanup status: %v", err)
	}
	got, _ := repo.GetWorkspaceGroup(ctx, g.ID)
	if got.CleanupStatus != models.WorkspaceCleanupStatusFailed {
		t.Errorf("status = %q", got.CleanupStatus)
	}
	if got.CleanupError != "rm: permission denied" {
		t.Errorf("error = %q", got.CleanupError)
	}

	if err := repo.UpdateWorkspaceGroupRestoreStatus(ctx, g.ID,
		models.WorkspaceRestoreStatusFailed, "missing config"); err != nil {
		t.Fatalf("update restore status: %v", err)
	}
	got, _ = repo.GetWorkspaceGroup(ctx, g.ID)
	if got.RestoreStatus != models.WorkspaceRestoreStatusFailed {
		t.Errorf("restore status = %q", got.RestoreStatus)
	}
	if got.RestoreError != "missing config" {
		t.Errorf("restore error = %q", got.RestoreError)
	}
}
