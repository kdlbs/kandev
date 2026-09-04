package service_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/configsync"
	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestDeleteWorkspaceStopsTasksDeletesDataAndConfig(t *testing.T) {
	ctx := context.Background()
	taskSvc := &fakeWorkspaceTaskService{
		workspace: &taskmodels.Workspace{ID: "ws-delete", Name: "default"},
		tasks: []*taskmodels.Task{
			{ID: "task-1", WorkspaceID: "ws-delete"},
			{ID: "task-2", WorkspaceID: "ws-delete"},
		},
	}
	groupCleaner := &fakeWorkspaceGroupCleaner{groupID: "group-1"}
	svc := newTestService(t, service.ServiceOptions{
		TaskWorkspace:         taskSvc,
		TaskCanceller:         &fakeTaskCanceller{},
		WorkspaceGroupCleaner: groupCleaner,
	})
	groupCleaner.svc = svc

	createTestAgent(t, svc, "ws-delete", "agent-delete")
	if err := svc.CreateSkill(ctx, &officemodels.Skill{
		ID:          "skill-delete",
		WorkspaceID: "ws-delete",
		Name:        "Delete Skill",
		Slug:        "delete-skill",
	}); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	now := time.Now().UTC()
	svc.ExecSQL(t, `INSERT INTO task_workspace_groups (
		id, workspace_id, owner_task_id, materialized_path, materialized_kind,
		owned_by_kandev, cleanup_policy, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"group-1", "ws-delete", "task-1", "/tmp/kandev-group", officemodels.WorkspaceGroupKindPlainFolder,
		true, officemodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel, now, now)

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	if taskSvc.deletedWorkspace != "ws-delete" {
		t.Fatalf("deleted workspace = %q, want ws-delete", taskSvc.deletedWorkspace)
	}
	if got := taskSvc.deletedTasks; len(got) != 2 || got[0] != "task-1" || got[1] != "task-2" {
		t.Fatalf("deleted tasks = %#v, want task-1/task-2", got)
	}
	if !groupCleaner.called {
		t.Fatal("workspace group cleaner was not called")
	}
	if !groupCleaner.groupExistedDuringCleanup {
		t.Fatal("workspace group row should exist while cleanup runs")
	}
	if group, err := svc.GetWorkspaceGroupForTest(ctx, "group-1"); err != nil {
		t.Fatalf("get group after deletion: %v", err)
	} else if group != nil {
		t.Fatal("workspace group row should be removed after workspace deletion")
	}
	if _, err := os.Stat(svc.ConfigWriter().WorkspacePath("default")); !os.IsNotExist(err) {
		t.Fatalf("workspace config should be removed, stat err: %v", err)
	}
}

func TestDeleteWorkspaceUsesFreshDataDeletionTimeoutAfterGroupCleanup(t *testing.T) {
	ctx := context.Background()
	taskSvc := &fakeWorkspaceTaskService{
		workspace: &taskmodels.Workspace{ID: "ws-delete", Name: "default"},
		tasks:     []*taskmodels.Task{{ID: "task-1", WorkspaceID: "ws-delete"}},
	}
	groupCleaner := &deadlineWorkspaceGroupCleaner{}
	svc := newTestService(t, service.ServiceOptions{
		TaskWorkspace:         taskSvc,
		TaskCanceller:         &fakeTaskCanceller{},
		WorkspaceGroupCleaner: groupCleaner,
	})

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	if groupCleaner.deadline.IsZero() {
		t.Fatal("workspace group cleanup did not receive a timeout")
	}
	if len(taskSvc.deletedTaskDeadlines) != 1 {
		t.Fatalf("deleted task deadlines = %#v, want one deadline", taskSvc.deletedTaskDeadlines)
	}
	if !taskSvc.deletedTaskDeadlines[0].After(groupCleaner.deadline) {
		t.Fatalf("task deletion deadline = %v, want after group cleanup deadline %v",
			taskSvc.deletedTaskDeadlines[0], groupCleaner.deadline)
	}
}

// TestDeleteWorkspaceCleansUpConfigSyncData guards against config sync's own
// tables (office_config_sync_configs, office_config_sync_manifest) surviving
// a workspace delete: they carry no FK/cascade onto the workspace row, so
// without this call a deleted workspace's poller would keep running and
// could resurrect entities into it.
func TestDeleteWorkspaceCleansUpConfigSyncData(t *testing.T) {
	ctx := context.Background()
	taskSvc := &fakeWorkspaceTaskService{
		workspace: &taskmodels.Workspace{ID: "ws-delete", Name: "default"},
		tasks:     []*taskmodels.Task{{ID: "task-1", WorkspaceID: "ws-delete"}},
	}
	configSyncCleaner := &fakeConfigSyncCleaner{}
	svc := newTestService(t, service.ServiceOptions{
		TaskWorkspace: taskSvc,
		TaskCanceller: &fakeTaskCanceller{},
	})
	svc.SetConfigSyncCleaner(configSyncCleaner)

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	if configSyncCleaner.workspaceID != "ws-delete" {
		t.Fatalf("config sync cleaner workspace = %q, want ws-delete", configSyncCleaner.workspaceID)
	}
}

// TestDeleteWorkspaceReallyDeletesConfigSyncData wires the real
// configsync.Service (not a fake) as the ConfigSyncCleaner, seeds an actual
// config row and several manifest entries, and asserts they are all gone
// from the database after DeleteWorkspace. This proves the observable
// end-state; it does not by itself distinguish a bulk delete from a
// per-entity release loop, since both leave the same rows gone — that
// distinction (PurgeForWorkspaceDeletion serializes against an in-flight run
// via the per-workspace lock, the same lock DeleteConfigForWorkspace's
// release loop takes) is covered directly in
// configsync.TestService_PurgeForWorkspaceDeletion_SerializesAgainstInFlightRun.
func TestDeleteWorkspaceReallyDeletesConfigSyncData(t *testing.T) {
	ctx := context.Background()
	taskSvc := &fakeWorkspaceTaskService{
		workspace: &taskmodels.Workspace{ID: "ws-delete", Name: "default"},
		tasks:     []*taskmodels.Task{{ID: "task-1", WorkspaceID: "ws-delete"}},
	}
	svc := newTestService(t, service.ServiceOptions{
		TaskWorkspace: taskSvc,
		TaskCanceller: &fakeTaskCanceller{},
	})

	repo := svc.RepoForTest()
	store, err := configsync.NewStore(repo.Writer(), repo.Writer())
	if err != nil {
		t.Fatalf("new config sync store: %v", err)
	}
	configSync := configsync.NewService(nil, store, logger.Default())
	svc.SetConfigSyncCleaner(configSync)

	path := "office"
	pollEnabled := true
	if _, err := configSync.SetConfigForWorkspace(ctx, "ws-delete", &configsync.SetConfigRequest{
		Provider: "github", RepoOwner: "acme", RepoName: "kandev", Path: &path,
		Branch: "main", PollEnabled: &pollEnabled, IntervalSeconds: 300,
	}); err != nil {
		t.Fatalf("seed config sync config: %v", err)
	}
	if err := store.UpsertManifestEntry(ctx, "ws-delete", "agent", "ceo", "agent-delete", "agents/ceo.yml"); err != nil {
		t.Fatalf("seed config sync manifest entry: %v", err)
	}
	if err := store.UpsertManifestEntry(ctx, "ws-delete", "skill", "onboarding", "skill-delete", "skills/onboarding/SKILL.md"); err != nil {
		t.Fatalf("seed second config sync manifest entry: %v", err)
	}

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	cfg, err := store.GetConfigForWorkspace(ctx, "ws-delete")
	if err != nil {
		t.Fatalf("get config sync config after delete: %v", err)
	}
	if cfg != nil {
		t.Fatalf("config sync config should be deleted, got %#v", cfg)
	}
	manifest, err := store.ListManifest(ctx, "ws-delete")
	if err != nil {
		t.Fatalf("list config sync manifest after delete: %v", err)
	}
	if len(manifest) != 0 {
		t.Fatalf("config sync manifest should be empty, got %#v", manifest)
	}
}

// TestDeleteWorkspaceFailsWhenConfigSyncCleanupFails asserts a config sync
// cleanup failure aborts the deletion rather than being ignored, leaving the
// workspace row (and the config sync rows the failed call couldn't clear)
// in place for a retry.
func TestDeleteWorkspaceFailsWhenConfigSyncCleanupFails(t *testing.T) {
	ctx := context.Background()
	taskSvc := &fakeWorkspaceTaskService{
		workspace: &taskmodels.Workspace{ID: "ws-delete", Name: "default"},
		tasks:     []*taskmodels.Task{{ID: "task-1", WorkspaceID: "ws-delete"}},
	}
	configSyncCleaner := &fakeConfigSyncCleaner{err: errors.New("boom")}
	svc := newTestService(t, service.ServiceOptions{
		TaskWorkspace: taskSvc,
		TaskCanceller: &fakeTaskCanceller{},
	})
	svc.SetConfigSyncCleaner(configSyncCleaner)

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err == nil {
		t.Fatal("DeleteWorkspace: want error when config sync cleanup fails")
	}
	if taskSvc.deletedWorkspace != "" {
		t.Fatalf("workspace row should not be deleted when config sync cleanup fails, got deletedWorkspace=%q", taskSvc.deletedWorkspace)
	}
}

// TestDeleteWorkspaceReleasesConfigSyncLockAfterTeardownCompletes proves the
// config sync lock PurgeForWorkspaceDeletion returns stays held across the
// rest of DeleteWorkspace's teardown, not released immediately after the
// purge call: an in-flight config sync run queued behind the lock must not
// be able to write rows back in once DeleteWorkspaceData (which never
// touches config sync's own tables) has already deleted the workspace's
// other data.
func TestDeleteWorkspaceReleasesConfigSyncLockAfterTeardownCompletes(t *testing.T) {
	ctx := context.Background()
	configSyncCleaner := &fakeConfigSyncCleaner{}
	taskSvc := &fakeWorkspaceTaskService{
		workspace:         &taskmodels.Workspace{ID: "ws-delete", Name: "default"},
		tasks:             []*taskmodels.Task{{ID: "task-1", WorkspaceID: "ws-delete"}},
		configSyncCleaner: configSyncCleaner,
	}
	svc := newTestService(t, service.ServiceOptions{
		TaskWorkspace: taskSvc,
		TaskCanceller: &fakeTaskCanceller{},
	})
	svc.SetConfigSyncCleaner(configSyncCleaner)

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	if configSyncCleaner.unlockedWhenWorkspaceRowDeleted {
		t.Fatal("config sync lock was released before the workspace row was deleted, want it held until teardown completes")
	}
	if !configSyncCleaner.unlocked {
		t.Fatal("config sync lock should be released once DeleteWorkspace completes")
	}
}

type fakeConfigSyncCleaner struct {
	workspaceID string
	err         error
	// unlocked and unlockedWhenWorkspaceRowDeleted let a test prove the
	// returned unlock func is held across the rest of DeleteWorkspace's
	// teardown rather than released immediately after the purge call.
	unlocked                        bool
	unlockedWhenWorkspaceRowDeleted bool
}

func (f *fakeConfigSyncCleaner) PurgeForWorkspaceDeletion(_ context.Context, workspaceID string) (func(), error) {
	f.workspaceID = workspaceID
	if f.err != nil {
		return nil, f.err
	}
	return func() { f.unlocked = true }, nil
}

type fakeWorkspaceTaskService struct {
	workspace            *taskmodels.Workspace
	tasks                []*taskmodels.Task
	deletedTasks         []string
	deletedTaskDeadlines []time.Time
	deletedWorkspace     string
	// configSyncCleaner, when set, lets DeleteWorkspace record whether the
	// config sync lock had already been released by the time the workspace
	// row itself is deleted (a later teardown step than the purge call).
	configSyncCleaner *fakeConfigSyncCleaner
}

func (f *fakeWorkspaceTaskService) GetWorkspace(context.Context, string) (*taskmodels.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeWorkspaceTaskService) ListWorkspaces(context.Context) ([]*taskmodels.Workspace, error) {
	return []*taskmodels.Workspace{f.workspace}, nil
}

func (f *fakeWorkspaceTaskService) DeleteWorkspace(_ context.Context, id string) error {
	f.deletedWorkspace = id
	if f.configSyncCleaner != nil {
		f.configSyncCleaner.unlockedWhenWorkspaceRowDeleted = f.configSyncCleaner.unlocked
	}
	return nil
}

func (f *fakeWorkspaceTaskService) ListTasksByWorkspace(
	context.Context,
	string,
	string,
	string,
	string,
	int,
	int,
	string,
	bool,
	bool,
	bool,
	bool,
) ([]*taskmodels.Task, int, error) {
	return f.tasks, len(f.tasks), nil
}

func (f *fakeWorkspaceTaskService) DeleteTask(ctx context.Context, id string) error {
	f.deletedTasks = append(f.deletedTasks, id)
	if deadline, ok := ctx.Deadline(); ok {
		f.deletedTaskDeadlines = append(f.deletedTaskDeadlines, deadline)
	}
	return nil
}

func (f *fakeWorkspaceTaskService) GetLastAgentMessage(context.Context, string) (string, error) {
	return "", nil
}

func (f *fakeWorkspaceTaskService) GetLastAgentMessageForTurn(context.Context, string) (string, error) {
	return "", nil
}

type fakeWorkspaceGroupCleaner struct {
	svc                       *service.Service
	groupID                   string
	called                    bool
	groupExistedDuringCleanup bool
}

func (f *fakeWorkspaceGroupCleaner) CleanupWorkspaceGroups(ctx context.Context, workspaceID string) error {
	f.called = true
	group, err := f.svc.GetWorkspaceGroupForTest(ctx, f.groupID)
	if err != nil {
		return err
	}
	f.groupExistedDuringCleanup = group != nil && group.WorkspaceID == workspaceID
	return nil
}

type deadlineWorkspaceGroupCleaner struct {
	deadline time.Time
}

func (f *deadlineWorkspaceGroupCleaner) CleanupWorkspaceGroups(ctx context.Context, _ string) error {
	if deadline, ok := ctx.Deadline(); ok {
		f.deadline = deadline
	}
	return nil
}

type fakeTaskCanceller struct {
	taskIDs []string
}

func (f *fakeTaskCanceller) CancelTaskExecution(_ context.Context, taskID string, _ string, _ bool) error {
	f.taskIDs = append(f.taskIDs, taskID)
	return nil
}
