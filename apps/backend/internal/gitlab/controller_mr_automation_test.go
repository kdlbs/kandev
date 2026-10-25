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
	// The switches are per-MR, so a PATCH only has somewhere to land once the
	// task has at least one linked MR.
	if err := store.UpsertTaskMR(context.Background(), newTestMR("task-1", "", "group/a", 1)); err != nil {
		t.Fatalf("seed linked MR: %v", err)
	}
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

// TestControllerPatchTaskMRAutomation_AutoFixEnabled covers AC2: PATCH with
// auto_fix_enabled returns 200 with it true, and a subsequent GET agrees.
func TestControllerPatchTaskMRAutomation_AutoFixEnabled(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	body := `{"auto_fix_enabled":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var patched TaskMRAutomationResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &patched); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !patched.AutoFixEnabled {
		t.Fatalf("auto_fix_enabled = false after PATCH, want true: %+v", patched)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/gitlab/tasks/task-1/mr-automation", nil)
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)
	var got TaskMRAutomationResponse
	if err := json.Unmarshal(getResp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if !got.AutoFixEnabled {
		t.Fatalf("GET auto_fix_enabled = false, want true: %+v", got)
	}
}

// TestControllerPatchTaskMRAutomation_RejectsUnknownAutomationField and
// TestControllerPatchTaskMRAutomation_RejectsNullAutoFixSwitch extend AC3's
// controller contract (inherited from #2125) to the new fields.
func TestControllerPatchTaskMRAutomation_RejectsUnknownAutomationField(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(`{"auto_fx_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "unknown MR automation field") {
		t.Fatalf("unexpected error body: %s", resp.Body.String())
	}
}

