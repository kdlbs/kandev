package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

func (h *Handlers) httpProbeAgentProfile(c *gin.Context) {
	agentName := strings.TrimSpace(c.Param("agentName"))
	if agentName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent name is required"})
		return
	}
	var req dto.ProfileCapabilityRequest
	if err := decodeStrictSettingsJSON(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid profile discovery request"})
		return
	}
	scope, ok := requireProfileDiscoveryScope(c)
	if !ok {
		return
	}
	req.AuthorizationScope = scope
	resp, err := h.controller.FetchProfileDynamicModels(c.Request.Context(), agentName, req)
	if err != nil {
		writeProfileDiscoveryError(c, h.logger, "failed to probe agent profile", err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpResolveAgentModelConfig(c *gin.Context) {
	agentName := strings.TrimSpace(c.Param("agentName"))
	if agentName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent name is required"})
		return
	}
	var req dto.ResolveAgentModelConfigRequest
	if err := decodeStrictSettingsJSON(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model resolution request"})
		return
	}
	if req.ProfileID != "" || req.LaunchSettings != nil {
		scope, ok := requireProfileDiscoveryScope(c)
		if !ok {
			return
		}
		req.AuthorizationScope = scope
	}
	resp, err := h.controller.ResolveAgentModelConfig(c.Request.Context(), agentName, req)
	if err != nil {
		if errors.Is(err, controller.ErrModelRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model is required"})
			return
		}
		if errors.Is(err, controller.ErrAgentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		if errors.Is(err, controller.ErrProfileDiscoveryMissing) {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}
		if errors.Is(err, controller.ErrInvalidProfileDiscovery) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid profile discovery context"})
			return
		}
		h.logger.Error("failed to resolve agent model options", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve agent model options"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func decodeStrictSettingsJSON(c *gin.Context, value any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func requireProfileDiscoveryScope(c *gin.Context) (string, bool) {
	identity, ok := authn.FromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return "", false
	}
	if !authz.SubjectOrgScopes(authz.SubjectFromGin(c)).Has(authz.ScopeOrgConfigManage) {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions", "scope": string(authz.ScopeOrgConfigManage)})
		return "", false
	}
	scope := identity.UserID
	if identity.Synthetic {
		scope = "synthetic"
	}
	if scope == "" {
		scope = identity.OrgID
	}
	if scope == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return "", false
	}
	return scope, true
}

func writeProfileDiscoveryError(c *gin.Context, log *logger.Logger, message string, err error) {
	switch {
	case errors.Is(err, controller.ErrAgentNotFound), errors.Is(err, controller.ErrProfileDiscoveryMissing):
		c.JSON(http.StatusNotFound, gin.H{"error": "profile discovery target not found"})
	case errors.Is(err, controller.ErrInvalidProfileDiscovery):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid profile discovery context"})
	default:
		log.Error(message, zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": message})
	}
}
