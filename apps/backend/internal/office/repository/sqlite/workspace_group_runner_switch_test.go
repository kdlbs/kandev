package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepoerrors "github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// newRunnerSwitchGroupTestRepos opens one shared writer/reader pair — the
// same production shape internal/backendapp/storage.go wires (one *sqlx.DB
// pool feeding both the task and office repositories) — and builds both
// repositories against it. This is required, not incidental: a runner
// switch and a workspace-group membership insert race for real only when
// both go through the same SQLite writer pool, exactly as they do in
// production.
func newRunnerSwitchGroupTestRepos(t *testing.T) (*tasksqlite.Repository, *officesqlite.Repository) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runner-switch-group.db")
	writerConn, err := dbutil.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite writer: %v", err)
	}
	writer := sqlx.NewDb(writerConn, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })

	readerConn, err := dbutil.OpenSQLiteReader(path)
	if err != nil {
		t.Fatalf("open sqlite reader: %v", err)
	}
	reader := sqlx.NewDb(readerConn, "sqlite3")
	t.Cleanup(func() { _ = reader.Close() })

	taskRepo, err := tasksqlite.NewWithDB(writer, reader, nil)
	if err != nil {
		t.Fatalf("new task repo: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(writer, reader, nil)
	if err != nil {
		t.Fatalf("new office repo: %v", err)
	}
	return taskRepo, officeRepo
}

// TestAddWorkspaceGroupMemberRacingSwitchTaskRunnerNeverBlends is the
// AC-TASKS-RUNNER-SWITCH-002.3a/3b acceptance evidence for the
// AddWorkspaceGroupMember retrofit: a runner switch and a workspace-group
// membership insert racing on the same task must resolve to exactly one of
// two outcomes — the switch commits first and the membership insert lands
// after (a state the switch never saw), or the membership lands first and
// the switch is rejected as workspace_group_member — never a switch that
// commits alongside a membership it should have seen and blocked on.
func TestAddWorkspaceGroupMemberRacingSwitchTaskRunnerNeverBlends(t *testing.T) {
	taskRepo, officeRepo := newRunnerSwitchGroupTestRepos(t)
	ctx := context.Background()

	if err := taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-1", Name: "ws-1"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := taskRepo.CreateTask(ctx, &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "runner switch group race"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	repository := &taskmodels.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "repo-1"}
	if err := taskRepo.CreateRepository(ctx, repository); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	taskRepository := &taskmodels.TaskRepository{ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main"}
	if err := taskRepo.CreateTaskRepository(ctx, taskRepository); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	stored, err := taskRepo.GetTaskRepository(ctx, taskRepository.ID)
	if err != nil {
		t.Fatalf("reload task repository: %v", err)
	}

	group := &officemodels.WorkspaceGroup{WorkspaceID: "ws-1", OwnerTaskID: "owner-task", MaterializedKind: officemodels.WorkspaceGroupKindSingleRepo}
	if err := officeRepo.CreateWorkspaceGroup(ctx, group); err != nil {
		t.Fatalf("create workspace group: %v", err)
	}

	req := taskmodels.RunnerSwitchRequest{
		TaskID:                      "task-1",
		ExecutorProfileID:           "profile-new",
		CompatibilityChecked:        true,
		CompatibilityCloneURLFound:  true,
		ResolvedRepositoryID:        stored.RepositoryID,
		ResolvedRepositoryUpdatedAt: stored.UpdatedAt,
		GroupMembershipChecker: func(ctx context.Context, taskID string) (bool, error) {
			g, err := officeRepo.GetWorkspaceGroupForTask(ctx, taskID)
			return g != nil, err
		},
	}

	var wg sync.WaitGroup
	var switchErr, memberErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, switchErr = taskRepo.SwitchTaskRunner(ctx, req)
	}()
	go func() {
		defer wg.Done()
		memberErr = officeRepo.AddWorkspaceGroupMember(ctx, group.ID, "task-1", "")
	}()
	wg.Wait()

	if memberErr != nil {
		t.Fatalf("AddWorkspaceGroupMember error = %v, want nil", memberErr)
	}

	switchRejectedAsGroupMember := false
	if switchErr != nil {
		var conflict *taskrepoerrors.ErrRunnerMutabilityConflict
		if !errors.As(switchErr, &conflict) || conflict.Reason != taskmodels.RunnerReasonWorkspaceGroupMember {
			t.Fatalf("switch error = %v, want nil or ErrRunnerMutabilityConflict{workspace_group_member}", switchErr)
		}
		switchRejectedAsGroupMember = true
	}

	task, err := taskRepo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	storedProfile, _ := task.Metadata[taskmodels.MetaKeyExecutorProfileID].(string)

	if switchRejectedAsGroupMember {
		if storedProfile == "profile-new" {
			t.Fatalf("switch was rejected but stored profile = %q, want unchanged", storedProfile)
		}
		return
	}
	if storedProfile != "profile-new" {
		t.Fatalf("switch committed but stored profile = %q, want profile-new", storedProfile)
	}
}
