package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type authorizationSessionFallback struct{ repository.SessionRepository }

func newSessionSummaryAuthorizationHandlers(t *testing.T, narrow bool) *TaskHandlers {
	t.Helper()
	database, _ := workflowHandlerTemplate.Open(t)
	repo := sqliterepo.NewWithInitializedDB(database, database, nil)
	ctx := t.Context()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "private-ws", OwnerID: "owner", OrgID: "owner-org", Name: "Private"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "private-task", WorkspaceID: "private-ws", Title: "Private task"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "private-session", TaskID: "private-task", State: models.TaskSessionStateCreated}); err != nil {
		t.Fatal(err)
	}
	var sessions repository.SessionRepository = repo
	if !narrow {
		sessions = authorizationSessionFallback{SessionRepository: repo}
	}
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo,
		Sessions: sessions, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, repo: repo, logger: log}
}

func TestTaskSessionSummaryTransportsPreserveAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, narrow := range []bool{true, false} {
		name := "narrow"
		if !narrow {
			name = "fallback"
		}
		t.Run(name, func(t *testing.T) {
			h := newSessionSummaryAuthorizationHandlers(t, narrow)
			for _, tc := range []struct {
				name, userID, taskID, orgID string
				allowed                     bool
			}{
				{"foreign private workspace", "outsider", "private-task", "owner-org", false},
				{"missing task", "owner", "missing-task", "owner-org", false},
				{"owner", "owner", "private-task", "owner-org", true},
				{"owner from another organization", "owner", "private-task", "foreign-org", false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: tc.userID, Role: authn.RoleMember, OrgID: tc.orgID})
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+tc.taskID+"/sessions", nil).WithContext(ctx)
					c.Params = gin.Params{{Key: "id", Value: tc.taskID}}
					h.httpListTaskSessions(c)
					wantHTTP := http.StatusNotFound
					if tc.allowed {
						wantHTTP = http.StatusOK
					}
					if recorder.Code != wantHTTP {
						t.Errorf("HTTP status=%d want=%d body=%s", recorder.Code, wantHTTP, recorder.Body.String())
					}
					payload, err := json.Marshal(map[string]string{"task_id": tc.taskID})
					if err != nil {
						t.Fatal(err)
					}
					response, err := h.wsListTaskSessions(ctx, &ws.Message{ID: "list", Action: ws.ActionTaskSessionList, Payload: payload})
					if err != nil {
						t.Fatal(err)
					}
					if tc.allowed == (response.Type == ws.MessageTypeError) {
						t.Errorf("WS authorization allowed=%v type=%s payload=%s", tc.allowed, response.Type, response.Payload)
					}
					if tc.allowed {
						var result struct {
							Sessions []struct {
								ID string `json:"id"`
							} `json:"sessions"`
						}
						if err := json.Unmarshal(response.Payload, &result); err != nil {
							t.Fatal(err)
						}
						if len(result.Sessions) != 1 || result.Sessions[0].ID != "private-session" {
							t.Fatalf("authorized WS did not return owned session: %s", response.Payload)
						}
					}
				})
			}
		})
	}
}
