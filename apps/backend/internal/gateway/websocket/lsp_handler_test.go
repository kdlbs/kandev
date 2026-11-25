package websocket

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/lsp/installer"
	"github.com/kandev/kandev/internal/lsp/protocol"
	"github.com/kandev/kandev/internal/user/models"
)

type staticLSPUserService struct {
	settings *models.UserSettings
	err      error
}

type recordingLSPLifecycleManager struct {
	runtimeName agentruntime.Runtime
	execution   *lifecycle.AgentExecution
	resolveErr  error
	ensureErr   error
	ensureCalls int
	accessErr   error
}

func (m *recordingLSPLifecycleManager) CheckSessionAccess(context.Context, string) error {
	return m.accessErr
}

func (m *recordingLSPLifecycleManager) ResolveSessionRuntime(context.Context, string) (agentruntime.Runtime, error) {
	return m.runtimeName, m.resolveErr
}

func (m *recordingLSPLifecycleManager) GetOrEnsureExecution(context.Context, string) (*lifecycle.AgentExecution, error) {
	m.ensureCalls++
	return m.execution, m.ensureErr
}

type recordingLSPMessageWriter struct {
	deadline        time.Time
	deadlineOnWrite time.Time
	messageType     int
	payload         []byte
}

func (w *recordingLSPMessageWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func (w *recordingLSPMessageWriter) WriteMessage(messageType int, payload []byte) error {
	w.deadlineOnWrite = w.deadline
	w.messageType = messageType
	w.payload = payload
	return nil
}

func (s staticLSPUserService) GetUserSettings(context.Context) (*models.UserSettings, error) {
	return s.settings, s.err
}

func TestContinuityUserSettingsFailureKeepsAuthenticatedOwner(t *testing.T) {
	handler := &LSPHandler{userService: staticLSPUserService{err: errors.New("settings unavailable")}, logger: testLogger()}
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1"})
	userID, configuration, autoInstall := handler.continuityUserSettings(ctx, "go")
	if userID != "user-1" {
		t.Fatalf("settings owner = %q, want authenticated user", userID)
	}
	if len(configuration) != 0 || autoInstall {
		t.Fatalf("settings fallback = (%#v, %v), want empty configuration and auto-install disabled", configuration, autoInstall)
	}
}

func TestResolveContinuityExecutionRequiresSessionAccess(t *testing.T) {
	manager := &recordingLSPLifecycleManager{
		runtimeName: agentruntime.RuntimeStandalone,
		execution:   &lifecycle.AgentExecution{ID: "execution-1", SessionID: "session-1"},
		accessErr:   errors.New("not authorized"),
	}
	handler := &LSPHandler{lifecycleMgr: manager, leases: newLSPLeaseManager(1, testLogger())}

	execution, closeCode, _ := handler.resolveContinuityExecution(context.Background(), "session-1")
	if execution != nil || closeCode != lspCloseSessionNotFound {
		t.Fatalf("continuity execution = (%v, %d), want access denied", execution, closeCode)
	}
	if manager.ensureCalls != 0 {
		t.Fatalf("GetOrEnsureExecution calls = %d, want none after access denial", manager.ensureCalls)
	}
}

func TestReadLSPMessage_Valid(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	reader := bufio.NewReader(strings.NewReader(raw))

	msg, err := protocol.ReadMessage(reader)
	if err != nil {
		t.Fatalf("readLSPMessage() error = %v", err)
	}
	if string(msg) != body {
		t.Errorf("readLSPMessage() = %q, want %q", string(msg), body)
	}
}

func TestReadLSPMessage_WithExtraHeaders(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":2}`
	raw := fmt.Sprintf("Content-Length: %d\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n%s", len(body), body)
	reader := bufio.NewReader(strings.NewReader(raw))

	msg, err := protocol.ReadMessage(reader)
	if err != nil {
		t.Fatalf("readLSPMessage() error = %v", err)
	}
	if string(msg) != body {
		t.Errorf("readLSPMessage() = %q, want %q", string(msg), body)
	}
}

func TestReadLSPMessage_MissingContentLength(t *testing.T) {
	raw := "Content-Type: application/json\r\n\r\n{}"
	reader := bufio.NewReader(strings.NewReader(raw))

	_, err := protocol.ReadMessage(reader)
	if err == nil {
		t.Fatal("readLSPMessage() should return error for missing Content-Length")
	}
	if !strings.Contains(err.Error(), "missing Content-Length") {
		t.Errorf("readLSPMessage() error = %q, want 'missing Content-Length'", err.Error())
	}
}

func TestReadLSPMessage_InvalidContentLength(t *testing.T) {
	raw := "Content-Length: abc\r\n\r\n{}"
	reader := bufio.NewReader(strings.NewReader(raw))

	_, err := protocol.ReadMessage(reader)
	if err == nil {
		t.Fatal("readLSPMessage() should return error for invalid Content-Length")
	}
	if !strings.Contains(err.Error(), "invalid Content-Length") {
		t.Errorf("readLSPMessage() error = %q, want 'invalid Content-Length'", err.Error())
	}
}

func TestReadLSPMessage_EOF(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader(nil))

	_, err := protocol.ReadMessage(reader)
	if err == nil {
		t.Fatal("readLSPMessage() should return error on EOF")
	}
	if err != io.EOF {
		t.Errorf("readLSPMessage() error = %v, want io.EOF", err)
	}
}

