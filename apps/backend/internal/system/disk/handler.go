package disk

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HandleGet serves GET /api/v1/system/disk-usage. Returns the cached
// breakdown (or null while computing) plus the Computing flag so the
// client can render a spinner without polling on a fixed interval.
func HandleGet(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result := s.Get(c.Request.Context())
		c.JSON(http.StatusOK, result)
	}
}

// HandleRefresh serves POST /api/v1/system/disk-usage/refresh and
// always kicks an async walk regardless of cache TTL. Returns
// 202 Accepted with the job_id so the client can correlate the
// subsequent system.job.update events.
func HandleRefresh(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		jobID := s.Refresh(c.Request.Context())
		c.JSON(http.StatusAccepted, gin.H{"job_id": jobID})
	}
}
