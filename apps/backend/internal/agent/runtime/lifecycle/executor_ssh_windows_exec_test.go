package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// These tests run generated PowerShell for real on a Windows machine. Each
// exec request goes through `cmd.exe /c <request>` with the envelope on stdin,
// exactly as stock Win32 OpenSSH runs it. Other platforms skip them.

const (
	windowsStandInEnv    = "KANDEV_TEST_WINDOWS_AGENTCTL_STAND_IN"
	windowsStandInMarker = "KANDEV_TEST_WINDOWS_MARKER"
)

// init turns a copy of this test binary into a stand-in agentctl.exe. It runs
// before flag parsing, so the stand-in accepts agentctl's --workdir argument.
func init() {
	if os.Getenv(windowsStandInEnv) == "" {
		return
	}
	workdir := ""
	for i, arg := range os.Args {
		if arg == "--workdir" && i+1 < len(os.Args) {
			workdir = os.Args[i+1]
		}
	}
	fmt.Printf("stand-in port=%s marker=%s\n", os.Getenv("AGENTCTL_PORT"), os.Getenv(windowsStandInMarker))
	child := exec.Command("ping", "-n", "600", "127.0.0.1")
	child.Stdout, child.Stderr = io.Discard, io.Discard
	if err := child.Start(); err == nil && workdir != "" {
		_ = os.WriteFile(filepath.Join(workdir, "stand-in-child.pid"), []byte(strconv.Itoa(child.Process.Pid)), 0o644)
	}
	fmt.Println(`{"msg":"HTTP server bound successfully"}`)
	_ = os.Stdout.Sync()
	time.Sleep(10 * time.Minute)
	os.Exit(0)
}

func requireWindowsExecution(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("runs generated PowerShell on a Windows machine")
	}
}

func windowsTestShells() []string {
	shells := []string{"powershell"}
	if _, err := exec.LookPath("pwsh"); err == nil {
		shells = append(shells, "pwsh")
	}
	return shells
}

// newLocalWindowsSSHServer is a fake SSH server whose exec requests run on this
// machine through cmd.exe, the stock Win32 OpenSSH default shell.
func newLocalWindowsSSHServer(t *testing.T) *fakeSSHServer {
	t.Helper()
	return newFakeSSHServer(t, func(command, stdin string) sshExecResult {
		cmd := exec.Command("cmd.exe")
		setRawCommandLine(cmd, "cmd.exe /c "+command)
		cmd.Stdin = strings.NewReader(stdin)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		result := sshExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr):
			result.ExitCode = exitErr.ExitCode()
		case err != nil:
			result.Stderr += err.Error()
			result.ExitCode = 255
		}
		return result
	})
}

func exitStatus(err error) int {
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitStatus()
	}
	if err != nil {
		return -1
	}
	return 0
}

