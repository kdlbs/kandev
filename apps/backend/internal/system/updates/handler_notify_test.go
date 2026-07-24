package updates

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

func newNotifyRouter(svc *Service) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1/system")
	api.GET("/updates/notification-settings", HandleGetNotifySettings(svc))
	api.PUT("/updates/notification-settings", HandleSaveNotifySettings(svc))
	return r
}

func newServiceWithNotifyStore(t *testing.T) *Service {
	t.Helper()
	pool := newTestPool(t)
	notifyStore := newTestNotifyStore(t)
	return NewService(pool, "v1.0.0", nil, logger.Default(), WithNotifyStore(notifyStore))
}

func TestHandleGetNotifySettings_ReturnsDefaults(t *testing.T) {
	svc := newServiceWithNotifyStore(t)
	r := newNotifyRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/updates/notification-settings", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got NotifySettings
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != DefaultNotifySettings() {
		t.Errorf("got %+v, want default %+v", got, DefaultNotifySettings())
	}
}

func TestHandleSaveNotifySettings_PersistsAndReturnsNormalized(t *testing.T) {
	svc := newServiceWithNotifyStore(t)
	r := newNotifyRouter(svc)

	body, _ := json.Marshal(NotifySettings{Enabled: false, Channel: NotifyChannelInView})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/updates/notification-settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got NotifySettings
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Enabled || got.Channel != NotifyChannelInView {
		t.Errorf("got %+v", got)
	}

	// Persisted: a fresh GET reflects the save.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/system/updates/notification-settings", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	var got2 NotifySettings
	if err := json.Unmarshal(w2.Body.Bytes(), &got2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got2 != got {
		t.Errorf("GET after save = %+v, want %+v", got2, got)
	}
}

func TestHandleSaveNotifySettings_InvalidChannelReturns400(t *testing.T) {
	svc := newServiceWithNotifyStore(t)
	r := newNotifyRouter(svc)

	body, _ := json.Marshal(map[string]string{"channel": "smoke-signal"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/updates/notification-settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleSaveNotifySettings_MalformedJSONReturns400(t *testing.T) {
	svc := newServiceWithNotifyStore(t)
	r := newNotifyRouter(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/updates/notification-settings", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
