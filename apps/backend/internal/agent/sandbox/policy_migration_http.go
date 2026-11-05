package sandbox

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Service) registerMigrationRoutes(group *gin.RouterGroup) {
	group.POST("/revisions/:id/migration/preview", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		var request PolicyMigrationRequest
		if !decodeSandboxRequest(c, &request) {
			return
		}
		preview, err := s.PreviewPolicyMigration(c.Request.Context(), c.Param("id"), request)
		sandboxJSON(c, http.StatusOK, preview, err)
	})
	group.POST("/revisions/:id/migration/publish", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		var request struct {
			PolicyMigrationRequest
			ReviewDigest string `json:"expected_review_digest"`
		}
		if !decodeSandboxRequest(c, &request) {
			return
		}
		revision, err := s.PublishPolicyMigration(c.Request.Context(), c.Param("id"), request.PolicyMigrationRequest, request.ReviewDigest)
		sandboxJSON(c, http.StatusCreated, revision, err)
	})
}
