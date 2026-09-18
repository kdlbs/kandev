package runtime

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
	"net/http"
)

type InputResolver interface {
	ReadInput(context.Context, *models.AssistantBinding, models.Attention) (*models.AttentionInput, error)
	ResolveInput(context.Context, *models.AssistantBinding, models.Attention, models.InputResponse) (any, error)
	StopSession(context.Context, *models.AssistantBinding, string, string) (string, error)
}
type attentionResponseRequest struct {
	models.OperationRequest
	models.InputResponse
	ExpectedBindingVersion int64    `json:"expected_binding_version"`
	ExpectedRevision       int64    `json:"expected_revision"`
	SourceRevision         string   `json:"source_revision"`
	SessionID              string   `json:"session_id"`
	ContextRef             string   `json:"context_ref"`
	MemoryIDs              []string `json:"memory_ids"`
}

func (h *Handler) inputBinding(c *gin.Context) (*runtimeauth.AgentClaims, *models.AssistantBinding, bool) {
	if _, agent := c.Get("agent_claims"); agent {
		return h.runtimeAssistant(c)
	}
	b, ok := h.humanAssistant(c)
	if !ok {
		return nil, nil, false
	}
	return &runtimeauth.AgentClaims{TaskID: b.ConversationID, WorkspaceID: b.WorkspaceID, RunID: "human:" + b.OwnerUserID}, b, true
}
func (h *Handler) scopedAttention(c *gin.Context, b *models.AssistantBinding) (*models.Attention, bool) {
	row, err := h.Service.Repo.AttentionByID(c.Request.Context(), b.ID, c.Param("id"))
	if err != nil || !h.attentionTargetMatches(c, b, row) || !h.Service.attentionVisible(c.Request.Context(), b, *row) {
		c.AbortWithStatus(404)
		return nil, false
	}
	if h.Service.Inputs == nil {
		c.AbortWithStatus(503)
		return nil, false
	}
	return row, true
}
func (h *Handler) attentionInput(c *gin.Context) {
	_, b, ok := h.inputBinding(c)
	if !ok {
		return
	}
	row, ok := h.scopedAttention(c, b)
	if !ok {
		return
	}
	if err := h.Service.ReconcileAttentionTask(c.Request.Context(), row.TaskID); err != nil {
		c.AbortWithStatus(503)
		return
	}
	row, ok = h.scopedAttention(c, b)
	if !ok {
		return
	}
	input, err := h.readScopedInput(c, b, row)
	if err != nil {
		inputError(c, err)
		return
	}
	if _, agent := c.Get("agent_claims"); agent {
		h.workspaceResponse(c, gin.H{"attention": row, "input": input}, "task_input")
		return
	}
	if _, _, err = h.Service.inputWorkspace(c.Request.Context(), b, row, "observe"); err != nil {
		c.AbortWithStatus(403)
		return
	}
	c.JSON(200, gin.H{"attention": row, "input": input})
}
func inputError(c *gin.Context, err error) {
	var rejected *models.InputRejection
	if errors.As(err, &rejected) {
		c.AbortWithStatusJSON(rejected.Status, gin.H{errorResponseKey: rejected.Reason})
		return
	}
	c.AbortWithStatusJSON(503, gin.H{errorResponseKey: "native_input_unavailable"})
}
func (h *Handler) resolveAttention(c *gin.Context) {
	claims, b, ok := h.inputBinding(c)
	if !ok {
		return
	}
	row, ok := h.scopedAttention(c, b)
	if !ok {
		return
	}
	var req attentionResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	h.performOperation(c, claims, req.OperationRequest, req, http.StatusOK, func() (any, error) {
		if err := h.validateInputResponse(c, claims, b, row, &req); err != nil {
			return nil, err
		}
		scoped, g, err := h.Service.inputWorkspace(c.Request.Context(), b, row, "coordinate")
		if err != nil {
			return nil, rejectOperation(403, "workspace_input_scope_required")
		}
		ctx := c.Request.Context()
		if g != nil {
			ctx = h.Service.workspaceEffectContext(ctx, b, g, "coordinate", "task_input")
		}
		result, err := h.Service.Inputs.ResolveInput(ctx, scoped, *row, req.InputResponse)
		if err != nil {
			var rejected *models.InputRejection
			if errors.As(err, &rejected) {
				return nil, rejectOperation(rejected.Status, rejected.Reason)
			}
			return nil, err
		}
		_ = h.Service.ReconcileAttentionTask(c.Request.Context(), row.TaskID)
		return result, nil
	})
}
func (h *Handler) validateInputResponse(c *gin.Context, claims *runtimeauth.AgentClaims, b *models.AssistantBinding, row *models.Attention, req *attentionResponseRequest) error {
	ctx := c.Request.Context()
	current, err := h.Service.Repo.AssistantBindingByID(ctx, b.ID)
	if err != nil || current.Version != req.ExpectedBindingVersion || current.Version != b.Version {
		return rejectOperation(409, "assistant_binding_superseded")
	}
	fresh, err := h.Service.Repo.AttentionByID(ctx, b.ID, row.ID)
	if err != nil || fresh.Revision != req.ExpectedRevision || fresh.SourceRevision != req.SourceRevision || fresh.SessionID != req.SessionID {
		return rejectOperation(409, "attention_superseded")
	}
	if !h.Service.attentionVisible(ctx, b, *fresh) {
		return rejectOperation(404, "attention_unavailable")
	}
	input, err := h.readScopedInput(c, b, fresh)
	if err != nil {
		return rejectOperation(409, "native_input_unavailable")
	}
	if !currentInputMatches(input, row, req) {
		return rejectOperation(409, "native_input_expired_or_superseded")
	}
	req.ActorType, req.ActorID = authorTypeUser, b.OwnerUserID
	if claims.Capabilities == assistantBrokerAudience {
		if err := h.authorizeTaskEffect(c, claims, executionModeDesign); err != nil {
			return err
		}
		if err := h.Service.knownAnswerContext(ctx, b, *row, input, req); err != nil {
			return rejectOperation(403, "known_answer_scope_required")
		}
		req.ActorType, req.ActorID, req.SourceMemoryIDs = authorTypeAgent, b.OrchestratorID, req.MemoryIDs
	}
	return nil
}
func (s *Service) knownAnswerContext(ctx context.Context, b *models.AssistantBinding, row models.Attention, input *models.AttentionInput, req *attentionResponseRequest) error {
	if input.Kind != attentionKindQuestion || len(input.Questions) == 0 || len(req.MemoryIDs) == 0 || len(req.MemoryIDs) > 20 || req.Rejected {
		return errors.New("only delegable questions with confirmed sources")
	}
	if !allQuestionsDelegable(input.Questions) {
		return errors.New("question requires human input")
	}
	task, err := s.Tasks.GetTask(ctx, row.TaskID)
	if err != nil {
		return err
	}
	if err = s.ValidateDispatchContext(ctx, req.ContextRef, task, input.ProfileID); err != nil {
		return err
	}
	packet, err := s.currentPacket(ctx, b, req.ContextRef)
	if err != nil {
		return err
	}
	available := map[string]bool{}
	for _, memory := range packet.Memory {
		if memory.Confirmed && !memory.Truncated {
			available[memory.ID] = true
		}
	}
	for _, id := range req.MemoryIDs {
		if !available[id] {
			return errors.New("source unavailable in worker context")
		}
	}
	return nil
}

func currentInputMatches(input *models.AttentionInput, row *models.Attention, req *attentionResponseRequest) bool {
	return input != nil && input.Kind == row.Kind && input.State == models.AttentionPending && input.SourceRevision == req.SourceRevision && input.SessionID == req.SessionID && input.TaskID == row.TaskID && input.SourceID == row.SourceID
}

func allQuestionsDelegable(questions []models.InputQuestion) bool {
	for _, q := range questions {
		if !q.AssistantDelegable {
			return false
		}
	}
	return len(questions) > 0
}
