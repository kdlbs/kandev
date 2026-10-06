package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func registeredReplacementFixture(t *testing.T) (*gin.Engine, *ws.Dispatcher, *taskrepo.Repository, *[]*bus.Event) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "replacement.db"))
	require.NoError(t, err)
	sqlDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	repo, cleanup, err := repository.Provide(sqlDB, sqlDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "replacement-ws", Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "replacement-wf", WorkspaceID: "replacement-ws", Name: "Workflow"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "replacement-task", WorkspaceID: "replacement-ws", WorkflowID: "replacement-wf", Title: "Replacement", Priority: "medium"}))
	for _, id := range []string{"original", "next-a", "next-b"} {
		require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: id, WorkspaceID: "replacement-ws", Name: id, DefaultBranch: "main"}))
	}
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "original-link", TaskID: "replacement-task", RepositoryID: "original", BaseBranch: "main", Metadata: map[string]interface{}{"retain": "all columns"}}))
	log := newTestLogger(t)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	recorded := []*bus.Event{}
	_, err = eventBus.Subscribe("task.updated", func(_ context.Context, event *bus.Event) error { recorded = append(recorded, event); return nil })
	require.NoError(t, err)
	_, err = eventBus.Subscribe("task.created", func(_ context.Context, event *bus.Event) error { recorded = append(recorded, event); return nil })
	require.NoError(t, err)
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo, BranchPolicies: repo}, eventBus, log, service.RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(&reorderHandlerStepGetter{steps: map[string]*wfmodels.WorkflowStep{"replacement-step": {ID: "replacement-step", WorkflowID: "replacement-wf", Name: "Ready"}}})
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	RegisterTaskRoutes(router, dispatcher, svc, nil, repo, nil, log)
	return router, dispatcher, repo, &recorded
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.6
func TestRegisteredTaskRepositoryReplacement(t *testing.T) {
	t.Run("fresh_branch_storage_failure", func(t *testing.T) {
		router, _, repo, recorded := registeredReplacementFixture(t)
		path := initHandlerGitRepository(t, filepath.Join(canonicalTempDir(t), "fresh"))
		repository, err := repo.GetRepository(context.Background(), "original")
		require.NoError(t, err)
		repository.SourceType = "local"
		repository.LocalPath = path
		require.NoError(t, repo.UpdateRepository(context.Background(), repository))
		now := time.Now().UTC()
		_, err = repo.DB().Exec(`INSERT INTO workflow_steps (id,workflow_id,name,position,created_at,updated_at) VALUES (?,?,?,?,?,?)`, "replacement-step", "replacement-wf", "Ready", 0, now, now)
		require.NoError(t, err)
		_, err = repo.DB().Exec(`CREATE TABLE fresh_original AS SELECT * FROM task_repositories WHERE 0`)
		require.NoError(t, err)
		_, err = repo.DB().Exec(`CREATE TRIGGER remember_fresh_original AFTER INSERT ON task_repositories WHEN NEW.task_id != 'replacement-task' AND NEW.base_branch='main' BEGIN INSERT INTO fresh_original SELECT * FROM task_repositories WHERE id=NEW.id; END`)
		require.NoError(t, err)
		// Creation's original main association succeeds; only the later rewritten
		// branch association fails after Git has already checked out the new branch.
		_, err = repo.DB().Exec(`CREATE TRIGGER reject_fresh_replacement BEFORE INSERT ON task_repositories WHEN NEW.base_branch='feature/retained-git' BEGIN SELECT RAISE(ABORT,'fresh replacement rejected'); END`)
		require.NoError(t, err)
		response := doJSON(t, router, http.MethodPost, "/api/v1/tasks", map[string]interface{}{"workspace_id": "replacement-ws", "workflow_id": "replacement-wf", "workflow_step_id": "replacement-step", "title": "Fresh boundary", "repositories": []map[string]interface{}{{"repository_id": "original", "base_branch": "main", "fresh_branch": true, "new_branch_name": "feature/retained-git"}}})
		require.Equal(t, 500, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "please check the repository")
		require.Equal(t, "feature/retained-git", handlerGitCurrentBranch(t, path))
		require.Len(t, *recorded, 1)
		require.Equal(t, "task.created", (*recorded)[0].Type)
		tasks, err := repo.ListTasks(context.Background(), "replacement-wf")
		require.NoError(t, err)
		var created *models.Task
		for _, task := range tasks {
			if task.Title == "Fresh boundary" {
				created = task
			}
		}
		require.NotNil(t, created)
		rows, err := repo.ListTaskRepositories(context.Background(), created.ID)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "main", rows[0].BaseBranch)
		require.Equal(t, "original", rows[0].RepositoryID)
		var differences int
		require.NoError(t, repo.DB().QueryRow(`SELECT COUNT(*) FROM (SELECT * FROM task_repositories WHERE task_id=? EXCEPT SELECT * FROM fresh_original)`, created.ID).Scan(&differences))
		require.Zero(t, differences)
		require.NoError(t, repo.DB().QueryRow(`SELECT COUNT(*) FROM (SELECT * FROM fresh_original EXCEPT SELECT * FROM task_repositories WHERE task_id=?)`, created.ID).Scan(&differences))
		require.Zero(t, differences)
	})

	for _, transport := range []string{"REST", "WS"} {
		for _, scenario := range []string{"resolution", "second_insert", "success", "clear", "omitted", "null"} {
			t.Run(transport+"/"+scenario, func(t *testing.T) {
				router, dispatcher, repo, recorded := registeredReplacementFixture(t)
				ctx := context.Background()
				original, err := repo.ListTaskRepositories(ctx, "replacement-task")
				require.NoError(t, err)
				body := map[string]interface{}{"id": "replacement-task", "repositories": []map[string]interface{}{{"repository_id": "next-b", "base_branch": "develop"}, {"repository_id": "next-a", "base_branch": "main"}}}
				switch scenario {
				case "resolution":
					body["repositories"] = []map[string]interface{}{{"repository_id": "next-b"}, {"repository_id": "missing"}}
				case "second_insert":
					_, err = repo.DB().Exec(`CREATE TRIGGER reject_replacement BEFORE INSERT ON task_repositories WHEN NEW.repository_id='next-a' BEGIN SELECT RAISE(ABORT,'second insert rejected'); END`)
					require.NoError(t, err)
				case "clear":
					body["repositories"] = []interface{}{}
				case "omitted":
					delete(body, "repositories")
				case "null":
					body["repositories"] = nil
				}
				failed := scenario == "resolution" || scenario == "second_insert"
				payload := requestRegisteredReplacement(t, router, dispatcher, transport, scenario, body)
				actual, err := repo.ListTaskRepositories(ctx, "replacement-task")
				require.NoError(t, err)
				if failed {
					require.Equal(t, original, actual)
					require.Empty(t, *recorded)
					return
				}
				switch scenario {
				case "omitted", "null":
					require.Equal(t, original, actual)
				case "clear":
					require.Empty(t, actual)
				default:
					require.Len(t, actual, 2)
					require.Equal(t, "next-b", actual[0].RepositoryID)
					require.Equal(t, "next-a", actual[1].RepositoryID)
				}
				var task dto.TaskDTO
				require.NoError(t, json.Unmarshal(payload, &task))
				require.Len(t, task.Repositories, len(actual))
				for i, row := range actual {
					require.Equal(t, row.ID, task.Repositories[i].ID)
					require.Equal(t, row.RepositoryID, task.Repositories[i].RepositoryID)
				}
				require.Len(t, *recorded, 1)
				eventJSON, err := json.Marshal((*recorded)[0].Data)
				require.NoError(t, err)
				var projected map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(eventJSON, &projected))
				require.Contains(t, projected, "repositories")
				if scenario == "clear" {
					require.JSONEq(t, "[]", string(projected["repositories"]))
				}
				var event struct {
					Repositories []dto.TaskRepositoryDTO `json:"repositories"`
				}
				require.NoError(t, json.Unmarshal(eventJSON, &event))
				require.Len(t, event.Repositories, len(actual))
				for i, row := range actual {
					require.Equal(t, row.ID, event.Repositories[i].ID)
				}
			})
		}
	}
}

func requestRegisteredReplacement(t *testing.T, router *gin.Engine, dispatcher *ws.Dispatcher, transport, scenario string, body map[string]interface{}) []byte {
	t.Helper()
	if transport == "REST" {
		response := doJSON(t, router, http.MethodPatch, "/api/v1/tasks/replacement-task", body)
		expectedCode := http.StatusOK
		switch scenario {
		case "resolution":
			expectedCode = http.StatusNotFound
		case "second_insert":
			expectedCode = http.StatusInternalServerError
		}
		require.Equal(t, expectedCode, response.Code, response.Body.String())
		return response.Body.Bytes()
	}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	response, err := dispatcher.Dispatch(context.Background(), &ws.Message{ID: "request-replacement", Action: ws.ActionTaskUpdate, Payload: encoded})
	require.NoError(t, err)
	require.NotNil(t, response)
	switch scenario {
	case "resolution", "second_insert":
		require.Equal(t, ws.MessageTypeError, response.Type)
		var failure ws.ErrorPayload
		require.NoError(t, json.Unmarshal(response.Payload, &failure))
		require.Equal(t, ws.ErrorCodeInternalError, failure.Code)
	default:
		require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	}
	return response.Payload
}
