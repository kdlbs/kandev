package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestAntigravityLoginAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "default", Role: authn.RoleAdmin, Synthetic: true})
		c.Next()
	})
	RegisterRoutes(router, nil, log, "test-interlock")

	request := httptest.NewRequest(http.MethodPost, "/api/v1/antigravity-acp/login", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK {
		t.Fatal("login start succeeded without the interim settings interlock")
	}
	if recorder.Code != http.StatusUnauthorized && recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want an authorization rejection", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/antigravity-acp/login", nil)
	request.Header.Set("X-Kandev-Interim-Settings-Interlock", "test-interlock")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want the handler's 503 for a nil host utility", recorder.Code)
	}

	handlers := &Handlers{hostUtility: nil, logger: log}
	recorder = httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/antigravity-acp/login", nil)
	handlers.startAntigravityLogin(c)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 for a nil host utility", recorder.Code)
	}
}
