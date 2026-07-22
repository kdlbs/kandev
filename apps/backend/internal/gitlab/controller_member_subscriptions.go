package gitlab

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	providerActionFailedMessage = "GitLab action failed"
	reviewerIneligibleMessage   = "selected reviewer is not eligible"
)

func (c *Controller) registerMemberSubscriptionRoutes(api *gin.RouterGroup) {
	api.GET("/projects/members", c.httpListProjectMembers)
	api.PUT("/mrs/reviewers", c.httpSetMRReviewers)
	api.GET("/mrs/subscription", c.httpGetMRSubscription)
	api.PUT("/mrs/subscription", c.httpSetMRSubscription)
	api.GET("/issues/subscription", c.httpGetIssueSubscription)
	api.PUT("/issues/subscription", c.httpSetIssueSubscription)
}

func (c *Controller) httpListProjectMembers(ctx *gin.Context) {
	project := strings.TrimSpace(ctx.Query("project"))
	if project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "project query parameter required"})
		return
	}
	var members []ProjectMember
	err := c.runWorkspaceClientAction(ctx, func(client Client) error {
		var actionErr error
		members, actionErr = client.ListProjectMembers(ctx.Request.Context(), project, ctx.Query("query"))
		return actionErr
	})
	if err != nil {
		writeWorkspaceClientActionError(ctx, err, "project members")
		return
	}
	ctx.JSON(http.StatusOK, members)
}

func (c *Controller) httpSetMRReviewers(ctx *gin.Context) {
	var req struct {
		Project     string   `json:"project"`
		IID         int      `json:"iid"`
		ReviewerIDs *[]int64 `json:"reviewer_ids"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Project) == "" || req.IID <= 0 || req.ReviewerIDs == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "project, positive iid, and reviewer_ids are required"})
		return
	}
	var mr *MR
	err := c.runWorkspaceClientAction(ctx, func(client Client) error {
		if actionErr := client.SetMRReviewers(ctx.Request.Context(), req.Project, req.IID, *req.ReviewerIDs); actionErr != nil {
			return actionErr
		}
		var actionErr error
		mr, actionErr = client.GetMR(ctx.Request.Context(), req.Project, req.IID)
		return actionErr
	})
	if err != nil {
		writeReviewerActionError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, mr)
}

func (c *Controller) httpGetMRSubscription(ctx *gin.Context) {
	c.httpGetSubscription(ctx, false)
}

func (c *Controller) httpSetMRSubscription(ctx *gin.Context) {
	c.httpSetSubscription(ctx, false)
}

func (c *Controller) httpGetIssueSubscription(ctx *gin.Context) {
	c.httpGetSubscription(ctx, true)
}

func (c *Controller) httpSetIssueSubscription(ctx *gin.Context) {
	c.httpSetSubscription(ctx, true)
}

func (c *Controller) httpGetSubscription(ctx *gin.Context, issue bool) {
	project, iid, err := parseProjectAndIID(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
		return
	}
	var state *SubscriptionState
	err = c.runWorkspaceClientAction(ctx, func(client Client) error {
		if issue {
			state, err = client.GetIssueSubscription(ctx.Request.Context(), project, iid)
		} else {
			state, err = client.GetMRSubscription(ctx.Request.Context(), project, iid)
		}
		return err
	})
	if err != nil {
		writeWorkspaceClientActionError(ctx, err, "subscription read")
		return
	}
	ctx.JSON(http.StatusOK, state)
}

func (c *Controller) httpSetSubscription(ctx *gin.Context, issue bool) {
	var req struct {
		Project    string `json:"project"`
		IID        int    `json:"iid"`
		Subscribed *bool  `json:"subscribed"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Project) == "" || req.IID <= 0 || req.Subscribed == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "project, positive iid, and subscribed are required"})
		return
	}
	var state *SubscriptionState
	err := c.runWorkspaceClientAction(ctx, func(client Client) error {
		var actionErr error
		if issue {
			state, actionErr = client.SetIssueSubscription(ctx.Request.Context(), req.Project, req.IID, *req.Subscribed)
		} else {
			state, actionErr = client.SetMRSubscription(ctx.Request.Context(), req.Project, req.IID, *req.Subscribed)
		}
		return actionErr
	})
	if err != nil {
		writeWorkspaceClientActionError(ctx, err, "subscription update")
		return
	}
	ctx.JSON(http.StatusOK, state)
}

func writeReviewerActionError(ctx *gin.Context, err error) {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{responseErrorKey: reviewerIneligibleMessage})
		return
	}
	writeProviderActionError(ctx, err, "reviewer update")
}

func writeProviderActionError(ctx *gin.Context, err error, action string) {
	status := http.StatusBadGateway
	message := providerActionFailedMessage + ": " + action
	switch {
	case errors.Is(err, ErrWorkspaceRequired):
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "workspace_id query parameter required"})
		return
	case errors.Is(err, ErrNotConfigured):
		ctx.JSON(http.StatusServiceUnavailable, gin.H{responseErrorKey: providerActionFailedMessage + ": connection resolution"})
		return
	case errors.Is(err, ErrWorkspaceHostMismatch):
		ctx.JSON(http.StatusNotFound, gin.H{responseErrorKey: "GitLab resource not found"})
		return
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			status = http.StatusForbidden
			message = "GitLab permission denied"
		case http.StatusNotFound:
			status = http.StatusNotFound
			message = "GitLab resource not found"
		}
	}
	ctx.JSON(status, gin.H{responseErrorKey: message})
}
