package clarification

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// A malformed saved group must not hide valid images: the web falls back to a
// gallery, so every image of the bundle stays readable.
// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-001.6
func TestQuestionImageReadIgnoresMalformedSavedGroups(t *testing.T) {
	msg := clarificationMessage("m", "bundle", "q1", 0, "Approve?", "yes")
	q := msg.Metadata["question"].(map[string]any)
	q["images"] = []any{map[string]any{"id": "screen", "path": "screen.png", "alt": "Layout"}}
	q["image_groups"] = []any{map[string]any{"kind": "pair", "image_ids": []any{"screen", "missing"}}}
	repo := imageMessageRepo{&stubMessageStore{messages: map[string][]*taskmodels.Message{"bundle": {msg}}}, "t1"}
	store := NewStore(time.Minute)
	creator := &stubMessageCreator{}
	events := &stubEventBus{}
	resolver := NewResolver(store, repo, creator, &keyedAuthorizer{allowed: map[string]bool{"t1": true}}, events, events, nil, logger.Default())
	reader := &imageReaderRecorder{}
	router := gin.New()
	RegisterRoutes(router, store, nil, creator, repo, events, resolver, logger.Default(), nil, nil, false, reader)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clarification/bundle/images/screen?question_id=q1", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, reader.calls)
}
