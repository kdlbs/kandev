package disk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/system/jobs"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newDiskRouter(svc *Service) *gin.Engine {
	r := gin.New()
	g := r.Group("/api/v1/system")
	g.POST("/disk-usage/open", HandleOpenFolder(svc))
	return r
}

func TestHandleOpenFolder_NoHomeDirReturns503(t *testing.T) {
	svc := NewService("", jobs.NewTracker(nil, logger.Default()), logger.Default())
	r := newDiskRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/disk-usage/open", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestHandleOpenFolder_ReturnsPathOnSupportedOS(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		t.Skipf("open-folder handler not exercised on %s", runtime.GOOS)
	}

	home := t.TempDir()
	svc := NewService(home, jobs.NewTracker(nil, logger.Default()), logger.Default())
	r := newDiskRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/disk-usage/open", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want, _ := filepath.Abs(home)
	if resp.Path != want {
		t.Errorf("path = %q, want %q", resp.Path, want)
	}
}

func TestHandleOpenFolder_UnsupportedPlatformReturns501(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("only runs on unsupported GOOS")
	}
	home := t.TempDir()
	svc := NewService(home, jobs.NewTracker(nil, logger.Default()), logger.Default())
	r := newDiskRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/disk-usage/open", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", w.Code)
	}
}
