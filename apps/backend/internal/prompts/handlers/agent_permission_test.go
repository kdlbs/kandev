package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptHTTPAgentPermission(t *testing.T) {
	router, cleanup := newTestRouter(t)
	t.Cleanup(cleanup)
	created := postJSON(t, router, "/api/v1/prompts", map[string]any{"name": "human", "content": "Original"})
	require.Equal(t, http.StatusOK, created.Code)
	var prompt map[string]any
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &prompt))
	require.Equal(t, false, prompt["allow_agent_edits"])
	for _, patch := range []string{`{"allow_agent_edits":true}`, `{"content":"Changed"}`} {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/prompts/"+prompt["id"].(string), bytes.NewBufferString(patch))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &prompt))
		require.Equal(t, true, prompt["allow_agent_edits"])
	}
	require.Equal(t, "Changed", prompt["content"])
}
