package handlers

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type observedCreationReader struct {
	hierarchy.TaskHierarchyReader
	cancelRead bool
	readError  error
}

func (r *observedCreationReader) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if r.cancelRead {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		ctx = cancelled
	}
	task, err := r.TaskHierarchyReader.GetTask(ctx, id)
	r.readError = err
	return task, err
}

type observedCreationRepo struct {
	*taskrepo.Repository
	cancelRead      bool
	readError       error
	validationError error
	beforeRead      func(context.Context, string)
	beforeRows      []map[string]interface{}
}

func (r *observedCreationRepo) ValidateTaskCreationParent(ctx context.Context, workspace, parent string, validate hierarchy.TaskParentValidator) error {
	r.beforeRead(ctx, parent)
	return r.Repository.ValidateTaskCreationParent(ctx, workspace, parent, func(ctx context.Context, reader hierarchy.TaskHierarchyReader, task *models.Task, parent string) error {
		observed := &observedCreationReader{TaskHierarchyReader: reader, cancelRead: r.cancelRead}
		err := validate(ctx, observed, task, parent)
		r.readError, r.validationError = observed.readError, err
		return err
	})
}

func registeredCreationReadFixture(t *testing.T, state string) (*ws.Dispatcher, *observedCreationRepo, chan *bus.Event) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	_, h, repo := newQueuedTaskDTOBuilder(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "root", WorkspaceID: "ws-1", Title: "Root"}))
	target := &models.Task{ID: "target", WorkspaceID: "ws-1", Title: "Target", Metadata: map[string]interface{}{"keep": "original"}}
	if state == "depth" {
		target.ParentID = "root"
	}
	require.NoError(t, repo.CreateTask(ctx, target))
	eventBus := bus.NewMemoryEventBus(h.logger)
	t.Cleanup(eventBus.Close)
	published := make(chan *bus.Event, 16)
	for _, event := range []string{events.TaskCreated, events.TaskUpdated} {
		_, err := eventBus.Subscribe(event, func(_ context.Context, e *bus.Event) error { published <- e; return nil })
		require.NoError(t, err)
	}
	observed := &observedCreationRepo{Repository: repo, cancelRead: state == "cancelled"}
	// The earlier project-inheritance read remains real. Change the persisted
	// target before admission so its transaction reader observes the failure.
	observed.beforeRead = func(ctx context.Context, parent string) {
		switch state {
		case "decode":
			_, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET workflow_agent_overrides='{' WHERE id=?`, parent)
			require.NoError(t, err)
		case "missing":
			require.NoError(t, repo.DeleteTask(ctx, parent))
		}
		observed.beforeRows = creationTaskRows(t, repo)
	}
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: observed, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo}, eventBus, h.logger, service.RepositoryDiscoveryConfig{})
	dispatcher := ws.NewDispatcher()
	RegisterTaskRoutes(gin.New(), dispatcher, svc, nil, repo, nil, h.logger)
	return dispatcher, observed, published
}

func creationTaskRows(t *testing.T, repo *taskrepo.Repository) []map[string]interface{} {
	t.Helper()
	rows, err := sqlx.NewDb(repo.DB(), "sqlite3").Queryx(`SELECT * FROM tasks ORDER BY id`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	var result []map[string]interface{}
	for rows.Next() {
		row := make(map[string]interface{})
		require.NoError(t, rows.MapScan(row))
		result = append(result, row)
	}
	require.NoError(t, rows.Err())
	return result
}

func TestTaskHierarchyAdmissionRegisteredWSCreationReadErrors(t *testing.T) {
	for _, state := range []string{"decode", "cancelled", "missing", "depth"} {
		t.Run(state, func(t *testing.T) {
			dispatcher, observed, published := registeredCreationReadFixture(t, state)
			parent := "target"
			request, err := ws.NewRequest("create", ws.ActionTaskCreate, map[string]interface{}{"workspace_id": "ws-1", "workflow_id": "wf-1", "title": "Requested", "parent_id": parent})
			require.NoError(t, err)
			response, err := dispatcher.Dispatch(context.Background(), request)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)
			var failure ws.ErrorPayload
			require.NoError(t, response.ParsePayload(&failure))
			switch state {
			case "decode":
				require.ErrorIs(t, observed.validationError, models.ErrMalformedWorkflowAgentOverrides)
				require.ErrorIs(t, observed.validationError, observed.readError)
				require.Equal(t, ws.ErrorCodeInternalError, failure.Code)
			case "cancelled":
				require.ErrorIs(t, observed.readError, context.Canceled)
				require.ErrorIs(t, observed.validationError, observed.readError)
				require.Equal(t, ws.ErrorCodeInternalError, failure.Code)
			case "missing":
				require.ErrorIs(t, observed.validationError, repoerrors.ErrTaskNotFound)
				require.NotErrorIs(t, observed.validationError, service.ErrInvalidParent)
				require.Equal(t, "invalid parent_id: "+observed.readError.Error(), observed.validationError.Error())
				require.Equal(t, ws.ErrorCodeValidation, failure.Code)
			case "depth":
				require.ErrorIs(t, observed.validationError, service.ErrSubtaskDepthExceeded)
				require.NotErrorIs(t, observed.validationError, service.ErrInvalidParent)
			}
			require.NotEmpty(t, observed.beforeRows)
			require.Equal(t, observed.beforeRows, creationTaskRows(t, observed.Repository))
			require.Empty(t, published)
		})
	}
}
