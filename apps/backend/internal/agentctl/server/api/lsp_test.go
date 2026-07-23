package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	tools "github.com/kandev/kandev/internal/tools/installer"
)

type orderedLSPReadCloser struct {
	readStarted     chan struct{}
	releaseRead     chan struct{}
	readReturned    chan struct{}
	closedAfterRead chan bool
	readOnce        sync.Once
	releaseOnce     sync.Once
	closeOnce       sync.Once
}

func newOrderedLSPReadCloser() *orderedLSPReadCloser {
	return &orderedLSPReadCloser{
		readStarted:     make(chan struct{}),
		releaseRead:     make(chan struct{}),
		readReturned:    make(chan struct{}),
		closedAfterRead: make(chan bool, 1),
	}
}

func (r *orderedLSPReadCloser) Read([]byte) (int, error) {
	r.readOnce.Do(func() { close(r.readStarted) })
	<-r.releaseRead
	select {
	case <-r.readReturned:
	default:
		close(r.readReturned)
	}
	return 0, io.EOF
}

func (r *orderedLSPReadCloser) Close() error {
	r.closeOnce.Do(func() {
		select {
		case <-r.readReturned:
			r.closedAfterRead <- true
		default:
			r.closedAfterRead <- false
		}
		r.release()
	})
	return nil
}

func (r *orderedLSPReadCloser) release() {
	r.releaseOnce.Do(func() { close(r.releaseRead) })
}

type discardLSPWriteCloser struct{}

func (discardLSPWriteCloser) Write(data []byte) (int, error) { return len(data), nil }
func (discardLSPWriteCloser) Close() error                   { return nil }

