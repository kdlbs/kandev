package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(token string, exemptPaths ...string) *gin.Engine {
	r := gin.New()
	r.Use(bearerTokenAuth(token, exemptPaths...))
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/v1/status", func(c *gin.Context) { c.JSON(200, gin.H{"status": "running"}) })
	r.GET("/api/v1/data", func(c *gin.Context) { c.JSON(200, gin.H{"data": "secret"}) })
	return r
}

func TestBearerTokenAuth_EmptyToken_DisablesAuth(t *testing.T) {
	r := setupRouter("")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestBearerTokenAuth_ValidToken(t *testing.T) {
	r := setupRouter("test-secret-token")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestBearerTokenAuth_MissingToken(t *testing.T) {
	r := setupRouter("test-secret-token")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestBearerTokenAuth_InvalidToken(t *testing.T) {
	r := setupRouter("test-secret-token")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestBearerTokenAuth_ExemptPath(t *testing.T) {
	r := setupRouter("test-secret-token", "/health")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (health should be exempt)", w.Code)
	}
}

func TestBearerTokenAuth_NonExemptPathRequiresAuth(t *testing.T) {
	r := setupRouter("test-secret-token", "/health")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestBearerTokenAuth_QueryParamIgnored(t *testing.T) {
	r := setupRouter("test-secret-token")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/status?token=test-secret-token", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 — query param tokens must be ignored, got %d", w.Code)
	}
}

func TestBearerTokenAuth_MalformedAuthHeader(t *testing.T) {
	r := setupRouter("test-secret-token")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-Bearer auth, got %d", w.Code)
	}
}

func TestTokenEqual_ConstantTime(t *testing.T) {
	if !tokenEqual("abc", "abc") {
		t.Error("expected equal tokens to match")
	}
	if tokenEqual("abc", "xyz") {
		t.Error("expected different tokens to not match")
	}
	if tokenEqual("abc", "ab") {
		t.Error("expected different length tokens to not match")
	}
}
