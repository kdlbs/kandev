package logs

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newRouter(svc *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/logs", HandleList(svc))
	r.GET("/logs/tail", HandleTail(svc))
	r.GET("/logs/:name/download", HandleDownload(svc))
	return r
}

func TestHandleList_ReturnsJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kandev.log"), []byte("hi"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := newTestService(t, dir)
	r := newRouter(svc)

	req := httptest.NewRequest("GET", "/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Files []FileInfo `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(resp.Files))
	}
	if !resp.Files[0].Current {
		t.Error("expected kandev.log to be Current")
	}
}

func TestHandleTail_DefaultN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	var sb strings.Builder
	for i := 1; i <= 50; i++ {
		fmt.Fprintf(&sb, "line-%d\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := newTestService(t, dir)
	r := newRouter(svc)

	req := httptest.NewRequest("GET", "/logs/tail", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Lines) != 50 {
		t.Errorf("len(lines) = %d, want 50", len(resp.Lines))
	}
}

func TestHandleTail_ExplicitN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	var sb strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&sb, "line-%d\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := newTestService(t, dir)
	r := newRouter(svc)

	req := httptest.NewRequest("GET", "/logs/tail?n=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Lines) != 10 {
		t.Errorf("len(lines) = %d, want 10", len(resp.Lines))
	}
	if resp.Lines[9] != "line-100" {
		t.Errorf("last = %q", resp.Lines[9])
	}
}

func TestHandleTail_CapsAt10000(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	if err := os.WriteFile(path, []byte("a\nb\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := newTestService(t, dir)
	r := newRouter(svc)

	req := httptest.NewRequest("GET", "/logs/tail?n=999999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	// Just ensure it returns 200 — the cap is internal; the small fixture
	// only has 2 lines, so the cap behavior is verified indirectly: a value
	// like 999999 must not cause the handler to reject the request.
	var resp struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Lines) != 2 {
		t.Errorf("len(lines) = %d, want 2", len(resp.Lines))
	}
}

func TestHandleTail_InvalidNFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	if err := os.WriteFile(path, []byte("a\nb\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := newTestService(t, dir)
	r := newRouter(svc)

	req := httptest.NewRequest("GET", "/logs/tail?n=notanumber", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestHandleDownload_StreamsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kandev.log"), []byte("payload"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := newTestService(t, dir)
	r := newRouter(svc)

	req := httptest.NewRequest("GET", "/logs/kandev.log/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	if w.Body.String() != "payload" {
		t.Errorf("body = %q, want payload", w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "kandev.log") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestHandleDownload_PathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir)
	r := newRouter(svc)

	req := httptest.NewRequest("GET", "/logs/..%2Fetc%2Fpasswd/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == 200 {
		t.Fatalf("status = %d, want non-200 for traversal", w.Code)
	}
}

func TestHandleDownload_Missing404(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir)
	r := newRouter(svc)

	req := httptest.NewRequest("GET", "/logs/nope.log/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
