package hostutility

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestAntigravityLoginAttempt(t *testing.T) {
	now := time.Now()
	attempt := &antigravityLoginAttempt{
		id:        "attempt-1",
		state:     "waiting_for_browser",
		expiresAt: now.Add(time.Minute),
		url:       "https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y",
		done:      make(chan struct{}),
		cancel:    func() {},
	}
	snap := attempt.snapshot()
	if snap.State != "waiting_for_browser" || snap.AuthorizationURL == "" {
		t.Fatalf("active snapshot = %+v", snap)
	}
	attempt.expiresAt = now.Add(-time.Second)
	if got := attempt.snapshot(); got.State != "expired" {
		t.Fatalf("expired state = %q", got.State)
	}
	attempt.update("succeeded", "", "", "")
	if got := attempt.snapshot(); got.AuthorizationURL != "" {
		t.Fatalf("terminal snapshot retained URL %q", got.AuthorizationURL)
	}
}

func TestAntigravityLoginManagerLookupAndCancel(t *testing.T) {
	closed := make(chan struct{})
	cancelled := false
	attempt := &antigravityLoginAttempt{
		id:        "attempt-1",
		state:     "waiting_for_browser",
		expiresAt: time.Now().Add(time.Minute),
		done:      closed,
		cancel:    func() { cancelled = true; close(closed) },
	}
	m := &Manager{loginAttempt: attempt}
	if _, ok := m.GetAntigravityLogin(context.Background(), "other"); ok {
		t.Error("foreign attempt id was accepted")
	}
	if _, ok := m.GetAntigravityLogin(context.Background(), "attempt-1"); !ok {
		t.Error("active attempt not found")
	}
	snap, ok := m.CancelAntigravityLogin(context.Background(), "attempt-1")
	if !ok || snap.State != "cancelled" || !cancelled {
		t.Fatalf("cancel = %+v, ok=%v, cancelled=%v", snap, ok, cancelled)
	}
	if snap.AuthorizationURL != "" {
		t.Error("cancelled snapshot retained URL")
	}
}

func TestAntigravityLoginTerminalFence(t *testing.T) {
	attempt := &antigravityLoginAttempt{
		id:        "attempt-1",
		state:     "waiting_for_browser",
		expiresAt: time.Now().Add(-time.Second),
		url:       "https://accounts.google.com/o/oauth2/v2/auth?state=secret",
		done:      make(chan struct{}),
		cancel:    func() {},
	}
	expired := attempt.snapshot()
	if expired.State != "expired" || expired.AuthorizationURL != "" {
		t.Fatalf("expired snapshot = %+v", expired)
	}
	attempt.update("succeeded", "", "", "")
	if got := attempt.snapshot(); got.State != "expired" {
		t.Fatalf("late success overwrote terminal state: %+v", got)
	}
}

func TestAntigravityLoginStartFencesUnfinishedAttempt(t *testing.T) {
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	dir := t.TempDir()
	serverName, harnessName := "agy_acp_server.par", "localharness_external"
	if runtime.GOOS == "windows" {
		serverName, harnessName = "agy_acp_server.exe", "localharness_external.exe"
	}
	if err := os.WriteFile(filepath.Join(dir, serverName), []byte("server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, harnessName), []byte("harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.NewRegistry(log)
	if err := reg.Register(agents.NewAntigravityACP()); err != nil {
		t.Fatal(err)
	}
	attempt := &antigravityLoginAttempt{
		id:        "attempt-1",
		state:     "cancelled",
		expiresAt: time.Now().Add(time.Minute),
		done:      make(chan struct{}),
		cancel:    func() {},
	}
	m := &Manager{registry: reg, loginAttempt: attempt}
	snap, err := m.StartAntigravityLogin(context.Background())
	if err != nil {
		t.Fatalf("StartAntigravityLogin: %v", err)
	}
	if snap.AttemptID != "attempt-1" {
		t.Fatalf("started a successor before cleanup: %+v", snap)
	}
}
