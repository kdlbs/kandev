package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// --- personal access tokens (self-service) ---

type mintTokenRequest struct {
	Name     string `json:"name"`
	TTLHours int    `json:"ttl_hours"`
}

func (h *Handlers) mintToken(c *gin.Context) {
	identity, ok := requireIdentity(c)
	if !ok {
		return
	}
	var req mintTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	record, token, err := h.svc.MintToken(c.Request.Context(), identity.UserID, req.Name, req.TTLHours)
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	// The raw token is returned exactly once; only its hash is stored.
	c.JSON(http.StatusCreated, gin.H{"token": token, "record": record})
}

func (h *Handlers) listTokens(c *gin.Context) {
	identity, ok := requireIdentity(c)
	if !ok {
		return
	}
	tokens, err := h.svc.ListTokens(c.Request.Context(), identity.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tokens"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tokens": tokens})
}

func (h *Handlers) revokeToken(c *gin.Context) {
	identity, ok := requireIdentity(c)
	if !ok {
		return
	}
	if err := h.svc.RevokeToken(c.Request.Context(), c.Param("id"), identity.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke token"})
		return
	}
	c.Status(http.StatusNoContent)
}

// --- invites ---

type createInviteRequest struct {
	Email    string `json:"email"`
	Role     string `json:"role"`
	TTLHours int    `json:"ttl_hours"`
}

func (h *Handlers) createInvite(c *gin.Context) {
	identity, ok := requireIdentity(c)
	if !ok {
		return
	}
	var req createInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	invite, token, err := h.svc.CreateInvite(c.Request.Context(), identity.UserID, req.Email, req.Role, req.TTLHours)
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	// Raw token returned once; the admin shares /invite?token=... out of band.
	c.JSON(http.StatusCreated, gin.H{"invite": invite, "url": "/invite?token=" + token})
}

func (h *Handlers) listInvites(c *gin.Context) {
	invites, err := h.svc.ListInvites(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list invites"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"invites": invites})
}

func (h *Handlers) deleteInvite(c *gin.Context) {
	if err := h.svc.DeleteInvite(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete invite"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handlers) previewInvite(c *gin.Context) {
	invite, err := h.svc.InspectInvite(c.Request.Context(), c.Query("token"))
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"email": invite.Email, "role": invite.Role, "expires_at": invite.ExpiresAt})
}

type acceptInviteRequest struct {
	Token       string `json:"token"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (h *Handlers) acceptInvite(c *gin.Context) {
	var req acceptInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	user, token, err := h.svc.AcceptInvite(c.Request.Context(), req.Token, req.Email, req.Password, req.DisplayName, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	setSessionCookie(c, h.svc.CookieNameForRequest(c.Request), token, h.svc.SessionTTL())
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// --- admin user management ---

type createUserRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

func (h *Handlers) createUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	user, err := h.svc.AdminCreateUser(c.Request.Context(), req.Email, req.Password, req.DisplayName, req.Role)
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": user})
}

func (h *Handlers) listUsers(c *gin.Context) {
	users, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

type updateUserRequest struct {
	Role   string `json:"role"`
	Status string `json:"status"`
}

func (h *Handlers) updateUser(c *gin.Context) {
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	// Self-demotion/disable goes through the same last-admin guard; the
	// service allows it only when another active admin remains.
	user, err := h.svc.AdminSetRoleStatus(c.Request.Context(), c.Param("id"), req.Role, req.Status)
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}
