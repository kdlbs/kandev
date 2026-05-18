package service

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

// fakeSessionReader implements SessionWorktreeReader for the
// materializer tests. Sessions and worktrees are keyed by the task /
// session ID so tests can build small graphs without a real DB.
type fakeSessionReader struct {
	mu        sync.Mutex
	sessions  map[string][]*models.TaskSession         // taskID → sessions
	worktrees map[string][]*models.TaskSessionWorktree // sessionID → worktrees
	tasks     map[string]*models.Task
	// sessionsWithRunningExecutor records sessions that should be
	// reported as still bound to an active executor by
	// HasExecutorRunningRow. Tests use this to exercise the
	// cleanup-safety gate (post-review #5).
	sessionsWithRunningExecutor map[string]bool
}

func newFakeSessionReader() *fakeSessionReader {
	return &fakeSessionReader{
		sessions:                    map[string][]*models.TaskSession{},
		worktrees:                   map[string][]*models.TaskSessionWorktree{},
		tasks:                       map[string]*models.Task{},
		sessionsWithRunningExecutor: map[string]bool{},
	}
}

func (f *fakeSessionReader) ListTaskSessions(_ context.Context, taskID string) ([]*models.TaskSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessions[taskID], nil
}

func (f *fakeSessionReader) ListTaskSessionWorktrees(_ context.Context, sessionID string) ([]*models.TaskSessionWorktree, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.worktrees[sessionID], nil
}

func (f *fakeSessionReader) GetTask(_ context.Context, id string) (*models.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tasks[id], nil
}

func (f *fakeSessionReader) HasExecutorRunningRow(_ context.Context, sessionID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessionsWithRunningExecutor[sessionID], nil
}

func newMaterializerService(t *testing.T, tasks *fakeTaskRepo, ws *fakeWSGroupRepoCascade, sr *fakeSessionReader) *HandoffService {
	t.Helper()
	tr := newCascadeRepo(tasks)
	svc := NewHandoffService(tr, nil, nil, nil, ws, nil)
	svc.SetSessionReader(sr)
	return svc
}

func TestMarkOwnerSessionMaterialized_FlipsOwnership(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("owner", "", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "owner",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
		CleanupPolicy:    orchmodels.WorkspaceCleanupPolicyNeverDelete,
	}
	groups.members["g1"] = map[string]string{"owner": orchmodels.WorkspaceMemberRoleOwner}

	sr := newFakeSessionReader()
	sr.sessions["owner"] = []*models.TaskSession{{ID: "s1", IsPrimary: true, TaskID: "owner"}}
	sr.worktrees["s1"] = []*models.TaskSessionWorktree{{
		WorktreeID: "wt-1", RepositoryID: "repo-1",
		WorktreePath: "/tmp/wt", WorktreeBranch: "feature/x",
	}}
	svc := newMaterializerService(t, tasks, groups, sr)

	svc.MarkOwnerSessionMaterialized(context.Background(), "owner")
	g := groups.groups["g1"]
	if !g.OwnedByKandev {
		t.Error("group should now be owned_by_kandev=true")
	}
	if g.CleanupPolicy != orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel {
		t.Errorf("cleanup_policy = %q, want delete_when_last_member_*", g.CleanupPolicy)
	}
	if g.MaterializedPath != "/tmp/wt" {
		t.Errorf("materialized_path = %q", g.MaterializedPath)
	}
	if g.MaterializedKind != orchmodels.WorkspaceGroupKindSingleRepo {
		t.Errorf("kind = %q", g.MaterializedKind)
	}
}

// TestMarkOwnerSessionMaterialized_InheritParentMemberWithoutWorktrees
// covers the inherit_parent semantics: a child task whose session
// inherits the parent's TaskEnvironment does NOT have worktrees of its
// own. The materializer reports ok=false from materializationFromSession
// and the group stays unmaterialized until the parent's launch produces
// its own worktrees.
func TestMarkOwnerSessionMaterialized_InheritParentMemberWithoutWorktrees(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("owner", "", "ws-1")
	tasks.addTask("member", "owner", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "owner",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
	}
	groups.members["g1"] = map[string]string{
		"owner":  orchmodels.WorkspaceMemberRoleOwner,
		"member": orchmodels.WorkspaceMemberRoleMember,
	}

	sr := newFakeSessionReader()
	// member session inherits parent env — no worktrees of its own.
	sr.sessions["member"] = []*models.TaskSession{{ID: "s2", IsPrimary: true, TaskID: "member"}}
	// no entry in sr.worktrees["s2"] — empty list
	svc := newMaterializerService(t, tasks, groups, sr)

	svc.MarkOwnerSessionMaterialized(context.Background(), "member")
	if groups.groups["g1"].OwnedByKandev {
		t.Error("inherit_parent member without worktrees must not flip ownership")
	}
}

