package restart

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func HandleCapability(manager Manager) gin.HandlerFunc {
	if manager == nil {
		manager = NewUnsupportedManager("")
	}
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, manager.Capability(c.Request.Context()))
	}
}

func HandleRequest(manager Manager) gin.HandlerFunc {
	if manager == nil {
		manager = NewUnsupportedManager("")
	}
	return func(c *gin.Context) {
		resp, err := manager.RequestRestart(c.Request.Context())
		if errors.Is(err, ErrUnsupported) {
			c.JSON(http.StatusNotImplemented, resp)
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, RestartResponse{
				Accepted: false,
				Message:  err.Error(),
			})
			return
		}
		c.JSON(http.StatusAccepted, resp)
	}
}
