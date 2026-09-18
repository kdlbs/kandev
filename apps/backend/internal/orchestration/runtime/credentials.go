package runtime

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// CredentialHealthReader has no reveal or vault-list operation.
type CredentialHealthReader interface {
	CredentialHealth(context.Context, string, models.CredentialDescriptor) models.CredentialValidation
}

type credentialEdit struct {
	models.CredentialDescriptor
	Expected int64 `json:"expected_revision"`
}

func strictAssistantJSON(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 24*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		c.AbortWithStatus(422)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		c.AbortWithStatus(422)
		return false
	}
	return true
}

func validCredential(d models.CredentialDescriptor) bool {
	for _, value := range []string{d.ProfileID, d.Reference, d.Account, d.Purpose, d.UnlockPolicy} {
		if value == "" {
			return false
		}
	}
	if d.Resolver != "kandev" && d.Resolver != "bitwarden" {
		return false
	}
	switch d.Scope {
	case scopeWorkspace:
	case "project", "environment", scopeTask:
		if d.ScopeID == "" {
			return false
		}
	default:
		return false
	}
	if len(d.Fields) == 0 || len(d.Fields) > 10 {
		return false
	}
	for _, v := range append([]string{d.ID, d.ProfileID, d.Reference, d.Purpose, d.Account, d.Environment, d.ScopeID, d.UnlockPolicy}, d.Fields...) {
		if len(v) > 500 || strings.ContainsAny(v, "\r\n") || redaction.NewRedactor().String(v) != v {
			return false
		}
	}
	return true
}

func (h *Handler) editCredential(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req credentialEdit
	if !strictAssistantJSON(c, &req) {
		return
	}
	d := req.CredentialDescriptor
	d.ID = c.Param("id")
	d.BindingID = b.ID
	if !validCredential(d) || req.Expected < 0 {
		c.AbortWithStatus(422)
		return
	}
	profile, err := h.Service.Personas.Profiles.GetAgentProfile(c.Request.Context(), d.ProfileID)
	if err != nil || profile == nil || !profile.Enabled || profile.Role != "" || (profile.WorkspaceID != "" && profile.WorkspaceID != b.WorkspaceID) {
		c.AbortWithStatus(422)
		return
	}
	scopeID, err := h.Service.validateMemoryScope(c.Request.Context(), b, d.Scope, d.ScopeID)
	if err != nil {
		c.AbortWithStatus(422)
		return
	}
	d.ScopeID = scopeID
	if err := h.Service.Repo.SaveCredentialDescriptor(c.Request.Context(), &d, req.Expected); err != nil {
		memoryFailure(c, err)
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, h.Service.credentialObservation(c.Request.Context(), b, d))
}

func (h *Handler) credential(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	rows, err := h.Service.Repo.CredentialDescriptors(c.Request.Context(), b.ID)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	visible := make([]models.ContextCredential, 0, len(rows))
	for _, d := range rows {
		if _, err := h.Service.validateMemoryScope(c.Request.Context(), b, d.Scope, d.ScopeID); err != nil {
			continue
		}
		observed := h.Service.credentialObservation(c.Request.Context(), b, d)
		if id := c.Param("id"); id != "" {
			if d.ID == id {
				c.JSON(200, observed)
				return
			}
		} else {
			visible = append(visible, observed)
		}
	}
	if c.Param("id") != "" {
		c.AbortWithStatus(404)
		return
	}
	c.JSON(200, gin.H{"credentials": visible})
}

func (h *Handler) forgetCredential(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req struct {
		Expected int64 `json:"expected_revision"`
	}
	if !strictAssistantJSON(c, &req) {
		return
	}
	if req.Expected < 1 {
		c.AbortWithStatus(422)
		return
	}
	if err := h.Service.Repo.ForgetCredentialDescriptor(c.Request.Context(), b.ID, c.Param("id"), req.Expected); err != nil {
		memoryFailure(c, err)
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, gin.H{"forgotten": true})
}

func (s *Service) contextCredentials(ctx context.Context, b *models.AssistantBinding, p *models.ContextPacket) error {
	rows, err := s.Repo.CredentialDescriptors(ctx, b.ID)
	if err != nil {
		return err
	}
	for _, d := range rows {
		if !d.Matches(b, p.ContextScope) {
			continue
		}
		if _, err := s.validateMemoryScope(ctx, b, d.Scope, d.ScopeID); err != nil {
			continue
		}
		p.Credentials = append(p.Credentials, s.credentialObservation(ctx, b, d))
	}
	return nil
}
