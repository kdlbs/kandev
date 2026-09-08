package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
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
	grandparent := createWorkspaceSourceAuthorizationTask(t, svc, parent.WorkspaceID, "Grandparent", "")
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "grandparent-session", TaskID: grandparent.ID}))
	_, err := repo.DB().ExecContext(ctx, "UPDATE tasks SET parent_id = ? WHERE id = ?", grandparent.ID, parent.ID)
	require.NoError(t, err)
	h := &Handlers{taskSvc: svc, sessionRepo: repo, logger: testLogger(t).WithFields()}

	for _, testCase := range []struct {
		name            string
		targetTaskID    string
		callerSessionID string
	}{
		{name: "sibling", targetTaskID: sibling.ID, callerSessionID: "parent-session"},
		{name: "grandparent", targetTaskID: child.ID, callerSessionID: "grandparent-session"},
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

func TestHandleAddWorkspaceSourcesReturnsInternalErrorWhenCallerSessionLookupFails(t *testing.T) {
	h := &Handlers{
		taskSvc:     &service.Service{},
		sessionRepo: failingCallerSessionRepository{err: errors.New("database unavailable")},
	}

	response, err := h.handleAddWorkspaceSources(context.Background(), makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
		"task_id": "target-task", "caller_task_id": "caller-task", "caller_session_id": "caller-session",
		"sources": []interface{}{map[string]interface{}{"kind": "folder", "local_path": t.TempDir(), "display_name": "docs"}},
	}))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeInternalError)
}

func TestHandleAddWorkspaceSourcesReturnsInternalErrorWhenCallerTaskLookupFails(t *testing.T) {
	h := &Handlers{
		taskSvc: service.NewService(service.Repos{
			Tasks: failingCallerTaskRepository{err: errors.New("database unavailable")},
		}, nil, testLogger(t), service.RepositoryDiscoveryConfig{}),
		sessionRepo: staticCallerSessionRepository{session: &models.TaskSession{ID: "caller-session", TaskID: "caller-task"}},
	}

	response, err := h.handleAddWorkspaceSources(context.Background(), makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
		"task_id": "target-task", "caller_task_id": "caller-task", "caller_session_id": "caller-session",
		"sources": []interface{}{map[string]interface{}{"kind": "folder", "local_path": t.TempDir(), "display_name": "docs"}},
	}))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeInternalError)
}

type failingCallerSessionRepository struct{ err error }

func (r failingCallerSessionRepository) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return nil, r.err
}

func (failingCallerSessionRepository) UpdateTaskSessionState(context.Context, string, models.TaskSessionState, string) error {
	return nil
}

func (failingCallerSessionRepository) SetSessionMetadataKeyIfAbsentOrDifferentStep(context.Context, string, string, string, interface{}) (bool, error) {
	return false, nil
}

type staticCallerSessionRepository struct{ session *models.TaskSession }

func (r staticCallerSessionRepository) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return r.session, nil
}

func (staticCallerSessionRepository) UpdateTaskSessionState(context.Context, string, models.TaskSessionState, string) error {
	return nil
}

func (staticCallerSessionRepository) SetSessionMetadataKeyIfAbsentOrDifferentStep(context.Context, string, string, string, interface{}) (bool, error) {
	return false, nil
}

type failingCallerTaskRepository struct {
	taskrepository.TaskRepository
	err error
}

func (r failingCallerTaskRepository) GetTask(context.Context, string) (*models.Task, error) {
	return nil, r.err
}

func TestHandleAddWorkspaceSourcesSelfTargetSucceeds(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, child, _ := newWorkspaceSourceAuthorizationFixtureWithEventBus(t)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "child-session", TaskID: child.ID}))
	h := &Handlers{taskSvc: svc, sessionRepo: repo, logger: testLogger(t).WithFields()}

	response, err := h.handleAddWorkspaceSources(ctx, makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
		"task_id": child.ID, "caller_task_id": child.ID, "caller_session_id": "child-session",
		"sources": []interface{}{map[string]interface{}{"kind": "folder", "local_path": t.TempDir(), "display_name": "docs"}},
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)

	folders, err := repo.ListTaskWorkspaceFolders(ctx, child.ID)
	require.NoError(t, err)
	require.Len(t, folders, 1)
}

