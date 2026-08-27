package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type RepositoryBranchPolicyHandlers struct {
	service *service.Service
	logger  *logger.Logger
}

func NewRepositoryBranchPolicyHandlers(svc *service.Service, log *logger.Logger) *RepositoryBranchPolicyHandlers {
	return &RepositoryBranchPolicyHandlers{service: svc, logger: log.WithFields(zap.String("component", "task-repository-branch-policy-handlers"))}
}

func RegisterRepositoryBranchPolicyRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *service.Service, log *logger.Logger) {
	h := NewRepositoryBranchPolicyHandlers(svc, log)
	h.registerHTTP(router)
	h.registerWS(dispatcher)
}

func (h *RepositoryBranchPolicyHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/repositories/:id/branch-policies", h.httpList)
	api.POST("/repositories/:id/branch-policies", h.httpCreate)
	api.POST("/repositories/:id/branch-policies/gitflow", h.httpGitflow)
	api.GET("/repository-branch-policies/:id", h.httpGet)
	api.PATCH("/repository-branch-policies/:id", h.httpUpdate)
	api.DELETE("/repository-branch-policies/:id", h.httpDelete)
}

func (h *RepositoryBranchPolicyHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionRepositoryBranchPolicyList, h.wsList)
	dispatcher.RegisterFunc(ws.ActionRepositoryBranchPolicyCreate, h.wsCreate)
	dispatcher.RegisterFunc(ws.ActionRepositoryBranchPolicyGet, h.wsGet)
	dispatcher.RegisterFunc(ws.ActionRepositoryBranchPolicyUpdate, h.wsUpdate)
	dispatcher.RegisterFunc(ws.ActionRepositoryBranchPolicyDelete, h.wsDelete)
	dispatcher.RegisterFunc(ws.ActionRepositoryBranchPolicyGitflow, h.wsGitflow)
}

type repositoryBranchPolicyCreateBody struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	BaseBranch        string `json:"base_branch"`
	BranchTemplate    string `json:"branch_template"`
	PullRequestTarget string `json:"pull_request_target"`
}

type repositoryBranchPolicyUpdateBody struct {
	Name              *string `json:"name"`
	Description       *string `json:"description"`
	BaseBranch        *string `json:"base_branch"`
	BranchTemplate    *string `json:"branch_template"`
	PullRequestTarget *string `json:"pull_request_target"`
}

type repositoryBranchPolicyGitflowBody struct {
	ProductionBranch  string `json:"production_branch"`
	DevelopmentBranch string `json:"development_branch"`
}

func branchPolicyListResponse(policies []*models.RepositoryBranchPolicy) dto.ListRepositoryBranchPoliciesResponse {
	items := make([]dto.RepositoryBranchPolicyDTO, 0, len(policies))
	for _, policy := range policies {
		if policy != nil {
			items = append(items, dto.FromRepositoryBranchPolicy(policy))
		}
	}
	return dto.ListRepositoryBranchPoliciesResponse{Policies: items, Total: len(items)}
}

