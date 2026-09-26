package websocket

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// failingWriter stands in for a PTY whose process died underneath the bridge.
type failingWriter struct{ writes int }

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	return 0, errors.New("pty gone")
}

// startDeferredUserShell registers a user shell whose PTY start is deferred to
// the first resize, so no OS process is ever spawned.
func startDeferredUserShell(
	t *testing.T,
	runner *process.InteractiveRunner,
	scopeID, sessionID, terminalID string,
) *process.InteractiveProcessInfo {
	t.Helper()
	info, err := runner.StartUserShell(
		context.Background(), scopeID, sessionID, terminalID, t.TempDir(), "", &process.UserShellOptions{Label: "Terminal"},
	)
	if err != nil {
		t.Fatalf("StartUserShell: %v", err)
	}
	t.Cleanup(func() { _ = runner.StopUserShell(context.Background(), scopeID, terminalID) })
	return info
}

// startDeferredPassthrough registers an agent passthrough process whose PTY
// start is deferred, so no OS process is ever spawned.
func startDeferredPassthrough(
	t *testing.T,
	runner *process.InteractiveRunner,
	sessionID string,
) *process.InteractiveProcessInfo {
	t.Helper()
	info, err := runner.Start(context.Background(), process.InteractiveStartRequest{
		SessionID:            sessionID,
		Command:              []string{"/bin/sh"},
		WorkingDir:           t.TempDir(),
		DisableTurnDetection: true,
	})
	if err != nil {
		t.Fatalf("Start passthrough process: %v", err)
	}
	t.Cleanup(func() { _ = runner.Stop(context.Background(), info.ID) })
	return info
}

func TestSetupPtyAccessCommonWiresWriterAndDirectOutput(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)
	info := startDeferredPassthrough(t, runner, "session-setup")

	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	pty := &bytes.Buffer{}
	var ptyWriter io.Writer
	ok := handler.setupPtyAccessCommon(
		"session-setup", info.ID, runner, wsw, &ptyWriter,
		func() (io.Writer, error) { return pty, nil },
	)
	if !ok {
		t.Fatal("setupPtyAccessCommon() = false, want true")
	}
	if ptyWriter != io.Writer(pty) {
		t.Fatalf("ptyWriter = %#v, want the writer returned by getWriter", ptyWriter)
	}
	if !runner.HasActiveWebSocket(info.ID) {
		t.Fatal("direct output was not registered on the process")
	}
}

func TestSetupPtyAccessCommonLeavesWriterUnsetWhenPtyUnavailable(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)
	info := startDeferredPassthrough(t, runner, "session-nopty")

	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	var ptyWriter io.Writer
	ok := handler.setupPtyAccessCommon(
		"session-nopty", info.ID, runner, wsw, &ptyWriter,
		func() (io.Writer, error) { return nil, errors.New("pty not ready") },
	)
	if ok {
		t.Fatal("setupPtyAccessCommon() = true, want false when the PTY writer is unavailable")
	}
	if ptyWriter != nil {
		t.Fatalf("ptyWriter = %#v, want nil", ptyWriter)
	}
	if runner.HasActiveWebSocket(info.ID) {
		t.Fatal("direct output must not be registered when PTY setup fails")
	}
	if got := countLogged(recorded, "PTY writer not ready yet"); got != 1 {
		t.Fatalf("not-ready records = %d, want 1", got)
	}
}

// A PTY writer must not be published to the bridge when direct output could not
// be attached: the bridge would then forward keystrokes into a process whose
// output nobody is reading.
func TestSetupPtyAccessCommonRejectsWhenDirectOutputCannotBeSet(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	pty := &bytes.Buffer{}
	var ptyWriter io.Writer
	ok := handler.setupPtyAccessCommon(
		"session-gone", "proc-gone", runner, wsw, &ptyWriter,
		func() (io.Writer, error) { return pty, nil },
	)
	if ok {
		t.Fatal("setupPtyAccessCommon() = true, want false when SetDirectOutput fails")
	}
	if ptyWriter != nil {
		t.Fatalf("ptyWriter = %#v, want nil when direct output could not be set", ptyWriter)
	}
	if got := countLogged(recorded, "failed to set direct output"); got != 1 {
		t.Fatalf("direct-output failure records = %d, want 1", got)
	}
}

