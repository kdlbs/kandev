package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

func TestHTTPPublishWorkspacePreviewAuthorizesReadySessionAndForwardsBuffer(t *testing.T) {
	var received agentctlclient.WorkspacePreviewRequest
	agentctlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/workspace/html-previews":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Errorf("decode forwarded preview request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(agentctlclient.WorkspacePreviewResponse{
				Port: 43127, Path: "/site/index.html", Version: 3,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(agentctlServer.Close)

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	lifecycleMgr := lifecycle.NewManager(
		registry.NewRegistry(log), nil, nil, nil, nil, nil,
		lifecycle.ExecutorFallbackWarn, "", log,
	)
	execution := &lifecycle.AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "session-1"}
	parsedClient := newAgentctlClient(t, agentctlServer.URL, log)
	execution.SetAgentCtlClientForTesting(parsedClient)
	if err := lifecycleMgr.ExecutionStoreForTesting().Add(execution); err != nil {
		t.Fatal(err)
	}

	repo := &mockRepository{sessions: map[string]*models.TaskSession{
		"session-1": {ID: "session-1", TaskID: "task-1"},
	}}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &ProcessHandlers{service: svc, lifecycleMgr: lifecycleMgr, logger: log}

	body, err := json.Marshal(map[string]string{
		"repo":    "frontend",
		"path":    "site/index.html",
		"content": "<body>unsaved</body>",
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-1/html-previews", bytes.NewReader(body))
	c.Request = c.Request.WithContext(context.Background())
	c.Params = gin.Params{{Key: "id", Value: "session-1"}}

	h.httpPublishWorkspacePreview(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response agentctlclient.WorkspacePreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Port != 43127 || response.Path != "/site/index.html" || response.Version != 3 {
		t.Fatalf("response = %+v, want forwarded response", response)
	}
	if received.Repo != "frontend" || received.Path != "site/index.html" || received.Content != "<body>unsaved</body>" {
		t.Fatalf("forwarded request = %+v, want current buffer", received)
	}
}
