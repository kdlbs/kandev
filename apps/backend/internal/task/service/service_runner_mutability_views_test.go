package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// fakeWorkspaceGroupMembershipReader is a test double for
// WorkspaceGroupMembershipReader: it reports the fixed set of task IDs as
// active group members regardless of what the real office schema would say.
type fakeWorkspaceGroupMembershipReader struct {
	memberTaskIDs map[string]bool
	read          func(context.Context) (map[string]bool, error)
}

func (f fakeWorkspaceGroupMembershipReader) HasWorkspaceGroupForTask(_ context.Context, taskID string) (bool, error) {
	return f.memberTaskIDs[taskID], nil
}

func (f fakeWorkspaceGroupMembershipReader) GetActiveWorkspaceGroupTaskIDs(ctx context.Context, taskIDs []string) (map[string]bool, error) {
	if f.read != nil {
		return f.read(ctx)
	}
	out := make(map[string]bool, len(taskIDs))
	for _, id := range taskIDs {
		if f.memberTaskIDs[id] {
			out[id] = true
		}
	}
	return out, nil
}

func TestBuildRunnerMutabilityViewsPreservesNonCancellationReadDiagnostics(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		deadline  bool
		readError func(context.Context, context.CancelFunc) error
		wantWarn  int
	}{
		{
			name:     "wrapped deadline remains a warning",
			deadline: true,
			readError: func(ctx context.Context, _ context.CancelFunc) error {
				<-ctx.Done()
				return fmt.Errorf("workspace group read deadline: %w", ctx.Err())
			},
			wantWarn: 1,
		},
		{
			name: "database failure concurrent with cancellation remains a warning",
			readError: func(_ context.Context, cancel context.CancelFunc) error {
				cancel()
				return errors.New("sqlite busy")
			},
			wantWarn: 1,
		},
		{
			name: "wrapped request cancellation stays quiet",
			readError: func(_ context.Context, cancel context.CancelFunc) error {
				cancel()
				return fmt.Errorf("workspace group read: %w", context.Canceled)
			},
			wantWarn: 0,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			svc, _, _, _ := newRunnerSwitchTestService(t)
			core, observed := observer.New(zapcore.DebugLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatalf("create observer logger: %v", err)
			}
			svc.logger = log
			ctx, cancel := context.WithCancel(context.Background())
			if testCase.deadline {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(time.Second))
			}
			defer cancel()
			readCalls := 0
			svc.SetWorkspaceGroupMembershipReader(fakeWorkspaceGroupMembershipReader{
				read: func(readCtx context.Context) (map[string]bool, error) {
					readCalls++
					return nil, testCase.readError(readCtx, cancel)
				},
			})

			views := svc.BuildRunnerMutabilityViews(ctx, []*models.Task{{ID: "task-cancel-log"}})
			if _, ok := views["task-cancel-log"]; !ok {
				t.Fatal("BuildRunnerMutabilityViews omitted the requested task")
			}
			if readCalls != 1 {
				t.Fatalf("workspace group read count = %d, want 1", readCalls)
			}
			if got := observed.FilterLevelExact(zapcore.WarnLevel).Len(); got != testCase.wantWarn {
				t.Fatalf("warning count = %d, want %d", got, testCase.wantWarn)
			}
		})
	}
}

