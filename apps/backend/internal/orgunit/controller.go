package orgunit

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
)

// Controller serves the unit tree.
type Controller struct {
	service *Service
	log     *logger.Logger
}

// NewController builds the controller.
func NewController(service *Service, log *logger.Logger) *Controller {
	return &Controller{service: service, log: log.WithFields(zap.String("component", "org-unit-controller"))}
}

// RegisterRoutes mounts the unit API.
//
// Reading the tree needs only an identity: a member has to see the shape of
// their organization to understand why they reach what they reach. Changing it
// needs `unit.manage`, which is where the shape of an organization is decided.
func (c *Controller) RegisterRoutes(router *gin.Engine) {
	read := router.Group("/api/v1/units", authn.RequireRealIdentity())
	read.GET("", c.list)
	read.GET("/:id/members", c.listMembers)

	write := router.Group("/api/v1/units",
		authn.RequireRealIdentity(), authz.RequireOrgScope(authz.ScopeUnitManage))
	write.POST("", c.create)
	write.PATCH("/:id", c.patch)
	write.DELETE("/:id", c.delete)
	write.PUT("/:id/members/:userId", c.setMember)
	write.DELETE("/:id/members/:userId", c.removeMember)
}

func (c *Controller) list(ctx *gin.Context) {
	subject := authz.SubjectFromGin(ctx)
	units, err := c.service.Store().ListByOrg(ctx.Request.Context(), subject.OrgID)
	if err != nil {
		c.fail(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"units": units, "total": len(units)})
}

func (c *Controller) listMembers(ctx *gin.Context) {
	id := ctx.Param("id")
	// Reads need the tenant check as much as writes: without it, knowing a
	// unit id from another organization is enough to enumerate its members.
	if !c.unitInCallerOrg(ctx, id) {
		return
	}
	members, err := c.service.Store().ListMembers(ctx.Request.Context(), id)
	if err != nil {
		c.fail(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"members": members, "total": len(members)})
}

type createUnitRequest struct {
	ParentID string `json:"parent_id"`
	Name     string `json:"name"`
}

func (c *Controller) create(ctx *gin.Context) {
	var req createUnitRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// A caller must not be able to graft a unit onto another tenant's tree, so
	// the parent is validated against the caller's own organization.
	if !c.parentInCallerOrg(ctx, req.ParentID) {
		return
	}
	unit, err := c.service.Create(ctx.Request.Context(), req.ParentID, req.Name)
	if err != nil {
		c.fail(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, unit)
}

type patchUnitRequest struct {
	Name     *string `json:"name,omitempty"`
	ParentID *string `json:"parent_id,omitempty"`
}

func (c *Controller) patch(ctx *gin.Context) {
	var req patchUnitRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id := ctx.Param("id")
	if !c.unitInCallerOrg(ctx, id) {
		return
	}
	if req.Name != nil {
		if err := c.service.Rename(ctx.Request.Context(), id, *req.Name); err != nil {
			c.fail(ctx, err)
			return
		}
	}
	if req.ParentID != nil {
		if !c.parentInCallerOrg(ctx, *req.ParentID) {
			return
		}
		if err := c.service.Move(ctx.Request.Context(), id, *req.ParentID); err != nil {
			c.fail(ctx, err)
			return
		}
	}
	unit, err := c.service.Store().Get(ctx.Request.Context(), id)
	if err != nil {
		c.fail(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, unit)
}

func (c *Controller) delete(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.unitInCallerOrg(ctx, id) {
		return
	}
	if err := c.service.Delete(ctx.Request.Context(), id); err != nil {
		c.fail(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

type setMemberRequest struct {
	Role string `json:"role"`
}

func (c *Controller) setMember(ctx *gin.Context) {
	var req setMemberRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id := ctx.Param("id")
	if !c.unitInCallerOrg(ctx, id) {
		return
	}
	role := authz.NormalizeWorkspaceRole(req.Role)
	if role == authz.WorkspaceRoleNone {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "role must be owner, collaborator, or viewer"})
		return
	}
	actor, _ := authn.FromGin(ctx)
	if err := c.service.SetMember(ctx.Request.Context(), id, ctx.Param("userId"), string(role), actor.UserID); err != nil {
		c.fail(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

func (c *Controller) removeMember(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.unitInCallerOrg(ctx, id) {
		return
	}
	if err := c.service.RemoveMember(ctx.Request.Context(), id, ctx.Param("userId")); err != nil {
		c.fail(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// unitInCallerOrg answers 404 rather than 403 for a unit in another tenant, so
// the tree of one organization is not enumerable from another.
func (c *Controller) unitInCallerOrg(ctx *gin.Context, id string) bool {
	unit, err := c.service.Store().Get(ctx.Request.Context(), id)
	if err != nil {
		c.fail(ctx, err)
		return false
	}
	subject := authz.SubjectFromGin(ctx)
	if !subject.Unscoped && subject.OrgID != unit.OrgID {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "unit not found"})
		return false
	}
	return true
}

func (c *Controller) parentInCallerOrg(ctx *gin.Context, parentID string) bool {
	if parentID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": ErrParentRequired.Error()})
		return false
	}
	return c.unitInCallerOrg(ctx, parentID)
}

// fail maps a tree error to its status. Each refusal names its blocking
// condition, so a caller can say what is in the way rather than that something
// went wrong.
func (c *Controller) fail(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUnitNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNotEmpty), errors.Is(err, ErrCycle),
		errors.Is(err, ErrProtectedUnit), errors.Is(err, ErrPersonalNoMember),
		errors.Is(err, ErrCrossOrgParent), errors.Is(err, ErrNameRequired),
		errors.Is(err, ErrParentRequired):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.log.Error("unit request failed", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "unit request failed"})
	}
}
