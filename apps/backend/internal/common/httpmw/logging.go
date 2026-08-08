package httpmw

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// statusClientClosedRequest is nginx's 499, returned by handlers when the
// caller abandoned the request. Go defines no constant for it.
const statusClientClosedRequest = 499

// RequestLogger logs HTTP request details after the handler completes.
func RequestLogger(log *logger.Logger, serverName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		size := c.Writer.Size()
		if size < 0 {
			size = 0
		}

		fields := []zap.Field{
			zap.String("server", serverName),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Int64("duration_ms", latency.Milliseconds()),
			zap.Int("bytes", size),
		}
		switch {
		// 404 and 499 are routine: one is a miss, the other is the caller
		// hanging up mid-request. Neither means something went wrong here.
		case status >= 400 && status != 404 && status != statusClientClosedRequest:
			log.Warn("http", fields...)
		default:
			log.Debug("http", fields...)
		}
	}
}
