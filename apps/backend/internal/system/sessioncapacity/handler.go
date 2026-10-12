package sessioncapacity

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const responseErrorKey = "error"

func RegisterRoutes(read, admin *gin.RouterGroup, service *Service) {
	read.GET("/session-capacity/settings", handleGet(service))
	admin.PATCH("/session-capacity/settings", handleUpdate(service))
}

func handleGet(service *Service) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		response, err := service.Get(ctx.Request.Context())
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				responseErrorKey: "failed to load session capacity settings",
			})
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}

func handleUpdate(service *Service) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		patch, err := decodePatch(ctx.Request.Body)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				responseErrorKey: "invalid session capacity settings request",
			})
			return
		}
		response, err := service.Update(ctx.Request.Context(), patch)
		if err != nil {
			switch {
			case errors.Is(err, ErrValidation):
				ctx.JSON(http.StatusBadRequest, gin.H{
					responseErrorKey: "invalid session capacity settings",
				})
			case errors.Is(err, ErrEnvironmentLocked):
				ctx.JSON(http.StatusConflict, gin.H{
					responseErrorKey: "session capacity settings are controlled by the environment",
				})
			default:
				ctx.JSON(http.StatusInternalServerError, gin.H{
					responseErrorKey: "failed to save session capacity settings",
				})
			}
			return
		}
		ctx.JSON(http.StatusOK, response)
	}
}

func decodePatch(body io.Reader) (SettingsPatch, error) {
	if body == nil {
		return SettingsPatch{}, errors.New("request body is required")
	}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var patch SettingsPatch
	if err := decoder.Decode(&patch); err != nil {
		return SettingsPatch{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return SettingsPatch{}, errors.New("request contains multiple JSON values")
		}
		return SettingsPatch{}, err
	}
	return patch, nil
}
