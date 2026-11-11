package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/prompts/controller"
	"github.com/kandev/kandev/internal/prompts/models"
	"github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/stretchr/testify/require"
)

func TestPromptHTTPAgentPermissionCompatibility(t *testing.T) {
	router, cleanup := newTestRouter(t)
	t.Cleanup(cleanup)
	created := postJSON(t, router, "/api/v1/prompts", map[string]any{"name": "human", "content": "Original"})
	require.Equal(t, http.StatusOK, created.Code)
	var prompt map[string]any
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &prompt))
	require.Equal(t, true, prompt["allow_agent_edits"])
	initialUpdatedAt := prompt["updated_at"]
	for _, allowed := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/prompts/"+prompt["id"].(string), bytes.NewBufferString(`{"allow_agent_edits":`+strconv.FormatBool(allowed)+`}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &prompt))
		require.Equal(t, true, prompt["allow_agent_edits"])
		require.Equal(t, "Original", prompt["content"])
		require.Equal(t, initialUpdatedAt, prompt["updated_at"])
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/prompts/"+prompt["id"].(string), bytes.NewBufferString(`{"allow_agent_edits":false,"content":"Changed"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &prompt))
	require.Equal(t, "Changed", prompt["content"])
	require.Equal(t, true, prompt["allow_agent_edits"])
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/prompts", nil))
	require.Equal(t, http.StatusOK, list.Code)
	var response struct {
		Prompts []map[string]any `json:"prompts"`
	}
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &response))
	var builtin map[string]any
	for _, item := range response.Prompts {
		if item["name"] == "code-review" {
			builtin = item
		}
	}
	require.NotNil(t, builtin)
	require.Equal(t, false, builtin["allow_agent_edits"])
}

type conflictingPromptStore struct{ promptstore.Repository }

func (conflictingPromptStore) GetPromptByID(context.Context, string) (*models.Prompt, error) {
	return &models.Prompt{ID: "prompt", Name: "review", Content: "Original"}, nil
}

func (conflictingPromptStore) UpdatePrompt(context.Context, *models.Prompt) error {
	return promptstore.ErrPromptWriteRejected
}

func TestPromptHTTPConcurrentUpdateReturnsConflict(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	handler := NewHandlers(controller.NewController(service.NewService(conflictingPromptStore{})), log)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "prompt"}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/prompts/prompt", bytes.NewBufferString(`{"content":"New"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler.httpUpdatePrompt(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), "read it again")
}
