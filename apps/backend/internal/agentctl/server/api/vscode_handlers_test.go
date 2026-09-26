package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestHandleVscodeStart_Success(t *testing.T) {
	s := prepareVscodeTestServer(t)

	body, _ := json.Marshal(VscodeStartRequest{Theme: "dark"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp types.VscodeStartResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success=true, got false (error: %s)", resp.Error)
	}
	if resp.Status != string(process.VscodeStatusInstalling) && resp.Status != string(process.VscodeStatusStarting) && resp.Status != string(process.VscodeStatusRunning) {
		t.Errorf("expected installing/starting/running, got %q", resp.Status)
	}
}

func TestHandleVscodeStart_InvalidBody(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/start", bytes.NewReader([]byte(`not json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleVscodeStart_ManagerStopping(t *testing.T) {
	s := newTestServer(t)
	s.procMgr.CloseAdmission()

	body, _ := json.Marshal(VscodeStartRequest{Theme: "dark"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleVscodeStop_WhenNotRunning(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/stop", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	// StopVscode on a non-running instance returns nil (success)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp types.VscodeStopResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success=true, got false")
	}
}

func TestHandleVscodeStatus_Initial(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vscode/status", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp types.VscodeStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Status != "stopped" {
		t.Errorf("expected status=stopped, got %q", resp.Status)
	}
}

func TestHandleVscodeOpenFile_MissingPath(t *testing.T) {
	s := newTestServer(t)

	body, _ := json.Marshal(types.VscodeOpenFileRequest{Path: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/open-file", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp types.VscodeOpenFileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Success {
		t.Error("expected success=false for empty path")
	}
}

func TestHandleVscodeOpenFile_InvalidBody(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/open-file", bytes.NewReader([]byte(`not json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleVscodeOpenFile_NotRunning_AutoStartAttempted(t *testing.T) {
	s := prepareVscodeTestServer(t)

	body, _ := json.Marshal(types.VscodeOpenFileRequest{Path: "main.go", Line: 10, Col: 5})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/open-file", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	waitForVscodeStatus(t, s, process.VscodeStatusRunning)

	var resp types.VscodeOpenFileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// The error should NOT be the old "is not running" message.
	if !resp.Success && strings.Contains(resp.Error, "is not running") {
		t.Errorf("expected auto-start to be attempted, not 'is not running'; got: %s", resp.Error)
	}
}

func TestHandleVscodeStatus_AfterStart(t *testing.T) {
	s := prepareVscodeTestServer(t)

	// Start VS Code
	startBody, _ := json.Marshal(VscodeStartRequest{Theme: "dark"})
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/start", bytes.NewReader(startBody))
	startReq.Header.Set("Content-Type", "application/json")
	startW := httptest.NewRecorder()
	s.router.ServeHTTP(startW, startReq)

	waitForVscodeStatus(t, s, process.VscodeStatusRunning)

	// Check status after the deterministic fixture is ready.
	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/vscode/status", nil)
	statusW := httptest.NewRecorder()
	s.router.ServeHTTP(statusW, statusReq)

	var resp types.VscodeStatusResponse
	if err := json.Unmarshal(statusW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Status != string(process.VscodeStatusRunning) {
		t.Errorf("expected running, got %q", resp.Status)
	}
}

// TestVscodeOpenFile_WaitsForRunning_E2E verifies the end-to-end flow where
// open-file is called while vscode is still starting up. The handler should
// wait for vscode to become ready before attempting to open the file.
// This uses the real HTTP handler chain: HTTP request → Gin handler →
// procMgr.VscodeOpenFile (auto-start + WaitForRunning) → VscodeManager.OpenFile.
func TestVscodeOpenFile_WaitsForRunning_E2E(t *testing.T) {
	s := prepareVscodeTestServer(t)
	ts := httptest.NewServer(s.router)
	t.Cleanup(ts.Close)

	body, _ := json.Marshal(types.VscodeOpenFileRequest{Path: "main.go", Line: 1})

	// open-file should block until the fixture has reached "running".
	resp, err := http.Post(ts.URL+"/api/v1/vscode/open-file", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var result types.VscodeOpenFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// The request fails because the fixture has no Remote CLI, but it must
	// have passed WaitForRunning rather than returning "not running".
	if result.Success {
		t.Error("expected success=false (no real code-server binary)")
	}
	if result.Error == "" {
		t.Error("expected an error message")
	}

	// Crucially, the error must NOT be the old "code-server is not running" message.
	if strings.Contains(result.Error, "is not running") {
		t.Errorf("expected error about binary/socket, not 'is not running'; got: %s", result.Error)
	}
	// The error should be about the binary path or IPC socket not being found.
	if !strings.Contains(result.Error, "binary") && !strings.Contains(result.Error, "remote CLI") &&
		!strings.Contains(result.Error, "IPC") && !strings.Contains(result.Error, "not resolved") {
		t.Logf("unexpected error (test may need updating): %s", result.Error)
	}
}
