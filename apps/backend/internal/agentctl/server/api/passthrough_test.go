package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newPassthroughAPIServer(t *testing.T) *Server {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("SSH passthrough runs on Unix targets")
	}
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session-1", InstanceID: "instance-1"}
	mgr := process.NewManager(cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, mgr.StopForTeardown(ctx))
	})
	return NewServer(cfg, mgr, nil, nil, log)
}

// @covers AC-EXECUTORS-SSH-PASSTHROUGH-001.7
func TestRemotePassthroughConcurrentStart(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	body := map[string]any{"session_id": "session-1", "operation_id": "start-1", "command": []string{"/bin/sh", "-c", "cat"}, "idle_timeout": int64(time.Second)}
	var wg sync.WaitGroup
	ids := make(chan string, 2)
	for range 2 {
		wg.Go(func() {
			rec := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/start", body)
			if rec.Code != http.StatusOK {
				t.Errorf("remote start: status %d, body %s", rec.Code, rec.Body.String())
				return
			}
			var snapshot struct {
				ProcessID string `json:"process_id"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
				t.Error(err)
				return
			}
			ids <- snapshot.ProcessID
		})
	}
	wg.Wait()
	close(ids)
	var values []string
	for id := range ids {
		values = append(values, id)
	}
	require.Len(t, values, 2)
	require.NotEmpty(t, values[0])
	require.Equal(t, values[0], values[1])
}

// @covers AC-EXECUTORS-SSH-PASSTHROUGH-001.11
func TestRemotePassthroughSessionAdmission(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	rec := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/start", map[string]any{
		"session_id": "another-session", "operation_id": "start-1", "command": []string{"/bin/sh", "-c", "cat"},
	})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}

func remoteStart(t *testing.T, srv *Server, command string) map[string]any {
	t.Helper()
	rec := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/start", map[string]any{
		"session_id": "session-1", "operation_id": "start-1", "command": []string{"/bin/sh", "-c", command},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var snapshot map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &snapshot))
	return snapshot
}

// @covers AC-EXECUTORS-SSH-PASSTHROUGH-001.6
func TestRemotePassthroughInputDedup(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	snapshot := remoteStart(t, srv, "stty -echo; while read line; do printf '%s\\n' \"$line\" >> accepted-lines; done")
	body := map[string]any{"process_id": snapshot["process_id"], "generation": snapshot["generation"], "submission_id": uuid.Must(uuid.NewV7()).String(), "data": "once\n"}
	for range 2 {
		rec := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/input", body)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}
	accepted := filepath.Join(srv.cfg.WorkDir, "accepted-lines")
	require.Eventually(t, func() bool { data, _ := os.ReadFile(accepted); return string(data) == "once\n" }, 5*time.Second, 10*time.Millisecond)
	stop := shellTerminalRequest(t, srv, http.MethodDelete, "/api/v1/passthrough/"+snapshot["process_id"].(string)+"?generation="+strconv.Itoa(int(snapshot["generation"].(float64))), nil)
	require.Equal(t, http.StatusOK, stop.Code, stop.Body.String())
	data, err := os.ReadFile(accepted)
	require.NoError(t, err)
	require.Equal(t, "once\n", string(data))
}

// @covers AC-EXECUTORS-SSH-PASSTHROUGH-001.8
func TestRemotePassthroughRejectsStaleInput(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	snapshot := remoteStart(t, srv, "cat")
	rec := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/input", map[string]any{
		"process_id": snapshot["process_id"], "generation": snapshot["generation"].(float64) + 1, "data": "wrong\n",
	})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	body := map[string]any{"session_id": "session-1", "operation_id": "another-start", "command": []string{"/bin/sh", "-c", "cat"}}
	rec = shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/start", body)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}

func TestRemotePassthroughAuthenticated(t *testing.T) {
	base := newPassthroughAPIServer(t)
	base.cfg.AuthToken = "private-token"
	srv := NewServer(base.cfg, base.procMgr, nil, nil, newTestLogger())
	for _, tc := range []struct {
		token, instance string
		status          int
	}{
		{"", "instance-1", http.StatusUnauthorized},
		{"private-token", "another-instance", http.StatusNotFound},
		{"private-token", "instance-1", http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/passthrough", nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("X-Instance-ID", tc.instance)
		rec := httptest.NewRecorder()
		srv.Router().ServeHTTP(rec, req)
		require.Equal(t, tc.status, rec.Code, rec.Body.String())
	}
}

// @covers AC-EXECUTORS-SSH-PASSTHROUGH-001.9
func TestRemotePassthroughStopReapsDescendants(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("uses Linux process inventory")
	}
	srv := newPassthroughAPIServer(t)
	snapshot := remoteStart(t, srv, "trap '' TERM; sleep 30 & echo $! > child-pid; wait")
	var childPID string
	require.Eventually(t, func() bool {
		data, _ := os.ReadFile(filepath.Join(srv.cfg.WorkDir, "child-pid"))
		childPID = strings.TrimSpace(string(data))
		return childPID != ""
	}, 5*time.Second, 10*time.Millisecond)
	rec := shellTerminalRequest(t, srv, http.MethodDelete, "/api/v1/passthrough/"+snapshot["process_id"].(string)+"?generation="+strconv.Itoa(int(snapshot["generation"].(float64))), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Eventually(t, func() bool {
		data, err := os.ReadFile("/proc/" + childPID + "/stat")
		return os.IsNotExist(err) || strings.Contains(string(data), ") Z ")
	}, 5*time.Second, 10*time.Millisecond)
}

func TestRemotePassthroughSafeLogs(t *testing.T) {
	core, entries := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session-1", InstanceID: "instance-1"}
	mgr := process.NewManager(cfg, log)
	t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
	srv := NewServer(cfg, mgr, nil, nil, log)
	snapshot := remoteStart(t, srv, "printf 'private-argument-secret\\n'; cat")
	stop := shellTerminalRequest(t, srv, http.MethodDelete, "/api/v1/passthrough/"+snapshot["process_id"].(string)+"?generation="+strconv.Itoa(int(snapshot["generation"].(float64))), nil)
	require.Equal(t, http.StatusOK, stop.Code)
	for _, entry := range entries.All() {
		encoded, err := json.Marshal(entry.ContextMap())
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "private-argument-secret", entry.Message)
	}
}

// @covers AC-EXECUTORS-SSH-PASSTHROUGH-001.4
func TestRemotePassthroughDisconnectPreservesProcess(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	snapshot := remoteStart(t, srv, "cat")
	server := httptest.NewServer(srv.Router())
	t.Cleanup(server.Close)
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/passthrough/stream?process_id=" + snapshot["process_id"].(string) + "&generation=" + strconv.Itoa(int(snapshot["generation"].(float64)))
	for index := range 2 {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		if index == 0 {
			require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte("remote-marker\n")))
		}
		text := readShellTerminalUntil(t, conn, "remote-marker")
		require.Contains(t, text, "remote-marker")
		require.NoError(t, conn.Close())
	}
	rec := shellTerminalRequest(t, srv, http.MethodGet, "/api/v1/passthrough", nil)
	var current map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &current))
	require.Equal(t, snapshot["process_id"], current["process_id"])
	require.Equal(t, snapshot["os_pid"], current["os_pid"])
}

func TestRemotePassthroughEarlyExitReplay(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	snapshot := remoteStart(t, srv, "exit 0")
	require.Eventually(t, func() bool {
		rec := shellTerminalRequest(t, srv, http.MethodGet, "/api/v1/passthrough/events?after=0", nil)
		return strings.Contains(rec.Body.String(), `"status":"exited"`)
	}, 5*time.Second, 10*time.Millisecond)
	rec := shellTerminalRequest(t, srv, http.MethodDelete, "/api/v1/passthrough/"+snapshot["process_id"].(string)+"?generation="+strconv.Itoa(int(snapshot["generation"].(float64))), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestRemotePassthroughEventStream(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	snapshot := remoteStart(t, srv, "cat")
	server := httptest.NewServer(srv.Router())
	t.Cleanup(server.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/v1/passthrough/events?after=0", nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(data), snapshot["process_id"].(string))
}

func TestRemotePassthroughTerminalSurvivesProcessReplacement(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	snapshot := remoteStart(t, srv, "cat")
	server := httptest.NewServer(srv.Router())
	t.Cleanup(server.Close)
	route := "/api/v1/passthrough/stream?process_id=" + snapshot["process_id"].(string) + "&generation=1"
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+route, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.Eventually(t, func() bool { return srv.procMgr.Passthrough().Snapshot().HasTerminal }, time.Second, time.Millisecond)
	resize := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/resize", map[string]any{"process_id": snapshot["process_id"], "generation": 1, "cols": 37, "rows": 12})
	require.Equal(t, http.StatusOK, resize.Code, resize.Body.String())
	rec := shellTerminalRequest(t, srv, http.MethodDelete, "/api/v1/passthrough/"+snapshot["process_id"].(string)+"?generation=1", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/start", map[string]any{
		"session_id": "session-1", "operation_id": "replacement", "expected_generation": 1, "command": []string{"/bin/sh", "-c", "stty size > replacement-size; printf replacement-ready; cat"},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(data), "replacement-ready")
	size, err := os.ReadFile(filepath.Join(srv.cfg.WorkDir, "replacement-size"))
	require.NoError(t, err)
	require.Equal(t, "12 37\n", string(size))
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte("after-reset\r")))
	for !strings.Contains(string(data), "after-reset") {
		_, data, err = conn.ReadMessage()
		require.NoError(t, err)
	}
}

func TestRemotePassthroughSnapshotTracksCurrentTurnReadiness(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	rec := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/start", map[string]any{
		"session_id": "session-1", "operation_id": "turn-state", "command": []string{"/bin/sh", "-c", "printf ready; cat"}, "idle_timeout": int64(40 * time.Millisecond),
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Eventually(t, func() bool {
		rec := shellTerminalRequest(t, srv, http.MethodGet, "/api/v1/passthrough", nil)
		var snapshot map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &snapshot))
		return snapshot["waiting_for_input"] == true
	}, time.Second, time.Millisecond)
	var snapshot map[string]any
	rec = shellTerminalRequest(t, srv, http.MethodGet, "/api/v1/passthrough", nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &snapshot))
	rec = shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/input", map[string]any{"process_id": snapshot["process_id"], "generation": snapshot["generation"], "data": "work\r"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = shellTerminalRequest(t, srv, http.MethodGet, "/api/v1/passthrough", nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &snapshot))
	require.Equal(t, false, snapshot["waiting_for_input"])
}

func TestRemotePassthroughInheritsInstanceEnvironment(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	srv.cfg.AgentEnv = []string{"E2E_REMOTE_INSTANCE_VALUE=selected"}
	remoteStart(t, srv, "printf '%s' \"$E2E_REMOTE_INSTANCE_VALUE\" > inherited; cat")
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(filepath.Join(srv.cfg.WorkDir, "inherited"))
		return err == nil && string(data) == "selected"
	}, time.Second, time.Millisecond)
}

func TestRemotePassthroughCtrlAIsInputNotResize(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	snapshot := remoteStart(t, srv, "cat")
	server := httptest.NewServer(srv.Router())
	t.Cleanup(server.Close)
	route := "/api/v1/passthrough/stream?process_id=" + snapshot["process_id"].(string) + "&generation=1"
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+route, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte{1}))
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte("after-control-a\r")))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var output strings.Builder
	for !strings.Contains(output.String(), "after-control-a") {
		_, data, err := conn.ReadMessage()
		require.NoError(t, err)
		output.Write(data)
	}
}
