package backendapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/httpmw"
)

// TestCORSMiddlewareAllowsBootTokenHeaderInPreflight pins that the operator
// boot-token header is in Access-Control-Allow-Headers — without it, a
// split-origin (Vite dev) plugin mutation carrying X-Kandev-Boot-Token is
// blocked by the browser's CORS preflight before it reaches the guarded route.
func TestCORSMiddlewareAllowsBootTokenHeaderInPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(corsMiddleware())
	router.POST("/api/plugins/install", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodOptions, "/api/plugins/install", nil)
	req.Host = "localhost:38429"
	req.Header.Set("Origin", "http://localhost:37429")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", httpmw.BootTokenHeader)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	allowed := rec.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowed, httpmw.BootTokenHeader) {
		t.Fatalf("Access-Control-Allow-Headers = %q, want it to include %q", allowed, httpmw.BootTokenHeader)
	}
}

func TestCORSMiddlewareEchoesOriginForCredentialedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(corsMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Host = "localhost:38429"
	req.Header.Set("Origin", "http://localhost:37429")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:37429" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want request origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
}

func TestCORSMiddlewareAllowsLoopbackAliasOriginForCredentialedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(corsMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Host = "127.0.0.1:38429"
	req.Header.Set("Origin", "http://localhost:37429")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:37429" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want loopback origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
}

func TestCORSMiddlewareRejectsCredentialedRequestsFromUnrelatedOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(corsMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Host = "localhost:38429"
	req.Header.Set("Origin", "https://example.invalid")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want no credentialed CORS", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want no credentialed CORS", got)
	}
}
