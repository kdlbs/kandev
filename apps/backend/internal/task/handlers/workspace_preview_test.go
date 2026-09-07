package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHTTPPublishWorkspacePreviewPropagatesAgentctlValidationStatuses(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			h := newWorkspacePreviewHandlers(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("source content must not escape the handler error"))
			}, true)
			rec := callWorkspacePreviewHandler(t, h, strings.NewReader(`{"path":"index.html","content":"<p>current</p>"}`))
			if rec.Code != status {
				t.Fatalf("status = %d, want agentctl status %d; body=%s", rec.Code, status, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "source content must not escape") {
				t.Fatalf("handler exposed agentctl response body: %s", rec.Body.String())
			}
		})
	}
}

func TestHTTPPublishWorkspacePreviewMapsMalformedAgentctlResponseToBadGateway(t *testing.T) {
	h := newWorkspacePreviewHandlers(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}, true)

	rec := callWorkspacePreviewHandler(t, h, strings.NewReader(`{"path":"index.html","content":"<p>current</p>"}`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHTTPPublishWorkspacePreviewRejectsMalformedAndOversizedRequests(t *testing.T) {
	h := newWorkspacePreviewHandlers(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("agentctl should not receive invalid requests")
		w.WriteHeader(http.StatusOK)
	}, true)

	malformed := callWorkspacePreviewHandler(t, h, strings.NewReader(`{"path":`))
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d, want 400", malformed.Code)
	}

	oversized := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(oversized)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-1/html-previews", strings.NewReader(`{}`))
	c.Request.ContentLength = maxWorkspacePreviewRequestBytes + 1
	c.Params = gin.Params{{Key: "id", Value: "session-1"}}
	h.httpPublishWorkspacePreview(c)
	if oversized.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d, want 413", oversized.Code)
	}
}

func TestHTTPPublishWorkspacePreviewReportsUnavailableAgentctl(t *testing.T) {
	h := newWorkspacePreviewHandlers(t, nil, false)
	rec := callWorkspacePreviewHandler(t, h, strings.NewReader(`{"path":"index.html","content":"<p>current</p>"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func callWorkspacePreviewHandler(t *testing.T, h *ProcessHandlers, body *strings.Reader) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-1/html-previews", body)
	c.Request = c.Request.WithContext(context.Background())
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "session-1"}}
	h.httpPublishWorkspacePreview(c)
	return rec
}

func newWorkspacePreviewHandlers(t *testing.T, previewHandler http.HandlerFunc, attachClient bool) *ProcessHandlers {
	t.Helper()
	agentctlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/workspace/html-previews":
			if previewHandler == nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			previewHandler(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(agentctlServer.Close)

	log := newTestLogger(t)
	lifecycleMgr := newLifecycleManager(t, log)
	execution := &lifecycle.AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "session-1"}
	if attachClient {
		execution.SetAgentCtlClientForTesting(newAgentctlClient(t, agentctlServer.URL, log))
	}
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
	return &ProcessHandlers{service: svc, lifecycleMgr: lifecycleMgr, logger: log}
}
