package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestHandleVscodeStart_Success(t *testing.T) {
	s := newTestServer(t)

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
	// Status should be "installing" since start is non-blocking
	if resp.Status != "installing" {
		t.Errorf("expected status=installing, got %q", resp.Status)
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

func TestHandleVscodeOpenFile_NotRunning(t *testing.T) {
	s := newTestServer(t)

	body, _ := json.Marshal(types.VscodeOpenFileRequest{Path: "main.go", Line: 10, Col: 5})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/vscode/open-file", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	// Code-server is not running, should get 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleVscodeStatus_AfterStart(t *testing.T) {
	s := newTestServer(t)

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
	// Status should be installing or error (since code-server binary won't exist in tests)
	if resp.Status != "installing" && resp.Status != "error" && resp.Status != "starting" {
		t.Errorf("expected installing/error/starting, got %q", resp.Status)
	}
}
