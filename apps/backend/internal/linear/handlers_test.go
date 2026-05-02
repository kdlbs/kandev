package linear

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b , c", []string{"a", "b", "c"}}, // surrounding spaces trimmed
		{"a,,b", []string{"a", "b"}},          // empty segment dropped
		{",a,b,", []string{"a", "b"}},         // leading/trailing commas dropped
		{"   ,   ", []string{}},               // whitespace-only segments dropped
	}
	for _, tc := range cases {
		got := splitCSV(tc.in)
		// splitCSV may return a non-nil empty slice; normalise to a length
		// comparison so a `nil` vs `[]string{}` mismatch doesn't fail the test.
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCSV(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

func newTestController(t *testing.T) (*Controller, *gin.Engine, *fakeClient) {
	t.Helper()
	store := newTestStore(t)
	secrets := newFakeSecretStore()
	client := &fakeClient{}
	svc := NewService(store, secrets, func(_ *LinearConfig, _ string) Client {
		return client
	}, logger.Default())
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := &Controller{service: svc, logger: logger.Default()}
	ctrl.RegisterHTTPRoutes(router)
	return ctrl, router, client
}

func TestWriteClientError_NotConfigured_Returns503WithCode(t *testing.T) {
	ctrl, _, _ := newTestController(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctrl.writeClientError(c, ErrNotConfigured)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	body := w.Body.String()
	// errCodeLinearNotConfigured is private — assert on the wire constant.
	if !strings.Contains(body, `"code":"LINEAR_NOT_CONFIGURED"`) {
		t.Errorf("body does not surface code: %s", body)
	}
}

func TestWriteClientError_APIError_PassesThroughKnownStatuses(t *testing.T) {
	cases := []struct {
		upstream int
		want     int
	}{
		{http.StatusUnauthorized, http.StatusUnauthorized},
		{http.StatusForbidden, http.StatusForbidden},
		{http.StatusNotFound, http.StatusNotFound},
		{http.StatusBadRequest, http.StatusBadRequest},
		{http.StatusBadGateway, http.StatusInternalServerError}, // unmapped → 500
	}
	ctrl, _, _ := newTestController(t)
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctrl.writeClientError(c, &APIError{StatusCode: tc.upstream, Message: "msg"})
		if w.Code != tc.want {
			t.Errorf("upstream %d → got status %d, want %d", tc.upstream, w.Code, tc.want)
		}
	}
}

func TestWriteClientError_GenericError_Returns500(t *testing.T) {
	ctrl, _, _ := newTestController(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctrl.writeClientError(c, errors.New("boom"))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestHTTPGetConfig_NoConfig_Returns204(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestHTTPListStates_RequiresTeamKey(t *testing.T) {
	_, router, _ := newTestController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/states", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHTTPListStates_RoutesThroughService(t *testing.T) {
	ctrl, router, client := newTestController(t)
	ctx := context.Background()
	if err := ctrl.service.store.UpsertConfig(ctx, &LinearConfig{
		AuthMethod: AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := ctrl.service.secrets.Set(ctx, SecretKey, "linear", "tok"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	var seenTeam string
	client.listStatesFn = func(teamKey string) ([]LinearWorkflowState, error) {
		seenTeam = teamKey
		return []LinearWorkflowState{{ID: "s1", Name: "In Progress"}}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/linear/states?team_key=ENG", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if seenTeam != "ENG" {
		t.Errorf("team key forwarded to client = %q, want ENG", seenTeam)
	}
}

func TestRegisterRoutes_RegistersWSHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	store := newTestStore(t)
	svc := NewService(store, newFakeSecretStore(), func(_ *LinearConfig, _ string) Client {
		return &fakeClient{}
	}, logger.Default())

	RegisterRoutes(router, dispatcher, svc, logger.Default())

	for _, action := range []string{
		ws.ActionLinearConfigGet,
		ws.ActionLinearConfigSet,
		ws.ActionLinearConfigDelete,
		ws.ActionLinearConfigTest,
		ws.ActionLinearIssueGet,
		ws.ActionLinearIssueTransition,
		ws.ActionLinearTeamsList,
	} {
		if !dispatcher.HasHandler(action) {
			t.Errorf("WS action %q not registered", action)
		}
	}
}
