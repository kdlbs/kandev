package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleRescanWorkspace_AcceptsEmptyBody covers the materializer's
// no-work_dir form: a rescan that doesn't touch cfg.WorkDir and just
// reconciles trackers against current on-disk state.
func TestHandleRescanWorkspace_AcceptsEmptyBody(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/rescan", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestHandleRescanWorkspace_AcceptsWorkDir covers the materializer's
// transition form: a rescan that promotes WorkDir to the task root before
// re-discovering repo subdirs.
func TestHandleRescanWorkspace_AcceptsWorkDir(t *testing.T) {
	s := newTestServer(t)

	body, _ := json.Marshal(RescanWorkspaceRequest{WorkDir: "/tmp/task-root"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspace/rescan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}
