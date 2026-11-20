package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

type httpArchiveInspectionCleaner struct{ path string }

func (c *httpArchiveInspectionCleaner) OnTaskDeleted(context.Context, string) error { return nil }
func (c *httpArchiveInspectionCleaner) GetAllByTaskID(_ context.Context, id string) ([]*worktree.Worktree, error) {
	if id != "child" {
		return nil, nil
	}
	return []*worktree.Worktree{{ID: "damaged", TaskID: id, Path: c.path}}, nil
}
func (c *httpArchiveInspectionCleaner) CaptureCleanupHeadOIDs(context.Context, []*worktree.Worktree) (map[string]string, error) {
	return nil, &worktree.CleanupInspectionError{Stage: worktree.CleanupInspectionStageCommit, Reason: worktree.CleanupInspectionReasonDeadlineExceeded, Err: context.DeadlineExceeded}
}
func (c *httpArchiveInspectionCleaner) CleanupWorktrees(context.Context, []*worktree.Worktree) error {
	return errors.New("unverified cleanup must not run")
}

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.1
func TestArchiveInspectionFailureHTTPCommitsTree(t *testing.T) {
	h, repo, _, eventBus := newCreationAttachmentHandler(t)
	svc := service.NewService(service.Repos{Tasks: repo, Sessions: repo, Executors: repo, TaskEnvironments: repo, ResourceCleanups: repo}, eventBus, newTestLogger(t), service.RepositoryDiscoveryConfig{})
	path := filepath.Join(t.TempDir(), "keep.txt")
	require.NoError(t, os.WriteFile(path, []byte("preserve"), 0600))
	svc.SetWorktreeCleanup(&httpArchiveInspectionCleaner{path: path})
	h.service = svc
	h.handoffSvc = service.NewHandoffService(repo, nil, nil, nil, nil, nil)
	h.handoffSvc.SetTaskResourceCleaner(svc)
	publisher := &archiveInspectionPublisher{}
	h.handoffSvc.SetTaskEventPublisher(publisher)
	for _, id := range []string{"root", "child"} {
		task := &models.Task{ID: id, Title: id, WorkspaceID: "ws-http-creation", WorkflowID: "wf-http-creation"}
		if id == "child" {
			task.ParentID = "root"
		}
		require.NoError(t, repo.CreateTask(t.Context(), task))
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "root"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/tasks/root?cascade=true", nil)
	h.httpArchiveTask(c)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	for _, id := range []string{"root", "child"} {
		task, err := repo.GetTask(t.Context(), id)
		require.NoError(t, err)
		require.NotNil(t, task.ArchivedAt)
	}
	require.ElementsMatch(t, []string{"root", "child"}, publisher.archived)
	require.FileExists(t, path)
	jobs, err := repo.ListTaskResourceCleanupJobs(t.Context(), "child")
	require.NoError(t, err)
	require.NotEmpty(t, jobs)
	require.Contains(t, jobs[0].ResourceSnapshot, "physical_identity_unavailable")
}

type archiveInspectionPublisher struct{ archived []string }

func (p *archiveInspectionPublisher) PublishTaskUpdated(_ context.Context, task *models.Task, _ ...string) {
	if task.ArchivedAt != nil {
		p.archived = append(p.archived, task.ID)
	}
}
func (*archiveInspectionPublisher) PublishTaskDeleted(context.Context, *models.Task) {}
