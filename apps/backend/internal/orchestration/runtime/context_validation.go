package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func (s *Service) currentPacket(ctx context.Context, b *models.AssistantBinding, ref string) (*models.ContextPacket, error) {
	raw, err := s.Repo.ContextPacket(ctx, b.ID, ref)
	if err != nil {
		return nil, fmt.Errorf("context reference unavailable")
	}
	var previous models.ContextPacket
	if err := json.Unmarshal([]byte(raw), &previous); err != nil {
		return nil, err
	}
	current, err := s.buildContext(ctx, b, previous.ObjectiveID, previous.ContextScope)
	if err != nil {
		return nil, err
	}
	if previous.ID != current.ID {
		return nil, fmt.Errorf("context changed; fetch a fresh packet before delegation")
	}
	return current, nil
}

func (h *Handler) attachDelegationContext(c *gin.Context, claims *runtimeauth.AgentClaims, o *models.Objective, ref *models.DelegationReference) error {
	if o == nil {
		return nil
	}
	b, err := h.Service.Repo.AssistantForConversation(c.Request.Context(), claims.TaskID)
	if err != nil {
		return err
	}
	p, err := h.Service.currentPacket(c.Request.Context(), b, ref.ContextRef)
	if err != nil {
		return err
	}
	if p.ObjectiveID != o.ID || p.WorkspaceID != claims.WorkspaceID {
		return fmt.Errorf("context must match this objective and workspace")
	}
	ref.Packet = p
	return nil
}

// ValidateDispatchContext is invoked by the native executor, including queue
// drains, resumes and steers. A packet is not a grant; native gates still apply.
func (s *Service) ValidateDispatchContext(ctx context.Context, ref string, task *taskmodels.Task, profile string) error {
	b, err := s.Repo.ContextBinding(ctx, ref)
	if err != nil {
		return fmt.Errorf("assistant context unavailable")
	}
	p, err := s.currentPacket(ctx, b, ref)
	if err != nil {
		return err
	}
	if p.ProfileID != profile || p.WorkspaceID != task.WorkspaceID {
		return fmt.Errorf("assistant context account/workspace mismatch")
	}
	if p.TaskID != "" && p.TaskID != task.ID {
		return fmt.Errorf("assistant context task mismatch")
	}
	actualScope := p.ContextScope
	actualScope.TaskID = task.ID
	if err := s.validateContextScope(ctx, task.WorkspaceID, actualScope); err != nil {
		return err
	}
	binding, _ := task.Metadata["orchestration_binding_id"].(string)
	objective, _ := task.Metadata["orchestration_objective_id"].(string)
	if p.BindingID != binding || p.ObjectiveID != objective {
		return fmt.Errorf("assistant context ownership mismatch")
	}
	o, err := s.Repo.Objective(ctx, b.ID, p.ObjectiveID)
	if err != nil || o.Status != statusActive || (o.Mode != executionModeExecute && o.Mode != executionModeDesign) {
		return fmt.Errorf("objective is not active delivery work")
	}
	return nil
}
