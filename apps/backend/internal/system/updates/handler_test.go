package updates

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRouter(svc *Service) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1/system")
	api.GET("/updates", HandleGet(svc))
	api.POST("/updates/check", HandleCheck(svc))
	return r
}

func TestHandleGet_ReturnsZeroValues(t *testing.T) {
	pool := newTestPool(t)
	svc := NewService(pool, "v1.0.0", nil, logger.Default())
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/updates", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp UpdatesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Current != "v1.0.0" {
		t.Errorf("current=%q", resp.Current)
	}
	if resp.UpdateAvailable {
		t.Errorf("expected UpdateAvailable=false")
	}
}

func TestHandleCheck_FirstCall200(t *testing.T) {
	pool := newTestPool(t)
	srv, _ := newStubGitHub(t, "v1.0.1", "https://example/v1.0.1")
	svc := NewService(pool, "v1.0.0", srv.Client(), logger.Default())
	svc.SetReleaseURL(srv.URL)
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/check", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp UpdatesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Latest != "v1.0.1" {
		t.Errorf("latest=%q", resp.Latest)
	}
	if !resp.UpdateAvailable {
		t.Errorf("expected UpdateAvailable=true")
	}
}

func TestHandleCheck_SecondCallReturns429(t *testing.T) {
	pool := newTestPool(t)
	srv, _ := newStubGitHub(t, "v1.0.1", "https://example/v1.0.1")
	svc := NewService(pool, "v1.0.0", srv.Client(), logger.Default())
	svc.SetReleaseURL(srv.URL)
	r := newRouter(svc)

	// First call seeds the limiter.
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/check", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first call status=%d", w1.Code)
	}

	// Second call within window is rate-limited.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/check", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d body=%s", w2.Code, w2.Body.String())
	}
	var body struct {
		Error             string `json:"error"`
		RetryAfterSeconds int64  `json:"retry_after_seconds"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.RetryAfterSeconds < 1 || body.RetryAfterSeconds > 30 {
		t.Errorf("retry_after_seconds out of range: %d", body.RetryAfterSeconds)
	}
	if body.Error == "" {
		t.Errorf("expected non-empty error")
	}
}

func TestHandleCheck_GitHubFailureReturns502(t *testing.T) {
	pool := newTestPool(t)
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer failSrv.Close()
	svc := NewService(pool, "v1.0.0", failSrv.Client(), logger.Default())
	svc.SetReleaseURL(failSrv.URL)
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/check", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
