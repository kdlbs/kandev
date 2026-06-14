package restart

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandleCapabilityReportsManualMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/system/restart-capability", HandleCapability())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/restart-capability", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if body := rec.Body.String(); body == "" || !strings.Contains(body, `"supported":false`) {
		t.Fatalf("body = %s, want unsupported capability", body)
	}
}

func TestHandleRequestRejectsUnsupportedRestart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/system/restart", HandleRequest())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/restart", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}
