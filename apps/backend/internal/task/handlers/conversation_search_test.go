package handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConversationSearchHTTPAuthorization(t *testing.T) {
	h := newForeignSessionHandlers(t)
	ctx, response := requestAs(t, "user-a", "ws-b")
	h.httpSearchTaskConversations(ctx)
	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	ctx, response = requestAs(t, "user-b", "ws-b")
	h.httpSearchTaskConversations(ctx)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	require.JSONEq(t, `{"tasks":[],"state":"unavailable","has_more":false}`, response.Body.String())
}
