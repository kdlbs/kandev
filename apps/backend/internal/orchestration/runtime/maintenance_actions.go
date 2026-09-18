package runtime

import (
	"context"
	"fmt"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/maintenance"
	"github.com/kandev/kandev/internal/orchestration/models"
)

type maintenanceRequest struct {
	models.OperationRequest
	ExpectedBindingVersion int64                  `json:"expected_binding_version"`
	CandidateRevision      int64                  `json:"candidate_revision"`
	GrantRevision          int64                  `json:"grant_revision"`
	Action                 string                 `json:"action"`
	File                   models.MaintenanceFile `json:"file"`
}

func (h *Handler) rejectMaintenanceTaskControl(c *gin.Context, id string) error {
	maintenance, err := h.Service.Repo.IsMaintenanceTask(c.Request.Context(), id)
	if err != nil || maintenance {
		return rejectOperation(403, "maintenance tasks require the closed repair controls")
	}
	return nil
}

func (h *Handler) maintenanceAction(c *gin.Context) {
	var b *models.AssistantBinding
	var claims *runtimeauth.AgentClaims
	var ok bool
	if _, agent := c.Get("agent_claims"); agent {
		claims, b, ok = h.runtimeAssistant(c)
	} else {
		b, ok = h.humanAssistant(c)
		if ok {
			claims = &runtimeauth.AgentClaims{TaskID: b.ConversationID, WorkspaceID: b.WorkspaceID, RunID: "human:" + b.OwnerUserID}
		}
	}
	if !ok {
		return
	}
	var req maintenanceRequest
	if c.ShouldBindJSON(&req) != nil || !slices.Contains([]string{"prepare", "patch", "check", "commit"}, req.Action) {
		c.AbortWithStatus(422)
		return
	}
	h.performOperation(c, claims, req.OperationRequest, req, 200, func() (any, error) {
		h.Service.maintenanceMu.Lock()
		defer h.Service.maintenanceMu.Unlock()
		grant, _, err := h.Service.currentMaintenance(c.Request.Context(), b, c.Param("id"), req)
		if err != nil {
			return nil, err
		}
		guard := maintenance.Guard(func(ctx context.Context) error {
			if err := h.authorizeTaskEffect(c, claims, executionModeExecute); err != nil {
				return err
			}
			_, _, err := h.Service.currentMaintenance(ctx, b, c.Param("id"), req)
			return err
		})
		return h.Service.performMaintenance(c.Request.Context(), b, *grant, req, guard)
	})
}

func (s *Service) currentMaintenance(ctx context.Context, b *models.AssistantBinding, id string, req maintenanceRequest) (*models.MaintenanceGrant, string, error) {
	if err := s.checkMaintenanceIntent(ctx, b, req); err != nil {
		return nil, "", err
	}
	grant, err := s.Repo.MaintenanceGrant(ctx, b.ID, id)
	if err != nil || grant.RevokedAt != nil || grant.Revision != req.GrantRevision || grant.BindingVersion != b.Version || !grant.ExpiresAt.After(s.attentionNow()) || grant.Scope.Validate() != nil {
		return nil, "", rejectOperation(403, "maintenance_grant_unavailable_or_superseded")
	}
	candidate, err := s.Repo.ImprovementCandidate(ctx, b.ID, id)
	if err != nil || candidate.WorkspaceID != b.WorkspaceID || !slices.Contains([]string{"proposed", "investigating"}, candidate.State) {
		return nil, "", rejectOperation(409, "maintenance_proposal_superseded")
	}
	source, err := s.currentMaintenanceResources(ctx, b, grant)
	return grant, source, err
}

func (s *Service) checkMaintenanceIntent(ctx context.Context, b *models.AssistantBinding, req maintenanceRequest) error {
	current, err := s.Repo.AssistantBindingByID(ctx, b.ID)
	if err != nil || current.Version != req.ExpectedBindingVersion || current.OwnerUserID != b.OwnerUserID || current.WorkspaceID != b.WorkspaceID {
		return rejectOperation(409, "assistant_binding_superseded")
	}
	if current.ExecutionMode != executionModeExecute {
		return rejectOperation(403, "maintenance_requires_execute_mode")
	}
	intent, err := s.Repo.IntentRevision(ctx, b.ConversationID)
	if err != nil || req.ExpectedIntentRevision == nil || *req.ExpectedIntentRevision != intent {
		return rejectOperation(409, "intent_superseded")
	}
	return nil
}

func (s *Service) currentMaintenanceResources(ctx context.Context, b *models.AssistantBinding, grant *models.MaintenanceGrant) (string, error) {
	authority, err := s.assistantAuthority(ctx, b.ConversationID)
	if err != nil || authority == nil || authority.Revision != grant.AuthorityRevision {
		return "", rejectOperation(409, "maintenance_authority_superseded")
	}
	profile, err := s.contextProfileRevision(ctx, b.WorkspaceID, grant.Scope.ProfileID)
	if err != nil || profile != grant.ProfileRevision {
		return "", rejectOperation(409, "maintenance_profile_superseded")
	}
	reader, ok := s.Manager.(MaintenanceScopeReader)
	if !ok || s.Maintenance == nil {
		return "", rejectOperation(422, "maintenance_sandbox_unavailable")
	}
	source, err := reader.ValidateMaintenanceScope(ctx, b.WorkspaceID, grant.Scope)
	if err != nil {
		return "", rejectOperation(403, "maintenance_resources_unavailable")
	}
	return source, nil
}

func (s *Service) performMaintenance(ctx context.Context, b *models.AssistantBinding, grant models.MaintenanceGrant, req maintenanceRequest, guard maintenance.Guard) (any, error) {
	if err := guard(ctx); err != nil {
		return nil, err
	}
	switch req.Action {
	case "prepare":
		return s.startMaintenance(ctx, b, grant, req, guard)
	case "patch":
		return s.Maintenance.Patch(ctx, grant, req.File, guard)
	case "check":
		return s.checkMaintenance(ctx, b, grant, guard)
	case "commit":
		return s.commitMaintenance(ctx, b, grant, guard)
	default:
		return nil, fmt.Errorf("unsupported maintenance action")
	}
}
