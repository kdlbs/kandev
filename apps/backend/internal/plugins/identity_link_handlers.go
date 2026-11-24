package plugins

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/store"
)

type identityLinkCodeResponse struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expires_at"`
}

type identityLinksResponse struct {
	Links []IdentityLinkDTO `json:"links"`
}

func identityLinkCaller(ctx *gin.Context) (authn.Identity, bool) {
	identity, authenticated := authn.FromGin(ctx)
	if !authenticated || identity.UserID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{actionErrorField: authenticationRequiredMessage})
		return authn.Identity{}, false
	}
	return identity, true
}

// createIdentityLinkCode issues a single-use code that links the external
// account presenting it, through this plugin, to the signed-in user.
func (c *Controller) createIdentityLinkCode(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	identity, ok := identityLinkCaller(ctx)
	if !ok {
		return
	}
	code, expiresAt, err := c.svc.CreateIdentityLinkCode(ctx.Param("id"), identity.UserID)
	if err != nil {
		c.writeIdentityLinkError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, identityLinkCodeResponse{Code: code, ExpiresAt: expiresAt.Format(time.RFC3339Nano)})
}

// listIdentityLinks returns the caller's links; scope=all returns every link
// of the installation and requires an instance admin.
func (c *Controller) listIdentityLinks(ctx *gin.Context) {
	identity, ok := identityLinkCaller(ctx)
	if !ok {
		return
	}
	all := ctx.Query("scope") == "all"
	if all && !identity.IsAdmin() {
		ctx.JSON(http.StatusForbidden, gin.H{actionErrorField: "admin access required"})
		return
	}
	links, err := c.svc.ListIdentityLinks(ctx.Request.Context(), ctx.Param("id"), identity.UserID, all)
	if err != nil {
		c.writeIdentityLinkError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, identityLinksResponse{Links: links})
}

// revokeIdentityLink removes the caller's own link, or any link for an
// instance admin.
func (c *Controller) revokeIdentityLink(ctx *gin.Context) {
	identity, ok := identityLinkCaller(ctx)
	if !ok {
		return
	}
	if err := c.svc.RevokeIdentityLink(ctx.Request.Context(), ctx.Param("id"), ctx.Param("link_id"), identity.UserID, identity.IsAdmin()); err != nil {
		c.writeIdentityLinkError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (c *Controller) writeIdentityLinkError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, ErrIdentityLinkNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{actionErrorField: "identity link not found"})
	case errors.Is(err, ErrIdentityLinkNotDeclared):
		ctx.JSON(http.StatusConflict, gin.H{actionErrorField: "plugin does not support linked accounts"})
	case errors.Is(err, ErrIdentityLinkForbidden):
		ctx.JSON(http.StatusForbidden, gin.H{actionErrorField: "identity link belongs to another user"})
	default:
		ctx.JSON(http.StatusServiceUnavailable, gin.H{actionErrorField: "identity linking is unavailable"})
	}
}