func TestRunLSPBridgeClosesStdoutAfterForwarderReturns(t *testing.T) {
	server := newTestServer(t)
	stdout := newOrderedLSPReadCloser()
	t.Cleanup(stdout.release)
	processDone := make(chan struct{})
	close(processDone)
	lspProcess := &lspServerProcess{
		id: "already-stopped", stdin: discardLSPWriteCloser{}, stdout: stdout, done: processDone,
	}
	handlerDone := make(chan error, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conn, err := server.upgrader.Upgrade(writer, request, nil)
		if err == nil {
			server.runLSPBridge(conn, "kotlin", lspProcess)
		}
		handlerDone <- err
	}))
	t.Cleanup(func() {
		stdout.release()
		httpServer.Close()
	})

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial bridge: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	select {
	case <-stdout.readStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("stdout forwarder did not start")
	}
	_ = conn.Close()
	stdout.release()
	select {
	case err := <-handlerDone:
		if err != nil {
			t.Fatalf("bridge handler: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("bridge handler did not return")
	}
	select {
	case closedAfterRead := <-stdout.closedAfterRead:
		if !closedAfterRead {
			t.Fatal("stdout closed before its active forwarder returned")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stdout was not closed after its forwarder returned")
	}
}

func TestStopLSPServerTimeoutClosesStdoutBeforeJoiningForwarder(t *testing.T) {
	server := newTestServer(t)
	stdout := newOrderedLSPReadCloser()
	forwarderDone := make(chan struct{})
	go func() {
		defer close(forwarderDone)
		_, _ = stdout.Read(nil)
	}()
	<-stdout.readStarted

	processDone := make(chan struct{})
	lspProcess := &lspServerProcess{
		id:            "timed-out-process",
		stdin:         discardLSPWriteCloser{},
		stdout:        stdout,
		done:          processDone,
		forwarderDone: forwarderDone,
	}
	stopReturned := make(chan struct{})
	t.Cleanup(func() {
		stdout.release()
		select {
		case <-stopReturned:
		case <-time.After(2 * time.Second):
			t.Error("timed-out stop did not return during cleanup")
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	go func() {
		server.stopLSPServerWithContext(ctx, lspProcess)
		close(stopReturned)
	}()

	select {
	case <-stopReturned:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed-out stop remained blocked on the stdout forwarder")
	}
	if closedAfterRead := <-stdout.closedAfterRead; closedAfterRead {
		t.Fatal("timed-out stop waited for the reader instead of closing stdout to unblock it")
	}
}

func TestWorkspaceFileURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "posix reserved characters", path: "/task root/A#B?100%.kt", want: "file:///task%20root/A%23B%3F100%25.kt"},
		{name: "strict path characters", path: "/task:root/A!B(C):D.kt", want: "file:///task%3Aroot/A%21B%28C%29%3AD.kt"},
		{name: "windows drive", path: `C:\Task Root\src\Main.kt`, want: "file:///C:/Task%20Root/src/Main.kt"},
		{name: "windows UNC", path: `\\build-server\work share\Main.kt`, want: "file://build-server/work%20share/Main.kt"},
		{name: "posix literal backslash", path: `/task\name/Main.kt`, want: "file:///task%5Cname/Main.kt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := workspaceFileURI(tt.path); got != tt.want {
				t.Fatalf("workspaceFileURI(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

type blockingLSPInstallStrategy struct {
	started  chan struct{}
	canceled chan struct{}
}

func (s *blockingLSPInstallStrategy) Name() string { return "blocking test install" }

func (s *blockingLSPInstallStrategy) Install(ctx context.Context) (*tools.InstallResult, error) {
	close(s.started)
	<-ctx.Done()
	close(s.canceled)
	return nil, ctx.Err()
}

type controlledLSPInstallStrategy struct {
	name    string
	started chan struct{}
	release <-chan struct{}
}

func (s *controlledLSPInstallStrategy) Name() string { return s.name }

func (s *controlledLSPInstallStrategy) Install(ctx context.Context) (*tools.InstallResult, error) {
	close(s.started)
	select {
	case <-s.release:
		return &tools.InstallResult{BinaryPath: s.name}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type fakeLSPInstallerRegistry struct {
	strategy   tools.Strategy
	strategies map[string]tools.Strategy
}

func (r *fakeLSPInstallerRegistry) BinaryPath(string) (string, error) {
	return "", os.ErrNotExist
}

func (r *fakeLSPInstallerRegistry) StrategyFor(language string) (tools.Strategy, error) {
	if strategy := r.strategies[language]; strategy != nil {
		return strategy, nil
	}
	return r.strategy, nil
}

func TestHandleLSPStream_MissingBinaryWithoutAutoInstallClosesWithBinaryNotFound(t *testing.T) {
	t.Setenv("PATH", "")

	s := newTestServer(t)
	ts := httptest.NewServer(s.router)
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/lsp/stream?language=typescript"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial lsp stream: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatal("expected close error")
	}
	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected websocket close error, got %T: %v", err, err)
	}
	if closeErr.Code != 4001 {
		t.Fatalf("close code = %d, want 4001", closeErr.Code)
	}
}

func TestHandleLSPStream_KotlinNeverAttemptsAutoInstall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "")

	s := newTestServer(t)
	ts := httptest.NewServer(s.router)
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/lsp/stream?language=kotlin&autoInstall=true"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial lsp stream: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected immediate close error, got %T: %v", err, err)
	}
	if closeErr.Code != 4001 {
		t.Fatalf("close code = %d, want 4001", closeErr.Code)
	}
}

func TestNewServerDoesNotResolveProjectControlledLSPBinary(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")

	projectBinary := filepath.Join(workDir, ".kandev", "lsp-servers", "kotlin-lsp")
	if err := os.MkdirAll(filepath.Dir(projectBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectBinary, []byte("project-controlled"), 0o755); err != nil {
		t.Fatal(err)
	}

	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: workDir, SessionID: "session-1"}
	procMgr := process.NewManager(cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = procMgr.StopForTeardown(ctx)
	})
	s := NewServer(cfg, procMgr, nil, nil, log)

	if path, err := s.lspInstaller.BinaryPath("kotlin"); err == nil {
		t.Fatalf("BinaryPath(kotlin) = %q from project directory, want not found", path)
	}
}

func TestLSPInstallCoordinatorSerializesSharedNpmPrefix(t *testing.T) {
	coordinator := newLSPInstallCoordinator()
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })
	firstDone := make(chan error, 1)
	go func() {
		_, err := coordinator.run(context.Background(), lspInstallMutationKey("typescript"), func() (string, error) {
			close(firstEntered)
			<-releaseFirst
			return "typescript", nil
		})
		firstDone <- err
	}()
	<-firstEntered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		_, err := coordinator.run(context.Background(), lspInstallMutationKey("python"), func() (string, error) {
			close(secondEntered)
			return "python", nil
		})
		secondDone <- err
	}()

	select {
	case <-secondEntered:
		t.Fatal("second install entered while the shared cache was being mutated")
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(releaseFirst) })
	if err := <-firstDone; err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second install: %v", err)
	}
}

