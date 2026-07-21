package disk

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"

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

// HandleOpenFolder serves POST /api/v1/system/disk-usage/open and reveals the
// configured data directory in the host OS file explorer. The path is always
// the resolved home directory the disk walker uses — there is no user input,
// so no shell-escaping concerns.
func HandleOpenFolder(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := s.HomeDir()
		if path == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "data directory is not configured"})
			return
		}
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", path)
		case "linux":
			cmd = exec.Command("xdg-open", path)
		case "windows":
			cmd = exec.Command("explorer", path)
		default:
			c.JSON(http.StatusNotImplemented, gin.H{"error": fmt.Sprintf("unsupported platform: %s", runtime.GOOS)})
			return
		}
		if err := cmd.Start(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Reap the helper in the background so we don't leak zombies; we don't
		// block the HTTP response on the file manager opening.
		go func() { _ = cmd.Wait() }()
		c.JSON(http.StatusOK, gin.H{"path": path})
	}
}
