package github

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
)

func (c *Controller) httpCreateCIRunGrant(ctx *gin.Context) {
	identity, ok := authn.FromGin(ctx)
	if !ok || !identity.IsAdmin() {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "administrator authorization is required"})
		return
	}
	var input CreateCIRunGrantInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid CI run grant payload"})
		return
	}
	grant, err := c.service.CreateCIRunGrant(ctx.Request.Context(), identity.UserID, input)
	if err != nil {
		writeCIRunGrantError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, grant)
}

func (c *Controller) httpRevokeCIRunGrant(ctx *gin.Context) {
	identity, ok := authn.FromGin(ctx)
	if !ok || !identity.IsAdmin() {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "administrator authorization is required"})
		return
	}
	workspaceID := strings.TrimSpace(ctx.Query("workspace_id"))
	if err := c.service.RevokeCIRunGrant(ctx.Request.Context(), identity.UserID,
		workspaceID, strings.TrimSpace(ctx.Param("grantId"))); err != nil {
		writeCIRunGrantError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"revoked": true})
}

func writeCIRunGrantError(ctx *gin.Context, err error) {
	var requestErr *CIRunRequestError
	if errors.As(err, &requestErr) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "failure_class": requestErr.Class})
		return
	}
	ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}
