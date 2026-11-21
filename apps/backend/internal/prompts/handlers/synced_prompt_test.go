package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/prompts/service"
)

func TestSyncedPromptHTTPReadOnlyAndProjected(t *testing.T) {
	router, svc, cleanup := newTestRouterWithService(t, authn.Identity{
		UserID: "test-admin", Role: authn.RoleAdmin, Synthetic: true,
	})
	defer cleanup()
	_, err := svc.ApplySyncedPrompts(context.Background(), service.SyncApplyInput{
		Desired: []service.SyncedPromptFile{{Name: "locked", Content: "Repo", Path: ".kandev/prompts/locked.md"}},
	})
	require.NoError(t, err)
	prompt, err := svc.GetPromptByName(context.Background(), "locked")
	require.NoError(t, err)

	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		request := httptest.NewRequest(method, "/api/v1/prompts/"+prompt.ID, strings.NewReader(`{"content":"edit"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assert.Equal(t, http.StatusConflict, response.Code, method)
		assert.Contains(t, response.Body.String(), "Prompt Sync", method)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/prompts", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Prompts []struct {
			Name            string `json:"name"`
			Content         string `json:"content"`
			Source          string `json:"source"`
			SourcePath      string `json:"source_path"`
			AllowAgentEdits bool   `json:"allow_agent_edits"`
		} `json:"prompts"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	found := false
	for _, p := range body.Prompts {
		if p.Name != "locked" {
			assert.Equal(t, "manual", p.Source, p.Name)
			continue
		}
		found = true
		assert.Equal(t, "Repo", p.Content)
		assert.Equal(t, "sync", p.Source)
		assert.Equal(t, ".kandev/prompts/locked.md", p.SourcePath)
		assert.False(t, p.AllowAgentEdits)
	}
	assert.True(t, found)
}
