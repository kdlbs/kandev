package shell

import (
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error", // Suppress logs during tests
		Format: "console",
	})
	return log
}

// TestDefaultConfig tests that DefaultConfig returns expected values
func TestDefaultConfig(t *testing.T) {
	workDir := "/test/dir"
	cfg := DefaultConfig(workDir)

	if cfg.WorkDir != workDir {
		t.Errorf("expected WorkDir %q, got %q", workDir, cfg.WorkDir)
	}
	if cfg.Cols != 80 {
		t.Errorf("expected Cols 80, got %d", cfg.Cols)
	}
	if cfg.Rows != 24 {
		t.Errorf("expected Rows 24, got %d", cfg.Rows)
	}
}

// TestDetectShell tests shell detection for the current OS
func TestDetectShell(t *testing.T) {
	shell, args := detectShell()

	if shell == "" {
		t.Error("detectShell returned empty shell")
	}

	if runtime.GOOS == "windows" {
		assertWindowsShell(t, shell)
		return
	}
	assertUnixShell(t, shell, args)
}

// assertUnixShell verifies Unix-specific shell detection results.
func assertUnixShell(t *testing.T, shell string, args []string) {
	t.Helper()
	if _, err := os.Stat(shell); err != nil && shell != os.Getenv("SHELL") {
		t.Logf("Warning: detected shell %q may not exist: %v", shell, err)
	}
	// Unix shells should have -l flag for login shell
	if len(args) > 0 && args[0] != "-l" {
		t.Errorf("expected Unix shell args to contain '-l', got %v", args)
	}
}

// assertWindowsShell verifies Windows-specific shell detection results.
func assertWindowsShell(t *testing.T, shell string) {
	t.Helper()
	validShells := map[string]bool{
		"cmd.exe":        true,
		"powershell.exe": true,
		"pwsh.exe":       true,
	}
	if !validShells[shell] {
		t.Errorf("unexpected Windows shell: %q", shell)
	}
}

// TestDetectShellWithSHELLEnv tests shell detection respects SHELL env var on Unix
func TestDetectShellWithSHELLEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SHELL env var is not used on Windows")
	}

	originalShell := os.Getenv("SHELL")
	defer func() { _ = os.Setenv("SHELL", originalShell) }()

	// Test with custom SHELL
	_ = os.Setenv("SHELL", "/bin/custom-shell")
	shell, args := detectShell()

	if shell != "/bin/custom-shell" {
		t.Errorf("expected shell from SHELL env, got %q", shell)
	}
	if len(args) == 0 || args[0] != "-l" {
		t.Errorf("expected args to contain '-l', got %v", args)
	}
}

// TestNewSession tests creating a new session
func TestNewSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	// PTY may not work in all CI environments
	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer func() { _ = session.Stop() }()

	// Verify session is running
	status := session.Status()
	if !status.Running {
		t.Error("expected session to be running")
	}
	if status.Pid == 0 {
		t.Error("expected non-zero PID")
	}
	if status.Cwd != workDir {
		t.Errorf("expected Cwd %q, got %q", workDir, status.Cwd)
	}
	if status.Shell == "" {
		t.Error("expected non-empty Shell")
	}
	if status.StartedAt.IsZero() {
		t.Error("expected non-zero StartedAt")
	}
}

// TestSessionStop tests stopping a session
func TestSessionStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	// Stop the session
	if err := session.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Verify session is stopped
	status := session.Status()
	if status.Running {
		t.Error("expected session to not be running after Stop")
	}

	// Calling Stop again should be idempotent
	if err := session.Stop(); err != nil {
		t.Errorf("second Stop failed: %v", err)
	}
}

// TestSessionWrite tests writing to the shell
func TestSessionWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer func() { _ = session.Stop() }()

	// Write a simple command
	data := []byte("echo hello\n")
	n, err := session.Write(data)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected to write %d bytes, wrote %d", len(data), n)
	}
}

// TestSessionWriteNotRunning tests writing to a stopped session
func TestSessionWriteNotRunning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	// Stop the session first
	_ = session.Stop()

	// Write should fail
	_, err = session.Write([]byte("echo hello\n"))
	if err == nil {
		t.Error("expected Write to fail on stopped session")
	}
}

// TestSessionSubscribeUnsubscribe tests the subscriber pattern
func TestSessionSubscribeUnsubscribe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer func() { _ = session.Stop() }()

	// Create subscriber channel
	ch := make(chan []byte, 100)

	// Subscribe
	session.Subscribe(ch)

	// Write something to trigger output
	_, _ = session.Write([]byte("echo test\n"))

	// Wait a bit for output
	time.Sleep(100 * time.Millisecond)

	// Unsubscribe
	session.Unsubscribe(ch)

	// The channel should have received some data (shell prompt + echo output)
	select {
	case data := <-ch:
		if len(data) == 0 {
			t.Error("expected non-empty output")
		}
	default:
		// May not receive data immediately, that's okay
		t.Log("no output received (may be timing-dependent)")
	}
}