func TestSetupPtyAccessVariantsRejectDeferredProcesses(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	passthrough := startDeferredPassthrough(t, runner, "session-deferred")
	var ptyWriter io.Writer
	if handler.setupPtyAccess("session-deferred", passthrough.ID, runner, wsw, &ptyWriter) {
		t.Fatal("setupPtyAccess() = true for a process that has not started")
	}

	shell := startDeferredUserShell(t, runner, "env-deferred", "session-deferred", "term-deferred")
	var shellWriter io.Writer
	if handler.setupUserShellPtyAccess(
		"session-deferred", "env-deferred", "term-deferred", shell.ID, runner, wsw, &shellWriter,
	) {
		t.Fatal("setupUserShellPtyAccess() = true for a shell that has not started")
	}
}

func TestHandleResizeRejectsMalformedAndZeroDimensions(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantLog string
	}{
		{name: "malformed json", payload: `{"cols":`, wantLog: "failed to parse resize command"},
		{name: "zero cols", payload: `{"cols":0,"rows":40}`, wantLog: "invalid resize dimensions"},
		{name: "zero rows", payload: `{"cols":80,"rows":0}`, wantLog: "invalid resize dimensions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, recorded := observedTerminalLogger(t)
			handler, _ := newBridgeTestHandler(t, log)
			runner := newBridgeTestRunner(t, log)

			got := handler.handleResize([]byte(tt.payload), "session-1", "proc-1", runner)
			if got != "proc-1" {
				t.Fatalf("handleResize() = %q, want the unchanged process id", got)
			}
			if n := countLogged(recorded, tt.wantLog); n != 1 {
				t.Fatalf("%q records = %d, want 1", tt.wantLog, n)
			}
			if n := countLogged(recorded, "PTY resized"); n != 0 {
				t.Fatalf("PTY resized records = %d, want 0", n)
			}
		})
	}
}

func TestHandleResizeKeepsProcessIDWhenSessionFallbackFails(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	got := handler.handleResize([]byte(`{"cols":80,"rows":24}`), "session-missing", "proc-missing", runner)
	if got != "proc-missing" {
		t.Fatalf("handleResize() = %q, want %q", got, "proc-missing")
	}
	if n := countLogged(recorded, "failed to resize PTY (both process and session)"); n != 1 {
		t.Fatalf("resize-failure records = %d, want 1", n)
	}
}

func TestDiscoverProcessBySessionAdoptsRestartedProcess(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)
	info := startDeferredPassthrough(t, runner, "session-restart")

	got := handler.discoverProcessBySession("session-restart", "stale-proc", runner)
	if got != info.ID {
		t.Fatalf("discoverProcessBySession() = %q, want the live process id %q", got, info.ID)
	}

	// Same ID in, same ID out — the bridge must not churn state on a no-op.
	if same := handler.discoverProcessBySession("session-restart", info.ID, runner); same != info.ID {
		t.Fatalf("discoverProcessBySession() = %q, want %q", same, info.ID)
	}
}

func TestDiscoverProcessBySessionKeepsOldIDWhenSessionHasNoProcess(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	if got := handler.discoverProcessBySession("session-empty", "stale-proc", runner); got != "stale-proc" {
		t.Fatalf("discoverProcessBySession() = %q, want %q", got, "stale-proc")
	}
}

func TestResizeBySessionFallbackKeepsOldProcessIDOnFailure(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	got := handler.resizeBySessionFallback("session-empty", "old-proc", 80, 24, runner)
	if got != "old-proc" {
		t.Fatalf("resizeBySessionFallback() = %q, want %q", got, "old-proc")
	}
	if n := countLogged(recorded, "failed to resize PTY (both process and session)"); n != 1 {
		t.Fatalf("resize-failure records = %d, want 1", n)
	}
}

// Enter is the signal that the user submitted a command; only then may the
// passthrough execution be promoted to RUNNING.
func TestDetectInputSubmissionMarksPassthroughRunningOnlyOnEnter(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, manager := newBridgeTestHandler(t, log)

	execution := &lifecycle.AgentExecution{
		ID:                   "exec-1",
		SessionID:            "session-enter",
		PassthroughProcessID: "proc-1",
		Status:               v1.AgentStatusStarting,
	}
	if err := manager.ExecutionStoreForTesting().Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	handler.detectInputSubmission("session-enter", []byte("echo hello"))
	if execution.Status != v1.AgentStatusStarting {
		t.Fatalf("status = %q after keystrokes without Enter, want %q",
			execution.Status, v1.AgentStatusStarting)
	}

	handler.detectInputSubmission("session-enter", []byte("\r"))
	if execution.Status != v1.AgentStatusRunning {
		t.Fatalf("status = %q after Enter, want %q", execution.Status, v1.AgentStatusRunning)
	}
}

