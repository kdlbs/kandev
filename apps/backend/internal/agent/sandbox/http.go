package sandbox

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func (s *Service) RegisterRoutes(router *gin.Engine) {
	if !s.enabled {
		return
	}
	group := router.Group("/api/v1/sandbox", authn.RequireRealIdentity())
	s.registerApprovalRoutes(group)
	s.registerLearningRoutes(group)
	group.GET("/sessions/:id/attempts/:attempt/report", s.readReportHTTP)
	s.registerCatalogRoutes(group)
	group.POST("/policies", s.publishHTTP)
	group.GET("/revisions/:id", func(c *gin.Context) {
		revision, err := s.GetRevision(c.Request.Context(), c.Param("id"))
		sandboxJSON(c, http.StatusOK, revision, err)
	})
	group.POST("/revisions/:id/revoke", s.revokeHTTP)
	group.PUT("/bindings/:scope/:id", s.putBindingHTTP)
	group.GET("/bindings/:scope/:id", func(c *gin.Context) {
		binding, err := s.GetBinding(c.Request.Context(), c.Param("scope"), c.Param("id"))
		sandboxJSON(c, http.StatusOK, binding, err)
	})
}

func (s *Service) publishHTTP(c *gin.Context) {
	var request struct {
		Policy   Policy `json:"policy"`
		Expected int64  `json:"expected_revision"`
	}
	if !decodeSandboxRequest(c, &request) {
		return
	}
	revision, err := s.Publish(c.Request.Context(), request.Policy, request.Expected)
	sandboxJSON(c, http.StatusCreated, revision, err)
}

func (s *Service) putBindingHTTP(c *gin.Context) {
	var request struct {
		Binding  Binding `json:"binding"`
		Expected int64   `json:"expected_version"`
	}
	if !decodeSandboxRequest(c, &request) {
		return
	}
	if request.Binding.Scope != c.Param("scope") || request.Binding.TargetID != c.Param("id") {
		sandboxJSON(c, 0, nil, contract.ErrInvalidPolicy)
		return
	}
	binding, err := s.PutBinding(c.Request.Context(), request.Binding, request.Expected)
	sandboxJSON(c, http.StatusOK, binding, err)
}

func (s *Service) revokeHTTP(c *gin.Context) {
	var request struct {
		Digest string `json:"digest"`
	}
	if !decodeSandboxRequest(c, &request) {
		return
	}
	err := s.Revoke(c.Request.Context(), c.Param("id"), request.Digest)
	sandboxJSON(c, http.StatusOK, gin.H{"revoked": err == nil}, err)
}

func decodeSandboxRequest(c *gin.Context, value any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		sandboxJSON(c, 0, nil, contract.ErrInvalidPolicy)
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		sandboxJSON(c, 0, nil, contract.ErrInvalidPolicy)
		return false
	}
	return true
}

func sandboxJSON(c *gin.Context, status int, value any, err error) {
	if err == nil {
		c.JSON(status, value)
		return
	}
	status, code := sandboxHTTPError(err)
	c.JSON(status, gin.H{"error": code})
}

func sandboxHTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrDisabled), errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "sandbox_unavailable"
	case errors.Is(err, ErrAuthority):
		return http.StatusForbidden, "sandbox_operator_required"
	case errors.Is(err, ErrConflict):
		return http.StatusConflict, "sandbox_revision_conflict"
	case errors.Is(err, contract.ErrInvalidPolicy):
		return http.StatusBadRequest, "sandbox_invalid_policy"
	case errors.Is(err, ErrIneligible), errors.Is(err, contract.ErrOutsideCeiling):
		return http.StatusUnprocessableEntity, "sandbox_policy_ineligible"
	default:
		return http.StatusInternalServerError, "sandbox_operation_failed"
	}
}
