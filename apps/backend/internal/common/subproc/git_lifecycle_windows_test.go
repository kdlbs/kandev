//go:build windows

package subproc

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsPrepareGitLifecycleCommandHidesConsoleWindow(t *testing.T) {
	cmd := exec.Command("git", "status")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.BELOW_NORMAL_PRIORITY_CLASS}
	if err := prepareGitLifecycleCommand(cmd); err != nil {
		t.Fatalf("prepareGitLifecycleCommand: %v", err)
	}
	flags := cmd.SysProcAttr.CreationFlags
	for _, want := range []uint32{
		windows.BELOW_NORMAL_PRIORITY_CLASS,
		syscall.CREATE_NEW_PROCESS_GROUP,
		windows.CREATE_SUSPENDED,
	} {
		if flags&want == 0 {
			t.Fatalf("CreationFlags = %#x, want %#x set", flags, want)
		}
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
	if flags&windows.CREATE_NO_WINDOW != 0 {
		t.Fatalf("CreationFlags = %#x, must not include CREATE_NO_WINDOW because descendants need a console", flags)
	}

	console := exec.Command("git", "status")
	console.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	if err := prepareGitLifecycleCommand(console); err == nil {
		t.Fatal("prepareGitLifecycleCommand accepted CREATE_NEW_CONSOLE")
	}
}

func TestWindowsPrepareGitLifecycleCommandRejectsConsoleDetachmentFlags(t *testing.T) {
	tests := []struct {
		name          string
		creationFlags uint32
	}{
		{name: "CREATE_NO_WINDOW", creationFlags: windows.CREATE_NO_WINDOW},
		{name: "DETACHED_PROCESS", creationFlags: windows.DETACHED_PROCESS},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command("git", "status")
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: test.creationFlags}
			if err := prepareGitLifecycleCommand(cmd); err == nil {
				t.Fatalf("prepareGitLifecycleCommand accepted %s", test.name)
			}
		})
	}
}

const windowsConsoleProbeEnv = "KANDEV_WINDOWS_CONSOLE_PROBE"

func TestWindowsDetachedParentKeepsConsoleHiddenForDescendants(t *testing.T) {
	switch os.Getenv(windowsConsoleProbeEnv) {
	case "detached-parent":
		testDetachedParentConsoleProbe(t)
		return
	case "managed-child":
		testManagedConsoleProbe(t)
		return
	case "descendant":
		printConsoleProbeResult(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsDetachedParentKeepsConsoleHiddenForDescendants$")
	cmd.Env = setWindowsConsoleProbeMode("detached-parent")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("detached-parent probe failed: %v\n%s", err, output)
	}
	var consoleWindow string
	results := 0
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, "console_hwnd=") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != "window_visible=false" {
			t.Fatalf("console probe result = %q, want a hidden console", line)
		}
		gotWindow := strings.TrimPrefix(fields[0], "console_hwnd=")
		if consoleWindow != "" && gotWindow != consoleWindow {
			t.Fatalf("descendant console handle = %s, want inherited handle %s", gotWindow, consoleWindow)
		}
		consoleWindow = gotWindow
		results++
	}
	if results != 2 {
		t.Fatalf("hidden console result count = %d, want 2; probe output:\n%s", results, output)
	}
}

func testDetachedParentConsoleProbe(t *testing.T) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsDetachedParentKeepsConsoleHiddenForDescendants$")
	cmd.Env = setWindowsConsoleProbeMode("managed-child")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := prepareGitLifecycleCommand(cmd); err != nil {
		t.Fatalf("prepare managed probe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start managed probe: %v", err)
	}
	lifecycle, err := installGitLifecycle(cmd)
	if err != nil {
		t.Fatalf("install managed probe lifecycle: %v", err)
	}
	waitErr := cmd.Wait()
	releaseErr := releaseGitLifecycle(lifecycle)
	output := stdout.String() + stderr.String()
	if waitErr != nil {
		t.Fatalf("wait for managed probe: %v\n%s", waitErr, output)
	}
	if releaseErr != nil {
		t.Fatalf("release managed probe lifecycle: %v", releaseErr)
	}
	if _, err := fmt.Fprint(os.Stdout, output); err != nil {
		t.Fatalf("forward managed probe output: %v", err)
	}
}

func testManagedConsoleProbe(t *testing.T) {
	t.Helper()
	printConsoleProbeResult(t)

	cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsDetachedParentKeepsConsoleHiddenForDescendants$")
	cmd.Env = setWindowsConsoleProbeMode("descendant")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("descendant probe failed: %v\n%s", err, output)
	}
	if _, err := fmt.Fprint(os.Stdout, string(output)); err != nil {
		t.Fatalf("forward descendant probe output: %v", err)
	}
}

func setWindowsConsoleProbeMode(mode string) []string {
	env := os.Environ()
	prefix := windowsConsoleProbeEnv + "="
	for i, entry := range env {
		if strings.HasPrefix(strings.ToUpper(entry), prefix) {
			env[i] = prefix + mode
			return env
		}
	}
	return append(env, prefix+mode)
}

func printConsoleProbeResult(t *testing.T) {
	t.Helper()
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	hwnd, _, _ := proc.Call()
	if hwnd == 0 {
		t.Fatal("process has no console window handle")
	}
	fmt.Fprintf(os.Stdout, "console_hwnd=%#x window_visible=%t\n", hwnd, windows.IsWindowVisible(windows.HWND(hwnd)))
}

func TestWindowsManagedGitJobCleanup(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	gitCommand := filepath.Join(dir, "git.cmd")
	if err := os.WriteFile(gitCommand, []byte("@echo off\r\nif \"%1\"==\"hold\" (\r\n  echo started>\"%GIT_TEST_STARTED%\"\r\n  cmd.exe /c \"ping.exe 127.0.0.1 -n 100 >nul\"\r\n) else (\r\n  echo quick\r\n)\r\n"), 0o700); err != nil {
		t.Fatalf("write Git fixture: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_TEST_STARTED", started)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, runErr, execErr := RunGitOutputAfterAcquire(ctx, GitLifecycle, 5*time.Second, func(execCtx context.Context) *exec.Cmd {
			return NewGitCommand(execCtx, "hold")
		})
		if runErr != nil {
			result <- runErr
			return
		}
		result <- execErr
	}()
	waitForWindowsGitFile(t, started, time.Second)
	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("canceled managed Git command unexpectedly succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("managed Git Job Object did not terminate the owned process tree")
	}
}

func waitForWindowsGitFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", path)
		}
	}
}
