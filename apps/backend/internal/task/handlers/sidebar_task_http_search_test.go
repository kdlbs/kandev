package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSidebarTaskQuerySearch(t *testing.T) {
	c, _ := taskRequestAs(t, "", http.MethodPost, "/api/v1/workspaces/ws-b/sidebar/query", "ws-b")
	c.Request.Body = io.NopCloser(strings.NewReader(`{"query":"literal%_needle"}`))

	query, malformed, err := decodeSidebarTaskQuery(c)
	require.False(t, malformed)
	require.NoError(t, err)
	require.Equal(t, "literal%_needle", query.SearchQuery)
	encoded, err := json.Marshal(query)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"query":"literal%_needle"`)
}