func TestLSPInstallMutationKeyMatchesStrategyTargets(t *testing.T) {
	tests := map[string]string{
		"typescript": "npm-prefix",
		"go":         "go:gopls",
		"rust":       "release:rust-analyzer",
	}
	for language, want := range tests {
		if got := lspInstallMutationKey(language); got != want {
			t.Errorf("lspInstallMutationKey(%q) = %q, want %q", language, got, want)
		}
	}
	if lspInstallMutationKey("python") != lspInstallMutationKey("typescript") {
		t.Error("Python and TypeScript installs must share the npm-prefix mutation key")
	}
}

func TestLSPInstallCoordinatorAllowsIndependentCacheMutations(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "independent-installs"}
	procMgr := process.NewManager(cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = procMgr.StopForTeardown(ctx)
	})

	releaseTypeScript := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseTypeScript) }) })
	typeScriptStarted := make(chan struct{})
	goStarted := make(chan struct{})
	server := NewServer(cfg, procMgr, nil, nil, log)
	server.lspInstaller = &fakeLSPInstallerRegistry{strategies: map[string]tools.Strategy{
		"typescript": &controlledLSPInstallStrategy{
			name:    "typescript",
			started: typeScriptStarted,
			release: releaseTypeScript,
		},
		"go": &controlledLSPInstallStrategy{
			name:    "go",
			started: goStarted,
			release: make(chan struct{}),
		},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	typeScriptDone := make(chan error, 1)
	go func() {
		_, err := server.awaitOrInstallLSP(ctx, "typescript")
		typeScriptDone <- err
	}()
	<-typeScriptStarted
	goDone := make(chan error, 1)
	go func() {
		_, err := server.awaitOrInstallLSP(ctx, "go")
		goDone <- err
	}()

	select {
	case <-goStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("independent Go install waited for the npm-prefix mutation")
	}
	releaseOnce.Do(func() { close(releaseTypeScript) })
	if err := <-typeScriptDone; err != nil {
		t.Fatalf("TypeScript install: %v", err)
	}
	cancel()
	if err := <-goDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Go install error = %v, want context canceled", err)
	}
}

func TestLSPInstallCoordinatorCanceledWaiterDoesNotInstall(t *testing.T) {
	coordinator := newLSPInstallCoordinator()
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })
	firstDone := make(chan error, 1)
	go func() {
		_, err := coordinator.run(context.Background(), lspInstallMutationKey("typescript"), func() (string, error) {
			close(firstEntered)
			<-releaseFirst
			return "typescript", nil
		})
		firstDone <- err
	}()
	<-firstEntered

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiterEntered := make(chan struct{})
	waiterDone := make(chan error, 1)
	go func() {
		_, err := coordinator.run(waiterCtx, lspInstallMutationKey("python"), func() (string, error) {
			close(waiterEntered)
			return "python", nil
		})
		waiterDone <- err
	}()
	cancelWaiter()
	if err := <-waiterDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v, want context canceled", err)
	}
	select {
	case <-waiterEntered:
		t.Fatal("canceled waiter entered the shared npm-prefix install")
	default:
	}

	releaseOnce.Do(func() { close(releaseFirst) })
	if err := <-firstDone; err != nil {
		t.Fatalf("first install: %v", err)
	}
}

func TestLSPAutoInstallIsCanceledAndDrainedByInstanceTeardown(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session-1"}
	procMgr := process.NewManager(cfg, log)
	strategy := &blockingLSPInstallStrategy{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
	server := NewServer(cfg, procMgr, nil, nil, log)
	server.lspInstaller = &fakeLSPInstallerRegistry{strategy: strategy}
	httpServer := httptest.NewServer(server.router)
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") +
		"/api/v1/lsp/stream?language=typescript&autoInstall=true"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial lsp stream: %v", err)
	}
	defer func() { _ = conn.Close() }()
	<-strategy.started
	if _, status, err := conn.ReadMessage(); err != nil || !strings.Contains(string(status), lspStatusInstalling) {
		t.Fatalf("installing status = %q, error = %v", status, err)
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- procMgr.StopForTeardown(context.Background()) }()
	<-strategy.canceled
	if err := <-stopDone; err != nil {
		t.Fatalf("StopForTeardown() error = %v", err)
	}
	_, _, err = conn.ReadMessage()
	closeErr, ok := err.(*websocket.CloseError)
	if !ok || closeErr.Code != websocket.CloseGoingAway {
		t.Fatalf("teardown close error = %T %v, want close code %d", err, err, websocket.CloseGoingAway)
	}
}