func TestControllerPatchTaskMRAutomation_RejectsNullAutoFixSwitch(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(`{"auto_fix_enabled":null}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

// TestControllerGetTaskMRAutomation_DefaultPromptFields covers AC4: the
// response always carries auto_fix_max_rounds, a non-empty
// effective_auto_fix_prompt, and using_default_prompt=true with no override.
func TestControllerGetTaskMRAutomation_DefaultPromptFields(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gitlab/tasks/task-1/mr-automation", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	var got TaskMRAutomationResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AutoFixMaxRounds != TaskMRAutoFixMaxRounds {
		t.Errorf("AutoFixMaxRounds = %d, want %d", got.AutoFixMaxRounds, TaskMRAutoFixMaxRounds)
	}
	if got.EffectiveAutoFixPrompt == "" {
		t.Error("EffectiveAutoFixPrompt is empty, want the embedded default")
	}
	if !got.UsingDefaultPrompt {
		t.Error("UsingDefaultPrompt = false, want true when no override is set")
	}
}

// TestControllerPatchTaskMRAutomation_PromptOverrideRoundTrip covers AC5.
func TestControllerPatchTaskMRAutomation_PromptOverrideRoundTrip(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	setOverride := `{"auto_fix_prompt_override":"custom prompt text"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(setOverride))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	var withOverride TaskMRAutomationResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &withOverride); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if withOverride.EffectiveAutoFixPrompt != "custom prompt text" {
		t.Errorf("EffectiveAutoFixPrompt = %q, want the override", withOverride.EffectiveAutoFixPrompt)
	}
	if withOverride.UsingDefaultPrompt {
		t.Error("UsingDefaultPrompt = true with an override set, want false")
	}

	clearOverride := `{"auto_fix_prompt_override":""}`
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(clearOverride))
	req2.Header.Set("Content-Type", "application/json")
	resp2 := httptest.NewRecorder()
	router.ServeHTTP(resp2, req2)
	var cleared TaskMRAutomationResponse
	if err := json.Unmarshal(resp2.Body.Bytes(), &cleared); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !cleared.UsingDefaultPrompt {
		t.Error("UsingDefaultPrompt = false after clearing the override, want true")
	}
	if cleared.EffectiveAutoFixPrompt == "custom prompt text" {
		t.Error("EffectiveAutoFixPrompt still the override after clearing it")
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

func TestControllerPatchTaskMRAutomation_RejectsUnknownField(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	body := `{"promt_on_merged":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

func TestControllerPatchTaskMRAutomation_RejectsUnknownFieldAlongsideValidOne(t *testing.T) {
	router, svc := newMRAutomationControllerFixture(t)
	// A typo'd field must not be silently dropped while a valid field in the
	// same body still applies — reject the whole request instead.
	body := `{"promt_on_merged":true,"prompt_on_closed":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	got, err := svc.GetTaskMRAutomationResponse(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("get options: %v", err)
	}
	if got.PromptOnClosed {
		t.Fatalf("expected no field applied from a rejected patch, got %+v", got)
	}
}

func TestControllerPatchTaskMRAutomation_RejectsNullSwitch(t *testing.T) {
	router, svc := newMRAutomationControllerFixture(t)
	body := `{"prompt_on_merged":null,"prompt_on_closed":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	got, err := svc.GetTaskMRAutomationResponse(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("get options: %v", err)
	}
	if got.PromptOnClosed {
		t.Fatalf("expected no field applied from a rejected patch, got %+v", got)
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

// TestControllerPatchTaskMRAutomation_ScopesToTheNamedMR covers the per-MR
// PATCH contract: a body carrying repository_id/project_path/mr_iid applies
// the switch to that MR alone.
func TestControllerPatchTaskMRAutomation_ScopesToTheNamedMR(t *testing.T) {
	router, svc := newMRAutomationControllerFixture(t)
	if err := svc.store.UpsertTaskMR(context.Background(), newTestMR("task-1", "", "group/b", 2)); err != nil {
		t.Fatalf("seed second MR: %v", err)
	}

	body := `{"repository_id":"","project_path":"group/a","mr_iid":1,"auto_merge_enabled":true}`
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
	if len(got.MROptions) != 2 {
		t.Fatalf("expected one mr_options entry per linked MR, got %+v", got.MROptions)
	}
	for _, opt := range got.MROptions {
		wantEnabled := opt.ProjectPath == "group/a"
		if opt.AutoMergeEnabled != wantEnabled {
			t.Errorf("MR %s auto_merge_enabled = %v, want %v", opt.ProjectPath, opt.AutoMergeEnabled, wantEnabled)
		}
	}
}

// TestControllerPatchTaskMRAutomation_RejectsBadMRIdentity keeps a caller
// mistake a 400 rather than a 500 — and, for a partial identity, keeps it
// from being silently reinterpreted as "apply to every linked MR".
func TestControllerPatchTaskMRAutomation_RejectsBadMRIdentity(t *testing.T) {
	router, _ := newMRAutomationControllerFixture(t)
	for name, body := range map[string]string{
		"unlinked MR":      `{"repository_id":"","project_path":"group/nope","mr_iid":9,"auto_fix_enabled":true}`,
		"partial identity": `{"project_path":"group/a","auto_fix_enabled":true}`,
		"null identity":    `{"repository_id":null,"project_path":null,"mr_iid":null,"auto_fix_enabled":true}`,
		"identity only":    `{"repository_id":"","project_path":"group/a","mr_iid":1}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-1/mr-automation", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (body = %s)", name, resp.Code, resp.Body.String())
		}
	}
}

// TestControllerPatchTaskMRAutomation_RejectsSwitchesWithNoLinkedMRs is the
// HTTP half of the zero-target rule: because the switches only exist per MR,
// a task with none has nowhere to store them, and answering 200 would report
// a write that never happened. The task-level prompt override stays accepted.
func TestControllerPatchTaskMRAutomation_RejectsSwitchesWithNoLinkedMRs(t *testing.T) {
	router, svc := newMRAutomationControllerFixture(t)
	seedTask(t, svc.store, "task-2", "ws-1")

	patch := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/gitlab/tasks/task-2/mr-automation", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}

	if resp := patch(`{"auto_merge_enabled":true}`); resp.Code != http.StatusBadRequest {
		t.Errorf("switch patch status = %d, want 400 (body = %s)", resp.Code, resp.Body.String())
	}
	if resp := patch(`{"auto_fix_prompt_override":"custom"}`); resp.Code != http.StatusOK {
		t.Errorf("prompt override status = %d, want 200 (body = %s)", resp.Code, resp.Body.String())
	}
}
