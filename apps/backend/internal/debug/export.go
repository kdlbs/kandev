package debug

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/logger/buffer"
)

var startTime = time.Now()

// ExportMetadata contains system runtime information for debugging.
type ExportMetadata struct {
	UptimeSeconds float64 `json:"uptime_seconds"`
	Version       string  `json:"version"`
	Commit        string  `json:"commit"`
	GoVersion     string  `json:"go_version"`
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	Goroutines    int     `json:"goroutines"`
	HeapAllocMB   float64 `json:"heap_alloc_mb"`
	CapturedAt    string  `json:"captured_at"`
}

// ExportResponse is the JSON response for the debug export endpoint.
type ExportResponse struct {
	Metadata ExportMetadata `json:"metadata"`
	Logs     []buffer.Entry `json:"logs"`
}

// RegisterExportRoute registers GET /api/v1/system/debug/export.
func RegisterExportRoute(router *gin.Engine, version, commit string, _ *logger.Logger) {
	api := router.Group("/api/v1/system/debug")
	api.GET("/export", handleExport(version, commit))
}

func handleExport(version, commit string) gin.HandlerFunc {
	return func(c *gin.Context) {
		logs := buffer.Default().Snapshot()

		// Optional level filter.
		if level := c.Query("level"); level != "" {
			filtered := make([]buffer.Entry, 0, len(logs))
			for _, e := range logs {
				if e.Level == level {
					filtered = append(filtered, e)
				}
			}
			logs = filtered
		}

		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		resp := ExportResponse{
			Metadata: ExportMetadata{
				UptimeSeconds: time.Since(startTime).Seconds(),
				Version:       version,
				Commit:        commit,
				GoVersion:     runtime.Version(),
				OS:            runtime.GOOS,
				Arch:          runtime.GOARCH,
				Goroutines:    runtime.NumGoroutine(),
				HeapAllocMB:   float64(mem.HeapAlloc) / (1024 * 1024),
				CapturedAt:    time.Now().UTC().Format(time.RFC3339),
			},
			Logs: logs,
		}

		c.JSON(http.StatusOK, resp)
	}
}
