package jobs

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HandleGet renders GET /api/v1/system/jobs/:id. Returns 404 when the job is
// unknown. Used by the frontend as a polling fallback in case the
// `system.job.update` WS broadcast was missed (which happens when the WS
// connection isn't open at the moment the job transitions). The same Job
// shape is published over the bus, so the polling path and the WS path feed
// into the same store slice.
func HandleGet(t *Tracker) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		job := t.Get(id)
		if job == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusOK, job)
	}
}
