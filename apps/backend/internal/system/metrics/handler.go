package metrics

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(g *gin.RouterGroup, svc *Service) {
	g.GET("/metrics/settings", handleGetSettings(svc))
	g.PATCH("/metrics/settings", handleSaveSettings(svc))
}

func handleGetSettings(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := svc.GetSettings(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"settings": settings})
	}
}

func handleSaveSettings(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req GlobalSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		settings, err := svc.SaveSettings(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"settings": settings})
	}
}
