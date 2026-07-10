package logs

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Default and maximum line counts for the tail endpoint.
const (
	defaultTailLines = 1000
	maxTailLines     = 10_000
)

// HandleList renders GET /api/v1/system/logs: the directory listing.
func HandleList(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		files, err := s.List()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"files": files})
	}
}

// HandleTail renders GET /api/v1/system/logs/tail?n=<lines>. n defaults to
// 1000 and is capped at 10000. Invalid values silently fall back to the
// default — the endpoint is read-only and lenient.
func HandleTail(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		n := resolveTailN(c.Query("n"))
		lines, err := s.Tail(n)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"lines": lines})
	}
}

// HandleDownload streams a single log file. The :name path parameter is
// validated to be a bare filename within the configured log directory;
// anything else (path traversal, separators, missing file) returns 4xx.
func HandleDownload(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		f, size, err := s.Open(name)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		defer func() { _ = f.Close() }()

		c.Header("Content-Type", "application/octet-stream")
		c.Header("Content-Length", strconv.FormatInt(size, 10))
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, f)
	}
}

// resolveTailN parses ?n=… , falling back to the default for empty or
// unparseable values, and clamping to [0, maxTailLines].
func resolveTailN(raw string) int {
	if raw == "" {
		return defaultTailLines
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultTailLines
	}
	if v < 0 {
		return 0
	}
	if v > maxTailLines {
		return maxTailLines
	}
	return v
}
