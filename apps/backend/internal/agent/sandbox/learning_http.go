package sandbox

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Service) registerLearningRoutes(group *gin.RouterGroup) {
	group.POST("/sessions/:id/attempts/:attempt/learning", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		var request LearningRequest
		if !decodeSandboxRequest(c, &request) {
			return
		}
		proposal, err := s.ProposeLearning(c.Request.Context(), c.Param("id"), c.Param("attempt"), request)
		sandboxJSON(c, http.StatusCreated, proposal, err)
	})
	group.GET("/learning/:id", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		proposal, err := s.ReadLearning(c.Request.Context(), c.Param("id"))
		sandboxJSON(c, http.StatusOK, proposal, err)
	})
	group.POST("/learning/:id/publish", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		var request struct {
			Digest string `json:"expected_digest"`
		}
		if !decodeSandboxRequest(c, &request) {
			return
		}
		revision, err := s.PublishLearning(c.Request.Context(), c.Param("id"), request.Digest)
		sandboxJSON(c, http.StatusCreated, revision, err)
	})
}
