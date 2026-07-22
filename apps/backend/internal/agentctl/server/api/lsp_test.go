package api

import (
	"context"
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

type fakeLSPInstallerRegistry struct {
	strategy tools.Strategy
}

func (r *fakeLSPInstallerRegistry) BinaryPath(string) (string, error) {
	return "", os.ErrNotExist
}

func (r *fakeLSPInstallerRegistry) StrategyFor(string) (tools.Strategy, error) {
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

func TestLSPInstallGateSerializesSharedCacheMutations(t *testing.T) {
	gate := newLSPInstallGate()
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })
	firstDone := make(chan error, 1)
	go func() {
		_, err := gate.run(context.Background(), func() (string, error) {
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
		_, err := gate.run(context.Background(), func() (string, error) {
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