func TestDetectInputSubmissionSurvivesUnknownSession(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)

	handler.detectInputSubmission("session-unknown", []byte("ls\n"))
	if n := countLogged(recorded, "failed to mark passthrough as running"); n != 1 {
		t.Fatalf("mark-failure records = %d, want 1", n)
	}
}

func TestWriteToReadyPtyForwardsInputVerbatim(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	pty := &bytes.Buffer{}
	var ptyWriter io.Writer = pty
	gotID, gotDirect := handler.writeToReadyPty(
		[]byte("git status"), "session-1", "proc-1", runner, wsw, &ptyWriter, true,
	)
	if gotID != "proc-1" || !gotDirect {
		t.Fatalf("writeToReadyPty() = (%q, %v), want (\"proc-1\", true)", gotID, gotDirect)
	}
	if pty.String() != "git status" {
		t.Fatalf("PTY received %q, want %q", pty.String(), "git status")
	}
}

// A dead PTY must reset bridge state immediately and re-discover the session's
// current process, instead of waiting for a resize that may never arrive.
func TestWriteToReadyPtyResetsAndRediscoversAfterWriteFailure(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)
	replacement := startDeferredPassthrough(t, runner, "session-dead")

	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	pty := &failingWriter{}
	var ptyWriter io.Writer = pty
	gotID, gotDirect := handler.writeToReadyPty(
		[]byte("ls\n"), "session-dead", "stale-proc", runner, wsw, &ptyWriter, true,
	)
	if gotID != replacement.ID {
		t.Fatalf("writeToReadyPty() process id = %q, want the rediscovered %q", gotID, replacement.ID)
	}
	if gotDirect {
		t.Fatal("writeToReadyPty() directOutputSet = true, want false after a PTY write failure")
	}
	if ptyWriter != nil {
		t.Fatalf("ptyWriter = %#v, want nil after a PTY write failure", ptyWriter)
	}
	if pty.writes != 1 {
		t.Fatalf("PTY writes = %d, want exactly 1", pty.writes)
	}
	if n := countLogged(recorded, "PTY write error"); n != 1 {
		t.Fatalf("PTY write error records = %d, want 1", n)
	}
	// The failed keystroke must not be reported as a submitted command.
	if n := countLogged(recorded, "failed to mark passthrough as running"); n != 0 {
		t.Fatalf("passthrough-mark attempts = %d, want 0 after a failed write", n)
	}
}

func TestWriteToUnreadyPtyDropsInputAndAdoptsDiscoveredProcess(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)
	replacement := startDeferredPassthrough(t, runner, "session-unready")

	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	var ptyWriter io.Writer
	gotID, gotDirect := handler.writeToUnreadyPty(
		[]byte("ls\n"), "session-unready", "stale-proc", runner, wsw, &ptyWriter, false,
	)
	if gotID != replacement.ID {
		t.Fatalf("writeToUnreadyPty() process id = %q, want %q", gotID, replacement.ID)
	}
	if gotDirect {
		t.Fatal("writeToUnreadyPty() directOutputSet = true, want false while the PTY is unavailable")
	}
	if n := countLogged(recorded, "PTY not ready, dropping input"); n != 1 {
		t.Fatalf("dropped-input records = %d, want 1", n)
	}
}

func TestHandleTerminalInputRoutesByPtyReadiness(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	pty := &bytes.Buffer{}
	var ready io.Writer = pty
	if id, direct := handler.handleTerminalInput(
		[]byte("a"), "session-1", "proc-1", runner, wsw, &ready, true,
	); id != "proc-1" || !direct {
		t.Fatalf("handleTerminalInput(ready) = (%q, %v), want (\"proc-1\", true)", id, direct)
	}
	if pty.String() != "a" {
		t.Fatalf("PTY received %q, want %q", pty.String(), "a")
	}

	var unready io.Writer
	if id, direct := handler.handleTerminalInput(
		[]byte("b"), "session-1", "proc-1", runner, wsw, &unready, false,
	); id != "proc-1" || direct {
		t.Fatalf("handleTerminalInput(unready) = (%q, %v), want (\"proc-1\", false)", id, direct)
	}
	if n := countLogged(recorded, "PTY not ready, dropping input"); n != 1 {
		t.Fatalf("dropped-input records = %d, want 1", n)
	}
	if pty.String() != "a" {
		t.Fatalf("PTY received %q, want the unready input to be dropped", pty.String())
	}
}

