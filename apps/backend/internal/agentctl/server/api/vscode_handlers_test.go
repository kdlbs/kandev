package api

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
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestHandleVscodeStart_Success(t *testing.T) {
	s := newTestServer(t)
	stopServer := prepareVscodeTestServer(t, s)

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
	if resp.Status != "installing" && resp.Status != "starting" && resp.Status != "running" {
		t.Errorf("expected asynchronous startup status, got %q", resp.Status)
	}
	waitForVscodeStatus(t, s, process.VscodeStatusRunning)
	stopServer()
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

func TestHandleVscodeStart_TempSetupFailure(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(tempFile, []byte("file"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	t.Setenv("TMPDIR", tempFile)
	s := newTestServer(t)

	body, _ := json.Marshal(VscodeStartRequest{Theme: "dark"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
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
	// Use isolated TMPDIR so the IPC socket search finds no sockets.
	t.Setenv("TMPDIR", t.TempDir())

	s := newTestServer(t)
	stopServer := prepareVscodeTestServer(t, s)

	body, _ := json.Marshal(types.VscodeOpenFileRequest{Path: "main.go", Line: 10, Col: 5})
	// StartVscode flips status asynchronously while the fixture becomes ready.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/open-file", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	// The bridge reaches running before OpenFile exercises its expected fake-
	// binary error path, proving auto-start completed rather than merely began.
	info := s.procMgr.VscodeInfo()
	if info.Status != process.VscodeStatusRunning {
		t.Fatalf("VS Code status = %q, want running", info.Status)
	}

	var resp types.VscodeOpenFileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// The error should NOT be the old "is not running" message.
	if !resp.Success && strings.Contains(resp.Error, "is not running") {
		t.Errorf("expected auto-start to be attempted, not 'is not running'; got: %s", resp.Error)
	}
	stopServer()
}

func TestHandleVscodeStatus_AfterStart(t *testing.T) {
	s := newTestServer(t)
	stopServer := prepareVscodeTestServer(t, s)

	// Start VS Code
	startBody, _ := json.Marshal(VscodeStartRequest{Theme: "dark"})
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/start", bytes.NewReader(startBody))
	startReq.Header.Set("Content-Type", "application/json")
	startW := httptest.NewRecorder()
	s.router.ServeHTTP(startW, startReq)

	// Check status — should be "installing" (async start)
	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/vscode/status", nil)
	statusW := httptest.NewRecorder()
	s.router.ServeHTTP(statusW, statusReq)

	var resp types.VscodeStatusResponse
	if err := json.Unmarshal(statusW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Status != "installing" && resp.Status != "starting" && resp.Status != "running" {
		t.Errorf("expected asynchronous startup status, got %q", resp.Status)
	}
	waitForVscodeStatus(t, s, process.VscodeStatusRunning)
	statusW = httptest.NewRecorder()
	s.router.ServeHTTP(statusW, statusReq)
	if err := json.Unmarshal(statusW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse running response: %v", err)
	}
	if resp.Status != "running" {
		t.Fatalf("status after fixture readiness = %q, want running", resp.Status)
	}
	stopServer()
}

// TestVscodeOpenFile_WaitsForRunning_E2E verifies the end-to-end flow where
// open-file is called while vscode is still starting up. The handler should
// wait for vscode to become ready before attempting to open the file.
// This uses the real HTTP handler chain: HTTP request → Gin handler →
// procMgr.VscodeOpenFile (auto-start + WaitForRunning) → VscodeManager.OpenFile.
func TestVscodeOpenFile_WaitsForRunning_E2E(t *testing.T) {
	s := newTestServer(t)
	ts := httptest.NewServer(s.router)
	defer ts.Close()

	// Inject a VscodeManager that transitions from "installing" to "running" after 200ms.
	s.procMgr.SetVscodeTransitionForTest(process.VscodeStatusInstalling, 0, 200*time.Millisecond)

	body, _ := json.Marshal(types.VscodeOpenFileRequest{Path: "main.go", Line: 1})

	// open-file should block waiting for vscode to become running.
	// It will reach "running" status, but then OpenFile will fail because
	// there's no real code-server binary/IPC socket. That's expected —
	// the important assertion is that WaitForRunning succeeded (no "not running" error).
	resp, err := http.Post(ts.URL+"/api/v1/vscode/open-file", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var result types.VscodeOpenFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// The request should fail because there's no real code-server binary,
	// but the error should be about the binary/IPC socket, NOT about
	// "code-server is not running" — proving WaitForRunning worked.
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
