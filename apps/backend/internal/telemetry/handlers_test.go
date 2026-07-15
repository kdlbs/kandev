package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestRouter(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, svc)
	return router
}

func doJSON(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGetConsentDefaults(t *testing.T) {
	svc, _, _ := newTestService(t, nil, Options{})
	router := newTestRouter(t, svc)

	rec := doJSON(t, router, http.MethodGet, "/api/v1/telemetry/consent", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var state ConsentState
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if state.Status != ConsentUnasked || state.EnvDisabled || state.InstallID != "" {
		t.Fatalf("unexpected state %+v", state)
	}
}

func TestPutConsentGrantAndDeny(t *testing.T) {
	svc, _, _ := newTestService(t, nil, Options{})
	router := newTestRouter(t, svc)

	rec := doJSON(t, router, http.MethodPut, "/api/v1/telemetry/consent", `{"status":"granted"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("grant status %d: %s", rec.Code, rec.Body.String())
	}
	var state ConsentState
	_ = json.Unmarshal(rec.Body.Bytes(), &state)
	if state.Status != ConsentGranted || state.InstallID == "" {
		t.Fatalf("unexpected grant state %+v", state)
	}

	rec = doJSON(t, router, http.MethodPut, "/api/v1/telemetry/consent", `{"status":"denied"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("deny status %d", rec.Code)
	}
	state = ConsentState{} // deny omits install_id; don't inherit the grant's value
	_ = json.Unmarshal(rec.Body.Bytes(), &state)
	if state.Status != ConsentDenied || state.InstallID != "" {
		t.Fatalf("unexpected deny state %+v", state)
	}
}

func TestPutConsentRejectsInvalid(t *testing.T) {
	svc, _, _ := newTestService(t, nil, Options{})
	router := newTestRouter(t, svc)

	for _, body := range []string{`{"status":"unasked"}`, `{"status":"maybe"}`, `{}`, `not json`} {
		rec := doJSON(t, router, http.MethodPut, "/api/v1/telemetry/consent", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: expected 400, got %d", body, rec.Code)
		}
	}
}

func TestPostEventsAcceptsAllowlistedOnly(t *testing.T) {
	svc, _, sink := newTestService(t, nil, Options{})
	grantConsent(t, svc)
	svc.drainQueue()
	router := newTestRouter(t, svc)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/telemetry/events",
		`{"events":[{"name":"ui_page_viewed","properties":{"page":"settings_system"}},{"name":"evil","properties":{"page":"x"}}]}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["accepted"] != 1 {
		t.Fatalf("expected 1 accepted, got %d", resp["accepted"])
	}
	svc.flushOnce(context.Background())
	if got := len(sink.sent()); got != 1 {
		t.Fatalf("expected 1 event sent, got %d", got)
	}
}

func TestPostEventsWithoutConsentAcceptsButSendsNothing(t *testing.T) {
	svc, _, sink := newTestService(t, nil, Options{})
	router := newTestRouter(t, svc)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/telemetry/events",
		`{"events":[{"name":"ui_page_viewed","properties":{"page":"settings_system"}}]}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d", rec.Code)
	}
	svc.flushOnce(context.Background())
	if got := len(sink.sent()); got != 0 {
		t.Fatalf("expected 0 events sent without consent, got %d", got)
	}
}

func TestPostEventsRejectsMalformedJSON(t *testing.T) {
	svc, _, _ := newTestService(t, nil, Options{})
	router := newTestRouter(t, svc)
	rec := doJSON(t, router, http.MethodPost, "/api/v1/telemetry/events", `{"events": "nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
