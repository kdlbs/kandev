package backendapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestration/personas"
	orchestrationstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	runstore "github.com/kandev/kandev/internal/runs/repository/sqlite"
	runservice "github.com/kandev/kandev/internal/runs/service"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceBrokerNativeLifecycle(t *testing.T) {
	a, svc := newOfficeTaskAdapterHarness(t)
	router, token, run := nativeWorkspaceRuntime(t, a)
	ctx := context.Background()
	wf, err := svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{WorkspaceID: "ws-1", Name: "Synthetic delivery"})
	require.NoError(t, err)
	for i, id := range []string{"backlog", "progress", "done"} {
		require.NoError(t, a.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: id, WorkflowID: wf.ID, Name: id, Position: i, AllowManualMove: true}))
	}
	call := func(path string, body map[string]any) *httptest.ResponseRecorder {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		request := httptest.NewRequest("POST", "/api/v1/orchestration/runtime/"+path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-Kandev-Run-Id", run)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	response := call("tasks", map[string]any{"title": "Synthetic broker lifecycle", "workflow_id": wf.ID, "execution_mode": "execute"})
	require.Equal(t, 201, response.Code, response.Body.String())
	var created map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	id := created["id"]
	require.NotEmpty(t, id)
	path := "tasks/" + id + "/manage"
	for _, body := range []map[string]any{
		{"action": "edit", "title": "Updated synthetic task", "description": "Generic example only", "priority": "high"},
		{"action": "move", "workflow_step_id": "progress"},
	} {
		response = call(path, body)
		require.Equal(t, 200, response.Code, response.Body.String())
	}
	task, err := svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "Updated synthetic task", task.Title)
	require.Equal(t, "progress", task.WorkflowStepID)
	response = call("tasks/"+id+"/status", map[string]any{"status": "in_review"})
	require.Equal(t, 200, response.Code, response.Body.String())
	task, err = svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.EqualValues(t, "REVIEW", task.State)
	response = call(path, map[string]any{"action": "archive"})
	require.Equal(t, 200, response.Code, response.Body.String())
	task, err = svc.GetTask(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, task.ArchivedAt)
	response = call(path, map[string]any{"action": "delete"})
	require.Equal(t, 200, response.Code, response.Body.String())
	_, err = svc.GetTask(ctx, id)
	require.Error(t, err)
}

func nativeWorkspaceRuntime(t *testing.T, a *taskCreatorAdapter) (*gin.Engine, string, string) {
	t.Helper()
	ctx := context.Background()
	db := sqlx.NewDb(a.taskRepo.DB(), "sqlite3")
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	profiles, _, err := settingsstore.Provide(db, db, log)
	require.NoError(t, err)
	require.NoError(t, profiles.CreateAgent(ctx, &settings.Agent{ID: "claude", Name: "Claude"}))
	require.NoError(t, profiles.CreateAgentProfile(ctx, &settings.AgentProfile{ID: "personal", AgentID: "claude", Name: "Personal", Model: "default"}))
	profile := &settings.AgentProfile{ID: "chief", AgentID: "claude", WorkspaceID: "ws-1", Role: settings.AgentRoleAssistant, Status: settings.AgentStatusIdle, Name: "Orchestrator", Settings: `{"routing":{"execution_profile_id":"personal"}}`, ExecutorPreference: `{"executor_profile_id":"local"}`}
	require.NoError(t, profiles.CreateAgentProfile(ctx, profile))
	repo := orchestrationstore.New(db, db)
	require.NoError(t, repo.Migrate())
	require.NoError(t, repo.RegisterOrchestrator(ctx, profile.ID, "ws-1", "chief-of-staff"))
	conversation, err := repo.EnsureAgentConversation(ctx, profile)
	require.NoError(t, err)
	runs := runstore.NewWithDB(db, db)
	require.NoError(t, runs.Migrate())
	s := &orchestrationruntime.Service{AssistantEnabled: true, Repo: repo, Personas: &personas.Service{Profiles: profiles, Repo: repo}, Runs: runs, Queue: runservice.New(runs, nil, log, nil), Auth: runtimeauth.NewAgentAuth(""), Tasks: a.taskSvc, Manager: a}
	s.UpdateStatus = func(ctx context.Context, ws, id, status string) error {
		return updateOrchestratedStatus(ctx, a.taskSvc, &Repositories{Workflow: a.workflow}, ws, id, status)
	}
	require.NoError(t, s.QueueTurn(ctx, "chief", conversation.TaskID, "task_comment", "synthetic-control", nil))
	run, err := runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, "session"))
	token, err := s.Auth.MintRuntimeJWT("chief", conversation.TaskID, "ws-1", run.ID, "session", "workspace_coordinator")
	require.NoError(t, err)
	router := gin.New()
	orchestrationruntime.RegisterRoutes(router.Group("/api/v1/orchestration", runtimeauth.Middleware(s.Auth, s.Personas)), &orchestrationruntime.Handler{Service: s})
	return router, token, run.ID
}