func TestReadLSPMessage_MultipleMessages(t *testing.T) {
	body1 := `{"jsonrpc":"2.0","id":1}`
	body2 := `{"jsonrpc":"2.0","id":2}`
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n%sContent-Length: %d\r\n\r\n%s",
		len(body1), body1, len(body2), body2)
	reader := bufio.NewReader(strings.NewReader(raw))

	msg1, err := protocol.ReadMessage(reader)
	if err != nil {
		t.Fatalf("first readLSPMessage() error = %v", err)
	}
	if string(msg1) != body1 {
		t.Errorf("first readLSPMessage() = %q, want %q", string(msg1), body1)
	}

	msg2, err := protocol.ReadMessage(reader)
	if err != nil {
		t.Fatalf("second readLSPMessage() error = %v", err)
	}
	if string(msg2) != body2 {
		t.Errorf("second readLSPMessage() = %q, want %q", string(msg2), body2)
	}
}

func TestLspCommand_ViaRegistry(t *testing.T) {
	tests := []struct {
		language   string
		wantBinary string
		wantArgs   []string
	}{
		{"typescript", "typescript-language-server", []string{"--stdio"}},
		{"go", "gopls", []string{"serve"}},
		{"rust", "rust-analyzer", nil},
		{"python", "pyright-langserver", []string{"--stdio"}},
		{"kotlin", "kotlin-lsp", []string{"--stdio"}},
		{"unknown", "", nil},
	}
	for _, tc := range tests {
		binary, args := installer.LspCommand(tc.language)
		if binary != tc.wantBinary {
			t.Errorf("LspCommand(%q) binary = %q, want %q", tc.language, binary, tc.wantBinary)
		}
		if len(args) != len(tc.wantArgs) {
			t.Errorf("LspCommand(%q) args = %v, want %v", tc.language, args, tc.wantArgs)
			continue
		}
		for i := range args {
			if args[i] != tc.wantArgs[i] {
				t.Errorf("LspCommand(%q) args[%d] = %q, want %q", tc.language, i, args[i], tc.wantArgs[i])
			}
		}
	}
}

func TestIsValidLSPLanguage_ViaRegistry(t *testing.T) {
	tests := []struct {
		language string
		want     bool
	}{
		{"typescript", true},
		{"go", true},
		{"rust", true},
		{"python", true},
		{"kotlin", true},
		{"java", false},
		{"", false},
		{"ruby", false},
	}
	for _, tc := range tests {
		if got := installer.IsSupported(tc.language); got != tc.want {
			t.Errorf("IsSupported(%q) = %v, want %v", tc.language, got, tc.want)
		}
	}
}

