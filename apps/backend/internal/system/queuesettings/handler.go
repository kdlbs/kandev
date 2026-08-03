package queuesettings

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const responseErrorKey = "error"

func RegisterRoutes(read, admin *gin.RouterGroup, service *Service) {
	read.GET("/message-queue/settings", handleGet(service))
	admin.PATCH("/message-queue/settings", handleUpdate(service))
}

func handleGet(service *Service) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		response, err := service.Get(ctx.Request.Context())
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: "failed to load message queue settings"})
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}

func handleUpdate(service *Service) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var settings Settings
		if err := ctx.ShouldBindJSON(&settings); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "invalid request"})
			return
		}
		response, err := service.Update(ctx.Request.Context(), settings)
		if err != nil {
			switch {
			case errors.Is(err, ErrValidation):
				ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
			case errors.Is(err, ErrEnvironmentLocked):
				ctx.JSON(http.StatusConflict, gin.H{responseErrorKey: err.Error()})
			default:
				ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: "failed to save message queue settings"})
			}
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}