// TestMarkOwnerSessionMaterialized_SharedGroupFirstMemberFlips covers
// the shared_group semantics: when a non-owner member of a shared_group
// is the first to launch (with real worktrees), it materializes the
// group so later members can inherit the env. The MaterializedPath
// idempotency guard prevents double-materialization on subsequent
// launches.
func TestMarkOwnerSessionMaterialized_SharedGroupFirstMemberFlips(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("creator", "", "ws-1")
	tasks.addTask("first-launcher", "", "ws-1")
	groups := newCascadeWSGroupRepo()
	// The group's owner is the task that created it ("creator"), but
	// "first-launcher" is going to materialize first.
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "creator",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
	}
	groups.members["g1"] = map[string]string{
		"creator":        orchmodels.WorkspaceMemberRoleOwner,
		"first-launcher": orchmodels.WorkspaceMemberRoleMember,
	}

	sr := newFakeSessionReader()
	sr.sessions["first-launcher"] = []*models.TaskSession{{ID: "sFL", IsPrimary: true, TaskID: "first-launcher"}}
	sr.worktrees["sFL"] = []*models.TaskSessionWorktree{{
		WorktreeID: "wt-fl", RepositoryID: "repo-1",
		WorktreePath: "/tmp/wt-fl", WorktreeBranch: "feature/x",
	}}
	svc := newMaterializerService(t, tasks, groups, sr)

	svc.MarkOwnerSessionMaterialized(context.Background(), "first-launcher")
	if !groups.groups["g1"].OwnedByKandev {
		t.Error("first shared_group member with real worktrees must materialize the group")
	}
	if groups.groups["g1"].MaterializedPath != "/tmp/wt-fl" {
		t.Errorf("materialized_path = %q, want /tmp/wt-fl",
			groups.groups["g1"].MaterializedPath)
	}
}

func TestMarkOwnerSessionMaterialized_Idempotent(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("owner", "", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "owner",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		MaterializedPath: "/already/set",
		OwnedByKandev:    true,
	}
	groups.members["g1"] = map[string]string{"owner": orchmodels.WorkspaceMemberRoleOwner}

	sr := newFakeSessionReader()
	sr.sessions["owner"] = []*models.TaskSession{{ID: "s1", IsPrimary: true, TaskID: "owner"}}
	sr.worktrees["s1"] = []*models.TaskSessionWorktree{{
		WorktreePath: "/tmp/wt-changed",
	}}
	svc := newMaterializerService(t, tasks, groups, sr)

	svc.MarkOwnerSessionMaterialized(context.Background(), "owner")
	if groups.groups["g1"].MaterializedPath != "/already/set" {
		t.Error("idempotent re-invocation must not overwrite the materialized path")
	}
}

func TestMarkOwnerSessionMaterialized_NoSessionYetSilent(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("owner", "", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "owner",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
	}
	groups.members["g1"] = map[string]string{"owner": orchmodels.WorkspaceMemberRoleOwner}
	svc := newMaterializerService(t, tasks, groups, newFakeSessionReader())

	// Should be a clean no-op when there's no session yet.
	svc.MarkOwnerSessionMaterialized(context.Background(), "owner")
	if groups.groups["g1"].OwnedByKandev {
		t.Error("no-session case must not flip owned_by_kandev")
	}
}

func TestGetSharedGroupEnvironment_ReturnsMaterializedEnv(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("member", "", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "creator",
		MaterializedKind:          orchmodels.WorkspaceGroupKindRemoteEnvironment,
		MaterializedEnvironmentID: "env-abc",
	}
	groups.members["g1"] = map[string]string{
		"member": orchmodels.WorkspaceMemberRoleMember,
	}
	svc := newMaterializerService(t, tasks, groups, newFakeSessionReader())

	if got := svc.GetSharedGroupEnvironment(context.Background(), "member"); got != "env-abc" {
		t.Errorf("env = %q, want env-abc", got)
	}
}