func TestHandleAddWorkspaceSourcesDenialsLeaveWorkspaceSourcesUntouched(t *testing.T) {
	ctx := context.Background()
	svc, repo, parent, child, eventBus := newWorkspaceSourceAuthorizationFixtureWithEventBus(t)
	events := observeWorkspaceSourceEvents(t, eventBus)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent-session", TaskID: parent.ID}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "child-session", TaskID: child.ID}))
	materializer := &workspaceSourceMaterializerRecorder{}
	refresher := &workspaceSourceProviderRefresherRecorder{}
	svc.SetWorkspaceSourceMaterializer(materializer)
	svc.SetWorkspaceSourceProviderRefresher(refresher)
	h := &Handlers{taskSvc: svc, sessionRepo: repo, logger: testLogger(t).WithFields()}

	otherWorkspace := &models.Workspace{ID: "workspace-sources-other", Name: "Other workspace"}
	require.NoError(t, repo.CreateWorkspace(ctx, otherWorkspace))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-sources-other", WorkspaceID: otherWorkspace.ID, Name: "Workflow"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "repository-sources-other", WorkspaceID: otherWorkspace.ID, Name: "other", DefaultBranch: "main"}))
	otherParentResult, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: otherWorkspace.ID, WorkflowID: "workflow-sources-other", Title: "Other parent",
		Repositories: []service.TaskRepositoryInput{{RepositoryID: "repository-sources-other", BaseBranch: "main"}},
	})
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "other-parent-session", TaskID: otherParentResult.Task.ID}))
	_, err = repo.DB().ExecContext(ctx, "UPDATE tasks SET parent_id = ? WHERE id = ?", otherParentResult.Task.ID, child.ID)
	require.NoError(t, err)

	for _, testCase := range []struct {
		name            string
		callerTaskID    string
		callerSessionID string
		expectedCode    string
	}{
		{name: "cross workspace direct parent", callerTaskID: otherParentResult.Task.ID, callerSessionID: "other-parent-session", expectedCode: ws.ErrorCodeForbidden},
		{name: "unknown caller session", callerTaskID: parent.ID, callerSessionID: "missing-parent-session", expectedCode: ws.ErrorCodeForbidden},
		{name: "mismatched caller session", callerTaskID: parent.ID, callerSessionID: "child-session", expectedCode: ws.ErrorCodeForbidden},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			events.reset()
			materializer.called = false
			refresher.calls = 0
			response, handlerErr := h.handleAddWorkspaceSources(ctx, makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
				"task_id": child.ID, "caller_task_id": testCase.callerTaskID, "caller_session_id": testCase.callerSessionID,
				"sources": []interface{}{map[string]interface{}{"kind": "folder", "local_path": t.TempDir(), "display_name": "docs"}},
			}))
			require.NoError(t, handlerErr)
			assertWSError(t, response, testCase.expectedCode)
			assertWorkspaceSourceStateUnchanged(t, ctx, repo, child.ID, events, materializer, refresher, 1, 0)
		})
	}
}

func TestHandleAddWorkspaceSourcesActiveChildRejectsExactRetryBeforeSideEffects(t *testing.T) {
	ctx := context.Background()
	svc, repo, parent, child, eventBus := newWorkspaceSourceAuthorizationFixtureWithEventBus(t)
	events := observeWorkspaceSourceEvents(t, eventBus)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent-session", TaskID: parent.ID}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "child-session", TaskID: child.ID}))
	h := &Handlers{taskSvc: svc, sessionRepo: repo, logger: testLogger(t).WithFields()}
	folder := t.TempDir()
	source := []interface{}{map[string]interface{}{"kind": "folder", "local_path": folder, "display_name": "docs"}}

	initial, err := h.handleAddWorkspaceSources(ctx, makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
		"task_id": child.ID, "caller_task_id": parent.ID, "caller_session_id": "parent-session", "sources": source,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, initial.Type)
	_, err = svc.StartTurn(ctx, "child-session")
	require.NoError(t, err)

	materializer := &workspaceSourceMaterializerRecorder{}
	refresher := &workspaceSourceProviderRefresherRecorder{}
	svc.SetWorkspaceSourceMaterializer(materializer)
	svc.SetWorkspaceSourceProviderRefresher(refresher)
	events.reset()
	retry, err := h.handleAddWorkspaceSources(ctx, makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
		"task_id": child.ID, "caller_task_id": parent.ID, "caller_session_id": "parent-session", "sources": source,
	}))
	require.NoError(t, err)
	assertWSError(t, retry, ws.ErrorCodeConflict)
	assertWorkspaceSourceStateUnchanged(t, ctx, repo, child.ID, events, materializer, refresher, 1, 1)
}

