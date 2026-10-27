package sandbox

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Service) registerApprovalRoutes(group *gin.RouterGroup) {
	group.GET("/sessions/:id/approvals", func(c *gin.Context) {
		approvals, err := s.ListApprovals(c.Request.Context(), c.Param("id"))
		sandboxJSON(c, http.StatusOK, approvals, err)
	})
	group.POST("/attempts/:attempt/approvals/:request/decision", s.decideApprovalHTTP)
}

func (s *Service) decideApprovalHTTP(c *gin.Context) {
	var request struct {
		Expected int64  `json:"expected_version"`
		Decision string `json:"decision"`
	}
	if !decodeSandboxRequest(c, &request) {
		return
	}
	approval, err := s.DecideApproval(c.Request.Context(), c.Param("attempt"), c.Param("request"), request.Expected, request.Decision)
	if err != nil {
		sandboxJSON(c, 0, nil, err)
		return
	}
	sandboxJSON(c, http.StatusOK, s.approvalView(c.Request.Context(), approval), nil)
}