func runWindowsTestGit(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=k", "GIT_AUTHOR_EMAIL=k@example.com", "GIT_COMMITTER_NAME=k", "GIT_COMMITTER_EMAIL=k@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newWindowsGitOrigin creates a bare origin with one commit on main.
func newWindowsGitOrigin(t *testing.T, root string) string {
	t.Helper()
	origin := filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	runWindowsTestGit(t, "init", "-q", "--bare", origin)
	runWindowsTestGit(t, "init", "-q", "-b", "main", seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runWindowsTestGit(t, "-C", seed, "add", ".")
	runWindowsTestGit(t, "-C", seed, "commit", "-q", "-m", "init")
	runWindowsTestGit(t, "-C", seed, "push", "-q", origin, "main")
	return "file:///" + filepath.ToSlash(origin)
}

func copyTestBinary(t *testing.T, dst string) {
	t.Helper()
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

func killWindowsProcessTree(pid int) {
	if pid > 0 {
		_ = exec.Command("taskkill.exe", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.9
func TestSSHWindowsExecEnvelopeImportsEnvironment(t *testing.T) {
	requireWindowsExecution(t)
	client := newLocalWindowsSSHServer(t).dial(t)
	value := "a b ' $x `y 中文"
	for _, shell := range windowsTestShells() {
		out, stderr, err := runPowerShellEnvelope(context.Background(), client, shell, powerShellEnvelope{
			Env: map[string]string{"KANDEV_TEST_VALUE": value}, Input: "payload",
			Script: "[Console]::Out.Write($env:KANDEV_TEST_VALUE + '|' + $KandevInput)",
		})
		if err != nil || out != value+"|payload" {
			t.Fatalf("[%s] out=%q err=%v stderr=%q, want %q", shell, out, err, stderr, value+"|payload")
		}
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.5
func TestSSHWindowsExecPropagatesExitCodes(t *testing.T) {
	requireWindowsExecution(t)
	client := newLocalWindowsSSHServer(t).dial(t)
	cases := []struct {
		script     string
		wantStatus int
		wantStderr string
	}{
		{script: "Write-Output before; exit 7", wantStatus: 7},
		{script: "throw 'kandev boom'", wantStatus: 1, wantStderr: "kandev boom"},
		{script: "cmd.exe /c exit 5", wantStatus: 5},
		{script: "cmd.exe /c exit 5; Write-Output ok", wantStatus: 0},
		{script: "Invoke-KandevNative { & git -C C:\\kandev-missing-dir rev-parse --git-dir *>$null }; Write-Output $LASTEXITCODE", wantStatus: 0},
	}
	for _, shell := range windowsTestShells() {
		for _, tc := range cases {
			_, stderr, err := runPowerShell(context.Background(), client, shell, tc.script, false)
			if got := exitStatus(err); got != tc.wantStatus || !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("[%s] %q: status=%d stderr=%q, want status %d containing %q", shell, tc.script, got, stderr, tc.wantStatus, tc.wantStderr)
			}
		}
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.8
func TestSSHWindowsExecDefaultPrepare(t *testing.T) {
	requireWindowsExecution(t)
	client := newLocalWindowsSSHServer(t).dial(t)
	root := filepath.Join(t.TempDir(), "kandev user's root")
	originURL := newWindowsGitOrigin(t, root)
	r := NewSSHExecutor(nil, nil, nil, newTestLogger())
	windows := SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}
	for _, shell := range windowsTestShells() {
		taskDir := filepath.Join(root, "tasks", "task-"+shell)
		if err := os.MkdirAll(taskDir, 0o755); err != nil {
			t.Fatal(err)
		}
		req := &ExecutorCreateRequest{Metadata: map[string]interface{}{
			MetadataKeySSHShell: shell, "repository_clone_url": originURL,
			MetadataKeyBaseBranch: "main", MetadataKeyWorktreeBranch: "feature/win-ssh",
			"repository_setup_script": "Set-Content -LiteralPath (Join-Path (Get-Location) 'setup-ran.txt') -Value ok",
		}}
		if err := r.runPrepareScript(context.Background(), client, taskDir, req, windows, ""); err != nil {
			t.Fatalf("[%s] fresh prepare: %v", shell, err)
		}
		if branch := runWindowsTestGit(t, "-C", taskDir, "branch", "--show-current"); branch != "feature/win-ssh" {
			t.Fatalf("[%s] branch = %q, want feature/win-ssh", shell, branch)
		}
		if _, err := os.Stat(filepath.Join(taskDir, "setup-ran.txt")); err != nil {
			t.Fatalf("[%s] repository setup did not run: %v", shell, err)
		}
		if err := r.verifyPrimaryCheckout(context.Background(), client, taskDir, req, windows); err != nil {
			t.Fatalf("[%s] verify checkout: %v", shell, err)
		}
		if err := r.runPrepareScript(context.Background(), client, taskDir, req, windows, ""); err != nil {
			t.Fatalf("[%s] reuse prepare: %v", shell, err)
		}
		conflict := &ExecutorCreateRequest{Metadata: map[string]interface{}{
			MetadataKeySSHShell: shell, "repository_clone_url": "https://example.invalid/other.git", MetadataKeyBaseBranch: "main",
		}}
		if err := r.runPrepareScript(context.Background(), client, taskDir, conflict, windows, ""); err == nil {
			t.Fatalf("[%s] a conflicting origin must abort preparation", shell)
		}
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.5
func TestSSHWindowsExecLargeCustomPrepare(t *testing.T) {
	requireWindowsExecution(t)
	client := newLocalWindowsSSHServer(t).dial(t)
	taskDir := filepath.Join(t.TempDir(), "task large")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "# " + strings.Repeat("x", 64*1024) + "\nSet-Content -LiteralPath (Join-Path (Get-Location) 'large-ran.txt') -Value ok"
	req := &ExecutorCreateRequest{Metadata: map[string]interface{}{MetadataKeySetupScript: script}}
	r := NewSSHExecutor(nil, nil, nil, newTestLogger())
	if err := r.runPrepareScript(context.Background(), client, taskDir, req, SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}, ""); err != nil {
		t.Fatalf("64 KiB custom prepare: %v", err)
	}
	if _, err := os.Stat(filepath.Join(taskDir, "large-ran.txt")); err != nil {
		t.Fatalf("custom prepare did not run: %v", err)
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.11
// @covers AC-EXECUTORS-SSH-WINDOWS-001.13
// @covers AC-EXECUTORS-SSH-WINDOWS-001.14
func TestSSHWindowsExecControllerLifecycle(t *testing.T) {
	requireWindowsExecution(t)
	ctx := context.Background()
	client := newLocalWindowsSSHServer(t).dial(t)
	root := filepath.Join(t.TempDir(), "kandev root")
	bin := filepath.Join(root, "bin", "agentctl.exe")
	copyTestBinary(t, bin)
	taskDir := filepath.Join(root, "tasks", "task-exec1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionDir, err := ensureWindowsRemoteSessionDir(ctx, client, taskDir, "s1")
	if err != nil {
		t.Fatal(err)
	}
	const port = 45311
	pid, err := startRemoteWindowsAgentctlOnPort(ctx, client, "powershell", bin, taskDir, sessionDir,
		map[string]string{windowsStandInEnv: "1", windowsStandInMarker: "x y"}, port, newTestLogger())
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	t.Cleanup(func() { killWindowsProcessTree(pid) })

	if alive, err := probeRemoteWindowsAgentctlLiveness(ctx, client, pid); err != nil || !alive {
		t.Fatalf("controller after its launch request returned: alive=%v err=%v", alive, err)
	}
	if logText := readRemoteWindowsAgentctlLog(ctx, client, sessionDir); !strings.Contains(logText, fmt.Sprintf("stand-in port=%d marker=x y", port)) {
		t.Fatalf("controller environment missing from log: %q", logText)
	}
	if owned, err := verifyRemoteWindowsAgentctlIdentity(ctx, client, pid, sessionDir, taskDir); err != nil || !owned {
		t.Fatalf("identity for its own task: owned=%v err=%v", owned, err)
	}
	if owned, _ := verifyRemoteWindowsAgentctlIdentity(ctx, client, pid, sessionDir, filepath.Join(root, "tasks", "task-other")); owned {
		t.Fatal("identity matched another task directory")
	}
	inventoryOut, _, err := runPowerShell(ctx, client, defaultWindowsLoginShell, sshWindowsOrphanInventoryScript(root), false)
	if err != nil {
		t.Fatalf("orphan inventory: %v", err)
	}
	inventory := parseSSHOrphanInventory(inventoryOut, root)
	if inventory.pidfilesByTask["task-exec1"][pid] != "s1" {
		t.Fatalf("inventory pidfiles = %v, want pid %d claimed by s1", inventory.pidfilesByTask, pid)
	}
	childPID := 0
	if data, err := os.ReadFile(filepath.Join(taskDir, "stand-in-child.pid")); err == nil {
		childPID, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	if childPID == 0 {
		t.Fatal("stand-in did not record its child process")
	}

	if err := stopRemoteAgentctl(ctx, client, sessionDir, pid); err != nil {
		t.Fatalf("stop: %v", err)
	}
	for _, target := range []int{pid, childPID} {
		if alive, _ := probeRemoteWindowsAgentctlLiveness(ctx, client, target); alive {
			t.Fatalf("pid %d survived the stop", target)
		}
	}
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Fatalf("session directory remains after stop: %v", err)
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.6
func TestSSHWindowsExecReplacesRunningHelper(t *testing.T) {
	requireWindowsExecution(t)
	root := filepath.Join(t.TempDir(), "kandev helper")
	if !strings.EqualFold(filepath.VolumeName(root), "C:") {
		t.Skipf("the SFTP drive emulation maps only C:, and the temp dir is %s", root)
	}
	server := newLocalWindowsSSHServer(t)
	server.enableSFTP()
	server.sftpHandler = newWindowsDriveSFTPFS(`C:\`).handlers()
	bin := filepath.Join(root, "bin", "agentctl.exe")
	copyTestBinary(t, bin)
	running := exec.Command(bin)
	running.Env = append(os.Environ(), windowsStandInEnv+"=1")
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killWindowsProcessTree(running.Process.Pid) })

	client := server.dial(t)
	replacement := []byte("MZ-replacement-helper")
	sum := sha256.Sum256(replacement)
	_, err := ensureWindowsAgentctlOnHost(context.Background(), client, hex.EncodeToString(sum[:]), replacement,
		"replacement", bin, bin+".sha256", newTestLogger())
	if err != nil {
		t.Fatalf("replacing a running helper on NTFS: %v", err)
	}
	if data, _ := os.ReadFile(bin); !bytes.Equal(data, replacement) {
		t.Fatal("agentctl.exe was not replaced")
	}
	aside, _ := filepath.Glob(bin + ".old-*")
	if len(aside) != 1 {
		t.Fatalf("aside files = %v, want the running image kept once", aside)
	}
	if alive, err := probeRemoteWindowsAgentctlLiveness(context.Background(), client, running.Process.Pid); err != nil || !alive {
		t.Fatalf("the running helper stopped during replacement: alive=%v err=%v", alive, err)
	}
}

// TestSSHWindowsScriptsWrapRedirectedNativeCommands guards a Windows
// PowerShell 5.1 trap. Under $ErrorActionPreference = 'Stop', redirecting a
// native command's stderr turns each stderr line into a terminating error, even
// when the command succeeds. Every such call must use Invoke-KandevNative.
func TestSSHWindowsScriptsWrapRedirectedNativeCommands(t *testing.T) {
	redirected := regexp.MustCompile(`&\s+[A-Za-z][\w.-]*(\.exe)?\s[^\n]*(\*>\$null|2>\$null|2>&1)`)
	for _, file := range []string{"executor_ssh_windows_scripts.go", "executor_ssh_winremote.go", "executor_ssh_windows_launch.go", "executor_ssh_powershell.go"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if redirected.MatchString(line) && !strings.Contains(line, "Invoke-KandevNative {") {
				t.Errorf("%s:%d redirects native stderr outside Invoke-KandevNative: %s", file, i+1, strings.TrimSpace(line))
			}
		}
	}
}