func TestGetSharedGroupEnvironment_EmptyWhenNoGroup(t *testing.T) {
	svc := newMaterializerService(t, newFakeTaskRepo(), newCascadeWSGroupRepo(), newFakeSessionReader())
	if got := svc.GetSharedGroupEnvironment(context.Background(), "no-task"); got != "" {
		t.Errorf("no-group env should be empty, got %q", got)
	}
}

func TestGetSharedGroupEnvironment_EmptyWhenGroupUnmaterialized(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("member", "", "ws-1")
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "creator",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		// MaterializedEnvironmentID intentionally empty
	}
	groups.members["g1"] = map[string]string{"member": orchmodels.WorkspaceMemberRoleMember}
	svc := newMaterializerService(t, tasks, groups, newFakeSessionReader())

	if got := svc.GetSharedGroupEnvironment(context.Background(), "member"); got != "" {
		t.Errorf("unmaterialized group env should be empty, got %q", got)
	}
}

func TestRestoreCleanedGroups_PlainFolderMkdirs(t *testing.T) {
	tasks := newFakeTaskRepo()
	groups := newCascadeWSGroupRepo()
	tmp := t.TempDir()
	target := filepath.Join(tmp, "restored")
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "owner",
		MaterializedKind:  orchmodels.WorkspaceGroupKindPlainFolder,
		MaterializedPath:  target,
		CleanupStatus:     orchmodels.WorkspaceCleanupStatusCleaned,
		OwnedByKandev:     true,
		RestoreConfigJSON: `{"kind":"plain_folder","path":"` + target + `"}`,
	}
	svc := newMaterializerService(t, tasks, groups, newFakeSessionReader())

	svc.restoreCleanedGroups(context.Background(), []string{"g1"})

	if _, err := os.Stat(target); err != nil {
		t.Errorf("plain folder restore should mkdir the path; stat err: %v", err)
	}
}

func TestRestoreCleanedGroups_RepoKindMarksRestorable(t *testing.T) {
	tasks := newFakeTaskRepo()
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "owner",
		MaterializedKind:  orchmodels.WorkspaceGroupKindSingleRepo,
		CleanupStatus:     orchmodels.WorkspaceCleanupStatusCleaned,
		OwnedByKandev:     true,
		RestoreConfigJSON: `{"kind":"single_repo","worktree_ids":{"r":"wt"}}`,
	}
	svc := newMaterializerService(t, tasks, groups, newFakeSessionReader())

	svc.restoreCleanedGroups(context.Background(), []string{"g1"})

	if got := groups.cleanupStatuses["g1"]; got != orchmodels.WorkspaceCleanupStatusActive {
		t.Errorf("repo restore should flip cleanup_status to active; got %q", got)
	}
}

// REGRESSION: an empty restore_config_json (impossible if the
// materializer ran, but possible if someone hand-edited the row) must
// not panic. The restore should mark the group restore_failed.
func TestRestoreCleanedGroups_EmptyConfigMarksFailed(t *testing.T) {
	tasks := newFakeTaskRepo()
	groups := newCascadeWSGroupRepo()
	groups.groups["g1"] = &orchmodels.WorkspaceGroup{
		ID: "g1", WorkspaceID: "ws-1", OwnerTaskID: "owner",
		MaterializedKind: orchmodels.WorkspaceGroupKindSingleRepo,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusCleaned,
		OwnedByKandev:    true,
		// RestoreConfigJSON intentionally empty.
	}
	svc := newMaterializerService(t, tasks, groups, newFakeSessionReader())
	svc.restoreCleanedGroups(context.Background(), []string{"g1"})
	// We don't have UpdateWorkspaceGroupRestoreStatus tracking on the
	// fake; the contract here is "must not panic and must not advance
	// cleanup_status to active".
	if got := groups.cleanupStatuses["g1"]; got == orchmodels.WorkspaceCleanupStatusActive {
		t.Error("empty restore config must not transition to active")
	}
}
