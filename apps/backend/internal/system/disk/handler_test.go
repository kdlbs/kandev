package disk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
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
	var opener string
	switch runtime.GOOS {
	case "linux":
		opener = "xdg-open"
	case "darwin":
		opener = "open"
	case "windows":
		opener = "explorer"
	default:
		t.Skipf("open-folder handler not exercised on %s", runtime.GOOS)
	}
	// The handler shells out to the platform's file-manager opener and only
	// returns 200 when the binary is actually present on PATH. Minimal CI
	// images (e.g. our `kandev-ci` container) omit `xdg-utils`, which makes
	// the assertion impossible — skip rather than asserting a false-positive.
	if _, err := exec.LookPath(opener); err != nil {
		t.Skipf("%s not on PATH (%v); handler integration cannot be exercised here", opener, err)
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