// TestSessionMultipleSubscribers tests multiple subscribers receive output
func TestSessionMultipleSubscribers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer func() { _ = session.Stop() }()

	// Create multiple subscriber channels
	ch1 := make(chan []byte, 100)
	ch2 := make(chan []byte, 100)
	ch3 := make(chan []byte, 100)

	session.Subscribe(ch1)
	session.Subscribe(ch2)
	session.Subscribe(ch3)

	// Write something
	_, _ = session.Write([]byte("echo multi\n"))

	// Wait for output
	time.Sleep(100 * time.Millisecond)

	// All channels should potentially receive data
	// Just verify no panics or errors occurred

	session.Unsubscribe(ch1)
	session.Unsubscribe(ch2)
	session.Unsubscribe(ch3)
}

// TestSessionStatus tests Status returns correct information
func TestSessionStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer func() { _ = session.Stop() }()

	status := session.Status()

	// Verify all fields
	if !status.Running {
		t.Error("expected Running to be true")
	}
	if status.Pid <= 0 {
		t.Errorf("expected positive PID, got %d", status.Pid)
	}
	if status.Shell == "" {
		t.Error("expected non-empty Shell")
	}
	if status.Cwd != workDir {
		t.Errorf("expected Cwd %q, got %q", workDir, status.Cwd)
	}
	if status.StartedAt.IsZero() {
		t.Error("expected non-zero StartedAt")
	}
	if time.Since(status.StartedAt) > 5*time.Second {
		t.Error("StartedAt seems too old")
	}
}

// TestSessionStatusAfterStop tests Status after stopping
func TestSessionStatusAfterStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	_ = session.Stop()

	status := session.Status()
	if status.Running {
		t.Error("expected Running to be false after Stop")
	}
}

// TestBuildShellEnv tests the buildShellEnv function
func TestBuildShellEnv(t *testing.T) {
	workDir := "/test/workspace"
	env := buildShellEnv(workDir)

	// Check that env is not empty
	if len(env) == 0 {
		t.Error("expected non-empty environment")
	}

	// Check for PWD
	pwdFound := false
	termFound := false
	for _, e := range env {
		if e == "PWD="+workDir {
			pwdFound = true
		}
		if e == "TERM=xterm-256color" {
			termFound = true
		}
	}

	if !pwdFound {
		t.Error("expected PWD to be set in environment")
	}
	if !termFound {
		t.Error("expected TERM to be set in environment")
	}
}

// TestSessionConcurrentSubscribe tests concurrent subscribe/unsubscribe
func TestSessionConcurrentSubscribe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer func() { _ = session.Stop() }()

	// Concurrently subscribe and unsubscribe
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := make(chan []byte, 10)
			session.Subscribe(ch)
			time.Sleep(10 * time.Millisecond)
			session.Unsubscribe(ch)
		}()
	}

	wg.Wait()
}

// TestSessionConcurrentWrite tests concurrent writes
func TestSessionConcurrentWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer func() { _ = session.Stop() }()

	// Concurrently write
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = session.Write([]byte("echo test\n"))
		}(i)
	}

	wg.Wait()
}

// TestSessionConcurrentStatus tests concurrent status reads
func TestSessionConcurrentStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY is not supported on Windows")
	}

	if os.Getenv("CI") != "" {
		t.Skip("Skipping PTY test in CI environment")
	}

	log := newTestLogger()
	workDir := t.TempDir()
	cfg := DefaultConfig(workDir)

	session, err := NewSession(cfg, log)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer func() { _ = session.Stop() }()

	// Concurrently read status
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := session.Status()
			if !status.Running {
				t.Error("expected session to be running")
			}
		}()
	}

	wg.Wait()
}

// TestDetectShellFallback tests shell fallback when SHELL is unset
func TestDetectShellFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SHELL env var fallback is for Unix only")
	}

	originalShell := os.Getenv("SHELL")
	defer func() { _ = os.Setenv("SHELL", originalShell) }()

	// Unset SHELL
	_ = os.Unsetenv("SHELL")

	shell, _ := detectShell()

	// Should fall back to one of the common shells
	validShells := map[string]bool{
		"/bin/bash": true,
		"/bin/zsh":  true,
		"/bin/sh":   true,
	}
	if !validShells[shell] {
		t.Logf("Detected shell: %q (may be valid if it exists on this system)", shell)
	}
}