func branchPolicyStatus(err error) int {
	switch {
	case errors.Is(err, service.ErrInvalidRepositoryBranchPolicy):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrRepositoryBranchPolicyNameConflict),
		errors.Is(err, service.ErrRepositoryBranchPolicyAlreadySeeded),
		errors.Is(err, service.ErrRepositoryBranchPolicyReadOnly):
		return http.StatusConflict
	case errors.Is(err, repoerrors.ErrRepositoryNotFound),
		errors.Is(err, repoerrors.ErrRepositoryBranchPolicyNotFound),
		errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func (h *RepositoryBranchPolicyHandlers) abort(c *gin.Context, action string, err error) {
	status := branchPolicyStatus(err)
	if status == http.StatusInternalServerError {
		h.logger.Error("repository branch policy request failed", zap.String("action", action), zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to " + action})
		return
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func (h *RepositoryBranchPolicyHandlers) httpList(c *gin.Context) {
	policies, err := h.service.ListRepositoryBranchPolicies(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.abort(c, "list repository branch policies", err)
		return
	}
	c.JSON(http.StatusOK, branchPolicyListResponse(policies))
}

func (h *RepositoryBranchPolicyHandlers) httpCreate(c *gin.Context) {
	var body repositoryBranchPolicyCreateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	policy, err := h.service.CreateRepositoryBranchPolicy(c.Request.Context(), &service.CreateRepositoryBranchPolicyRequest{
		RepositoryID: c.Param("id"), Name: body.Name, Description: body.Description,
		BaseBranch: body.BaseBranch, BranchTemplate: body.BranchTemplate, PullRequestTarget: body.PullRequestTarget,
	})
	if err != nil {
		h.abort(c, "create repository branch policy", err)
		return
	}
	c.JSON(http.StatusCreated, dto.FromRepositoryBranchPolicy(policy))
}

func (h *RepositoryBranchPolicyHandlers) httpGitflow(c *gin.Context) {
	var body repositoryBranchPolicyGitflowBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	policies, err := h.service.CreateGitflowRepositoryBranchPolicies(c.Request.Context(), &service.CreateGitflowRepositoryBranchPoliciesRequest{
		RepositoryID: c.Param("id"), ProductionBranch: body.ProductionBranch, DevelopmentBranch: body.DevelopmentBranch,
	})
	if err != nil {
		h.abort(c, "seed Gitflow repository branch policies", err)
		return
	}
	c.JSON(http.StatusCreated, branchPolicyListResponse(policies))
}

func (h *RepositoryBranchPolicyHandlers) httpGet(c *gin.Context) {
	policy, err := h.service.GetRepositoryBranchPolicy(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.abort(c, "get repository branch policy", err)
		return
	}
	c.JSON(http.StatusOK, dto.FromRepositoryBranchPolicy(policy))
}

func (h *RepositoryBranchPolicyHandlers) httpUpdate(c *gin.Context) {
	var body repositoryBranchPolicyUpdateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	policy, err := h.service.GetRepositoryBranchPolicy(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.abort(c, "get repository branch policy", err)
		return
	}
	updated, err := h.service.UpdateRepositoryBranchPolicy(c.Request.Context(), policy.ID, &service.UpdateRepositoryBranchPolicyRequest{
		Name: body.Name, Description: body.Description, BaseBranch: body.BaseBranch,
		BranchTemplate: body.BranchTemplate, PullRequestTarget: body.PullRequestTarget,
	})
	if err != nil {
		h.abort(c, "update repository branch policy", err)
		return
	}
	c.JSON(http.StatusOK, dto.FromRepositoryBranchPolicy(updated))
}

func (h *RepositoryBranchPolicyHandlers) httpDelete(c *gin.Context) {
	policy, err := h.service.GetRepositoryBranchPolicy(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.abort(c, "get repository branch policy", err)
		return
	}
	if err := h.service.DeleteRepositoryBranchPolicy(c.Request.Context(), policy.ID); err != nil {
		h.abort(c, "delete repository branch policy", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *RepositoryBranchPolicyHandlers) wsError(msg *ws.Message, action string, err error) (*ws.Message, error) {
	switch branchPolicyStatus(err) {
	case http.StatusBadRequest:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), taskErrorDetails(err))
	case http.StatusConflict:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, err.Error(), taskErrorDetails(err))
	case http.StatusNotFound:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), taskErrorDetails(err))
	default:
		h.logger.Error("repository branch policy request failed", zap.String("action", action), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to "+action, nil)
	}
}

func (h *RepositoryBranchPolicyHandlers) wsList(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		RepositoryID string `json:"repository_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	policies, err := h.service.ListRepositoryBranchPolicies(ctx, req.RepositoryID)
	if err != nil {
		return h.wsError(msg, "list repository branch policies", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, branchPolicyListResponse(policies))
}

func (h *RepositoryBranchPolicyHandlers) wsCreate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		RepositoryID string `json:"repository_id"`
		repositoryBranchPolicyCreateBody
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	policy, err := h.service.CreateRepositoryBranchPolicy(ctx, &service.CreateRepositoryBranchPolicyRequest{
		RepositoryID: req.RepositoryID, Name: req.Name, Description: req.Description,
		BaseBranch: req.BaseBranch, BranchTemplate: req.BranchTemplate, PullRequestTarget: req.PullRequestTarget,
	})
	if err != nil {
		return h.wsError(msg, "create repository branch policy", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepositoryBranchPolicy(policy))
}

func (h *RepositoryBranchPolicyHandlers) wsGet(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	policy, err := h.service.GetRepositoryBranchPolicy(ctx, req.ID)
	if err != nil {
		return h.wsError(msg, "get repository branch policy", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepositoryBranchPolicy(policy))
}

func (h *RepositoryBranchPolicyHandlers) wsUpdate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		ID string `json:"id"`
		repositoryBranchPolicyUpdateBody
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	updated, err := h.service.UpdateRepositoryBranchPolicy(ctx, req.ID, &service.UpdateRepositoryBranchPolicyRequest{
		Name: req.Name, Description: req.Description, BaseBranch: req.BaseBranch,
		BranchTemplate: req.BranchTemplate, PullRequestTarget: req.PullRequestTarget,
	})
	if err != nil {
		return h.wsError(msg, "update repository branch policy", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepositoryBranchPolicy(updated))
}

func (h *RepositoryBranchPolicyHandlers) wsDelete(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if err := h.service.DeleteRepositoryBranchPolicy(ctx, req.ID); err != nil {
		return h.wsError(msg, "delete repository branch policy", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, gin.H{"id": req.ID, "deleted": true})
}

func (h *RepositoryBranchPolicyHandlers) wsGitflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		RepositoryID string `json:"repository_id"`
		repositoryBranchPolicyGitflowBody
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	policies, err := h.service.CreateGitflowRepositoryBranchPolicies(ctx, &service.CreateGitflowRepositoryBranchPoliciesRequest{
		RepositoryID: req.RepositoryID, ProductionBranch: req.ProductionBranch, DevelopmentBranch: req.DevelopmentBranch,
	})
	if err != nil {
		return h.wsError(msg, "seed Gitflow repository branch policies", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, branchPolicyListResponse(policies))
}
