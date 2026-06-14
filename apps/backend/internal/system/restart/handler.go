package restart

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Capability struct {
	Supported bool   `json:"supported"`
	Mode      string `json:"mode"`
	Reason    string `json:"reason,omitempty"`
}

type RestartResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message"`
}

const unsupportedReason = "Automatic restart is not available for this launch mode. Restart Kandev from the terminal or service manager."

func HandleCapability() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, Capability{
			Supported: false,
			Mode:      "manual",
			Reason:    unsupportedReason,
		})
	}
}

func HandleRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, RestartResponse{
			Accepted: false,
			Message:  unsupportedReason,
		})
	}
}
