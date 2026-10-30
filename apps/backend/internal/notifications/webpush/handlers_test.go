package webpush

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

type ensurerSpy struct{ users []string }

func (e *ensurerSpy) EnsureWebPushProvider(_ context.Context, userID string) error {
	e.users = append(e.users, userID)
	return nil
}

func newTestRouter(t *testing.T, sender Sender) (*gin.Engine, *ensurerSpy) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, _ := newTestService(t, sender)
	log, _ := logger.NewFromZap(zap.NewNop())
	spy := &ensurerSpy{}
	router := gin.New()
	RegisterRoutes(router, svc, spy, func(context.Context) string { return "u1" }, log)
	return router, spy
}

func do(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPublicKeyEndpoint(t *testing.T) {
	router, _ := newTestRouter(t, &fakeSender{})
	rec := do(router, http.MethodGet, "/api/v1/push/public-key", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"public_key":"`) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestSubscribeEndpointStoresDeviceAndEnsuresProvider(t *testing.T) {
	router, spy := newTestRouter(t, &fakeSender{})
	body := `{"endpoint":"https://fcm.googleapis.com/a","keys":{"p256dh":"k","auth":"a"}}`
	if rec := do(router, http.MethodPost, "/api/v1/push/subscriptions", body); rec.Code != http.StatusOK {
		t.Fatalf("subscribe status %d: %s", rec.Code, rec.Body.String())
	}
	if len(spy.users) != 1 || spy.users[0] != "u1" {
		t.Fatalf("provider ensured for %v, want [u1]", spy.users)
	}
	rec := do(router, http.MethodGet, "/api/v1/push/subscriptions", "")
	if !strings.Contains(rec.Body.String(), "https://fcm.googleapis.com/a") {
		t.Fatalf("list = %s", rec.Body.String())
	}
}

func TestSubscribeEndpointRejectsBadInput(t *testing.T) {
	router, spy := newTestRouter(t, &fakeSender{})
	for name, body := range map[string]string{
		"not json":      `nope`,
		"http endpoint": `{"endpoint":"http://x/a","keys":{"p256dh":"k","auth":"a"}}`,
		"missing keys":  `{"endpoint":"https://fcm.googleapis.com/a"}`,
	} {
		if rec := do(router, http.MethodPost, "/api/v1/push/subscriptions", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
	if len(spy.users) != 0 {
		t.Fatal("a rejected subscription must not create a provider")
	}
}

func TestUnsubscribeEndpoint(t *testing.T) {
	router, _ := newTestRouter(t, &fakeSender{})
	const body = `{"endpoint":"https://fcm.googleapis.com/a","keys":{"p256dh":"k","auth":"a"}}`
	do(router, http.MethodPost, "/api/v1/push/subscriptions", body)

	if rec := do(router, http.MethodDelete, "/api/v1/push/subscriptions", `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("missing endpoint: status %d, want 400", rec.Code)
	}
	if rec := do(router, http.MethodDelete, "/api/v1/push/subscriptions", `{"endpoint":"https://fcm.googleapis.com/a"}`); rec.Code != http.StatusOK {
		t.Errorf("unsubscribe: status %d, want 200", rec.Code)
	}
	if rec := do(router, http.MethodDelete, "/api/v1/push/subscriptions", `{"endpoint":"https://fcm.googleapis.com/a"}`); rec.Code != http.StatusNotFound {
		t.Errorf("repeat unsubscribe: status %d, want 404", rec.Code)
	}
}

func TestTestEndpointReportsDeliveryOutcome(t *testing.T) {
	sender := &fakeSender{}
	router, _ := newTestRouter(t, sender)
	if rec := do(router, http.MethodPost, "/api/v1/push/test", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("no devices: status %d, want 404", rec.Code)
	}
	do(router, http.MethodPost, "/api/v1/push/subscriptions", `{"endpoint":"https://fcm.googleapis.com/a","keys":{"p256dh":"k","auth":"a"}}`)
	rec := do(router, http.MethodPost, "/api/v1/push/test", "")
	if rec.Code != http.StatusOK || len(sender.sent) != 1 {
		t.Fatalf("status %d, sent %d", rec.Code, len(sender.sent))
	}
	var payload Payload
	if err := json.Unmarshal(sender.sent[0].body, &payload); err != nil {
		t.Fatalf("decode test payload: %v", err)
	}
	if payload.URL != "/settings/preferences/notifications" {
		t.Fatalf("test notification URL = %q", payload.URL)
	}
}
