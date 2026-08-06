package loginpty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	gorillaws "github.com/gorilla/websocket"
)

type hostShellBinderStub struct {
	tabID     string
	sessionID string
	err       error
}

func (s *hostShellBinderStub) BindHostShellSession(_ context.Context, tabID, sessionID string) error {
	s.tabID = tabID
	s.sessionID = sessionID
	return s.err
}

func TestDetectShellWindows(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		available map[string]bool
		wantShell string
		wantArgs  []string
	}{
		{
			name:      "prefers PowerShell 7",
			available: map[string]bool{"pwsh.exe": true, "powershell.exe": true},
			wantShell: "pwsh.exe",
			wantArgs:  []string{"-NoLogo", "-NoExit"},
		},
		{
			name:      "falls back to Windows PowerShell",
			available: map[string]bool{"powershell.exe": true},
			wantShell: "powershell.exe",
			wantArgs:  []string{"-NoLogo", "-NoExit"},
		},
		{
			name:      "uses COMSPEC when PowerShell is unavailable",
			env:       map[string]string{"COMSPEC": `C:\\Windows\\System32\\cmd.exe`},
			wantShell: `C:\\Windows\\System32\\cmd.exe`,
		},
		{
			name:      "falls back to cmd when COMSPEC is unset",
			wantShell: "cmd.exe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shell, args := detectShellForOS(
				"windows",
				func(key string) string { return tt.env[key] },
				func(candidate string) bool { return tt.available[candidate] },
			)
			if shell != tt.wantShell {
				t.Fatalf("shell = %q, want %q", shell, tt.wantShell)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, tt.wantArgs)
			}
		})
	}
}

func newHostShellRouterWithBinder(t *testing.T, mgr *Manager, binder HostShellSessionBinder) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := NewHandlers(mgr, nil, mgr.log.Zap(), nil)
	handlers.SetHostShellSessionBinder(binder)
	handlers.RegisterRoutes(router)
	return router
}

func newHostShellRouter(t *testing.T, mgr *Manager) *gin.Engine {
	return newHostShellRouterWithBinder(t, mgr, nil)
}

func startHostShellRequest(t *testing.T, router http.Handler, body map[string]any) (Status, int) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/host-shell/start", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		return Status{}, response.Code
	}
	var status Status
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	return status, response.Code
}

func stopSession(t *testing.T, mgr *Manager, sessionID string) {
	t.Helper()
	if sess := mgr.GetByID(sessionID); sess != nil {
		sess.stop()
	}
}

func connectHostShellStream(t *testing.T, serverURL, sessionID string) *gorillaws.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + fmt.Sprintf("/api/v1/agent-login/sessions/%s/stream", sessionID)
	conn, _, err := gorillaws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial stream: %v", err)
	}
	return conn
}

func waitForHostShellOutput(t *testing.T, conn *gorillaws.Conn, needle string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var output strings.Builder
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read message: %v; output so far: %q", err, output.String())
		}
		if messageType != gorillaws.BinaryMessage {
			continue
		}
		output.Write(data)
		if strings.Contains(output.String(), needle) {
			return
		}
	}
	t.Fatalf("timed out waiting for %q in %q", needle, output.String())
}

func TestHostShellClientIDIsIdempotentAndIndependent(t *testing.T) {
	mgr := newTestManager(t, nil)
	router := newHostShellRouter(t, mgr)

	firstClient := uuid.NewString()
	secondClient := uuid.NewString()
	first, code := startHostShellRequest(t, router, map[string]any{
		"client_id": firstClient,
		"cols":      80,
		"rows":      24,
	})
	if code != http.StatusOK {
		t.Fatalf("first start status = %d", code)
	}
	t.Cleanup(func() { stopSession(t, mgr, first.ID) })
	if first.AgentID != hostShellAgentID {
		t.Fatalf("agent id = %q, want %q", first.AgentID, hostShellAgentID)
	}

	repeated, code := startHostShellRequest(t, router, map[string]any{"client_id": firstClient})
	if code != http.StatusOK {
		t.Fatalf("repeated start status = %d", code)
	}
	if repeated.ID != first.ID {
		t.Fatalf("repeated start id = %q, want %q", repeated.ID, first.ID)
	}

	second, code := startHostShellRequest(t, router, map[string]any{"client_id": secondClient})
	if code != http.StatusOK {
		t.Fatalf("second start status = %d", code)
	}
	t.Cleanup(func() { stopSession(t, mgr, second.ID) })
	if second.ID == first.ID {
		t.Fatalf("different client id reused session %q", second.ID)
	}
	if second.AgentID != hostShellAgentID {
		t.Fatalf("second agent id = %q, want %q", second.AgentID, hostShellAgentID)
	}

	stopSession(t, mgr, first.ID)
	if got := mgr.GetByID(second.ID); got == nil || !got.Status().Running {
		t.Fatal("stopping the first client session affected its sibling")
	}
}

