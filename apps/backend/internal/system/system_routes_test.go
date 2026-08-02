package system

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/system/frontenderrors"
	"go.uber.org/zap"
)

func TestRegisterRoutesAllowsMemberFrontendErrorReports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "member-1", Role: authn.RoleMember})
		c.Next()
	})
	service := &Service{FrontendErrors: frontenderrors.New(log, nil)}
	service.RegisterRoutes(router, log)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/system/logs/frontend-errors",
		bytes.NewBufferString(`{"source":"sonner","title":"visible error"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("member report status = %d, body = %s", response.Code, response.Body.String())
	}
}
