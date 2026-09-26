package client

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestShellStatus_DecodesSessionState(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"running":true,"pid":8123,"shell":"/bin/bash","cwd":"/workspace/task-1"}`))

	status, err := newHTTPOnlyClient(srv.URL).ShellStatus(context.Background())
	if err != nil {
		t.Fatalf("ShellStatus: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/shell/status" {
		t.Errorf("request = %s %s, want GET /api/v1/shell/status", got.Method, got.Path)
	}
	if !status.Running {
		t.Error("Running = false, want true")
	}
	if status.Pid != 8123 {
		t.Errorf("Pid = %d, want 8123", status.Pid)
	}
	if status.Shell != "/bin/bash" {
		t.Errorf("Shell = %q, want /bin/bash", status.Shell)
	}
	if status.Cwd != "/workspace/task-1" {
		t.Errorf("Cwd = %q, want /workspace/task-1", status.Cwd)
	}
}

func TestShellStatus_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-OK status", http.StatusNotFound, `{}`, "shell status failed"},
		{"malformed body", http.StatusOK, `{"running":`, "unexpected EOF"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			status, err := newHTTPOnlyClient(srv.URL).ShellStatus(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if status != nil {
				t.Errorf("status = %+v, want nil on failure", status)
			}
		})
	}
}

func TestShellBuffer_ReturnsDataField(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"data":"$ ls\r\nREADME.md\r\n"}`))

	data, err := newHTTPOnlyClient(srv.URL).ShellBuffer(context.Background())
	if err != nil {
		t.Fatalf("ShellBuffer: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/shell/buffer" {
		t.Errorf("request = %s %s, want GET /api/v1/shell/buffer", got.Method, got.Path)
	}
	if data != "$ ls\r\nREADME.md\r\n" {
		t.Errorf("data = %q, want the buffered shell output", data)
	}
}

func TestShellBuffer_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-OK status", http.StatusInternalServerError, `{}`, "shell buffer failed"},
		{"malformed body", http.StatusOK, `{"data":`, "unexpected EOF"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			data, err := newHTTPOnlyClient(srv.URL).ShellBuffer(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if data != "" {
				t.Errorf("data = %q, want empty on failure", data)
			}
		})
	}
}

func TestStartShell_PostsToShellStart(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{}`))

	if err := newHTTPOnlyClient(srv.URL).StartShell(context.Background()); err != nil {
		t.Fatalf("StartShell: %v", err)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v1/shell/start" {
		t.Errorf("request = %s %s, want POST /api/v1/shell/start", got.Method, got.Path)
	}
}

func TestStartShell_FailsOnNonOKStatus(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusConflict, `{}`))

	err := newHTTPOnlyClient(srv.URL).StartShell(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start shell failed") {
		t.Fatalf("error = %v, want the start failure named", err)
	}
	if !strings.Contains(err.Error(), "409") {
		t.Errorf("error = %v, want the HTTP status carried through", err)
	}
}

func TestShellEndpoints_HonourContextCancellation(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{}`))
	c := newHTTPOnlyClient(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := map[string]func() error{
		"ShellStatus": func() error {
			_, err := c.ShellStatus(ctx)
			return err
		},
		"ShellBuffer": func() error {
			_, err := c.ShellBuffer(ctx)
			return err
		},
		"StartShell": func() error { return c.StartShell(ctx) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "context canceled") {
				t.Fatalf("error = %v, want context canceled", err)
			}
		})
	}
}
