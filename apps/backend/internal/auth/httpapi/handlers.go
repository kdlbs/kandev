// Package httpapi exposes the authentication HTTP surface:
//
//	POST   /api/v1/auth/setup            public (setup mode only)
//	POST   /api/v1/auth/login            public, rate-limited
//	POST   /api/v1/auth/logout           session
//	GET    /api/v1/auth/me               any
//	PATCH  /api/v1/auth/password         session
//	GET    /api/v1/auth/sessions         session
//	DELETE /api/v1/auth/sessions/:id     session
//	GET    /api/v1/auth/tokens           session
//	POST   /api/v1/auth/tokens           session (returns raw PAT once)
//	DELETE /api/v1/auth/tokens/:id       session
//	GET    /api/v1/auth/invites          admin
//	POST   /api/v1/auth/invites          admin (returns invite URL once)
//	DELETE /api/v1/auth/invites/:id      admin
//	GET    /api/v1/auth/invites/preview  public (token inspection for the accept page)
//	POST   /api/v1/auth/invites/accept   public
//	GET    /api/v1/users                 admin
//	POST   /api/v1/users                 admin
//	PATCH  /api/v1/users/:id             admin
//
// Authentication is enabled/disabled through the `features.auth` runtime flag
// (Settings > System > Feature Toggles), not an endpoint here.
package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
)

// Handlers carries the auth HTTP handler dependencies.
type Handlers struct {
	svc *auth.Service
	log *logger.Logger
}

// RegisterRoutes mounts the auth API on the main router.
func RegisterRoutes(router *gin.Engine, svc *auth.Service, log *logger.Logger) {
	h := &Handlers{svc: svc, log: log}
	group := router.Group("/api/v1/auth")
	// Public bootstrap + credential-issuing endpoints (allowlisted in httpmw).
	group.POST("/setup", h.setup)
	group.POST("/login", h.login)
	group.POST("/logout", h.logout)
	group.GET("/me", h.me)
	group.GET("/invites/preview", h.previewInvite)
	group.POST("/invites/accept", h.acceptInvite)

	// Credential/session management requires a real (non-synthetic) session:
	// while auth is disabled the synthetic admin must not mint PATs or manage
	// sessions — those credentials would outlive enablement.
	managed := group.Group("", authn.RequireRealIdentity())
	managed.PATCH("/password", h.changePassword)
	managed.GET("/sessions", h.listSessions)
	managed.DELETE("/sessions/:id", h.revokeSession)
	managed.GET("/tokens", h.listTokens)
	managed.POST("/tokens", h.mintToken)
	managed.DELETE("/tokens/:id", h.revokeToken)

	// RequireAdmin allows the synthetic admin, so RequireRealIdentity must
	// precede it to keep these locked while auth is disabled.
	admin := group.Group("", authn.RequireRealIdentity(), authz.RequireOrgScope(authz.ScopeOrgMembersManage))
	admin.GET("/invites", h.listInvites)
	admin.POST("/invites", h.createInvite)
	admin.DELETE("/invites/:id", h.deleteInvite)

	users := router.Group("/api/v1/users", authn.RequireRealIdentity(), authz.RequireOrgScope(authz.ScopeOrgMembersManage))
	users.GET("", h.listUsers)
	users.POST("", h.createUser)
	users.PATCH("/:id", h.updateUser)
}

type credentialsRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (h *Handlers) setup(c *gin.Context) {
	var req credentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	user, token, err := h.svc.Setup(c.Request.Context(), req.Email, req.Password, req.DisplayName, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	setSessionCookie(c, h.svc.CookieNameForRequest(c.Request), token, h.svc.SessionTTL())
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handlers) login(c *gin.Context) {
	var req credentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	user, token, err := h.svc.Login(c.Request.Context(), req.Email, req.Password, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	setSessionCookie(c, h.svc.CookieNameForRequest(c.Request), token, h.svc.SessionTTL())
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// logout terminates the current session (if any) and clears its port-scoped
// session cookie. The unprefixed base name is deliberately left untouched —
// on a host serving a default-port instance it is that instance's live
// session cookie (see spec: no proactive legacy scrubbing).
func (h *Handlers) logout(c *gin.Context) {
	identity, ok := authn.FromGin(c)
	if ok && identity.SessionID != "" {
		if err := h.svc.Logout(c.Request.Context(), identity.SessionID, identity.UserID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "logout failed"})
			return
		}
	}
	// Clear only the effective (port-scoped) name. The unprefixed base name is
	// deliberately left alone: on a host serving a default-port instance (or a
	// not-yet-upgraded one), it is that instance's LIVE session cookie, not an
	// inert legacy jar — expiring it would log the other instance out (see
	// docs/specs/fix-multi-instance-cookie-isolation/spec.md: the upgraded
	// instance does not proactively delete the legacy cookie).
	clearSessionCookie(c, h.svc.CookieNameForRequest(c.Request))
	c.Status(http.StatusNoContent)
}

func (h *Handlers) me(c *gin.Context) {
	var identityPtr *authn.Identity
	if identity, ok := authn.FromGin(c); ok {
		identityPtr = &identity
	}
	c.JSON(http.StatusOK, h.svc.StateFor(c.Request.Context(), identityPtr))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *Handlers) changePassword(c *gin.Context) {
	identity, ok := requireIdentity(c)
	if !ok {
		return
	}
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), identity.UserID, identity.SessionID, req.CurrentPassword, req.NewPassword); err != nil {
		h.writeAuthError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) listSessions(c *gin.Context) {
	identity, ok := requireIdentity(c)
	if !ok {
		return
	}
	sessions, err := h.svc.ListSessions(c.Request.Context(), identity.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}
	items := make([]gin.H, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, gin.H{
			"id":           session.ID,
			"created_at":   session.CreatedAt,
			"last_seen_at": session.LastSeenAt,
			"user_agent":   session.UserAgent,
			"ip":           session.IP,
			"current":      session.ID == identity.SessionID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"sessions": items})
}

func (h *Handlers) revokeSession(c *gin.Context) {
	identity, ok := requireIdentity(c)
	if !ok {
		return
	}
	if err := h.svc.RevokeSession(c.Request.Context(), c.Param("id"), identity.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke session"})
		return
	}
	c.Status(http.StatusNoContent)
}

// requireIdentity is a belt-and-braces guard: the global middleware already
// rejects unauthenticated /api requests, but handlers still verify before
// acting on the caller's user ID.
func requireIdentity(c *gin.Context) (authn.Identity, bool) {
	identity, ok := authn.FromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return authn.Identity{}, false
	}
	return identity, true
}

func (h *Handlers) writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
	case errors.Is(err, auth.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	// 403, not 401: the credential was accepted, so the browser must not
	// treat this as a session challenge and loop back to the sign-in form.
	case errors.Is(err, auth.ErrOrgUnavailable):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "code": "org_suspended"})
	case errors.Is(err, auth.ErrSetupNotAvailable):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, auth.ErrInviteInvalid):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, auth.ErrEmailTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, auth.ErrLastAdmin):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, auth.ErrValidation):
		c.JSON(http.StatusBadRequest, gin.H{"error": "email must be valid and password at least 8 characters"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}
