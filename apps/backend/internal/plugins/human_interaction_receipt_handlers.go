package plugins

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type issueHumanInteractionResponseReceiptRequest struct {
	WorkspaceID             string                                     `json:"workspace_id"`
	InteractionID           string                                     `json:"interaction_id"`
	ExpectedResourceVersion string                                     `json:"expected_resource_version"`
	Kind                    string                                     `json:"kind"`
	OptionID                string                                     `json:"option_id,omitempty"`
	Cancelled               bool                                       `json:"cancelled,omitempty"`
	Answers                 []issueHumanInteractionClarificationAnswer `json:"answers,omitempty"`
}

type issueHumanInteractionClarificationAnswer struct {
	QuestionID      string   `json:"question_id"`
	SelectedOptions []string `json:"selected_options,omitempty"`
	CustomText      string   `json:"custom_text,omitempty"`
}

type issueHumanInteractionResponseReceiptResponse struct {
	ID              string `json:"id"`
	InteractionID   string `json:"interaction_id"`
	ResourceVersion string `json:"resource_version"`
	ExpiresAt       string `json:"expires_at"`
}

//nolint:cyclop // The endpoint validates one authenticated human response before issuing its receipt.
func (c *Controller) issueHumanInteractionResponseReceipt(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	var request issueHumanInteractionResponseReceiptRequest
	if err := c.bindCapabilityApprovalJSON(ctx, &request); err != nil {
		return
	}
	identity, authenticated := authn.FromGin(ctx)
	if !authenticated || identity.UserID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	workspaceID := request.WorkspaceID
	if !isBoundedApprovalIdentifier(workspaceID) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}
	if c.svc.humanInteractionResponseAuthorizer == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "human response authorization is unavailable"})
		return
	}
	if err := c.svc.humanInteractionResponseAuthorizer(ctx.Request.Context(), workspaceID); err != nil {
		if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		} else {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "workspace session-control permission is required"})
		}
		return
	}
	response := HumanInteractionResponse{Kind: request.Kind, OptionID: request.OptionID, Cancelled: request.Cancelled}
	switch request.Kind {
	case string(models.InteractionKindPermission):
		if len(request.Answers) != 0 || (request.Cancelled && request.OptionID != "") || (!request.Cancelled && request.OptionID == "") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid human permission response"})
			return
		}
	case string(models.InteractionKindClarification):
		if request.OptionID != "" || request.Cancelled || len(request.Answers) == 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid human clarification response"})
			return
		}
		response.Answers = make([]pluginsdk.ClarificationAnswer, len(request.Answers))
		for i, answer := range request.Answers {
			response.Answers[i] = pluginsdk.ClarificationAnswer{QuestionID: answer.QuestionID, SelectedOptions: answer.SelectedOptions, CustomText: answer.CustomText}
		}
	default:
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "unsupported human interaction response"})
		return
	}
	receipt, err := c.svc.IssueHumanInteractionResponseReceipt(ctx.Request.Context(), identity.UserID, workspaceID,
		request.InteractionID, request.ExpectedResourceVersion, response)
	if err != nil {
		writeHumanInteractionReceiptError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, issueHumanInteractionResponseReceiptResponse(receipt))
}

func writeHumanInteractionReceiptError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrHumanInteractionResponseInvalid):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid human interaction response"})
	case errors.Is(err, ErrHumanInteractionNotPending):
		ctx.JSON(http.StatusConflict, gin.H{"error": "interaction is no longer pending"})
	case errors.Is(err, ErrHumanInteractionWorkspaceMismatch):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "interaction not found"})
	case errors.Is(err, ErrHumanInteractionVersionChanged):
		ctx.JSON(http.StatusConflict, gin.H{"error": "interaction changed; reload before responding"})
	default:
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "human interaction response service unavailable"})
	}
}