func TestCloseCodeConstants(t *testing.T) {
	// Verify close codes are in the expected range (4000-4999 for application-specific)
	codes := []struct {
		name string
		code int
	}{
		{"lspCloseSessionNotFound", lspCloseSessionNotFound},
		{"lspCloseUnsupportedExecutor", lspCloseUnsupportedExecutor},
		{"lspCloseCapacityExceeded", lspCloseCapacityExceeded},
		{"lspCloseTransport", lspCloseTransport},
		{"lspCloseRuntimeStopped", lspCloseRuntimeStopped},
	}
	for _, tc := range codes {
		if tc.code < 4000 || tc.code > 4999 {
			t.Errorf("%s = %d, want value in range 4000-4999", tc.name, tc.code)
		}
	}
	// Verify they're distinct
	seen := make(map[int]string)
	for _, tc := range codes {
		if prev, ok := seen[tc.code]; ok {
			t.Errorf("%s and %s have the same code %d", prev, tc.name, tc.code)
		}
		seen[tc.code] = tc.name
	}
}

func TestWriteLSPProxyMessageSetsDeadlineBeforeWriting(t *testing.T) {
	writer := &recordingLSPMessageWriter{}
	startedAt := time.Now()

	if err := writeLSPProxyMessage(writer, gorillaws.TextMessage, []byte("payload")); err != nil {
		t.Fatalf("writeLSPProxyMessage() error = %v", err)
	}
	if writer.deadlineOnWrite.IsZero() {
		t.Fatal("WriteMessage() called without a write deadline")
	}
	if writer.deadlineOnWrite.Before(startedAt.Add(lspProxyWriteTimeout)) {
		t.Fatalf("write deadline = %v, want at least %v", writer.deadlineOnWrite, startedAt.Add(lspProxyWriteTimeout))
	}
}

func TestLSPRuntimeSupported(t *testing.T) {
	tests := []struct {
		runtime agentruntime.Runtime
		want    bool
	}{
		{agentruntime.RuntimeStandalone, true},
		{agentruntime.RuntimeDocker, true},
		{agentruntime.RuntimeSprites, false},
		{agentruntime.RuntimeRemoteDocker, false},
		{agentruntime.RuntimeSSH, false},
	}
	for _, tc := range tests {
		if got := lspRuntimeSupported(tc.runtime); got != tc.want {
			t.Fatalf("lspRuntimeSupported(%q) = %v, want %v", tc.runtime, got, tc.want)
		}
	}
}

func TestResolveContinuityExecutionEnsuresLocalDockerRuntime(t *testing.T) {
	const sessionID = "session-docker"
	execution := &lifecycle.AgentExecution{
		ID: "execution-docker", SessionID: sessionID, RuntimeName: agentruntime.RuntimeDocker,
	}
	manager := &recordingLSPLifecycleManager{
		runtimeName: agentruntime.RuntimeDocker,
		execution:   execution,
	}
	handler := newLSPHandler(manager, nil, testLogger(), 1, nil, nil)

	got, closeCode, message := handler.resolveContinuityExecution(t.Context(), sessionID)
	if got != execution || closeCode != 0 {
		t.Fatalf("resolveContinuityExecution() = (%p, %d, %q), want (%p, 0, empty)", got, closeCode, message, execution)
	}
	if manager.ensureCalls != 1 {
		t.Fatalf("GetOrEnsureExecution calls = %d, want 1", manager.ensureCalls)
	}
}

func TestShouldAutoInstallRejectsManualOnlyLanguage(t *testing.T) {
	handler := &LSPHandler{userService: staticLSPUserService{settings: &models.UserSettings{
		LspAutoInstallLanguages: []string{"kotlin", "python", "rust"},
	}}}

	if _, _, autoInstall := handler.continuityUserSettings(t.Context(), "kotlin"); autoInstall {
		t.Fatal("Kotlin must remain manual-install-only even if stale settings contain it")
	}
	if _, _, autoInstall := handler.continuityUserSettings(t.Context(), "python"); !autoInstall {
		t.Fatal("Python should honor its auto-install setting")
	}
	if _, _, autoInstall := handler.continuityUserSettings(t.Context(), "rust"); !autoInstall {
		t.Fatal("Rust preference must reach agentctl so the task host decides platform support")
	}
}