func TestUserShellInputRoutesByPtyReadiness(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	pty := &bytes.Buffer{}
	var ready io.Writer = pty
	if direct := handler.handleUserShellInput(
		[]byte("pwd\n"), "session-1", "env-1", "term-1", "proc-1", runner, wsw, &ready, true,
	); !direct {
		t.Fatal("handleUserShellInput(ready) = false, want true")
	}
	if pty.String() != "pwd\n" {
		t.Fatalf("shell PTY received %q, want %q", pty.String(), "pwd\n")
	}

	var unready io.Writer
	if direct := handler.handleUserShellInput(
		[]byte("pwd\n"), "session-1", "env-1", "term-1", "proc-1", runner, wsw, &unready, false,
	); direct {
		t.Fatal("handleUserShellInput(unready) = true, want false")
	}
	if n := countLogged(recorded, "PTY not ready, dropping input"); n != 1 {
		t.Fatalf("dropped-input records = %d, want 1", n)
	}
}

func TestWriteToReadyUserShellPtyKeepsStateWhenReconnectFails(t *testing.T) {
	log, recorded := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)

	_, server := newTerminalWSPair(t)
	wsw := newWsWriter(server)
	t.Cleanup(func() { _ = wsw.Close() })

	pty := &failingWriter{}
	var ptyWriter io.Writer = pty
	direct := handler.writeToReadyUserShellPty(
		[]byte("ls\n"), "session-1", "env-1", "term-gone", "proc-gone", runner, wsw, &ptyWriter, false,
	)
	if direct {
		t.Fatal("writeToReadyUserShellPty() = true, want false when the shell cannot be re-established")
	}
	if pty.writes != 1 {
		t.Fatalf("PTY writes = %d, want exactly 1 (no retry without a new writer)", pty.writes)
	}
	if n := countLogged(recorded, "PTY write error"); n != 1 {
		t.Fatalf("PTY write error records = %d, want 1", n)
	}
}

func TestHandleUserShellResizeRejectsBadPayloads(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantLog string
	}{
		{name: "malformed json", payload: `not-json`, wantLog: "failed to parse resize command"},
		{name: "zero dimensions", payload: `{"cols":0,"rows":0}`, wantLog: "invalid resize dimensions"},
		{name: "unknown shell", payload: `{"cols":80,"rows":24}`, wantLog: "failed to resize user shell PTY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, recorded := observedTerminalLogger(t)
			handler, _ := newBridgeTestHandler(t, log)
			runner := newBridgeTestRunner(t, log)

			handler.handleUserShellResize([]byte(tt.payload), "session-1", "env-1", "term-1", runner)

			if n := countLogged(recorded, tt.wantLog); n != 1 {
				t.Fatalf("%q records = %d, want 1", tt.wantLog, n)
			}
			if n := countLogged(recorded, "user shell PTY resized"); n != 0 {
				t.Fatalf("success records = %d, want 0", n)
			}
		})
	}
}

func TestOSPIDForInteractiveProcessHandlesMissingInputs(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	runner := newBridgeTestRunner(t, log)
	deferredProc := startDeferredPassthrough(t, runner, "session-pid")

	if got := osPIDForInteractiveProcess(nil, "proc-1"); got != 0 {
		t.Fatalf("osPIDForInteractiveProcess(nil runner) = %d, want 0", got)
	}
	if got := osPIDForInteractiveProcess(runner, ""); got != 0 {
		t.Fatalf("osPIDForInteractiveProcess(empty id) = %d, want 0", got)
	}
	if got := osPIDForInteractiveProcess(runner, "proc-unknown"); got != 0 {
		t.Fatalf("osPIDForInteractiveProcess(unknown id) = %d, want 0", got)
	}
	if got := osPIDForInteractiveProcess(runner, deferredProc.ID); got != 0 {
		t.Fatalf("osPIDForInteractiveProcess(deferred process) = %d, want 0", got)
	}
}