// seedRunnerMutabilityViewsTasks creates one workspace/workflow and seven
// tasks, each isolated to trigger exactly one of BuildRunnerMutabilityViews'
// six batched signals (or none, for the eligible baseline). Returns the
// tasks keyed by scenario name.
func seedRunnerMutabilityViewsTasks(t *testing.T, svc *Service, repo *sqliterepo.Repository) map[string]*models.Task {
	t.Helper()
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-mutability-views", Name: "Mutability Views WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-mutability-views", WorkspaceID: "ws-mutability-views", Name: "Board"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-mutability-views", WorkspaceID: "ws-mutability-views", Name: "repo-a", DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	newTask := func(title string, withRepo bool) *models.Task {
		req := &CreateTaskRequest{
			WorkspaceID: "ws-mutability-views", WorkflowID: "wf-mutability-views", WorkflowStepID: "step-1",
			Title: title,
		}
		if withRepo {
			req.Repositories = []TaskRepositoryInput{{RepositoryID: "repo-mutability-views", BaseBranch: "main"}}
		}
		result, err := svc.CreateTask(ctx, req)
		if err != nil {
			t.Fatalf("CreateTask(%q): %v", title, err)
		}
		return result.Task
	}

	tasks := map[string]*models.Task{
		"eligible":         newTask("eligible", true),
		"no_repository":    newTask("no repository", false),
		"session":          newTask("session exists", true),
		"environment":      newTask("environment exists", true),
		"executor_running": newTask("executor running", true),
		"workspace_folder": newTask("workspace folder attached", true),
		"group_member":     newTask("workspace group member", true),
	}

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "mv-session-1", TaskID: tasks["session"].ID, State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "mv-env-1", TaskID: tasks["environment"].ID, ExecutorType: "worktree",
		WorkspacePath: "/tmp/mv-env-1", Status: models.TaskEnvironmentStatusCreating,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	// executors_running.session_id has no foreign key to task_sessions, so
	// this signal can be seeded in isolation without also tripping the
	// higher-precedence session_exists check.
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: "mv-session-for-exec", TaskID: tasks["executor_running"].ID,
		ExecutorID: "mv-executor-1", Status: "starting",
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	if err := repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{
		TaskID: tasks["workspace_folder"].ID,
		Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{
			LocalPath: "/tmp/mv-folder", DisplayName: "folder",
		}}},
	}); err != nil {
		t.Fatalf("CreateWorkspaceSourceBatch: %v", err)
	}

	return tasks
}

// TestBuildRunnerMutabilityViewsSeedsEachSignalIndependently proves that
// BuildRunnerMutabilityViews reads all six of its batched signals — task
// repository count, session existence, environment existence, executor
// running existence, workspace folder existence, and (via a wired
// WorkspaceGroupMembershipReader) active group membership — rather than only
// being exercised incidentally through SwitchTaskRunner's in-transaction
// evaluation, which reads the same six signals through a different code
// path (direct queries, not the batch).
func TestBuildRunnerMutabilityViewsSeedsEachSignalIndependently(t *testing.T) {
	svc, _, repo, _ := newRunnerSwitchTestService(t)
	tasks := seedRunnerMutabilityViewsTasks(t, svc, repo)

	svc.SetWorkspaceGroupMembershipReader(fakeWorkspaceGroupMembershipReader{
		memberTaskIDs: map[string]bool{tasks["group_member"].ID: true},
	})

	ordered := make([]*models.Task, 0, len(tasks))
	for _, task := range tasks {
		ordered = append(ordered, task)
	}
	views := svc.BuildRunnerMutabilityViews(context.Background(), ordered)

	wantReason := map[string]string{
		"eligible":         models.RunnerReasonEligible,
		"no_repository":    models.RunnerReasonNoRepository,
		"session":          models.RunnerReasonSessionExists,
		"environment":      models.RunnerReasonEnvironmentExists,
		"executor_running": models.RunnerReasonExecutorRunning,
		"workspace_folder": models.RunnerReasonWorkspaceFolderAttached,
		"group_member":     models.RunnerReasonWorkspaceGroupMember,
	}

	for scenario, task := range tasks {
		view, ok := views[task.ID]
		if !ok {
			t.Errorf("%s: missing view for task %s", scenario, task.ID)
			continue
		}
		wantEditable := scenario == "eligible"
		if view.Editable != wantEditable {
			t.Errorf("%s: Editable = %v, want %v", scenario, view.Editable, wantEditable)
		}
		if view.Reason != wantReason[scenario] {
			t.Errorf("%s: Reason = %q, want %q", scenario, view.Reason, wantReason[scenario])
		}
	}
}
