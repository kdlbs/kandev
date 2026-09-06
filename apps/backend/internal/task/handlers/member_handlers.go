package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/service"
)

// MemberHandlers serves workspace membership and the
// reduced user directory that backs the member picker.
type MemberHandlers struct {
	service *service.Service
	logger  *logger.Logger
}

func NewMemberHandlers(svc *service.Service, log *logger.Logger) *MemberHandlers {
	return &MemberHandlers{
		service: svc,
		logger:  log.WithFields(zap.String("component", "workspace-member-handlers")),
	}
}

// RegisterMemberRoutes mounts the membership surface.
func RegisterMemberRoutes(router *gin.Engine, svc *service.Service, log *logger.Logger) {
	h := NewMemberHandlers(svc, log)
	api := router.Group("/api/v1")
	api.GET("/workspaces/:id/members", h.listMembers)
	api.PUT("/workspaces/:id/members/:userId", h.putMember)
	api.DELETE("/workspaces/:id/members/:userId", h.deleteMember)
	api.POST("/workspaces/:id/transfer-ownership", h.transferOwnership)
	api.GET("/users/directory", h.listDirectory)
	api.GET("/authz/scopes", h.listScopes)
}

type memberDTO struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	AddedBy     string `json:"added_by,omitempty"`
}

func (h *MemberHandlers) listMembers(c *gin.Context) {
	ctx := c.Request.Context()
	members, err := h.service.ListWorkspaceMembers(ctx, c.Param("id"))
	if err != nil {
		h.respondError(c, err, "failed to list workspace members")
		return
	}
	names := h.displayNames(c)
	out := make([]memberDTO, 0, len(members))
	for _, member := range members {
		out = append(out, memberDTO{
			UserID:      member.UserID,
			DisplayName: names[member.UserID],
			Role:        member.Role,
			AddedBy:     member.AddedBy,
		})
	}
	c.JSON(http.StatusOK, gin.H{"members": out, "total": len(out)})
}

type putMemberRequest struct {
	Role string `json:"role"`
}

func (h *MemberHandlers) putMember(c *gin.Context) {
	var body putMemberRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	member, err := h.service.UpsertWorkspaceMember(c.Request.Context(), c.Param("id"), c.Param("userId"), body.Role)
	if err != nil {
		h.respondError(c, err, "failed to add workspace member")
		return
	}
	c.JSON(http.StatusOK, memberDTO{UserID: member.UserID, Role: member.Role, AddedBy: member.AddedBy})
}

func (h *MemberHandlers) deleteMember(c *gin.Context) {
	if err := h.service.RemoveWorkspaceMember(c.Request.Context(), c.Param("id"), c.Param("userId")); err != nil {
		h.respondError(c, err, "failed to remove workspace member")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type transferOwnershipRequest struct {
	UserID string `json:"user_id"`
}

func (h *MemberHandlers) transferOwnership(c *gin.Context) {
	var body transferOwnershipRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.UserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	if err := h.service.TransferWorkspaceOwnership(c.Request.Context(), c.Param("id"), body.UserID); err != nil {
		h.respondError(c, err, "failed to transfer workspace ownership")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *MemberHandlers) listDirectory(c *gin.Context) {
	users, err := h.service.ListDirectoryUsers(c.Request.Context())
	if err != nil {
		h.respondError(c, err, "failed to list users")
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users, "total": len(users)})
}

// listScopes exposes the registry so the UI labels roles from one source
// rather than keeping its own copy of what each role can do.
func (h *MemberHandlers) listScopes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"scopes": authz.Registry()})
}

func (h *MemberHandlers) displayNames(c *gin.Context) map[string]string {
	names := make(map[string]string)
	users, err := h.service.ListDirectoryUsers(c.Request.Context())
	if err != nil {
		return names
	}
	for _, user := range users {
		names[user.ID] = user.DisplayName
	}
	return names
}

// respondError maps membership failures to status codes. Validation failures
// carry their message through so the UI can say what went wrong; anything
// unrecognized stays generic.
func (h *MemberHandlers) respondError(c *gin.Context, err error, message string) {
	switch {
	case service.IsForbidden(err):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case isNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	case isMemberValidationError(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		h.logger.Error(message, zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": message})
	}
}

func isMemberValidationError(err error) bool {
	for _, sentinel := range []error{
		service.ErrMemberUserNotFound,
		service.ErrMemberUserDisabled,
		service.ErrMemberIsOwner,
		service.ErrMemberRoleInvalid,
		service.ErrMemberSelf,
		service.ErrTransferTargetNotMember,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}
