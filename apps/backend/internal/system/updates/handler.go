package updates

import (
	"context"
	"errors"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
)

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

type applyRequestBody struct {
	Confirm string `json:"confirm"`
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

// HandleApply queues a service-managed self-update. It is deliberately gated
// behind service metadata and a browser same-origin check because the helper
// mutates the local Kandev installation and restarts the service.
func HandleApply(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sameOriginOrNoOrigin(c.Request) {
			c.JSON(http.StatusForbidden, gin.H{"error": "cross-origin update apply is not allowed"})
			return
		}
		var req applyRequestBody
		_ = c.ShouldBindJSON(&req)
		jobID, err := svc.Apply(context.Background(), req.Confirm)
		if errors.Is(err, ErrApplyConfirm) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, ErrNoUpdateAvailable) || errors.Is(err, ErrApplyUnsupported) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, ApplyResponse{JobID: jobID})
	}
}