func TestHandleAddWorkspaceSourcesRejectsChildReparentedBeforeDurableCommit(t *testing.T) {
	ctx := context.Background()
	svc, repo, parent, child, eventBus, _ := newWorkspaceSourceAuthorizationFixtureWithReparentingStore(t)
	events := observeWorkspaceSourceEvents(t, eventBus)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent-session", TaskID: parent.ID}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "child-session", TaskID: child.ID}))
	materializer := &workspaceSourceMaterializerRecorder{}
	refresher := &workspaceSourceProviderRefresherRecorder{}
	svc.SetWorkspaceSourceMaterializer(materializer)
	svc.SetWorkspaceSourceProviderRefresher(refresher)
	h := &Handlers{taskSvc: svc, sessionRepo: repo, logger: testLogger(t).WithFields()}

	response, err := h.handleAddWorkspaceSources(ctx, makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
		"task_id": child.ID, "caller_task_id": parent.ID, "caller_session_id": "parent-session",
		"sources": []interface{}{map[string]interface{}{"kind": "folder", "local_path": t.TempDir(), "display_name": "docs"}},
	}))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeForbidden)
	assertWorkspaceSourceStateUnchanged(t, ctx, repo, child.ID, events, materializer, refresher, 1, 0)

	session, err := repo.GetTaskSession(ctx, "child-session")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCreated, session.State)
}

func TestHandleAddWorkspaceSourcesRejectsExactRetryAfterChildReparenting(t *testing.T) {
	ctx := context.Background()
	svc, repo, parent, child, eventBus, reparentingStore := newWorkspaceSourceAuthorizationFixtureWithReparentingStore(t)
	events := observeWorkspaceSourceEvents(t, eventBus)
	reparentingStore.beforeCreate = nil
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "parent-session", TaskID: parent.ID}))
	materializer := &workspaceSourceMaterializerRecorder{}
	refresher := &workspaceSourceProviderRefresherRecorder{}
	svc.SetWorkspaceSourceMaterializer(materializer)
	svc.SetWorkspaceSourceProviderRefresher(refresher)
	h := &Handlers{taskSvc: svc, sessionRepo: repo, logger: testLogger(t).WithFields()}
	source := []interface{}{map[string]interface{}{"kind": "folder", "local_path": t.TempDir(), "display_name": "docs"}}

	initial, err := h.handleAddWorkspaceSources(ctx, makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
		"task_id": child.ID, "caller_task_id": parent.ID, "caller_session_id": "parent-session", "sources": source,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, initial.Type)
	events.reset()
	materializer.called, refresher.calls = false, 0
	reparentingStore.beforeCreate = func(context.Context) error {
		_, err := repo.DB().ExecContext(ctx, "UPDATE tasks SET parent_id = '' WHERE id = ?", child.ID)
		return err
	}

	retry, err := h.handleAddWorkspaceSources(ctx, makeWSMessage(t, ws.ActionMCPAddWorkspaceSources, map[string]interface{}{
		"task_id": child.ID, "caller_task_id": parent.ID, "caller_session_id": "parent-session", "sources": source,
	}))
	require.NoError(t, err)
	assertWSError(t, retry, ws.ErrorCodeForbidden)
	assertWorkspaceSourceStateUnchanged(t, ctx, repo, child.ID, events, materializer, refresher, 1, 1)
}

type workspaceSourceMaterializerRecorder struct{ called bool }

func (m *workspaceSourceMaterializerRecorder) MaterializeWorkspaceSources(context.Context, string, *models.WorkspaceSourceBatch) (*service.WorkspaceSourceMaterializationResult, error) {
	m.called = true
	return &service.WorkspaceSourceMaterializationResult{}, nil
}

