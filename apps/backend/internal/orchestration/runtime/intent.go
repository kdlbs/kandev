package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (s *Service) withIntentRevision(ctx context.Context, taskID string, payload map[string]any) (map[string]any, error) {
	copy := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		copy[key] = value
	}
	if _, present := copy["intent_revision"]; !present {
		revision, err := s.Repo.IntentRevision(ctx, taskID)
		if err != nil {
			return nil, err
		}
		copy["intent_revision"] = revision
	}
	if _, present := copy["binding_version"]; !present {
		id, version, err := s.bindingSnapshot(ctx, taskID)
		if err != nil {
			return nil, err
		}
		copy["binding_id"], copy["binding_version"] = id, version
	}
	return copy, nil
}

func (s *Service) bindingSnapshot(ctx context.Context, taskID string) (string, int64, error) {
	owner, err := s.Repo.ConversationUserOwner(ctx, taskID)
	if err != nil {
		return "", 0, err
	}
	if owner != "" && !s.AssistantEnabled {
		return "", 0, ErrAssistantDisabled
	}
	row, err := s.Repo.AssistantForConversation(ctx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		if owner != "" {
			return "", 0, models.ErrConflict
		}
		return "", 0, nil
	}
	if err != nil {
		return "", 0, err
	}
	if row.OwnerUserID != owner {
		return "", 0, models.ErrConflict
	}
	return row.ID, row.Version, nil
}

func (h *Handler) currentIntent(c *gin.Context, taskID, payload string) bool {
	var snapshot struct {
		Revision int64 `json:"intent_revision"`
	}
	if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
		c.AbortWithStatus(http.StatusForbidden)
		return false
	}
	revision, err := h.Service.Repo.IntentRevision(c.Request.Context(), taskID)
	if err != nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return false
	}
	if snapshot.Revision != revision {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{errorResponseKey: "intent_superseded", "intent_revision": revision})
		return false
	}
	return h.currentBinding(c, taskID, payload)
}

func (s *Service) validateBindingSnapshot(ctx context.Context, taskID, payload string) error {
	var snapshot struct {
		ID      string `json:"binding_id"`
		Version int64  `json:"binding_version"`
	}
	if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
		return err
	}
	id, version, err := s.bindingSnapshot(ctx, taskID)
	if err != nil {
		return err
	}
	if snapshot.ID != id || snapshot.Version != version {
		return models.ErrConflict
	}
	return nil
}

func (h *Handler) currentBinding(c *gin.Context, taskID, payload string) bool {
	err := h.Service.validateBindingSnapshot(c.Request.Context(), taskID, payload)
	return bindingCheck(c, err)
}

func bindingCheck(c *gin.Context, err error) bool {
	if errors.Is(err, ErrAssistantDisabled) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errorResponseKey: err.Error()})
		return false
	}
	if errors.Is(err, models.ErrConflict) {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{errorResponseKey: "assistant_binding_superseded"})
		return false
	}
	if err != nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return false
	}
	return true
}
