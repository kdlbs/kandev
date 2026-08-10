package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func processRequest(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	switch v := body.(type) {
	case nil:
	case string:
		payload = []byte(v)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		payload = encoded
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	return rec
}

func decodeProcessError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return body.Error
}

// awaitProcessStatus polls the get-process endpoint until the process reaches
// one of the terminal states. It drives a real subprocess, so there is nothing
// to synchronise on but the observable status; the deadline is a failure
// detector rather than a timing assumption.
func awaitProcessStatus(t *testing.T, srv *Server, id string, want ...types.ProcessStatus) process.ProcessInfo {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last process.ProcessInfo
	for time.Now().Before(deadline) {
		rec := processRequest(t, srv, http.MethodGet, "/api/v1/processes/"+id+"?include_output=true", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("get process status = %d (body %s)", rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &last); err != nil {
			t.Fatalf("decode process info: %v", err)
		}
		for _, status := range want {
			if last.Status == status {
				return last
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %s never reached %v (last status %q)", id, want, last.Status)
	return last
}

func processOutputText(info process.ProcessInfo) string {
	var builder strings.Builder
	for _, chunk := range info.Output {
		builder.WriteString(chunk.Data)
	}
	return builder.String()
}

// TestHandleStartProcess_Rejections covers each 400: an unparseable body gets a
// fixed message (the handler deliberately does not echo the parse error), while
// the runner's own required-field errors are forwarded verbatim.
func TestHandleStartProcess_Rejections(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name      string
		body      any
		wantError string
	}{
		{"malformed body", "{not json", "invalid request body"},
		{"missing session id", process.StartProcessRequest{Command: "true"}, "session_id is required"},
		{"missing command", process.StartProcessRequest{SessionID: "s1"}, "command is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := processRequest(t, srv, http.MethodPost, "/api/v1/processes/start", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			var body startProcessResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Process != nil {
				t.Errorf("a process was returned alongside the refusal: %+v", body.Process)
			}
			if body.Error != tc.wantError {
				t.Errorf("error = %q, want %q", body.Error, tc.wantError)
			}
		})
	}
}

// TestProcessLifecycle_StartListGetCapturesOutput drives a real short-lived
// command through every read endpoint: the start response, the session-scoped
// list, and the per-process fetch with buffered output. Asserting the captured
// stdout is what proves the output plumbing is wired, not just the status field.
func TestProcessLifecycle_StartListGetCapturesOutput(t *testing.T) {
	srv := newTestServer(t)

	start := processRequest(t, srv, http.MethodPost, "/api/v1/processes/start", process.StartProcessRequest{
		SessionID:  "session-a",
		Kind:       types.ProcessKindCustom,
		ScriptName: "marker",
		Command:    "printf 'kandev-marker\\n'",
	})
	if start.Code != http.StatusOK {
		t.Fatalf("start status = %d (body %s)", start.Code, start.Body.String())
	}
	var started startProcessResponse
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if started.Process == nil || started.Process.ID == "" {
		t.Fatalf("start response carries no process: %s", start.Body.String())
	}
	if started.Process.SessionID != "session-a" || started.Process.ScriptName != "marker" {
		t.Errorf("process = %+v, want the requested session and script name", started.Process)
	}

	listed := processRequest(t, srv, http.MethodGet, "/api/v1/processes?session_id=session-a", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d (body %s)", listed.Code, listed.Body.String())
	}
	var infos []process.ProcessInfo
	if err := json.Unmarshal(listed.Body.Bytes(), &infos); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(infos) != 1 || infos[0].ID != started.Process.ID {
		t.Errorf("list for session-a = %+v, want exactly the started process", infos)
	}

	other := processRequest(t, srv, http.MethodGet, "/api/v1/processes?session_id=session-b", nil)
	var otherInfos []process.ProcessInfo
	if err := json.Unmarshal(other.Body.Bytes(), &otherInfos); err != nil {
		t.Fatalf("decode other list: %v", err)
	}
	if len(otherInfos) != 0 {
		t.Errorf("list for session-b = %+v, want no processes; the filter is not applied", otherInfos)
	}

	final := awaitProcessStatus(t, srv, started.Process.ID, types.ProcessStatusExited, types.ProcessStatusFailed)
	if final.Status != types.ProcessStatusExited {
		t.Fatalf("final status = %q, want %q", final.Status, types.ProcessStatusExited)
	}
	if final.ExitCode == nil || *final.ExitCode != 0 {
		t.Errorf("exit_code = %v, want 0", final.ExitCode)
	}
	if got := processOutputText(final); !strings.Contains(got, "kandev-marker") {
		t.Errorf("captured output = %q, want it to contain the printed marker", got)
	}
}

// TestHandleGetProcess_OmitsOutputByDefault pins the include_output query
// parameter. Buffered output can be megabytes, so the list-facing fetch must
// not carry it unless asked.
func TestHandleGetProcess_OmitsOutputByDefault(t *testing.T) {
	srv := newTestServer(t)
	id := startTestProcess(t, srv, "session-c", "printf 'kandev-marker\\n'")
	awaitProcessStatus(t, srv, id, types.ProcessStatusExited, types.ProcessStatusFailed)

	rec := processRequest(t, srv, http.MethodGet, "/api/v1/processes/"+id, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var info process.ProcessInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode process info: %v", err)
	}
	if len(info.Output) != 0 {
		t.Errorf("output = %+v, want none without include_output=true", info.Output)
	}
}

// TestHandleGetProcess_UnknownIDIs404 covers the miss branch.
func TestHandleGetProcess_UnknownIDIs404(t *testing.T) {
	srv := newTestServer(t)

	rec := processRequest(t, srv, http.MethodGet, "/api/v1/processes/no-such-process", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", rec.Code, rec.Body.String())
	}
	if got := decodeProcessError(t, rec); got != "process not found" {
		t.Errorf("error = %q, want %q", got, "process not found")
	}
}

// TestHandleStopProcess_Rejections covers the two 400 branches ahead of the
// runner call.
func TestHandleStopProcess_Rejections(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name      string
		body      any
		wantError string
	}{
		{"malformed body", "{not json", "invalid request body"},
		{"missing process id", process.StopProcessRequest{}, "process_id is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := processRequest(t, srv, http.MethodPost, "/api/v1/processes/stop", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			var body stopProcessResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Success {
				t.Errorf("success = true, want false")
			}
			if body.Error != tc.wantError {
				t.Errorf("error = %q, want %q", body.Error, tc.wantError)
			}
		})
	}
}

