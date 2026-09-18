package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/auth/authn"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const workspaceCoordinatorAudience = "workspace_coordinator"
const statusClaimed = "claimed"

const assistantBrokerAudience = "assistant_broker"
const AssistantPolicyMetadata = "assistant_broker_policy"

// CheckAssistantSession rejects native launch/resume/steer paths lacking the
// current broker run and its server-authored session restriction.
func (s *Service) CheckAssistantSession(ctx context.Context, taskID string, session *taskmodels.TaskSession, profileID string) error {
	if err := s.CheckConversationExecution(ctx, taskID); err != nil {
		return err
	}
	if session == nil || session.TaskID != taskID || session.Metadata[AssistantPolicyMetadata] != string(mcpprofile.SurfaceAssistantBroker) {
		return models.ErrConflict
	}
	run, err := s.Runs.LatestRunForSession(ctx, session.ID)
	if err != nil || run.Status != statusClaimed {
		return models.ErrConflict
	}
	if err := s.validateBindingSnapshot(ctx, taskID, run.Payload); err != nil {
		return err
	}
	authority, err := s.validateAssistantAuthority(ctx, taskID, run.Payload)
	if err != nil {
		return err
	}
	if authority == nil || (profileID != authority.ProfileID && profileID != run.AgentProfileID) {
		return models.ErrConflict
	}
	var snapshot struct {
		Revision int64 `json:"intent_revision"`
	}
	if json.Unmarshal([]byte(run.Payload), &snapshot) != nil {
		return models.ErrConflict
	}
	revision, err := s.Repo.IntentRevision(ctx, taskID)
	if err != nil {
		return err
	}
	if revision != snapshot.Revision {
		return models.ErrConflict
	}
	return nil
}

type AssistantAuthorityReader interface {
	ResolveAssistantAuthority(context.Context, models.AssistantBinding, string, string) (models.AssistantAuthority, error)
}

func assistantEffectAllowed(mode, effect string) bool {
	switch mode {
	case "answer", "inspect", executionModeDesign, executionModeExecute:
	default:
		return false
	}
	return effect == "read" || effect == "receipt" || (effect == "task_write" && (mode == executionModeDesign || mode == executionModeExecute))
}

func (s *Service) assistantAuthority(ctx context.Context, taskID string) (*models.AssistantAuthority, error) {
	binding, err := s.Repo.AssistantForConversation(ctx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !s.AssistantEnabled {
		return nil, ErrAssistantDisabled
	}
	persona, err := s.Personas.GetAgentInstance(ctx, binding.OrchestratorID)
	if err != nil {
		return nil, err
	}
	if paused(persona) {
		return nil, ErrAssistantPaused
	}
	profile, executor, err := s.executionSelection(ctx, persona)
	if err != nil {
		return nil, err
	}
	if s.Authority == nil {
		return nil, errors.New("assistant_authority_unavailable")
	}
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: binding.OwnerUserID, Role: authn.RoleMember})
	row, err := s.Authority.ResolveAssistantAuthority(ctx, *binding, profile, executor)
	if err != nil {
		return nil, err
	}
	row.ProfileID, row.ExecutorID, row.Mode = profile, executor, binding.ExecutionMode
	row.BindingID, row.BindingVersion = binding.ID, binding.Version
	row.IntentRevision, err = s.Repo.IntentRevision(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) validateAssistantAuthority(ctx context.Context, taskID, payload string) (*models.AssistantAuthority, error) {
	row, err := s.assistantAuthority(ctx, taskID)
	if err != nil || row == nil {
		return row, err
	}
	var snapshot struct {
		Authority *models.AssistantAuthority `json:"assistant_authority"`
		Revision  int64                      `json:"intent_revision"`
	}
	if json.Unmarshal([]byte(payload), &snapshot) != nil || snapshot.Authority == nil || *snapshot.Authority != *row || snapshot.Revision != row.IntentRevision {
		return nil, models.ErrConflict
	}
	if row.UnsupportedReason != "" {
		return nil, errors.New("assistant_policy_unsupported")
	}
	return row, nil
}

func (h *Handler) assistantInvocation(c *gin.Context, claims *runtimeauth.AgentClaims, payload string) bool {
	row, err := h.Service.validateAssistantAuthority(c.Request.Context(), claims.TaskID, payload)
	if err != nil {
		code := http.StatusUnprocessableEntity
		if errors.Is(err, models.ErrConflict) {
			code = http.StatusConflict
		}
		c.AbortWithStatusJSON(code, gin.H{errorResponseKey: "assistant_authority_unavailable_or_superseded"})
		return false
	}
	if row == nil {
		if claims.Capabilities == workspaceCoordinatorAudience {
			return true
		}
		c.AbortWithStatus(http.StatusForbidden)
		return false
	}
	if claims.Capabilities != assistantBrokerAudience || claims.Audience != "kandev:assistant-broker" || !assistantEffectAllowed(row.Mode, assistantRequestEffect(c)) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{errorResponseKey: "assistant_effect_denied"})
		return false
	}
	binding, err := h.Service.Repo.AssistantForConversation(c.Request.Context(), claims.TaskID)
	if err != nil {
		c.AbortWithStatus(http.StatusForbidden)
		return false
	}
	c.Request = c.Request.WithContext(authn.WithIdentity(c.Request.Context(), authn.Identity{UserID: binding.OwnerUserID, Role: authn.RoleMember}))
	return true
}

func (h *Handler) authorizeTaskEffect(c *gin.Context, claims *runtimeauth.AgentClaims, mode string) error {
	if claims.Capabilities != assistantBrokerAudience {
		return nil
	}
	run, err := h.Service.Runs.GetRunByID(c.Request.Context(), claims.RunID)
	if err != nil || run.Status != statusClaimed {
		return rejectOperation(409, "run_superseded")
	}
	if err := h.Service.validateBindingSnapshot(c.Request.Context(), claims.TaskID, run.Payload); err != nil {
		return rejectOperation(409, "binding_superseded")
	}
	row, err := h.Service.validateAssistantAuthority(c.Request.Context(), claims.TaskID, run.Payload)
	if err != nil || row == nil {
		return rejectOperation(409, "authority_superseded")
	}
	if !assistantEffectAllowed(row.Mode, "task_write") || (row.Mode == executionModeDesign && mode != executionModeDesign) {
		return rejectOperation(403, "assistant_effect_denied")
	}
	var snapshot struct {
		Revision int64 `json:"intent_revision"`
	}
	if json.Unmarshal([]byte(run.Payload), &snapshot) != nil {
		return rejectOperation(409, "intent_superseded")
	}
	revision, err := h.Service.Repo.IntentRevision(c.Request.Context(), claims.TaskID)
	if err != nil || revision != snapshot.Revision {
		return rejectOperation(409, "intent_superseded")
	}
	return nil
}

func assistantRequestEffect(c *gin.Context) string {
	if c.Request.Method == http.MethodGet {
		return "read"
	}
	path := strings.TrimPrefix(c.FullPath(), "/api/v1/orchestration")
	switch path {
	case "/runtime/comments", "/runtime/objectives", "/runtime/objectives/:id":
		return "receipt"
	case "/runtime/tasks", "/runtime/tasks/:id/manage", "/runtime/tasks/:id/status", "/runtime/attention/:id/answer":
		return "task_write"
	default:
		return statusUnknown
	}
}
