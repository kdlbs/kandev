package sentry

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestMockController_SeedRoutesRequireInstanceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := NewMockController(NewMockClient(), newTestStore(t), logger.Default())
	ctrl.RegisterRoutes(router)

	for _, tc := range []struct {
		name   string
		method string
		target string
		body   string
	}{
		{
			name:   "auth health",
			method: http.MethodPut,
			target: "/api/v1/sentry/mock/auth-health",
			body:   `{"ok":true}`,
		},
		{
			name:   "organizations",
			method: http.MethodPost,
			target: "/api/v1/sentry/mock/organizations",
			body:   `{"organizations":[]}`,
		},
		{
			name:   "projects",
			method: http.MethodPost,
			target: "/api/v1/sentry/mock/projects",
			body:   `{"projects":[]}`,
		},
		{
			name:   "issues",
			method: http.MethodPost,
			target: "/api/v1/sentry/mock/issues",
			body:   `{"issues":[]}`,
		},
		{
			name:   "get issue error",
			method: http.MethodPut,
			target: "/api/v1/sentry/mock/get-issue-error",
			body:   `{"statusCode":500,"message":"upstream failed"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := do(router, tc.method, tc.target, tc.body)
			if resp.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body=%s", resp.Code, resp.Body.String())
			}
		})
	}
}

// TestMockController_SetAuthResult_AllowsEmptyInstanceID pins the pre-save
// "Test connection" flow: TestConnectionCandidate builds a client for a
// not-yet-persisted config (empty instance ID), so E2E specs must be able to
// seed that empty-ID dataset via /mock/auth-result without an instanceId.
func TestMockController_SetAuthResult_AllowsEmptyInstanceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	mock := NewMockClient()
	ctrl := NewMockController(mock, newTestStore(t), logger.Default())
	ctrl.RegisterRoutes(router)

	resp := do(router, http.MethodPut, "/api/v1/sentry/mock/auth-result", `{"ok":false,"error":"invalid token"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	result, err := mock.testAuth("")
	if err != nil {
		t.Fatalf("test auth: %v", err)
	}
	if result.OK || result.Error != "invalid token" {
		t.Errorf("candidate auth result = %+v, want seeded failure", result)
	}
}

// TestMockController_SetAuthHealth_MissingInstance404 pins the P2 fix: seeding
// auth-health for a missing/misspelled instance returns 404 (not {set:true}),
// so E2E seeding fails fast instead of silently succeeding.
func TestMockController_SetAuthHealth_MissingInstance404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := NewMockController(NewMockClient(), newTestStore(t), logger.Default())
	ctrl.RegisterRoutes(router)

	resp := do(router, http.MethodPut, "/api/v1/sentry/mock/auth-health?instanceId=ghost", `{"ok":true}`)
	if resp.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%s", resp.Code, resp.Body.String())
	}
}
