package backups

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/system/jobs"
)

func newRouter(svc *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1/system")
	RegisterRoutes(g, svc)
	return r
}

func TestHandleList_ReturnsSnapshots(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupsDir, "manual-1.db"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := newRouter(svc)

	req := httptest.NewRequest("GET", "/api/v1/system/backups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Snapshots []Snapshot `json:"snapshots"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Snapshots) != 1 || resp.Snapshots[0].Kind != "manual" {
		t.Errorf("unexpected snapshots: %+v", resp.Snapshots)
	}
}

func TestHandleCreate_Returns202WithJobID(t *testing.T) {
	svc, _ := newTestService(t)
	r := newRouter(svc)

	req := httptest.NewRequest("POST", "/api/v1/system/backups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["job_id"] == "" {
		t.Errorf("expected job_id, body = %s", w.Body.String())
	}
	waitForJob(t, svc.jobs, resp["job_id"], jobs.StateSucceeded)
}

func TestHandleRestore_WrongConfirmReturns400(t *testing.T) {
	svc, _ := newTestService(t)
	r := newRouter(svc)

	body := bytes.NewBufferString(`{"confirm":"NOPE"}`)
	req := httptest.NewRequest("POST", "/api/v1/system/backups/manual-1.db/restore", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleDelete_RejectsTraversal(t *testing.T) {
	svc, _ := newTestService(t)
	r := newRouter(svc)
	// gin's :name will refuse a slash, but ensure dot-segments are rejected.
	req := httptest.NewRequest("DELETE", "/api/v1/system/backups/..", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleDelete_PreResetSnapshotReturns403(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	name := "kandev-pre-reset-20260101T000000Z.db"
	if err := os.WriteFile(filepath.Join(backupsDir, name), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/system/backups/"+name, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestHandleDelete_MissingSnapshotReturns404(t *testing.T) {
	svc, _ := newTestService(t)
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/system/backups/manual-missing.db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestHandleDelete_ManualSnapshotReturns204(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupsDir, "manual-9.db"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/system/backups/manual-9.db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if _, err := os.Stat(filepath.Join(backupsDir, "manual-9.db")); !os.IsNotExist(err) {
		t.Errorf("expected file removed, err=%v", err)
	}
}

func TestHandleDownload_StreamsFile(t *testing.T) {
	svc, dataDir := newTestService(t)
	if err := os.MkdirAll(filepath.Join(dataDir, "backups"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "backups", "manual-7.db"), []byte("payload"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := newRouter(svc)

	req := httptest.NewRequest("GET", "/api/v1/system/backups/manual-7.db/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "manual-7.db") {
		t.Errorf("missing attachment header: %q", w.Header().Get("Content-Disposition"))
	}
	if w.Body.String() != "payload" {
		t.Errorf("body = %q", w.Body.String())
	}
}

// Sanity-check the round-trip: create via handler, list via handler, file
// present on disk under manual- prefix.
func TestHandlerRoundTrip_CreateThenList(t *testing.T) {
	svc, dataDir := newTestService(t)
	r := newRouter(svc)

	req := httptest.NewRequest("POST", "/api/v1/system/backups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var created map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	waitForJob(t, svc.jobs, created["job_id"], jobs.StateSucceeded)

	// Confirm file appears on disk.
	entries, _ := os.ReadDir(filepath.Join(dataDir, "backups"))
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "manual-") {
			found = true
		}
	}
	if !found {
		t.Fatal("manual snapshot not created")
	}

	// And shows up in list.
	req2 := httptest.NewRequest("GET", "/api/v1/system/backups", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	var listResp struct {
		Snapshots []Snapshot `json:"snapshots"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listResp.Snapshots) < 1 {
		t.Errorf("expected >=1 snapshot, got %+v", listResp.Snapshots)
	}
	_ = context.Background()
}
