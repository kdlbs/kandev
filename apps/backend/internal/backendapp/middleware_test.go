package backendapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

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

// TestCORSMiddlewareBlocksDisallowedOriginStateChange is the regression guard
// for the drive-by CSRF RCE: a disallowed cross-origin state-changing request
// (here the multipart/form-data POST used to reach /api/plugins/install) must
// be rejected with 403 before the handler — and its side effect — can run.
func TestCORSMiddlewareBlocksDisallowedOriginStateChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			handlerRan := false
			router := gin.New()
			router.Use(corsMiddleware())
			router.Handle(method, "/api/plugins/install", func(c *gin.Context) {
				handlerRan = true
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(method, "/api/plugins/install", nil)
			req.Host = "localhost:38429"
			req.Header.Set("Origin", "https://evil.invalid")
			// The CORS-safelisted content type that skips preflight.
			req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 for disallowed cross-origin %s", rec.Code, method)
			}
			if handlerRan {
				t.Fatalf("handler executed for disallowed cross-origin %s — side effect not blocked", method)
			}
		})
	}
}

// TestCORSMiddlewareBlocksDisallowedOriginGET guards against treating GET as
// side-effect-free: some endpoints (e.g. plugin webhooks, registered for GET as
// well as POST) mutate state on GET, so a disallowed-origin GET must also be
// rejected with 403 before the handler runs.
func TestCORSMiddlewareBlocksDisallowedOriginGET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlerRan := false
	router := gin.New()
	router.Use(corsMiddleware())
	router.GET("/api/plugins/:id/webhooks/:key", func(c *gin.Context) {
		handlerRan = true
		c.String(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/plugins/p/webhooks/k", nil)
	req.Host = "localhost:38429"
	req.Header.Set("Origin", "https://evil.invalid")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for disallowed cross-origin GET", rec.Code)
	}
	if handlerRan {
		t.Fatal("handler executed for disallowed cross-origin GET — side effect not blocked")
	}
}

// TestCORSMiddlewareAllowsNoOriginStateChange ensures the fix does not break
// non-browser clients (CLI/curl) that send no Origin header.
func TestCORSMiddlewareAllowsNoOriginStateChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlerRan := false
	router := gin.New()
	router.Use(corsMiddleware())
	router.POST("/api/plugins/install", func(c *gin.Context) {
		handlerRan = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/plugins/install", nil)
	req.Host = "localhost:38429"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if !handlerRan || rec.Code != http.StatusOK {
		t.Fatalf("no-Origin POST: handlerRan=%v status=%d, want handler run + 200", handlerRan, rec.Code)
	}
}

// TestCORSMiddlewareAllowsAllowedOriginStateChange ensures the legitimate
// same/loopback-origin frontend can still make mutating requests.
func TestCORSMiddlewareAllowsAllowedOriginStateChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlerRan := false
	router := gin.New()
	router.Use(corsMiddleware())
	router.POST("/api/plugins/install", func(c *gin.Context) {
		handlerRan = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/plugins/install", nil)
	req.Host = "localhost:38429"
	req.Header.Set("Origin", "http://127.0.0.1:37429")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if !handlerRan || rec.Code != http.StatusOK {
		t.Fatalf("allowed-origin POST: handlerRan=%v status=%d, want handler run + 200", handlerRan, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:37429" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want echoed allowed origin", got)
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
