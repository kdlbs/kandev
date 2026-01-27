package httpmw

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

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

		if status >= 500 {
			log.Error("http",
				zap.String("server", serverName),
				zap.String("method", c.Request.Method),
				zap.String("path", path),
				zap.Int("status", status),
				zap.Int64("duration_ms", latency.Milliseconds()),
				zap.Int("bytes", size),
			)
		} else {
			log.Debug("http",
				zap.String("server", serverName),
				zap.String("method", c.Request.Method),
				zap.String("path", path),
				zap.Int("status", status),
				zap.Int64("duration_ms", latency.Milliseconds()),
				zap.Int("bytes", size),
			)
		}
	}
}
