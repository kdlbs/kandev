package sandbox

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Service) registerCatalogRoutes(group *gin.RouterGroup) {
	group.POST("/availability", func(c *gin.Context) {
		var request PreviewContext
		if !decodeSandboxRequest(c, &request) {
			return
		}
		result, err := s.CheckAvailability(c.Request.Context(), request)
		sandboxJSON(c, http.StatusOK, result, err)
	})
	group.GET("/policies", func(c *gin.Context) {
		catalog, err := s.ListPolicies(c.Request.Context(), c.Query("workspace_id"), c.Query("after"))
		sandboxJSON(c, http.StatusOK, catalog, err)
	})
	group.GET("/providers", func(c *gin.Context) {
		providers, err := s.ListProviders(c.Request.Context())
		sandboxJSON(c, http.StatusOK, gin.H{"providers": providers}, err)
	})
	group.POST("/preview", func(c *gin.Context) {
		var request PreviewContext
		if !decodeSandboxRequest(c, &request) {
			return
		}
		preview, err := s.PreviewSelection(c.Request.Context(), request)
		sandboxJSON(c, http.StatusOK, preview, err)
	})
}
