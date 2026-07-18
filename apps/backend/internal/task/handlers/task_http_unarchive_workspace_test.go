package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

type unarchiveWorkspaceRepo struct {
	mockRepository
	archived bool
}

func (r *unarchiveWorkspaceRepo) GetTask(context.Context, string) (*models.Task, error) {
	task := &models.Task{ID: "task-1", WorkspaceID: "workspace-1"}
	if r.archived {
		now := time.Now().UTC()
		task.ArchivedAt = &now
	}
	return task, nil
}

func (r *unarchiveWorkspaceRepo) UnarchiveTask(context.Context, string) (bool, error) {
	r.archived = false
	return true, nil
}

type recordingWorkspaceRestorer struct {
	calls  []string
	result storageworkspaces.WorkspaceRecovery
}

type blockingWorkspaceRestorer struct {
	contextErr chan error
}

type branchFirstWorktreeCleanup struct {
	called chan struct{}
}

func (c *branchFirstWorktreeCleanup) OnTaskDeleted(context.Context, string) error { return nil }
func (c *branchFirstWorktreeCleanup) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	close(c.called)
	return nil, nil
}
func (c *branchFirstWorktreeCleanup) BranchRecoveryStatus(context.Context, string, string) string {
	return worktree.BranchStatusMissing
}

type orderingWorkspaceRestorer struct {
	branchCalled <-chan struct{}
	wrongOrder   bool
}

func (r *orderingWorkspaceRestorer) RestoreTask(_ context.Context, taskID string) storageworkspaces.WorkspaceRecovery {
	select {
	case <-r.branchCalled:
	default:
		r.wrongOrder = true
	}
	return storageworkspaces.WorkspaceRecovery{TaskID: taskID, Status: "restored"}
}

func (r *blockingWorkspaceRestorer) RestoreTask(ctx context.Context, taskID string) storageworkspaces.WorkspaceRecovery {
	<-ctx.Done()
	r.contextErr <- ctx.Err()
	return storageworkspaces.WorkspaceRecovery{TaskID: taskID, Status: "failed", Message: ctx.Err().Error()}
}

func (r *recordingWorkspaceRestorer) RestoreTask(_ context.Context, taskID string) storageworkspaces.WorkspaceRecovery {
	r.calls = append(r.calls, taskID)
	result := r.result
	result.TaskID = taskID
	return result
}

func TestHTTPUnarchiveReportsWorkspaceRestoreFailureWithoutBlocking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &unarchiveWorkspaceRepo{archived: true}
	taskSvc := service.NewService(service.Repos{}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})
	handler := &TaskHandlers{
		service: taskSvc, handoffSvc: service.NewHandoffService(repo, nil, nil, nil, nil, nil),
		logger: newTestLogger(t),
	}
	restorer := &recordingWorkspaceRestorer{result: storageworkspaces.WorkspaceRecovery{
		Status: "failed", Message: "restore path conflict",
	}}
	handler.SetWorkspaceQuarantineRestorer(restorer)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/api/v1/tasks/task-1/unarchive", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "task-1"}}

	handler.httpUnarchiveTask(ctx)

	if recorder.Code != 200 || repo.archived {
		t.Fatalf("unarchive response code=%d archived=%v body=%s", recorder.Code, repo.archived, recorder.Body.String())
	}
	if len(restorer.calls) != 1 || restorer.calls[0] != "task-1" {
		t.Fatalf("restore calls = %v", restorer.calls)
	}
	var response struct {
		WorkspaceRecovery []storageworkspaces.WorkspaceRecovery `json:"workspace_recovery"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.WorkspaceRecovery) != 1 || response.WorkspaceRecovery[0].Status != "failed" {
		t.Fatalf("workspace recovery response = %#v", response.WorkspaceRecovery)
	}
}

func TestHTTPUnarchiveBoundsDetachedWorkspaceRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &unarchiveWorkspaceRepo{archived: true}
	taskSvc := service.NewService(service.Repos{}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})
	handler := &TaskHandlers{
		service: taskSvc, handoffSvc: service.NewHandoffService(repo, nil, nil, nil, nil, nil),
		logger: newTestLogger(t), unarchiveRecoveryTimeout: 20 * time.Millisecond,
	}
	restorer := &blockingWorkspaceRestorer{contextErr: make(chan error, 1)}
	handler.SetWorkspaceQuarantineRestorer(restorer)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/api/v1/tasks/task-1/unarchive", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "task-1"}}

	done := make(chan struct{})
	go func() {
		handler.httpUnarchiveTask(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("unarchive remained blocked after detached recovery timeout")
	}
	if recorder.Code != 200 || repo.archived {
		t.Fatalf("unarchive response code=%d archived=%v body=%s", recorder.Code, repo.archived, recorder.Body.String())
	}
	select {
	case err := <-restorer.contextErr:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocking restorer context error = %v", err)
		}
	default:
		t.Fatal("blocking restorer did not observe bounded context cancellation")
	}
}

func TestHTTPUnarchiveRecoversBranchesBeforeWorkspaceIO(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &unarchiveWorkspaceRepo{archived: true}
	taskSvc := service.NewService(service.Repos{}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})
	branchCleanup := &branchFirstWorktreeCleanup{called: make(chan struct{})}
	taskSvc.SetWorktreeCleanup(branchCleanup)
	restorer := &orderingWorkspaceRestorer{branchCalled: branchCleanup.called}
	handler := &TaskHandlers{
		service: taskSvc, handoffSvc: service.NewHandoffService(repo, nil, nil, nil, nil, nil),
		logger: newTestLogger(t),
	}
	handler.SetWorkspaceQuarantineRestorer(restorer)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/api/v1/tasks/task-1/unarchive", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "task-1"}}

	handler.httpUnarchiveTask(ctx)

	if restorer.wrongOrder {
		t.Fatal("workspace recovery ran before branch recovery")
	}
}
