package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func newMRAutomationControllerFixture(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-1", "ws-1")
	if err := store.SaveConfigForWorkspace(context.Background(), "ws-1", &GitLabConfig{
		Host: "https://gitlab.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	secrets := &configTestSecrets{values: map[string]string{SecretKeyForWorkspace("ws-1"): "token"}}
	svc := newWorkspaceConfigService(t, store, secrets)
	mock := NewMockClient("https://gitlab.example.com")
	mock.SetUser("alice")
	svc.workspaceClientFn = func(_ context.Context, _ *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}
	router := gin.New()
	controller := NewController(svc, newTestLogger(t))
	controller.RegisterMRAutomationHTTPRoutes(router.Group("/api/v1/gitlab"))
	return router, svc
}

func TestControllerGetTaskMRAutomation_ImplicitDefault(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gitlab/tasks/task-1/mr-automation", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var got TaskMRAutomationResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TaskID != "task-1" || got.PromptOnReviewRequested || got.PromptOnMerged || got.PromptOnClosed ||
		got.ReviewReviewerUsername != "" || got.MRStates == nil {
		t.Fatalf("unexpected implicit default response: %+v", got)
	}
}

func TestControllerPatchTaskMRAutomation_SetsSingleSwitch(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	body := `{"prompt_on_merged":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var got TaskMRAutomationResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.PromptOnMerged || got.PromptOnReviewRequested || got.PromptOnClosed {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestControllerPatchTaskMRAutomation_EmptyBodyRejected(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "at least one MR automation option is required") {
		t.Fatalf("unexpected error body: %s", resp.Body.String())
	}
}

func TestControllerPatchTaskMRAutomation_RejectsTrailingContent(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	body := `{"prompt_on_merged":true}{"prompt_on_merged":false}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

func TestControllerGetTaskMRAutomation_UnknownTaskNotFound(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gitlab/tasks/does-not-exist/mr-automation", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

func TestControllerPatchTaskMRAutomation_UnknownTaskNotFound(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/does-not-exist/mr-automation",
		strings.NewReader(`{"prompt_on_merged":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

func TestControllerPatchTaskMRAutomation_RejectsLifecycleOverrides(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	for _, key := range []string{"review_prompt_override", "merged_prompt_override", "closed_prompt_override"} {
		body := `{"` + key + `":"custom"}`
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, body = %s", key, resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "lifecycle prompt overrides are not supported") {
			t.Fatalf("%s: unexpected error body: %s", key, resp.Body.String())
		}
	}
}

func TestControllerPatchTaskMRAutomation_ResolvesAndClearsReviewer(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation",
		strings.NewReader(`{"prompt_on_review_requested":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("enable status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var enabled TaskMRAutomationResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &enabled); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if enabled.ReviewReviewerUsername != "alice" {
		t.Fatalf("expected resolved reviewer, got %+v", enabled)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation",
		strings.NewReader(`{"prompt_on_review_requested":false}`))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var disabled TaskMRAutomationResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &disabled); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if disabled.ReviewReviewerUsername != "" {
		t.Fatalf("expected cleared reviewer, got %+v", disabled)
	}
}

func TestControllerPatchTaskMRAutomation_PublishesEvent(t *testing.T) {
	router, svc := newMRAutomationControllerFixture(t)
	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	svc.SetEventBus(memBus)

	received := make(chan *bus.Event, 1)
	if _, err := memBus.Subscribe(events.GitLabTaskMROptionsUpdated, func(_ context.Context, e *bus.Event) error {
		received <- e
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation",
		strings.NewReader(`{"prompt_on_closed":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}

	select {
	case e := <-received:
		payload, ok := e.Data.(*TaskMRAutomationResponse)
		if !ok || payload.TaskID != "task-1" || !payload.PromptOnClosed {
			t.Fatalf("unexpected event payload: %+v", e.Data)
		}
	default:
		t.Fatal("expected GitLabTaskMROptionsUpdated event to be published")
	}
}
