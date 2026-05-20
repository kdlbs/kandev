package debug

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger/buffer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleExport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Push a test entry into the default buffer.
	buffer.Default().Push(buffer.Entry{
		Level:   "error",
		Logger:  "test",
		Caller:  "debug/export_test.go:20",
		Message: "something went wrong",
		Fields:  map[string]any{"task_id": "abc"},
	})

	router := gin.New()
	RegisterExportRoute(router, "1.0.0-test", "deadbeef", nil)

	t.Run("returns metadata and logs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/system/debug/export", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ExportResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		assert.Equal(t, "1.0.0-test", resp.Metadata.Version)
		assert.Equal(t, "deadbeef", resp.Metadata.Commit)
		assert.NotEmpty(t, resp.Metadata.GoVersion)
		assert.NotEmpty(t, resp.Metadata.OS)
		assert.NotEmpty(t, resp.Metadata.Arch)
		assert.Greater(t, resp.Metadata.Goroutines, 0)
		assert.Greater(t, resp.Metadata.UptimeSeconds, 0.0)
		assert.NotEmpty(t, resp.Metadata.CapturedAt)

		assert.NotEmpty(t, resp.Logs)
	})

	t.Run("filters by level", func(t *testing.T) {
		buffer.Default().Push(buffer.Entry{
			Level:   "info",
			Message: "normal operation",
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/system/debug/export?level=error", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp ExportResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		for _, entry := range resp.Logs {
			assert.Equal(t, "error", entry.Level)
		}
	})
}
