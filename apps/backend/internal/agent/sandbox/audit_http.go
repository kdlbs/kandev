package sandbox

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetAuditVerifier binds core-owned verification dispatch before HTTP serving starts.
func (s *Service) SetAuditVerifier(enqueue func(context.Context, string, string, AuditVerificationRequest) (*AuditVerification, error)) {
	s.enqueueAudit = enqueue
}

func (s *Service) registerAuditRoutes(group *gin.RouterGroup) {
	group.POST("/sessions/:id/attempts/:attempt/audit/verifications", s.verifyAuditHTTP)
	group.GET("/sessions/:id/attempts/:attempt/audit/verifications", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		page, err := s.ListAuditVerifications(c.Request.Context(), c.Param("id"), c.Param("attempt"), c.Query("cursor"))
		sandboxJSON(c, http.StatusOK, page, err)
	})
	group.GET("/sessions/:id/attempts/:attempt/audit/verifications/:verification", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		view, err := s.ReadAuditVerification(c.Request.Context(), c.Param("id"), c.Param("attempt"), c.Param("verification"))
		sandboxJSON(c, http.StatusOK, view, err)
	})
	group.GET("/sessions/:id/attempts/:attempt/audit/trust", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		revision, err := s.GetAuditTrust(c.Request.Context(), c.Param("id"), c.Param("attempt"))
		sandboxJSON(c, http.StatusOK, revision, err)
	})
	group.PUT("/sessions/:id/attempts/:attempt/audit/trust", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		var request AuditTrustRequest
		if !decodeSandboxRequest(c, &request) {
			return
		}
		revision, err := s.SetAuditTrust(c.Request.Context(), c.Param("id"), c.Param("attempt"), request)
		if err == nil {
			_, err = s.authorizeReport(c.Request.Context(), c.Param("id"), c.Param("attempt"))
		}
		sandboxJSON(c, http.StatusOK, revision, err)
	})
}

func (s *Service) verifyAuditHTTP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var request AuditVerificationRequest
	if !decodeSandboxRequest(c, &request) {
		return
	}
	view, err := s.RequestAuditVerification(c.Request.Context(), c.Param("id"), c.Param("attempt"), request)
	sandboxJSON(c, http.StatusAccepted, view, err)
}

func (s *Service) RequestAuditVerification(ctx context.Context, session, attempt string, request AuditVerificationRequest) (*AuditVerificationView, error) {
	if _, err := s.operator(ctx); err != nil {
		return nil, err
	}
	if _, err := s.authorizeReport(ctx, session, attempt); err != nil {
		return nil, err
	}
	if s.enqueueAudit == nil {
		return nil, ErrIneligible
	}
	job, err := s.enqueueAudit(ctx, session, attempt, request)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, ErrConflict
	}
	return s.ReadAuditVerification(ctx, session, attempt, job.ID)
}
