package org

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
)

// Controller serves the organization surface.
//
// Two route groups with deliberately different audiences:
//
//   - /api/v1/orgs/current is the caller's OWN organization. It never accepts
//     an org ID, because accepting one would be the first step toward a
//     request choosing its tenant.
//   - /api/v1/instance/orgs is the operator tier: managing organizations
//     themselves. An operator holds no org scopes, so nothing here reads or
//     writes any organization's workspaces, tasks or secrets.
type Controller struct {
	service *Service
	log     *logger.Logger
}

// NewController builds the controller.
func NewController(service *Service, log *logger.Logger) *Controller {
	return &Controller{service: service, log: log.WithFields(zap.String("component", "org-controller"))}
}

// RegisterRoutes mounts the organization API. With organizations off nothing
// is mounted, so the surface simply does not exist.
func (c *Controller) RegisterRoutes(router *gin.Engine) {
	if !c.service.Enabled() {
		return
	}
	current := router.Group("/api/v1/orgs/current", authn.RequireRealIdentity())
	current.GET("", c.getCurrent)

	instance := router.Group("/api/v1/instance/orgs", authn.RequireRealIdentity(), RequireOperator())
	instance.GET("", c.list)
	instance.POST("", c.create)
	instance.PATCH("/:id", c.patch)
	instance.DELETE("/:id", c.delete)
	instance.POST("/:id/admins", c.createFirstAdmin)
}

// RequireOperator aborts unless the caller holds the instance operator tier.
//
// It responds 404 rather than 403: on an instance where the caller is not an
// operator, the organization-management surface should not appear to exist.
func RequireOperator() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		identity, ok := authn.FromGin(ctx)
		if !ok || !identity.Instance {
			ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		ctx.Next()
	}
}

func (c *Controller) getCurrent(ctx *gin.Context) {
	identity, _ := authn.FromGin(ctx)
	if identity.OrgID == "" {
		ctx.JSON(http.StatusOK, gin.H{"org": nil, "is_operator": identity.Instance})
		return
	}
	org, err := c.service.Get(ctx.Request.Context(), identity.OrgID)
	if err != nil {
		c.respond(ctx, err, "failed to read the organization")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"org": org, "is_operator": identity.Instance})
}

func (c *Controller) list(ctx *gin.Context) {
	orgs, err := c.service.List(ctx.Request.Context())
	if err != nil {
		c.respond(ctx, err, "failed to list organizations")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"orgs": orgs, "total": len(orgs)})
}

type createRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (c *Controller) create(ctx *gin.Context) {
	var body createRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	org, err := c.service.Create(ctx.Request.Context(), body.Name, body.Slug)
	if err != nil {
		c.respond(ctx, err, "failed to create the organization")
		return
	}
	ctx.JSON(http.StatusOK, org)
}

type patchRequest struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (c *Controller) patch(ctx *gin.Context) {
	var body patchRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	var (
		org *Org
		err error
	)
	if body.Name != "" {
		org, err = c.service.Rename(ctx.Request.Context(), ctx.Param("id"), body.Name)
	}
	if err == nil && body.Status != "" {
		org, err = c.service.SetStatus(ctx.Request.Context(), ctx.Param("id"), body.Status)
	}
	if err != nil {
		c.respond(ctx, err, "failed to update the organization")
		return
	}
	if org == nil {
		org, err = c.service.Get(ctx.Request.Context(), ctx.Param("id"))
		if err != nil {
			c.respond(ctx, err, "failed to read the organization")
			return
		}
	}
	ctx.JSON(http.StatusOK, org)
}

type firstAdminRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// createFirstAdmin provisions an organization's first administrator. It is
// operator-only and names the org explicitly, which is the one place an org ID
// legitimately appears in a request: the caller is acting on organizations
// themselves rather than inside one.
func (c *Controller) createFirstAdmin(ctx *gin.Context) {
	var body firstAdminRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	err := c.service.CreateFirstAdmin(
		ctx.Request.Context(), ctx.Param("id"), body.Email, body.Password, body.DisplayName)
	if err != nil {
		c.respond(ctx, err, "failed to create the organization administrator")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

type deleteRequest struct {
	Slug string `json:"slug"`
}

func (c *Controller) delete(ctx *gin.Context) {
	var body deleteRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := c.service.Delete(ctx.Request.Context(), ctx.Param("id"), body.Slug); err != nil {
		c.respond(ctx, err, "failed to delete the organization")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// respond maps organization errors to status codes, keeping each refusal's
// reason visible so an operator is told what actually blocked them.
func (c *Controller) respond(ctx *gin.Context, err error, message string) {
	switch {
	case errors.Is(err, ErrOrgNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "organization not found"})
	case errors.Is(err, ErrSlugMismatch), errors.Is(err, ErrSlugTaken),
		errors.Is(err, ErrNameRequired), errors.Is(err, ErrCannotDeleteLast),
		errors.Is(err, ErrCannotDeleteActive):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.log.Error(message, zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": message})
	}
}
