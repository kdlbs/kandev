package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
)

func TestSubprocessAdmissionRouteRequiresBearerToken(t *testing.T) {
	server := NewControlServer(&config.Config{AuthToken: "admission-secret"}, &instance.Manager{}, logger.Default())

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/debug/subprocess-admission", nil)
	unauthenticatedResponse := httptest.NewRecorder()
	server.Router().ServeHTTP(unauthenticatedResponse, unauthenticated)
	if unauthenticatedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthenticatedResponse.Code, http.StatusUnauthorized)
	}

	authenticated := httptest.NewRequest(http.MethodGet, "/api/v1/debug/subprocess-admission", nil)
	authenticated.Header.Set("Authorization", "Bearer admission-secret")
	authenticatedResponse := httptest.NewRecorder()
	server.Router().ServeHTTP(authenticatedResponse, authenticated)
	if authenticatedResponse.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want %d", authenticatedResponse.Code, http.StatusOK)
	}
	var snapshot subproc.Snapshot
	if err := json.Unmarshal(authenticatedResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snapshot.Pool != "git" {
		t.Fatalf("snapshot pool = %q, want git", snapshot.Pool)
	}
}
