package service

import (
	"context"
	"testing"
	"time"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// A retained stopped executor row must not prevent workspace cleanup.
func TestCleanupWorkspaceGroupsDoesNotWaitOnStoppedMemberSession(t *testing.T) {
	repo := newCleanupSafetyTaskRepo(t, models.ExecutorRunningStatusStopped)
	groups := newCascadeWSGroupRepo()
	createCleanupTestGroup(t, groups)
	reader := &observingSessionWorktreeReader{
		SessionWorktreeReader: repo,
		checks:                make(chan bool, 1),
	}
	cleaner := &cleanupChannelWorkspaceCleaner{plainFolders: make(chan string, 1)}
	svc := NewHandoffService(nil, nil, nil, nil, groups, nil)
	svc.SetSessionReader(reader)
	svc.SetWorkspaceCleaner(cleaner)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.CleanupWorkspaceGroups(ctx, "ws-delete"); err != nil {
		t.Fatalf("cleanup workspace groups: %v", err)
	}
	if active := <-reader.checks; active {
		t.Fatal("stopped executor row was reported active")
	}
	if reader.checkCount != 1 {
		t.Fatalf("active executor checks = %d, want one", reader.checkCount)
	}
	if exists, err := repo.HasExecutorRunningRow(ctx, "session-1"); err != nil || !exists {
		t.Fatalf("retained executor row exists = %v, err = %v, want true", exists, err)
	}
	if got := <-cleaner.plainFolders; got != "/tmp/kandev-owned-group" {
		t.Fatalf("cleaned folder = %q, want owned group path", got)
	}
}

// A failed stream can leave its process alive after its group member is
// released. Cleanup must wait until the lifecycle owner confirms termination.
func TestCleanupWorkspaceGroupsWaitsForReleasedFailedExecutorUntilStopped(t *testing.T) {
	repo := newCleanupSafetyTaskRepo(t, models.ExecutorRunningStatusFailed)
	groups := newCascadeWSGroupRepo()
	createCleanupTestGroup(t, groups)
	groups.allMembers["group-owned"] = map[string]string{"task-1": "owner"}
	if err := groups.ReleaseWorkspaceGroupMember(context.Background(), "group-owned", "task-1", "archived", "cascade-1"); err != nil {
		t.Fatalf("release workspace group member: %v", err)
	}
	reader := &observingSessionWorktreeReader{
		SessionWorktreeReader: repo,
		checks:                make(chan bool, 3),
		heldChecks:            2,
		releaseChecks:         make(chan struct{}, 2),
	}
	defer close(reader.releaseChecks)
	cleaner := &cleanupChannelWorkspaceCleaner{plainFolders: make(chan string, 1)}
	svc := NewHandoffService(nil, nil, nil, nil, groups, nil)
	svc.SetSessionReader(reader)
	svc.SetWorkspaceCleaner(cleaner)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cleanupDone := make(chan error, 1)
	go func() {
		cleanupDone <- svc.CleanupWorkspaceGroups(ctx, "ws-delete")
	}()

	if active := waitForExecutorCheck(t, reader.checks); !active {
		t.Fatal("failed executor row was reported inactive before teardown")
	}
	assertWorkspaceNotCleaned(t, cleaner.plainFolders)
	reader.releaseChecks <- struct{}{}

	if active := waitForExecutorCheck(t, reader.checks); !active {
		t.Fatal("failed executor row was reported inactive while teardown was held")
	}
	assertWorkspaceNotCleaned(t, cleaner.plainFolders)
	if err := repo.UpdateExecutorRunningStatus(ctx, "session-1", models.ExecutorRunningStatusStopped); err != nil {
		t.Fatalf("confirm executor stopped: %v", err)
	}
	reader.releaseChecks <- struct{}{}

	if active := waitForExecutorCheck(t, reader.checks); active {
		t.Fatal("stopped executor row was reported active")
	}
	select {
	case err := <-cleanupDone:
		if err != nil {
			t.Fatalf("cleanup workspace groups: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("cleanup did not finish after executor stopped: %v", ctx.Err())
	}
	if got := <-cleaner.plainFolders; got != "/tmp/kandev-owned-group" {
		t.Fatalf("cleaned folder = %q, want owned group path", got)
	}
}

func newCleanupSafetyTaskRepo(t *testing.T, status string) *sqliterepo.Repository {
	t.Helper()
	_, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-delete", Name: "Workflow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-delete", WorkflowID: "wf-1", Title: "Cleanup test",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", IsPrimary: true,
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: "session-1", TaskID: "task-1", AgentExecutionID: "execution-1", Status: status,
	}); err != nil {
		t.Fatalf("create executor row: %v", err)
	}
	return repo
}

func createCleanupTestGroup(t *testing.T, groups *fakeWSGroupRepoCascade) {
	t.Helper()
	ctx := context.Background()
	if err := groups.CreateWorkspaceGroup(ctx, &orchmodels.WorkspaceGroup{
		ID:               "group-owned",
		WorkspaceID:      "ws-delete",
		OwnerTaskID:      "task-1",
		MaterializedPath: "/tmp/kandev-owned-group",
		MaterializedKind: orchmodels.WorkspaceGroupKindPlainFolder,
		OwnedByKandev:    true,
		CleanupPolicy:    orchmodels.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel,
		CleanupStatus:    orchmodels.WorkspaceCleanupStatusActive,
	}); err != nil {
		t.Fatalf("create owned group: %v", err)
	}
	if err := groups.AddWorkspaceGroupMember(ctx, "group-owned", "task-1", "owner"); err != nil {
		t.Fatalf("add group member: %v", err)
	}
}

type observingSessionWorktreeReader struct {
	SessionWorktreeReader
	checks        chan bool
	heldChecks    int
	releaseChecks chan struct{}
	checkCount    int
}

func (r *observingSessionWorktreeReader) HasActiveExecutorRunningRow(ctx context.Context, sessionID string) (bool, error) {
	active, err := r.SessionWorktreeReader.HasActiveExecutorRunningRow(ctx, sessionID)
	r.checkCount++
	if err != nil {
		return false, err
	}
	if r.checks != nil {
		r.checks <- active
	}
	if active && r.checkCount <= r.heldChecks {
		<-r.releaseChecks
	}
	return active, nil
}

func waitForExecutorCheck(t *testing.T, checks <-chan bool) bool {
	t.Helper()
	select {
	case active := <-checks:
		return active
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for executor activity check")
		return false
	}
}

func assertWorkspaceNotCleaned(t *testing.T, cleaned <-chan string) {
	t.Helper()
	select {
	case path := <-cleaned:
		t.Fatalf("workspace %q was cleaned before executor teardown", path)
	default:
	}
}

type cleanupChannelWorkspaceCleaner struct {
	plainFolders chan string
}

func (c *cleanupChannelWorkspaceCleaner) ValidateManagedRoot(string) error {
	return nil
}

func (c *cleanupChannelWorkspaceCleaner) CleanupPlainFolder(_ context.Context, path string) error {
	c.plainFolders <- path
	return nil
}

func (c *cleanupChannelWorkspaceCleaner) CleanupSingleRepoWorktree(context.Context, string) error {
	return nil
}

func (c *cleanupChannelWorkspaceCleaner) CleanupMultiRepoRoot(context.Context, string, []string) error {
	return nil
}

func (c *cleanupChannelWorkspaceCleaner) CleanupRemoteEnvironment(context.Context, string, string) error {
	return nil
}