type workspaceSourceProviderRefresherRecorder struct{ calls int }

func (r *workspaceSourceProviderRefresherRecorder) RefreshTaskMCPProviders(context.Context, string) error {
	r.calls++
	return nil
}

type workspaceSourceEventObserver struct{ count int }

func observeWorkspaceSourceEvents(t *testing.T, eventBus *bus.MemoryEventBus) *workspaceSourceEventObserver {
	t.Helper()
	observer := &workspaceSourceEventObserver{}
	subscription, err := eventBus.Subscribe(">", func(context.Context, *bus.Event) error {
		observer.count++
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	return observer
}

func (o *workspaceSourceEventObserver) reset() { o.count = 0 }

func assertWorkspaceSourceStateUnchanged(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID string, events *workspaceSourceEventObserver, materializer *workspaceSourceMaterializerRecorder, refresher *workspaceSourceProviderRefresherRecorder, wantRepositories, wantFolders int) {
	t.Helper()
	repositories, err := repo.ListTaskRepositories(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, repositories, wantRepositories)
	folders, err := repo.ListTaskWorkspaceFolders(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, folders, wantFolders)
	require.Zero(t, events.count)
	require.False(t, materializer.called)
	require.Zero(t, refresher.calls)
}

func newWorkspaceSourceAuthorizationFixture(t *testing.T) (*service.Service, *sqliterepo.Repository, *models.Task, *models.Task) {
	svc, repo, parent, child, _ := newWorkspaceSourceAuthorizationFixtureWithEventBus(t)
	return svc, repo, parent, child
}

func newWorkspaceSourceAuthorizationFixtureWithEventBus(t *testing.T) (*service.Service, *sqliterepo.Repository, *models.Task, *models.Task, *bus.MemoryEventBus) {
	t.Helper()
	svc, repo, eventBus := newTestTaskServiceWithEventBus(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-sources", Name: "Workspace sources"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-sources", WorkspaceID: "workspace-sources", Name: "Workflow"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "repository-sources", WorkspaceID: "workspace-sources", Name: "app", DefaultBranch: "main"}))
	parent := createWorkspaceSourceAuthorizationTask(t, svc, "workspace-sources", "Parent", "")
	child := createWorkspaceSourceAuthorizationTask(t, svc, "workspace-sources", "Child", parent.ID)
	return svc, repo, parent, child, eventBus
}

func newWorkspaceSourceAuthorizationFixtureWithReparentingStore(t *testing.T) (*service.Service, *sqliterepo.Repository, *models.Task, *models.Task, *bus.MemoryEventBus, *reparentingWorkspaceSourceStore) {
	t.Helper()
	_, repo, eventBus := newTestTaskServiceWithEventBus(t)
	ctx := context.Background()
	store := &reparentingWorkspaceSourceStore{TaskWorkspaceFolderRepository: repo}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, WorkspaceFolders: store, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, Reviews: repo,
	}, eventBus, testLogger(t), service.RepositoryDiscoveryConfig{})
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-sources", Name: "Workspace sources"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-sources", WorkspaceID: "workspace-sources", Name: "Workflow"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "repository-sources", WorkspaceID: "workspace-sources", Name: "app", DefaultBranch: "main"}))
	parent := createWorkspaceSourceAuthorizationTask(t, svc, "workspace-sources", "Parent", "")
	child := createWorkspaceSourceAuthorizationTask(t, svc, "workspace-sources", "Child", parent.ID)
	store.beforeCreate = func(context.Context) error {
		_, err := repo.DB().ExecContext(ctx, "UPDATE tasks SET parent_id = '' WHERE id = ?", child.ID)
		return err
	}
	return svc, repo, parent, child, eventBus, store
}

type reparentingWorkspaceSourceStore struct {
	taskrepository.TaskWorkspaceFolderRepository
	beforeCreate func(context.Context) error
}

func (s *reparentingWorkspaceSourceStore) CreateWorkspaceSourceBatch(ctx context.Context, batch *models.WorkspaceSourceBatch) error {
	if s.beforeCreate != nil {
		if err := s.beforeCreate(ctx); err != nil {
			return err
		}
	}
	return s.TaskWorkspaceFolderRepository.CreateWorkspaceSourceBatch(ctx, batch)
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