// TestHandleStopProcess_UnknownIDIsIdempotentSuccess pins the deliberate
// asymmetry with the fetch endpoint: stopping something already gone is the
// caller's desired end state, so it answers success rather than 404. The UI
// stop button relies on that to stay usable after a process exits on its own.
func TestHandleStopProcess_UnknownIDIsIdempotentSuccess(t *testing.T) {
	srv := newTestServer(t)

	rec := processRequest(t, srv, http.MethodPost, "/api/v1/processes/stop", process.StopProcessRequest{
		ProcessID: "no-such-process",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var body stopProcessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Success || body.Error != "" {
		t.Errorf("response = %+v, want an unqualified success", body)
	}
}

// TestHandleStopProcess_RetiresRunningProcess asserts the stop reaches a
// long-running command: the call returns success and the process is retired
// from both the per-session list and the per-id fetch. A `sleep 120` that was
// merely marked stopped would still be listed.
func TestHandleStopProcess_RetiresRunningProcess(t *testing.T) {
	srv := newTestServer(t)
	id := startTestProcess(t, srv, "session-d", "sleep 120")

	listed := processRequest(t, srv, http.MethodGet, "/api/v1/processes?session_id=session-d", nil)
	var before []process.ProcessInfo
	if err := json.Unmarshal(listed.Body.Bytes(), &before); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("list before stop = %+v, want the running process", before)
	}

	rec := processRequest(t, srv, http.MethodPost, "/api/v1/processes/stop", process.StopProcessRequest{ProcessID: id})
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var body stopProcessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode stop response: %v", err)
	}
	if !body.Success || body.Error != "" {
		t.Fatalf("stop reported failure: %+v", body)
	}

	after := processRequest(t, srv, http.MethodGet, "/api/v1/processes?session_id=session-d", nil)
	var remaining []process.ProcessInfo
	if err := json.Unmarshal(after.Body.Bytes(), &remaining); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("list after stop = %+v, want it emptied", remaining)
	}
	if got := processRequest(t, srv, http.MethodGet, "/api/v1/processes/"+id, nil); got.Code != http.StatusNotFound {
		t.Errorf("get after stop status = %d, want 404 (body %s)", got.Code, got.Body.String())
	}
}

func startTestProcess(t *testing.T, srv *Server, sessionID, command string) string {
	t.Helper()
	rec := processRequest(t, srv, http.MethodPost, "/api/v1/processes/start", process.StartProcessRequest{
		SessionID: sessionID,
		Kind:      types.ProcessKindCustom,
		Command:   command,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d (body %s)", rec.Code, rec.Body.String())
	}
	var started startProcessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if started.Process == nil {
		t.Fatalf("start response carries no process: %s", rec.Body.String())
	}
	return started.Process.ID
}
