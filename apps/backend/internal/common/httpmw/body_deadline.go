package httpmw

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// BoundRequestBody applies one budget to body reads and downstream processing.
// The read deadline remains installed for net/http's unread-body drain; the
// server resets it when admitting the next keep-alive request.
func BoundRequestBody(c *gin.Context, budget time.Duration, maxBytes int64) context.CancelFunc {
	bounded, cancel := context.WithTimeout(c.Request.Context(), budget)
	c.Request = c.Request.WithContext(bounded)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	deadline, _ := bounded.Deadline()
	_ = http.NewResponseController(c.Writer).SetReadDeadline(deadline)
	return cancel
}