func TestHostShellClientIDValidationAndLegacySingleton(t *testing.T) {
	mgr := newTestManager(t, nil)
	router := newHostShellRouter(t, mgr)

	invalid, code := startHostShellRequest(t, router, map[string]any{"client_id": "not-a-uuid"})
	if code != http.StatusBadRequest {
		if invalid.ID != "" {
			stopSession(t, mgr, invalid.ID)
		}
		t.Fatalf("invalid client id status = %d, want %d", code, http.StatusBadRequest)
	}

	first, code := startHostShellRequest(t, router, map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("legacy first start status = %d", code)
	}
	t.Cleanup(func() { stopSession(t, mgr, first.ID) })
	second, code := startHostShellRequest(t, router, map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("legacy repeated start status = %d", code)
	}
	if second.ID != first.ID {
		t.Fatalf("legacy repeated start id = %q, want %q", second.ID, first.ID)
	}
}

func TestHostShellRejectsMalformedJSON(t *testing.T) {
	mgr := newTestManager(t, nil)
	router := newHostShellRouter(t, mgr)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/host-shell/start",
		bytes.NewBufferString(`{"client_id":`),
	)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed body status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	mgr.mu.Lock()
	activeSessions := len(mgr.sessions)
	mgr.mu.Unlock()
	if activeSessions != 0 {
		t.Fatal("malformed body started a host shell session")
	}
}

func TestHostShellClientIDBindsSessionBeforeResponding(t *testing.T) {
	mgr := newTestManager(t, nil)
	binder := &hostShellBinderStub{}
	router := newHostShellRouterWithBinder(t, mgr, binder)
	tabID := uuid.NewString()

	status, code := startHostShellRequest(t, router, map[string]any{"client_id": tabID})
	if code != http.StatusOK {
		t.Fatalf("start status = %d", code)
	}
	t.Cleanup(func() { stopSession(t, mgr, status.ID) })
	if binder.tabID != tabID {
		t.Fatalf("binder tab id = %q, want %q", binder.tabID, tabID)
	}
	if binder.sessionID != status.ID {
		t.Fatalf("binder session id = %q, want %q", binder.sessionID, status.ID)
	}
}

func TestHostShellSessionSurvivesWebSocketReconnect(t *testing.T) {
	mgr := newTestManager(t, nil)
	router := newHostShellRouter(t, mgr)
	server := httptest.NewServer(router)
	defer server.Close()

	clientID := uuid.NewString()
	status, code := startHostShellRequest(t, router, map[string]any{"client_id": clientID})
	if code != http.StatusOK {
		t.Fatalf("start status = %d", code)
	}
	t.Cleanup(func() { stopSession(t, mgr, status.ID) })

	firstConn := connectHostShellStream(t, server.URL, status.ID)
	waitForHostShellOutput(t, firstConn, "%")
	if err := firstConn.WriteMessage(gorillaws.BinaryMessage, []byte("export KANDEV_QT_ONE=QUICK_TERMINAL_ONE && echo $KANDEV_QT_ONE\n")); err != nil {
		t.Fatalf("write export command: %v", err)
	}
	waitForHostShellOutput(t, firstConn, "QUICK_TERMINAL_ONE")
	_ = firstConn.Close()

	secondConn := connectHostShellStream(t, server.URL, status.ID)
	defer func() { _ = secondConn.Close() }()
	if err := secondConn.WriteMessage(gorillaws.BinaryMessage, []byte("echo $KANDEV_QT_ONE\n")); err != nil {
		t.Fatalf("write echo command: %v", err)
	}
	waitForHostShellOutput(t, secondConn, "QUICK_TERMINAL_ONE")
}
