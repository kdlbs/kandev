package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestStartProcess_MarshalsRequestAndDecodesProcessInfo(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
		"process":{
			"id":"proc-1","session_id":"sess-1","kind":"script","script_name":"dev",
			"command":"pnpm dev","working_dir":"/workspace/task-1","status":"running",
			"exit_code":null,
			"started_at":"2026-08-10T10:00:00Z","updated_at":"2026-08-10T10:00:05Z",
			"output":[
				{"stream":"stdout","data":"listening\n","timestamp":"2026-08-10T10:00:01Z"},
				{"stream":"stderr","data":"warn\n","timestamp":"2026-08-10T10:00:02Z"}
			]
		}
	}`))

	req := StartProcessRequest{
		SessionID:      "sess-1",
		Kind:           "script",
		ScriptName:     "dev",
		Command:        "pnpm dev",
		WorkingDir:     "/workspace/task-1",
		Env:            map[string]string{"PORT": "3000"},
		BufferMaxBytes: 65536,
	}
	info, err := newHTTPOnlyClient(srv.URL).StartProcess(context.Background(), req)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/processes/start" {
		t.Errorf("request = %s %s, want POST /api/v1/processes/start", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent StartProcessRequest
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent.SessionID != "sess-1" || sent.Kind != "script" || sent.ScriptName != "dev" {
		t.Errorf("sent session/kind/script = %q / %q / %q", sent.SessionID, sent.Kind, sent.ScriptName)
	}
	if sent.Command != "pnpm dev" || sent.WorkingDir != "/workspace/task-1" {
		t.Errorf("sent command/dir = %q / %q", sent.Command, sent.WorkingDir)
	}
	if sent.Env["PORT"] != "3000" || sent.BufferMaxBytes != 65536 {
		t.Errorf("sent env/buffer = %v / %d", sent.Env, sent.BufferMaxBytes)
	}

	if info.ID != "proc-1" || info.SessionID != "sess-1" {
		t.Errorf("id/session = %q / %q", info.ID, info.SessionID)
	}
	if info.Kind != "script" || info.ScriptName != "dev" {
		t.Errorf("kind/script = %q / %q", info.Kind, info.ScriptName)
	}
	if info.Command != "pnpm dev" || info.WorkingDir != "/workspace/task-1" {
		t.Errorf("command/dir = %q / %q", info.Command, info.WorkingDir)
	}
	if info.Status != "running" {
		t.Errorf("Status = %q, want running", info.Status)
	}
	if info.ExitCode != nil {
		t.Errorf("ExitCode = %v, want nil while running", info.ExitCode)
	}
	if want := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC); !info.StartedAt.Equal(want) {
		t.Errorf("StartedAt = %v, want %v", info.StartedAt, want)
	}
	if want := time.Date(2026, 8, 10, 10, 0, 5, 0, time.UTC); !info.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v", info.UpdatedAt, want)
	}
	if len(info.Output) != 2 {
		t.Fatalf("Output = %d chunks, want 2", len(info.Output))
	}
	if info.Output[0].Stream != "stdout" || info.Output[0].Data != "listening\n" {
		t.Errorf("Output[0] = %+v, want the stdout chunk", info.Output[0])
	}
	if want := time.Date(2026, 8, 10, 10, 0, 1, 0, time.UTC); !info.Output[0].Timestamp.Equal(want) {
		t.Errorf("Output[0].Timestamp = %v, want %v", info.Output[0].Timestamp, want)
	}
	if info.Output[1].Stream != "stderr" || info.Output[1].Data != "warn\n" {
		t.Errorf("Output[1] = %+v, want the stderr chunk", info.Output[1])
	}
}

func TestStartProcess_DecodesExitCodeOnFinishedProcess(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK,
		`{"process":{"id":"p","status":"exited","exit_code":137}}`))

	info, err := newHTTPOnlyClient(srv.URL).StartProcess(
		context.Background(), StartProcessRequest{SessionID: "s"})
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	if info.ExitCode == nil {
		t.Fatal("ExitCode = nil, want a decoded 137")
	}
	if *info.ExitCode != 137 {
		t.Errorf("ExitCode = %d, want 137", *info.ExitCode)
	}
}

func TestStartProcess_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "non-2xx status echoes body",
			status:  http.StatusBadRequest,
			body:    `{"error":"working_dir is required"}`,
			wantErr: "start process failed with status 400",
		},
		{
			name:    "malformed body",
			status:  http.StatusOK,
			body:    `{"process":`,
			wantErr: "failed to parse start process response",
		},
		{
			name:    "nil process with error field",
			status:  http.StatusOK,
			body:    `{"error":"spawn failed"}`,
			wantErr: "start process failed: spawn failed",
		},
		{
			name:    "nil process without error field",
			status:  http.StatusOK,
			body:    `{}`,
			wantErr: "start process failed: no process returned",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			info, err := newHTTPOnlyClient(srv.URL).StartProcess(
				context.Background(), StartProcessRequest{SessionID: "s"})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if info != nil {
				t.Errorf("info = %+v, want nil on failure", info)
			}
		})
	}
}

func TestStartProcess_EchoesServerBodyInStatusError(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusBadRequest, `{"error":"working_dir is required"}`))

	_, err := newHTTPOnlyClient(srv.URL).StartProcess(
		context.Background(), StartProcessRequest{SessionID: "s"})
	if err == nil || !strings.Contains(err.Error(), "working_dir is required") {
		t.Fatalf("error = %v, want the server body included for diagnosis", err)
	}
}

func TestStopProcess_PostsProcessIDInBody(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))

	if err := newHTTPOnlyClient(srv.URL).StopProcess(context.Background(), "proc-7"); err != nil {
		t.Fatalf("StopProcess: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/processes/stop" {
		t.Errorf("request = %s %s, want POST /api/v1/processes/stop", got.Method, got.Path)
	}
	if got.ContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.ContentType)
	}

	var sent map[string]string
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	if sent["process_id"] != "proc-7" {
		t.Errorf("sent process_id = %q, want proc-7", sent["process_id"])
	}
}

func TestStopProcess_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusNotFound, `no such process`, "stop process failed with status 404"},
		{"malformed body", http.StatusOK, `{"success":`, "failed to parse stop process response"},
		{"unsuccessful", http.StatusOK, `{"success":false,"error":"already exited"}`, "stop process failed: already exited"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			err := newHTTPOnlyClient(srv.URL).StopProcess(context.Background(), "proc-7")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestListProcesses_FiltersBySessionIDAndDecodesList(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `[
		{"id":"p1","session_id":"sess-1","kind":"script","command":"a","status":"running"},
		{"id":"p2","session_id":"sess-1","kind":"user_shell","command":"b","status":"exited","exit_code":0}
	]`))

	processes, err := newHTTPOnlyClient(srv.URL).ListProcesses(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("ListProcesses: %v", err)
	}

	if got.Method != http.MethodGet || got.Path != "/api/v1/processes" {
		t.Errorf("request = %s %s, want GET /api/v1/processes", got.Method, got.Path)
	}
	if sid := got.Query.Get("session_id"); sid != "sess-1" {
		t.Errorf("session_id = %q, want sess-1", sid)
	}

	if len(processes) != 2 {
		t.Fatalf("processes = %d, want 2", len(processes))
	}
	if processes[0].ID != "p1" || processes[0].Kind != "script" || processes[0].Status != "running" {
		t.Errorf("processes[0] = %+v", processes[0])
	}
	if processes[1].ID != "p2" || processes[1].Kind != "user_shell" || processes[1].Status != "exited" {
		t.Errorf("processes[1] = %+v", processes[1])
	}
	if processes[1].ExitCode == nil || *processes[1].ExitCode != 0 {
		t.Errorf("processes[1].ExitCode = %v, want pointer to 0", processes[1].ExitCode)
	}
}

func TestListProcesses_OmitsQueryWhenSessionIDIsEmpty(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK, `[]`))

	processes, err := newHTTPOnlyClient(srv.URL).ListProcesses(context.Background(), "")
	if err != nil {
		t.Fatalf("ListProcesses: %v", err)
	}
	if got.RawQuery != "" {
		t.Errorf("raw query = %q, want empty when unfiltered", got.RawQuery)
	}
	if len(processes) != 0 {
		t.Errorf("processes = %+v, want empty", processes)
	}
}

func TestListProcesses_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusInternalServerError, `[]`, "list processes failed with status 500"},
		{"malformed body", http.StatusOK, `[{"id":`, "unexpected EOF"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			processes, err := newHTTPOnlyClient(srv.URL).ListProcesses(context.Background(), "")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if processes != nil {
				t.Errorf("processes = %+v, want nil on failure", processes)
			}
		})
	}
}

func TestGetProcess_PutsIDInPathAndRequestsOutputOnDemand(t *testing.T) {
	tests := []struct {
		name          string
		includeOutput bool
		wantQuery     string
	}{
		{"without output", false, ""},
		{"with output", true, "include_output=true"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := captureServer(t, jsonResponder(http.StatusOK, `{
				"id":"proc-4","session_id":"sess-2","kind":"script","command":"go test",
				"working_dir":"/w","status":"running",
				"output":[{"stream":"stdout","data":"ok\n","timestamp":"2026-08-10T10:00:00Z"}]
			}`))

			info, err := newHTTPOnlyClient(srv.URL).GetProcess(
				context.Background(), "proc-4", tc.includeOutput)
			if err != nil {
				t.Fatalf("GetProcess: %v", err)
			}

			if got.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", got.Method)
			}
			if got.Path != "/api/v1/processes/proc-4" {
				t.Errorf("path = %q, want the ID as the last segment", got.Path)
			}
			if got.RawQuery != tc.wantQuery {
				t.Errorf("raw query = %q, want %q", got.RawQuery, tc.wantQuery)
			}

			if info.ID != "proc-4" || info.SessionID != "sess-2" {
				t.Errorf("id/session = %q / %q", info.ID, info.SessionID)
			}
			if info.Command != "go test" || info.WorkingDir != "/w" || info.Status != "running" {
				t.Errorf("command/dir/status = %q / %q / %q", info.Command, info.WorkingDir, info.Status)
			}
			if len(info.Output) != 1 || info.Output[0].Data != "ok\n" {
				t.Errorf("Output = %+v, want one stdout chunk", info.Output)
			}
		})
	}
}

func TestGetProcess_FailureModes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"non-2xx status", http.StatusNotFound, `{}`, "get process failed with status 404"},
		{"malformed body", http.StatusOK, `{"id":`, "unexpected EOF"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(tc.status, tc.body))

			info, err := newHTTPOnlyClient(srv.URL).GetProcess(context.Background(), "p", false)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if info != nil {
				t.Errorf("info = %+v, want nil on failure", info)
			}
		})
	}
}

func TestProcessEndpoints_HonourContextCancellation(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"success":true}`))
	c := newHTTPOnlyClient(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := map[string]func() error{
		"StartProcess": func() error {
			_, err := c.StartProcess(ctx, StartProcessRequest{SessionID: "s"})
			return err
		},
		"StopProcess": func() error { return c.StopProcess(ctx, "p") },
		"ListProcesses": func() error {
			_, err := c.ListProcesses(ctx, "")
			return err
		},
		"GetProcess": func() error {
			_, err := c.GetProcess(ctx, "p", false)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "context canceled") {
				t.Fatalf("error = %v, want context canceled", err)
			}
		})
	}
}
