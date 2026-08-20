package handlers

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRegisterHandlers_LogsMeasuredRegistrationDelta(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}

	dispatcher := ws.NewDispatcher()
	before := dispatcher.HandlerCount()
	(&Handlers{logger: log}).RegisterHandlers(dispatcher)
	delta := dispatcher.HandlerCount() - before

	entries := logs.FilterMessage("registered MCP handlers").All()
	if len(entries) != 1 {
		t.Fatalf("registration log entries = %d, want 1", len(entries))
	}
	count, err := strconv.Atoi(fmt.Sprint(entries[0].ContextMap()["count"]))
	if err != nil {
		t.Fatalf("registration log count field = %#v, want numeric value: %v", entries[0].ContextMap()["count"], err)
	}
	if count != delta {
		t.Fatalf("logged handler delta = %d, measured dispatcher delta = %d", count, delta)
	}
}

func TestHandleAddWorkspaceSourcesDirectParentAuthorizesAndAudits(t *testing.T) {
	ctx := context.Background()
	svc, repo, parent, child := newWorkspaceSourceAuthorizationFixture(t)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent-session", TaskID: parent.ID}))

	core, observed := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	h := &Handlers{taskSvc: svc, sessionRepo: repo, logger: log.WithFields()}
	folder := t.TempDir()

	response, err := h.handleAddWorkspaceSources(ctx, makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
		"task_id": child.ID, "caller_task_id": parent.ID, "caller_session_id": "parent-session",
		"sources": []interface{}{map[string]interface{}{"kind": "folder", "local_path": folder, "display_name": "docs"}},
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)

	folders, err := repo.ListTaskWorkspaceFolders(ctx, child.ID)
	require.NoError(t, err)
	require.Len(t, folders, 1)
	require.Equal(t, "docs", folders[0].DisplayName)

	entries := observed.FilterMessage("workspace sources attached through task MCP").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, parent.ID, fields["caller_task_id"])
	require.Equal(t, "parent-session", fields["caller_session_id"])
	require.Equal(t, child.ID, fields["target_task_id"])
	require.Equal(t, int64(1), fields["requested_source_count"])
	require.Equal(t, true, fields["durable_state_changed"])
	require.NotContains(t, fmt.Sprint(fields), folder)
}

func TestHandleAddWorkspaceSourcesRejectsUnrelatedAndMismatchedCallers(t *testing.T) {
	ctx := context.Background()
	svc, repo, parent, child := newWorkspaceSourceAuthorizationFixture(t)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent-session", TaskID: parent.ID}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "child-session", TaskID: child.ID}))
	sibling := createWorkspaceSourceAuthorizationTask(t, svc, parent.WorkspaceID, "Sibling", "")
	h := &Handlers{taskSvc: svc, sessionRepo: repo, logger: testLogger(t).WithFields()}

	for _, testCase := range []struct {
		name            string
		targetTaskID    string
		callerSessionID string
	}{
		{name: "sibling", targetTaskID: sibling.ID, callerSessionID: "parent-session"},
		{name: "mismatched session", targetTaskID: child.ID, callerSessionID: "child-session"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response, err := h.handleAddWorkspaceSources(ctx, makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
				"task_id": testCase.targetTaskID, "caller_task_id": parent.ID, "caller_session_id": testCase.callerSessionID,
				"sources": []interface{}{map[string]interface{}{"kind": "folder", "local_path": t.TempDir(), "display_name": "docs"}},
			}))
			require.NoError(t, err)
			assertWSError(t, response, ws.ErrorCodeForbidden)

			folders, listErr := repo.ListTaskWorkspaceFolders(ctx, testCase.targetTaskID)
			require.NoError(t, listErr)
			require.Empty(t, folders)
		})
	}
}

func newWorkspaceSourceAuthorizationFixture(t *testing.T) (*service.Service, *sqliterepo.Repository, *models.Task, *models.Task) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-sources", Name: "Workspace sources"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-sources", WorkspaceID: "workspace-sources", Name: "Workflow"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "repository-sources", WorkspaceID: "workspace-sources", Name: "app", DefaultBranch: "main"}))
	parent := createWorkspaceSourceAuthorizationTask(t, svc, "workspace-sources", "Parent", "")
	child := createWorkspaceSourceAuthorizationTask(t, svc, "workspace-sources", "Child", parent.ID)
	return svc, repo, parent, child
}

func createWorkspaceSourceAuthorizationTask(t *testing.T, svc *service.Service, workspaceID, title, parentID string) *models.Task {
	t.Helper()
	result, err := svc.CreateTask(context.Background(), &service.CreateTaskRequest{
		WorkspaceID:  workspaceID,
		WorkflowID:   "workflow-sources",
		Title:        title,
		ParentID:     parentID,
		Repositories: []service.TaskRepositoryInput{{RepositoryID: "repository-sources", BaseBranch: "main"}},
	})
	require.NoError(t, err)
	return result.Task
}
