package pause_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/pause"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

func newPauseTestRouter(t *testing.T, svc *pause.Service, asAgentCaller bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if asAgentCaller {
		r.Use(func(c *gin.Context) {
			// "agent_caller" mirrors office/agents/handler.go's own
			// unexported context key literal, which is how
			// officeagents.CallerFromContext observes an agent caller.
			c.Set("agent_caller", &models.AgentInstance{ID: "agent-1"})
			c.Next()
		})
	}
	group := r.Group("/api/v1/office")
	pause.RegisterRoutes(group, pause.NewHandler(svc))
	return r
}

func doRequest(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != "" {
		reqBody = bytes.NewBufferString(body)
	} else {
		reqBody = bytes.NewBufferString("")
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestGetPause_UnknownWorkspaceReturns404 proves AC-006.9 on the read
// endpoint specifically: PauseState alone cannot distinguish an unknown
// workspace from a running one.
func TestGetPause_UnknownWorkspaceReturns404(t *testing.T) {
	svc := newTestService(&fakeRepo{}, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{}})
	r := newPauseTestRouter(t, svc, false)

	rec := doRequest(r, http.MethodGet, "/api/v1/office/workspaces/ws-missing/pause", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

// TestGetPause_RunningWorkspaceReportsNotPaused proves the ordinary read.
func TestGetPause_RunningWorkspaceReportsNotPaused(t *testing.T) {
	svc := newTestService(&fakeRepo{}, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})
	r := newPauseTestRouter(t, svc, false)

	rec := doRequest(r, http.MethodGet, "/api/v1/office/workspaces/ws-1/pause", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["paused"] != false {
		t.Fatalf("paused = %v, want false", body["paused"])
	}
	if body["pause"] != nil {
		t.Fatalf("pause = %v, want nil", body["pause"])
	}
}

// TestPostPause_AgentCallerRejected proves AC-006.11: an agent caller may
// not pause a workspace.
func TestPostPause_AgentCallerRejected(t *testing.T) {
	svc := newTestService(&fakeRepo{}, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})
	r := newPauseTestRouter(t, svc, true)

	rec := doRequest(r, http.MethodPost, "/api/v1/office/workspaces/ws-1/pause", `{"reason":"incident"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
}

// TestPostResume_AgentCallerRejected mirrors the pause case for resume.
func TestPostResume_AgentCallerRejected(t *testing.T) {
	svc := newTestService(&fakeRepo{}, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})
	r := newPauseTestRouter(t, svc, true)

	rec := doRequest(r, http.MethodPost, "/api/v1/office/workspaces/ws-1/resume", `{}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
}

// TestPostPause_MissingReasonReturns400 proves the reason validation
// surfaces as 400 over HTTP.
func TestPostPause_MissingReasonReturns400(t *testing.T) {
	svc := newTestService(&fakeRepo{}, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})
	r := newPauseTestRouter(t, svc, false)

	rec := doRequest(r, http.MethodPost, "/api/v1/office/workspaces/ws-1/pause", `{"reason":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

// TestPostPause_Success proves the happy path response shape, including
// the sweep object AC-003.7/AC-004.6 require.
func TestPostPause_Success(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})
	r := newPauseTestRouter(t, svc, false)

	rec := doRequest(r, http.MethodPost, "/api/v1/office/workspaces/ws-1/pause", `{"reason":"incident"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["paused"] != true {
		t.Fatalf("paused = %v, want true", body["paused"])
	}
	if body["sweep"] == nil {
		t.Fatal("expected a sweep object in the response")
	}
	if body["workspace_id"] != "ws-1" {
		t.Fatalf("workspace_id = %v, want ws-1", body["workspace_id"])
	}
}

// TestPostPause_ContendedReturns409WithoutReason proves the pause-
// contended 409 body carries paused:false and no reason field, distinct
// from a blocked-caller 409 (which this handler never returns).
func TestPostPause_ContendedReturns409WithoutReason(t *testing.T) {
	repo := &fakeRepo{
		createErr:   []error{officesqlite.ErrWorkspaceAlreadyPaused, officesqlite.ErrWorkspaceAlreadyPaused},
		activeReads: []*models.WorkspacePause{nil, nil},
	}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})
	r := newPauseTestRouter(t, svc, false)

	rec := doRequest(r, http.MethodPost, "/api/v1/office/workspaces/ws-1/pause", `{"reason":"incident"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["paused"] != false {
		t.Fatalf("paused = %v, want false", body["paused"])
	}
	if _, hasReason := body["reason"]; hasReason {
		t.Fatalf("body = %v, must not carry a reason field", body)
	}
}

// TestPostResume_Success proves the resume response shape.
func TestPostResume_Success(t *testing.T) {
	repo := &fakeRepo{
		activeReads:   []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}},
		releaseResult: true,
	}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})
	r := newPauseTestRouter(t, svc, false)

	rec := doRequest(r, http.MethodPost, "/api/v1/office/workspaces/ws-1/resume", `{"reason":"resolved"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["paused"] != false {
		t.Fatalf("paused = %v, want false", body["paused"])
	}
	if body["pause"] != nil {
		t.Fatalf("pause = %v, want nil", body["pause"])
	}
}

// TestPostResume_UnknownWorkspaceReturns404 proves AC-006.9 on resume.
func TestPostResume_UnknownWorkspaceReturns404(t *testing.T) {
	svc := newTestService(&fakeRepo{}, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{}})
	r := newPauseTestRouter(t, svc, false)

	rec := doRequest(r, http.MethodPost, "/api/v1/office/workspaces/ws-missing/resume", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}
