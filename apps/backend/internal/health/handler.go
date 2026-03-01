package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
)

// RegisterRoutes registers the system health endpoint.
func RegisterRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	api := router.Group("/api/v1/system")
	api.GET("/health", handleGetHealth(svc))
	log.Debug("Registered System Health handlers (HTTP)")
}

func handleGetHealth(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result := svc.RunChecks(c.Request.Context())
		c.JSON(http.StatusOK, result)
	}
}
