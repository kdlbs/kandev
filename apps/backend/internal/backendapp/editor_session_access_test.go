package backendapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	editorhandlers "github.com/kandev/kandev/internal/editors/handlers"
	editormodels "github.com/kandev/kandev/internal/editors/models"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	usermodels "github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/pkg/api/v1"
)

const (
	editorAccessWorkspaceID = "workspace-editor-access"
	editorAccessTaskID      = "task-editor-access"
	editorAccessSessionID   = "session-editor-access"
)

type editorAccessRepository struct {
	editorstore.Repository
	editor *editormodels.Editor
}

func (r *editorAccessRepository) GetEditorByID(context.Context, string) (*editormodels.Editor, error) {
	return r.editor, nil
}

type editorAccessTaskReader struct {
	session *taskmodels.TaskSession
}

func (r *editorAccessTaskReader) GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error) {
	return r.session, nil
}

func (*editorAccessTaskReader) GetRepository(context.Context, string) (*taskmodels.Repository, error) {
	return nil, nil
}

func (*editorAccessTaskReader) GetExecutor(context.Context, string) (*taskmodels.Executor, error) {
	return nil, nil
}

type editorAccessUserSettings struct{}

func (editorAccessUserSettings) GetUserSettings(context.Context) (*usermodels.UserSettings, error) {
	return &usermodels.UserSettings{}, nil
}

func (editorAccessUserSettings) ClearDefaultEditorID(context.Context, string) error { return nil }

func seedEditorAccessSession(t *testing.T, harness bootStateTestHarness) {
	t.Helper()
	ctx := context.Background()
	if err := harness.taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{
		ID: editorAccessWorkspaceID, Name: "Editor access", OwnerID: "editor-owner",
	}); err != nil {
		t.Fatalf("create editor access workspace: %v", err)
	}
	if err := harness.taskRepo.CreateTask(ctx, &taskmodels.Task{
		ID: editorAccessTaskID, WorkspaceID: editorAccessWorkspaceID,
		Title: "Editor access", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create editor access task: %v", err)
	}
	if err := harness.taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{
		ID: editorAccessSessionID, TaskID: editorAccessTaskID,
		ExecutorProfileID: "deleted-executor-profile",
		State:             taskmodels.TaskSessionStateCompleted,
	}); err != nil {
		t.Fatalf("create editor access session: %v", err)
	}
	for _, member := range []taskmodels.WorkspaceMember{
		{WorkspaceID: editorAccessWorkspaceID, UserID: "editor-viewer", Role: "viewer", AddedBy: "editor-owner"},
		{WorkspaceID: editorAccessWorkspaceID, UserID: "editor-collaborator", Role: "collaborator", AddedBy: "editor-owner"},
	} {
		if err := harness.taskRepo.UpsertWorkspaceMember(ctx, &member); err != nil {
			t.Fatalf("add workspace member %q: %v", member.UserID, err)
		}
	}
}

func editorOpenRoute(t *testing.T, svc *editorservice.Service, identity authn.Identity) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, identity)
		c.Next()
	})
	editorhandlers.RegisterRoutes(router, editorcontroller.NewController(svc), log)
	return router
}

func TestEditorOpenRouteRequiresSessionExec(t *testing.T) {
	harness := newBootStateTestHarness(t)
	seedEditorAccessSession(t, harness)
	const editorID = "hosted-editor"
	svc := editorservice.NewService(
		&editorAccessRepository{editor: &editormodels.Editor{
			ID: editorID, Kind: "custom_hosted_url", Enabled: true,
			Config: json.RawMessage(`{"url":"https://editor.example.test"}`),
		}},
		&editorAccessTaskReader{session: &taskmodels.TaskSession{
			ID:        editorAccessSessionID,
			Worktrees: []*taskmodels.TaskEnvironmentRepo{{WorktreePath: t.TempDir()}},
		}},
		editorAccessUserSettings{},
	)
	wireTaskWorkflowCrossReferences(
		harness.taskSvc, harness.workflowSvc, svc, nil,
		&Repositories{Task: harness.taskRepo}, newTestLogger(),
	)

	cases := []struct {
		name     string
		identity authn.Identity
		status   int
	}{
		{name: "viewer", identity: authn.Identity{UserID: "editor-viewer", Role: authn.RoleMember}, status: http.StatusForbidden},
		{name: "no reach", identity: authn.Identity{UserID: "editor-outsider", Role: authn.RoleMember}, status: http.StatusNotFound},
		{name: "collaborator with deleted executor profile", identity: authn.Identity{UserID: "editor-collaborator", Role: authn.RoleMember}, status: http.StatusOK},
		{name: "owner", identity: authn.Identity{UserID: "editor-owner", Role: authn.RoleMember}, status: http.StatusOK},
		{name: "synthetic identity with deleted executor profile", identity: authn.Identity{UserID: "local", Synthetic: true}, status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := editorOpenRoute(t, svc, tc.identity)
			body := bytes.NewBufferString(`{"editor_id":"` + editorID + `","file_path":"main.go"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/"+editorAccessSessionID+"/open-editor", body)
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != tc.status {
				t.Fatalf("open editor status = %d, want %d; body: %s", resp.Code, tc.status, resp.Body.String())
			}
		})
	}
}
