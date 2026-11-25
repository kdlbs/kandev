package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
)

// extendUpdateStatusWriteDeadline lets a synchronous runtime-update status or
// preview response outlive the server-wide write timeout, so that a native
// check that reaches its own timeout can still deliver its reason. A writer
// without deadline support keeps the server default.
func extendUpdateStatusWriteDeadline(c *gin.Context) {
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(controller.UpdateStatusResponseBudget))
}
