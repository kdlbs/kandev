package azuredevops

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
)

// RegisterMockRoutes mounts E2E controls only when the service owns a mock.
func RegisterMockRoutes(router *gin.Engine, service *Service, log *logger.Logger) {
	if service == nil || service.MockClient() == nil {
		return
	}
	api := router.Group("/api/v1/azure-devops/mock")
	api.POST("/state", func(ctx *gin.Context) {
		var state MockState
		if err := ctx.ShouldBindJSON(&state); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
		service.MockClient().Seed(state)
		ctx.JSON(http.StatusOK, gin.H{"seeded": true})
	})
	api.DELETE("/state", func(ctx *gin.Context) {
		service.MockClient().Seed(MockState{Authenticated: true, User: TestConnectionResult{OK: true, ID: "mock-user", DisplayName: "Mock User"}})
		ctx.JSON(http.StatusOK, gin.H{"reset": true})
	})
	log.Info("registered Azure DevOps mock control endpoints")
}
