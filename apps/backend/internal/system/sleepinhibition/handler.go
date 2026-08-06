package sleepinhibition

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(read, admin *gin.RouterGroup, service *Service) {
	read.GET("/sleep-inhibition", handleGet(service))
	admin.PATCH("/sleep-inhibition", handleUpdate(service))
}

func handleGet(service *Service) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		response, err := service.Get(ctx.Request.Context())
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sleep inhibition settings"})
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}

func handleUpdate(service *Service) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var request struct {
			Enabled *bool `json:"enabled"`
		}
		if err := ctx.ShouldBindJSON(&request); err != nil || request.Enabled == nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "enabled must be a boolean"})
			return
		}
		response, err := service.Update(ctx.Request.Context(), Settings{Enabled: *request.Enabled})
		if err != nil {
			if errors.Is(err, ErrInvalidPersisted) {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load sleep inhibition settings"})
				return
			}
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save sleep inhibition settings"})
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}
