package runtime

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) delegationObjective(c *gin.Context, claims *runtimeauth.AgentClaims, id, mode string) (*models.Objective, error) {
	if id == "" {
		if mode != "" || claims.Capabilities == assistantBrokerAudience {
			return nil, fmt.Errorf("explicit delivery requires an objective")
		}
		return nil, nil // legacy workspace coordinators retain their existing contract
	}
	b, err := h.Service.Repo.AssistantForConversation(c.Request.Context(), claims.TaskID)
	if err != nil {
		return nil, fmt.Errorf("assistant binding required")
	}
	o, err := h.Service.Repo.Objective(c.Request.Context(), b.ID, id)
	if err != nil || o.WorkspaceID != claims.WorkspaceID {
		return nil, fmt.Errorf("objective unavailable")
	}
	if o.Mode != executionModeExecute && o.Mode != executionModeDesign {
		return nil, fmt.Errorf("answer and inspect do not delegate delivery work")
	}
	if mode != "" && mode != o.Mode {
		return nil, fmt.Errorf("delivery mode must match the objective")
	}
	if o.Status != statusActive {
		return nil, fmt.Errorf("objective must be active before delegation")
	}
	revision, err := h.Service.Repo.IntentRevision(c.Request.Context(), claims.TaskID)
	if err != nil {
		return nil, err
	}
	if o.IntentRevision != revision {
		return nil, fmt.Errorf("update the objective for current user intent before delegation")
	}
	return o, nil
}

func delegationReference(o *models.Objective, contextRef, operationID string) models.DelegationReference {
	ref := models.DelegationReference{ContextRef: contextRef, DispatchOperationID: operationID}
	if o != nil {
		ref.ObjectiveID = o.ID
		ref.AcceptanceRevision = o.AcceptanceRevision
		ref.SourceCommentID = o.SourceCommentID
		ref.Acceptance = o.Acceptance
	}
	return ref
}
