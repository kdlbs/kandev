package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/state"
)

// newTestUserStateStore returns an in-memory-sqlite-backed *state.UserStore.
func newTestUserStateStore(t *testing.T) *state.UserStore {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	st, err := state.NewUserStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new user state store: %v", err)
	}
	return st
}

// userStatePluginPackage builds a valid, runtime-managed plugin tar.gz
// declaring (or not) the user_state capability.
func userStatePluginPackage(t *testing.T, id string, userStateEnabled bool) *bytes.Buffer {
	t.Helper()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: 1.0.0
display_name: Test Plugin
capabilities:
  user_state: %v
runtime:
  type: binary
  executables:
    %s: server/plugin
`, id, userStateEnabled, platformKey)

	var buf bytes.Buffer
	files := map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}
	if err := pkgtartest.WritePackage(&buf, files); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buf
}

// newUserStateTestRouter wires a Service (with UserState + EventBus) behind
// a router that injects an identity from the X-Test-User-ID header
// (defaulting to "user_1"), mirroring how the real global auth middleware
// injects authn.Identity before plugin routes run.
func newUserStateTestRouter(t *testing.T) (*gin.Engine, *Service, bus.EventBus) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _, _ := newTestService(t)
	svc.SetUserState(newTestUserStateStore(t))
	memBus := bus.NewMemoryEventBus(testLogger(t))
	svc.eventBus = memBus

	router := gin.New()
	router.Use(func(c *gin.Context) {
		userID := c.GetHeader("X-Test-User-ID")
		if userID == "" {
			userID = "user_1"
		}
		authn.SetOnGin(c, authn.Identity{UserID: userID, Role: authn.RoleMember})
		c.Next()
	})
	RegisterRoutes(router, svc, nil, testLogger(t))
	return router, svc, memBus
}

func installUserStatePlugin(t *testing.T, svc *Service, id string, userStateEnabled bool) {
	t.Helper()
	if _, err := svc.Install(context.Background(), userStatePluginPackage(t, id, userStateEnabled)); err != nil {
		t.Fatalf("install %q: %v", id, err)
	}
}

func userStateReq(router *gin.Engine, method, path, userID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if userID != "" {
		req.Header.Set("X-Test-User-ID", userID)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestUserStatePUTThenGETRoundTripsForCallingUser pins AC15's positive path.
func TestUserStatePUTThenGETRoundTripsForCallingUser(t *testing.T) {
	router, svc, _ := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)

	rec := userStateReq(router, http.MethodPut, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", `{"value":"hello"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	rec = userStateReq(router, http.MethodGet, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(resp.Value) != `"hello"` {
		t.Fatalf("value = %q, want %q", resp.Value, `"hello"`)
	}
}

// TestUserStateGETFromDifferentUserReturns404 pins AC15's cross-user negative.
func TestUserStateGETFromDifferentUserReturns404(t *testing.T) {
	router, svc, _ := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)

	rec := userStateReq(router, http.MethodPut, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", `{"value":"alice's note"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	rec = userStateReq(router, http.MethodGet, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_2", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET (different user) status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

// TestUserStateWithoutCapabilityReturns403 pins AC17.
func TestUserStateWithoutCapabilityReturns403(t *testing.T) {
	router, svc, _ := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-no-user-state", false)

	rec := userStateReq(router, http.MethodPut, "/api/plugins/kandev-plugin-no-user-state/user-state/task/task_1/note", "user_1", `{"value":"x"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
}

// TestUserStateUnknownPluginReturns404 and disabled-plugin variant pin AC17's
// 404 half.
func TestUserStateUnknownPluginReturns404(t *testing.T) {
	router, _, _ := newUserStateTestRouter(t)

	rec := userStateReq(router, http.MethodGet, "/api/plugins/does-not-exist/user-state/task/task_1/note", "user_1", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserStateDisabledPluginReturns404(t *testing.T) {
	router, svc, _ := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)
	if err := svc.Disable("kandev-plugin-notes"); err != nil {
		t.Fatalf("disable: %v", err)
	}

	rec := userStateReq(router, http.MethodGet, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

// TestUserStateInvalidScopeReturns400 and TestUserStateInvalidScopeIDReturns400
// pin AC18.
func TestUserStateInvalidScopeReturns400(t *testing.T) {
	router, svc, _ := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)

	rec := userStateReq(router, http.MethodGet, "/api/plugins/kandev-plugin-notes/user-state/bogus-scope/task_1/note", "user_1", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserStateInvalidScopeIDReturns400(t *testing.T) {
	router, svc, _ := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)

	// A leading "." fails the [A-Za-z0-9] leading-char requirement.
	rec := userStateReq(router, http.MethodGet, "/api/plugins/kandev-plugin-notes/user-state/task/.bad/note", "user_1", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserStateInvalidKeyReturns400(t *testing.T) {
	router, svc, _ := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)

	rec := userStateReq(router, http.MethodPut, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/.bad-key", "user_1", `{"value":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

// TestUserStateOversizedBodyReturns413 pins AC18's body cap.
func TestUserStateOversizedBodyReturns413(t *testing.T) {
	router, svc, _ := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)

	huge := `{"value":"` + strings.Repeat("x", maxUserStateBodyBytes+1) + `"}`
	rec := userStateReq(router, http.MethodPut, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", huge)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413, body=%s", rec.Code, rec.Body.String())
	}
}

// TestUserStateListReturnsOrderedEntries pins AC27.
func TestUserStateListReturnsOrderedEntries(t *testing.T) {
	router, svc, _ := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)

	for _, key := range []string{"zeta", "alpha"} {
		rec := userStateReq(router, http.MethodPut, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/"+key, "user_1", `{"value":1}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s status = %d, body=%s", key, rec.Code, rec.Body.String())
		}
	}

	rec := userStateReq(router, http.MethodGet, "/api/plugins/kandev-plugin-notes/user-state/task/task_1", "user_1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("LIST status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Entries []state.UserStateEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Entries) != 2 || resp.Entries[0].Key != "alpha" || resp.Entries[1].Key != "zeta" {
		t.Fatalf("entries = %+v, want [alpha, zeta] order", resp.Entries)
	}
}

// TestUserStateConditionalWriteConflictReturns409 pins AC28.
func TestUserStateConditionalWriteConflictReturns409(t *testing.T) {
	router, svc, _ := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)

	rec := userStateReq(router, http.MethodPut, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", `{"value":"v1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("first PUT status = %d, body=%s", rec.Code, rec.Body.String())
	}

	stale := `{"value":"v2","ifUnmodifiedSince":"2000-01-01T00:00:00Z"}`
	rec = userStateReq(router, http.MethodPut, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", stale)
	if rec.Code != http.StatusConflict {
		t.Fatalf("conflicting PUT status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}

	rec = userStateReq(router, http.MethodGet, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", "")
	var resp struct {
		Value json.RawMessage `json:"value"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if string(resp.Value) != `"v1"` {
		t.Fatalf("value after conflicting write = %q, want unchanged \"v1\"", resp.Value)
	}
}

// TestUserStatePUTPublishesEventScopedToWriter pins AC24: a successful PUT
// publishes plugin.user-state.updated carrying the writer's user_id.
//
// user_id is deliberately readable here, on the raw published bus event —
// that is what lets UserEventBroadcaster route it under a NATS-backed
// EventBus, which marshals every event before publishing and would silently
// drop an unexported field (see pluginUserStateUpdatedEvent's doc comment).
// The "never reaches the browser" half of that contract is enforced one
// layer up, in the broadcaster itself: see
// internal/gateway/websocket/user_notifications_test.go's
// TestRegisterUserNotifications_PluginUserStatePayloadShape and its
// NATS-transport-simulating sibling.
func TestUserStatePUTPublishesEventScopedToWriter(t *testing.T) {
	router, svc, memBus := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)

	var gotPayload map[string]any
	_, err := memBus.Subscribe(events.PluginUserStateUpdated, func(_ context.Context, evt *bus.Event) error {
		raw, _ := json.Marshal(evt.Data)
		_ = json.Unmarshal(raw, &gotPayload)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	rec := userStateReq(router, http.MethodPut, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", `{"value":"hi","writerId":"tab-42"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", rec.Code, rec.Body.String())
	}

	if gotPayload["user_id"] != "user_1" {
		t.Fatalf("published event user_id = %v, want %q (must survive marshal for NATS routing)", gotPayload["user_id"], "user_1")
	}
	if gotPayload["pluginId"] != "kandev-plugin-notes" || gotPayload["scope"] != "task" ||
		gotPayload["scopeId"] != "task_1" || gotPayload["key"] != "note" || gotPayload["writerId"] != "tab-42" {
		t.Fatalf("event payload = %+v, missing expected fields", gotPayload)
	}
	if deleted, _ := gotPayload["deleted"].(bool); deleted {
		t.Fatalf("PUT event payload.deleted = true, want false/absent")
	}
}

// TestUserStateDELETEPublishesDeletedEvent covers the DELETE half of AC24.
func TestUserStateDELETEPublishesDeletedEvent(t *testing.T) {
	router, svc, memBus := newUserStateTestRouter(t)
	installUserStatePlugin(t, svc, "kandev-plugin-notes", true)

	var gotDeleted bool
	_, err := memBus.Subscribe(events.PluginUserStateUpdated, func(_ context.Context, evt *bus.Event) error {
		raw, _ := json.Marshal(evt.Data)
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		gotDeleted, _ = payload["deleted"].(bool)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	rec := userStateReq(router, http.MethodPut, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", `{"value":"hi"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", rec.Code, rec.Body.String())
	}
	rec = userStateReq(router, http.MethodDelete, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !gotDeleted {
		t.Fatalf("expected DELETE to publish an event with deleted=true")
	}

	rec = userStateReq(router, http.MethodGet, "/api/plugins/kandev-plugin-notes/user-state/task/task_1/note", "user_1", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET after delete status = %d, want 404", rec.Code)
	}
}
