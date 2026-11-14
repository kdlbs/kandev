package handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatterAPIIsOwnerScopedAndCredentialFree(t *testing.T) {
	router := newProviderAPI(t)
	provider := createProviderAs(t, router, "alice", "Mail", "slack://private-receiver")
	path := "/api/v1/notification-providers/" + provider.ID + "/formatter"
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		foreign := callAs(t, router, "bob", method, path, `{"expected_revision":""}`)
		missing := callAs(t, router, "bob", method, "/api/v1/notification-providers/missing/formatter", `{"expected_revision":""}`)
		require.Equal(t, http.StatusNotFound, foreign.Code)
		require.Equal(t, missing.Body.String(), foreign.Body.String())
	}
	response := callAs(t, router, "alice", http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, response.Code)
	require.NotContains(t, response.Body.String(), "private-receiver")
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	cleared := callAs(t, router, "alice", http.MethodPut, path, `{"expected_revision":"","binding":null}`)
	require.Equal(t, http.StatusOK, cleared.Code)
	stale := callAs(t, router, "alice", http.MethodPut, path, `{"expected_revision":"","binding":null}`)
	require.Equal(t, http.StatusConflict, stale.Code)
}
