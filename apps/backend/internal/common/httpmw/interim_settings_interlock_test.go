package httpmw

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInterimSettingsInterlockRejectsMissingWrongBearerAndEmptyServerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name          string
		serverToken   string
		requestToken  string
		authorization string
	}{
		{name: "missing token", serverToken: "per-boot-token"},
		{name: "wrong token", serverToken: "per-boot-token", requestToken: "wrong"},
		{name: "bearer credential", serverToken: "per-boot-token", requestToken: "per-boot-token", authorization: "Bearer office-jwt"},
		{name: "empty server token"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			router := gin.New()
			router.POST("/settings", InterimSettingsInterlock(tc.serverToken), func(c *gin.Context) {
				called = true
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodPost, "/settings", nil)
			request.Header.Set(InterimSettingsInterlockHeader, tc.requestToken)
			request.Header.Set("Authorization", tc.authorization)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
			if called {
				t.Fatal("handler was called")
			}
		})
	}
}

func TestInterimSettingsInterlockAllowsMatchingTokenWithoutBearerCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	router := gin.New()
	router.POST("/settings", InterimSettingsInterlock("per-boot-token"), func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/settings", nil)
	request.Header.Set(InterimSettingsInterlockHeader, "per-boot-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestNewInterimSettingsInterlockTokenIsRandomAndNonEmpty(t *testing.T) {
	first, err := NewInterimSettingsInterlockToken()
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	second, err := NewInterimSettingsInterlockToken()
	if err != nil {
		t.Fatalf("second token: %v", err)
	}
	if first == "" || second == "" {
		t.Fatal("token is empty")
	}
	if first == second {
		t.Fatal("generated tokens match")
	}
}
