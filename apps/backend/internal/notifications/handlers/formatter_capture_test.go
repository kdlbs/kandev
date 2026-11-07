package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/stretchr/testify/require"
)

func TestFormatterCaptureAPIRequiresAdminAndBlocksActivation(t *testing.T) {
	router := newProviderAPI(t)
	provider := createProviderAs(t, router, "alice", "Mail", "json://example.invalid")
	const base = "/api/v1/notification-formatters/capture"
	denied := callAs(t, router, "alice", http.MethodPost, base, "")
	require.Equal(t, http.StatusForbidden, denied.Code)
	admin := func(method, path, owner string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		request = request.WithContext(authn.WithIdentity(request.Context(), authn.Identity{UserID: owner, Role: authn.RoleAdmin}))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	started := admin(http.MethodPost, base, "alice")
	require.Equal(t, http.StatusOK, started.Code)
	var lease struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(started.Body.Bytes(), &lease))
	t.Cleanup(func() { admin(http.MethodPost, base+"/"+lease.ID+"/release", "alice") })
	blocked := callAs(t, router, "alice", http.MethodPut, "/api/v1/notification-providers/"+provider.ID+"/formatter", `{"expected_revision":"","binding":null}`)
	require.Equal(t, http.StatusServiceUnavailable, blocked.Code)
	require.Equal(t, http.StatusNotFound, admin(http.MethodPost, base+"/"+lease.ID+"/release", "bob").Code)
	require.Equal(t, http.StatusOK, admin(http.MethodPost, base+"/"+lease.ID+"/release", "alice").Code)
	cleared := callAs(t, router, "alice", http.MethodPut, "/api/v1/notification-providers/"+provider.ID+"/formatter", `{"expected_revision":"","binding":null}`)
	require.Equal(t, http.StatusOK, cleared.Code)
}
