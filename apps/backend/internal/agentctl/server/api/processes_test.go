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

// awaitProcessOutput polls the get-process endpoint until the buffered output
// contains marker, and returns the snapshot that carried it. The process must
// still be running: a finished process is reaped out of the runner's map (see
// ProcessRunner.ensureProcessGroupReaped), so its output is only observable
// while it is alive.
func awaitProcessOutput(t *testing.T, srv *Server, id, marker string) process.ProcessInfo {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last process.ProcessInfo
	for time.Now().Before(deadline) {
		rec := processRequest(t, srv, http.MethodGet, "/api/v1/processes/"+id+"?include_output=true", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("get process = %d, want 200 — the process was retired before its output could be read (body %s)",
				rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &last); err != nil {
			t.Fatalf("decode process info: %v", err)
		}
		if strings.Contains(processOutputText(last), marker) {
			return last
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %s never emitted %q (last snapshot %+v)", id, marker, last)
	return last
}

// awaitProcessRetired polls until the process is gone from the runner. Exiting
// and being reaped are one step as far as the HTTP surface is concerned: there
// is no window in which a finished process is still fetchable, so this is the
// only terminal state a caller can observe over REST.
func awaitProcessRetired(t *testing.T, srv *Server, id string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if rec := processRequest(t, srv, http.MethodGet, "/api/v1/processes/"+id, nil); rec.Code == http.StatusNotFound {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %s was still fetchable after it should have exited and been reaped", id)
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

// TestProcessLifecycle_StartListGetCapturesOutput drives a real command through
// every read endpoint: the start response, the session-scoped list, and the
// per-process fetch with buffered output. Asserting the captured stdout is what
// proves the output plumbing is wired, not just the status field.
//
// The command prints and then blocks. A command that exits immediately cannot be
// read back at all — the runner reaps a finished process out of its map, so the
// fetch races the exit and answers 404 as soon as the machine is quick enough
// (which is exactly how this test first failed in CI but not locally).
func TestProcessLifecycle_StartListGetCapturesOutput(t *testing.T) {
	srv := newTestServer(t)

	start := processRequest(t, srv, http.MethodPost, "/api/v1/processes/start", process.StartProcessRequest{
		SessionID:  "session-a",
		Kind:       types.ProcessKindCustom,
		ScriptName: "marker",
		Command:    "printf 'kandev-marker\\n'; sleep 120",
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
	stopProcessOnCleanup(t, srv, started.Process.ID)

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

	live := awaitProcessOutput(t, srv, started.Process.ID, "kandev-marker")
	if live.Status != types.ProcessStatusRunning {
		t.Errorf("status = %q, want %q while the command is still blocked", live.Status, types.ProcessStatusRunning)
	}
	if live.ExitCode != nil {
		t.Errorf("exit_code = %v, want nil for a running process", *live.ExitCode)
	}
	if live.Command != "printf 'kandev-marker\\n'; sleep 120" {
		t.Errorf("command = %q, want the command as submitted", live.Command)
	}
}

// TestHandleGetProcess_OmitsOutputByDefault pins the include_output query
// parameter. Buffered output can be megabytes, so the list-facing fetch must
// not carry it unless asked — and must carry it when it is.
func TestHandleGetProcess_OmitsOutputByDefault(t *testing.T) {
	srv := newTestServer(t)
	id := startTestProcess(t, srv, "session-c", "printf 'kandev-marker\\n'; sleep 120")
	awaitProcessOutput(t, srv, id, "kandev-marker")

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
	if info.ID != id {
		t.Errorf("id = %q, want %q", info.ID, id)
	}
}

// TestProcessLifecycle_ExitedProcessIsRetired pins the reaping contract that
// makes the two tests above use a blocking command: once a process exits, the
// runner removes it, so REST has no "exited" state to report — the process
// simply stops existing. Anything that needs the exit code must observe the
// `process_status` event instead.
func TestProcessLifecycle_ExitedProcessIsRetired(t *testing.T) {
	srv := newTestServer(t)
	id := startTestProcess(t, srv, "session-e", "printf 'done\\n'")

	awaitProcessRetired(t, srv, id)

	listed := processRequest(t, srv, http.MethodGet, "/api/v1/processes?session_id=session-e", nil)
	var remaining []process.ProcessInfo
	if err := json.Unmarshal(listed.Body.Bytes(), &remaining); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("list after exit = %+v, want it emptied", remaining)
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
	stopProcessOnCleanup(t, srv, started.Process.ID)
	return started.Process.ID
}

// stopProcessOnCleanup reaps the subprocess when the test ends, including on an
// early t.Fatal. Several of these tests deliberately start a command that blocks
// for two minutes; without this the process — and the runner goroutines waiting
// on it — outlive the test and trip goleak. Stopping an already-retired process
// is a no-op success, so this is safe for every caller.
func stopProcessOnCleanup(t *testing.T, srv *Server, id string) {
	t.Helper()
	t.Cleanup(func() {
		rec := processRequest(t, srv, http.MethodPost, "/api/v1/processes/stop", process.StopProcessRequest{
			ProcessID: id,
		})
		if rec.Code != http.StatusOK {
			t.Errorf("cleanup stop of %s = %d (body %s)", id, rec.Code, rec.Body.String())
		}
	})
}
