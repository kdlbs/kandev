package updates

import (
	"errors"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

// RegisterRoutes wires the updates endpoints onto the existing system API
// group. Callers may either call this helper or use HandleGet / HandleCheck
// directly when composing routes elsewhere.
func RegisterRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	api := router.Group("/api/v1/system")
	api.GET("/updates", HandleGet(svc))
	api.POST("/updates/check", HandleCheck(svc))
	if log != nil {
		log.Debug("Registered System Updates handlers (HTTP)")
	}
}

// HandleGet returns the cached kandev_meta view of latest version. It never
// hits GitHub. Errors from the meta read are surfaced as 500.
func HandleGet(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := svc.Get()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// HandleCheck triggers a synchronous GitHub poll. When the per-process
// limiter denies the request a 429 is returned with retry_after_seconds.
// Other errors are surfaced as 502 since the upstream is GitHub.
func HandleCheck(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := svc.Check(c.Request.Context())
		if errors.Is(err, ErrRateLimited) {
			retry := svc.RetryAfter()
			seconds := int64(math.Ceil(retry.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":               ErrRateLimited.Error(),
				"retry_after_seconds": seconds,
			})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}
