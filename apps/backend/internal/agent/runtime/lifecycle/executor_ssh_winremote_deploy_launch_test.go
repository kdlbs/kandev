package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"

	"github.com/kandev/kandev/internal/agent/executor"
)

// windowsDriveSFTPFS serves /C:/... SFTP paths from a local directory with the
// semantics of Windows OpenSSH sftp-server: Rename never overwrites, Setstat is
// unsupported, and the image of a running process (inUse) cannot be removed but
// can be renamed, with the lock following the file.
type windowsDriveSFTPFS struct {
	root  string
	mu    sync.Mutex
	inUse map[string]bool
	// failTempRename fails the final rename of an uploaded temporary file.
	failTempRename bool
	writes         int
}

func newWindowsDriveSFTPFS(root string) *windowsDriveSFTPFS {
	return &windowsDriveSFTPFS{root: root, inUse: map[string]bool{}}
}

func (f *windowsDriveSFTPFS) handlers() *sftp.Handlers {
	return &sftp.Handlers{FileGet: f, FilePut: f, FileCmd: f, FileList: f}
}

func (f *windowsDriveSFTPFS) localPath(sftpPath string) (string, error) {
	if !strings.HasPrefix(sftpPath, "/C:") && !strings.HasPrefix(sftpPath, "/c:") {
		return "", sftp.ErrSSHFxNoSuchFile
	}
	rel := strings.TrimPrefix(sftpPath[3:], "/")
	if rel == "" {
		return f.root, nil
	}
	return filepath.Join(f.root, filepath.FromSlash(rel)), nil
}

func (f *windowsDriveSFTPFS) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	local, err := f.localPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	return os.Open(local)
}

func (f *windowsDriveSFTPFS) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	local, err := f.localPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.writes++
	f.mu.Unlock()
	return os.OpenFile(local, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
}

func (f *windowsDriveSFTPFS) Filecmd(r *sftp.Request) error {
	local, err := f.localPath(r.Filepath)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case "Setstat":
		return sftp.ErrSSHFxOpUnsupported
	case "Rename":
		return f.rename(local, r.Target)
	case "Mkdir":
		return os.Mkdir(local, 0o755)
	case "Remove":
		if f.inUse[local] {
			return sftp.ErrSSHFxFailure
		}
		return os.Remove(local)
	case "Rmdir":
		// pkg/sftp retries a failed file removal as a directory removal; a
		// Windows rmdir never deletes a file.
		if info, statErr := os.Stat(local); statErr == nil && !info.IsDir() {
			return sftp.ErrSSHFxFailure
		}
		return os.Remove(local)
	default:
		return sftp.ErrSSHFxOpUnsupported
	}
}

// rename emulates MoveFile without MOVEFILE_REPLACE_EXISTING.
func (f *windowsDriveSFTPFS) rename(local, target string) error {
	dst, err := f.localPath(target)
	if err != nil {
		return err
	}
	if f.failTempRename && strings.Contains(filepath.Base(local), ".tmp.") {
		return sftp.ErrSSHFxFailure
	}
	if _, statErr := os.Stat(local); statErr != nil {
		return sftp.ErrSSHFxNoSuchFile
	}
	if _, existsErr := os.Stat(dst); existsErr == nil {
		return sftp.ErrSSHFxFailure
	}
	if err := os.Rename(local, dst); err != nil {
		return err
	}
	if f.inUse[local] {
		delete(f.inUse, local)
		f.inUse[dst] = true
	}
	return nil
}

type sftpStaticLister []fs.FileInfo

func (l sftpStaticLister) ListAt(dst []fs.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}
	n := copy(dst, l[offset:])
	if int(offset)+n >= len(l) {
		return n, io.EOF
	}
	return n, nil
}

func (f *windowsDriveSFTPFS) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	local, err := f.localPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	switch r.Method {
	case "List":
		entries, err := os.ReadDir(local)
		if err != nil {
			return nil, err
		}
		infos := make([]fs.FileInfo, 0, len(entries))
		for _, entry := range entries {
			if info, infoErr := entry.Info(); infoErr == nil {
				infos = append(infos, info)
			}
		}
		return sftpStaticLister(infos), nil
	case "Stat":
		info, err := os.Stat(local)
		if os.IsNotExist(err) {
			return nil, sftp.ErrSSHFxNoSuchFile
		}
		if err != nil {
			return nil, err
		}
		return sftpStaticLister{info}, nil
	default:
		return nil, sftp.ErrSSHFxOpUnsupported
	}
}

func TestSSHWindowsDetectRemoteInfoFallsBackToPowerShell(t *testing.T) {
	for _, tc := range []struct {
		name     string
		winArch  string
		wantArch string
	}{
		{name: "windows amd64", winArch: "AMD64", wantArch: "amd64"},
		{name: "windows arm64", winArch: "ARM64", wantArch: "arm64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeSSHServer(t, func(command, stdin string) sshExecResult {
				if strings.HasPrefix(command, "uname ") {
					return sshFail("'uname' is not recognized as an internal or external command")
				}
				script, ok := decodeTestPowerShell(command, stdin)
				if !ok {
					return sshFail("expected PowerShell -EncodedCommand, got: " + command)
				}
				switch {
				case strings.Contains(script, "KANDEV_WIN_INFO"):
					return sshOut("KANDEV_WIN_INFO\tWindows_NT\t" + tc.winArch + "\tMicrosoft Windows NT 10.0.22631.0\r\n")
				case strings.Contains(script, "git --version"):
					return sshOut("git version 2.47.0.windows.1\r\n")
				default:
					return sshFail("unexpected PowerShell script: " + script)
				}
			})

			info, err := SSHProbeRemote(context.Background(), server.dial(t))
			if err != nil {
				t.Fatalf("SSHProbeRemote: %v", err)
			}
			if info.OS != "Windows_NT" || info.Arch != tc.winArch {
				t.Fatalf("OS/Arch = %q/%q, want Windows_NT/%s", info.OS, info.Arch, tc.winArch)
			}
			if info.Platform.GOOS != "windows" || info.Platform.GOARCH != tc.wantArch {
				t.Fatalf("Platform = %s, want windows/%s", info.Platform, tc.wantArch)
			}
			if info.GitVer != "git version 2.47.0.windows.1" {
				t.Fatalf("GitVer = %q", info.GitVer)
			}
		})
	}
}

func TestSSHWindowsEnsureAgentctlUploadsOnceAndReportsCache(t *testing.T) {
	host := newWindowsHelperHost(t)
	if err := os.WriteFile(host.helper, []byte("MZ-agentctl-v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := host.server.dial(t)
	platform := SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}
	resolver := NewAgentctlResolver(newTestLogger())

	if cached, err := SSHCheckAgentctlCached(context.Background(), client, resolver, platform); err != nil || cached {
		t.Fatalf("SSHCheckAgentctlCached before upload = (%v, %v), want (false, nil)", cached, err)
	}
	remoteBin, err := ensureAgentctlOnHost(context.Background(), client, resolver, platform, newTestLogger())
	if err != nil {
		t.Fatalf("ensureAgentctlOnHost: %v", err)
	}
	if want := `C:\Users\Kandev User\.kandev\bin\agentctl.exe`; remoteBin != want {
		t.Fatalf("remoteBin = %q, want %q", remoteBin, want)
	}
	sum := sha256.Sum256([]byte("MZ-agentctl-v1"))
	if got := strings.TrimSpace(host.entries(t, ".sha256")["agentctl.exe.sha256"]); got != hex.EncodeToString(sum[:]) {
		t.Fatalf("sidecar = %q, want the uploaded digest", got)
	}
	if cached, err := SSHCheckAgentctlCached(context.Background(), client, resolver, platform); err != nil || !cached {
		t.Fatalf("SSHCheckAgentctlCached after upload = (%v, %v), want (true, nil)", cached, err)
	}

	host.fs.mu.Lock()
	writes := host.fs.writes
	host.fs.mu.Unlock()
	if _, err := ensureAgentctlOnHost(context.Background(), client, resolver, platform, newTestLogger()); err != nil {
		t.Fatalf("second ensureAgentctlOnHost: %v", err)
	}
	host.fs.mu.Lock()
	defer host.fs.mu.Unlock()
	if host.fs.writes != writes {
		t.Fatal("a matching sha256 must skip uploading agentctl.exe")
	}
}

// windowsSSHLaunchHarness is a Windows SSH host for one task (task-win) and
// session (session-win): PowerShell over a fake SSH server, a /C:/ SFTP drive,
// and a forwarded agentctl control API.
type windowsSSHLaunchHarness struct {
	server           *fakeSSHServer
	driveRoot        string
	winHome          string
	prepareScripts   []string
	prepareStdinJSON []string
	startScripts     []string
	stopScripts      []string
	cleanupScripts   []string
	alive            atomic.Bool
	mu               sync.Mutex
}

func newWindowsSSHLaunchHarness(t *testing.T) *windowsSSHLaunchHarness {
	t.Helper()
	withSSHKeepaliveTuning(t, 5*time.Second, 20*time.Second)
	driveRoot := t.TempDir()
	winHomeRel := filepath.Join("Users", "Kandev User")
	if err := os.MkdirAll(filepath.Join(driveRoot, winHomeRel), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &windowsSSHLaunchHarness{
		driveRoot: driveRoot,
		winHome:   `C:\Users\Kandev User`,
	}
	h.alive.Store(true)

	wantWorkspace := `C:\Users\Kandev User\.kandev\tasks\task-task-win`
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		switch {
		case r.URL.Path == "/auth/handshake":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"token": "win-launch-token"})
		case r.URL.Path == "/api/v1/instances":
			_, _ = io.WriteString(w, `{"id":"win-remote-instance","port":45123,"scoped_mcp_routes":true}`)
		case strings.HasPrefix(r.URL.Path, "/api/v1/instances/"):
			auth := r.Header.Get("Authorization")
			if auth != "Bearer win-launch-token" && auth != "Bearer persisted-win-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/instances/")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": id, "port": 45123, "status": "running",
				// Simulate Windows case/slash difference to prove normalized comparison.
				"workspace_path":    strings.ToLower(wantWorkspace),
				"session_id":        "session-win",
				"task_id":           "task-win",
				"scoped_mcp_routes": true,
			})
		case r.URL.Path == "/health":
			_, _ = io.WriteString(w, "ok")
		case r.URL.Path == "/status":
			_, _ = io.WriteString(w, `{"agent_running":true}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(control.Close)

	helperPath := filepath.Join(t.TempDir(), "agentctl-windows-amd64.exe")
	if err := os.WriteFile(helperPath, []byte("MZ-win-agentctl"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KANDEV_AGENTCTL_WINDOWS_AMD64_BINARY", helperPath)

	h.server = newFakeSSHServer(t, func(command, stdin string) sshExecResult {
		if strings.HasPrefix(command, "uname ") {
			return sshFail("uname not found")
		}
		script, ok := decodeTestPowerShell(command, stdin)
		if !ok {
			return sshFail("expected PowerShell -EncodedCommand, got: " + command)
		}
		return h.handlePowerShell(script, stdin)
	})
	h.server.enableSFTP()
	h.server.sftpHandler = newWindowsDriveSFTPFS(driveRoot).handlers()
	h.server.forwardTo(strings.TrimPrefix(control.URL, "http://"))
	return h
}

func (h *windowsSSHLaunchHarness) newExecutor(t *testing.T) *SSHExecutor {
	t.Helper()
	exec := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())
	t.Cleanup(func() { _ = exec.Close() })
	return exec
}

// request returns a create request for the harness task and session.
func (h *windowsSSHLaunchHarness) request(t *testing.T) *ExecutorCreateRequest {
	t.Helper()
	metadata := sshConnectionMetadata(t, h.server)
	metadata["repository_clone_url"] = "https://github.com/example/demo.git"
	return &ExecutorCreateRequest{
		InstanceID: "instance-win",
		TaskID:     "task-win",
		SessionID:  "session-win",
		Protocol:   "acp",
		Metadata:   metadata,
	}
}

func (h *windowsSSHLaunchHarness) counts() (starts, stops, cleanups int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.startScripts), len(h.stopScripts), len(h.cleanupScripts)
}

func (h *windowsSSHLaunchHarness) toHostPath(winPath string) string {
	cleaned := cleanWindowsRemotePath(winPath)
	rel := strings.TrimPrefix(cleaned, `C:\`)
	return filepath.Join(h.driveRoot, filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/")))
}

func (h *windowsSSHLaunchHarness) handlePowerShell(script, stdin string) sshExecResult {
	switch {
	case strings.Contains(script, "KANDEV_WIN_INFO"):
		return sshOut("KANDEV_WIN_INFO\tWindows_NT\tAMD64\tMicrosoft Windows NT 10.0.22631.0\r\n")
	case strings.Contains(script, "git --version"):
		return sshOut("git version 2.47.0.windows.1\r\n")
	case strings.Contains(script, "$env:USERPROFILE"):
		return sshOut(h.winHome)
	case strings.Contains(script, "Invoke-CimMethod -ClassName Win32_Process -MethodName Create"):
		h.mu.Lock()
		h.startScripts = append(h.startScripts, script)
		h.mu.Unlock()
		h.alive.Store(true)
		sessionHostDir := h.toHostPath(`C:\Users\Kandev User\.kandev\tasks\task-task-win\.kandev\sessions\session-win`)
		_ = os.MkdirAll(sessionHostDir, 0o755)
		_ = os.WriteFile(filepath.Join(sessionHostDir, "agentctl.pid"), []byte("4242\r\n"), 0o644)
		_ = os.WriteFile(filepath.Join(sessionHostDir, "agentctl.log"), []byte(`{"msg":"HTTP server bound successfully","address":"127.0.0.1:45124"}`+"\r\n"), 0o644)
		return sshOut("4242\r\n")
	case strings.Contains(script, "Join-Path") && strings.Contains(script, "'agentctl.log'"):
		logFile := h.toHostPath(`C:\Users\Kandev User\.kandev\tasks\task-task-win\.kandev\sessions\session-win\agentctl.log`)
		data, err := os.ReadFile(logFile)
		if err != nil {
			return sshFail(err.Error())
		}
		return sshOut(string(data))
	case strings.Contains(script, "# Prepare an SSH task workspace on a Windows remote host."):
		h.mu.Lock()
		h.prepareScripts = append(h.prepareScripts, script)
		h.prepareStdinJSON = append(h.prepareStdinJSON, stdin)
		h.mu.Unlock()
		return sshOK
	case strings.Contains(script, "KANDEV_WIN_CLEANUP_MARKER"):
		h.mu.Lock()
		h.cleanupScripts = append(h.cleanupScripts, script)
		h.mu.Unlock()
		return sshOK
	case strings.Contains(script, "rev-parse --show-toplevel"):
		// Git for Windows outputs forward slashes and can lowercase the drive letter.
		return sshOut("c:/Users/Kandev User/.kandev/tasks/task-task-win\r\n")
	case strings.Contains(script, "config --get remote.origin.url"):
		return sshOut("https://github.com/example/demo.git\r\n")
	case strings.Contains(script, "rev-parse --verify HEAD"):
		return sshOut("0123456789abcdef0123456789abcdef01234567\r\n")
	case strings.Contains(script, "(Get-Item -LiteralPath") && strings.Contains(script, ").FullName"):
		return sshOut(`C:\Users\Kandev User\.kandev\tasks\task-task-win`)
	case strings.Contains(script, "taskkill.exe /PID"):
		h.mu.Lock()
		h.stopScripts = append(h.stopScripts, script)
		h.mu.Unlock()
		h.alive.Store(false)
		sessionHostDir := h.toHostPath(`C:\Users\Kandev User\.kandev\tasks\task-task-win\.kandev\sessions\session-win`)
		_ = os.RemoveAll(sessionHostDir)
		return sshOK
	case strings.Contains(script, "Get-Process -Id"):
		if h.alive.Load() {
			return sshOut("ALIVE\r\n")
		}
		return sshOut("ABSENT\r\n")
	case strings.Contains(script, "agentctl.exe.sha256") && strings.Contains(script, "ReadAllText"):
		shaFile := h.toHostPath(`C:\Users\Kandev User\.kandev\bin\agentctl.exe.sha256`)
		binFile := h.toHostPath(`C:\Users\Kandev User\.kandev\bin\agentctl.exe`)
		if _, err := os.Stat(binFile); err != nil {
			return sshOK
		}
		data, err := os.ReadFile(shaFile)
		if err != nil {
			return sshOK
		}
		return sshOut(strings.TrimSpace(string(data)))
	case strings.Contains(script, "CreateDirectory"):
		for _, winPath := range []string{
			`C:\Users\Kandev User\.kandev\tasks\task-task-win`,
			`C:\Users\Kandev User\.kandev\tasks\task-task-win\.kandev\sessions\session-win`,
		} {
			if strings.Contains(script, winPath) {
				_ = os.MkdirAll(h.toHostPath(winPath), 0o755)
			}
		}
		return sshOK
	default:
		return sshOK
	}
}

func TestSSHWindowsCreateInstancePowerShellEndToEnd(t *testing.T) {
	harness := newWindowsSSHLaunchHarness(t)
	exec := harness.newExecutor(t)
	req := harness.request(t)
	// Simulate a profile that snapshotted the built-in POSIX prepare script on
	// creation; Windows PowerShell execution must translate it automatically.
	req.Metadata[MetadataKeySetupScript] = defaultSSHPrepareScript
	req.Metadata[MetadataKeyBaseBranch] = "main"
	req.Metadata[MetadataKeyWorktreeBranch] = "feature/win-ssh"
	req.Env = map[string]string{envKeyGitHubToken: "ghs_secret_win_token"}

	instance, err := exec.CreateInstance(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateInstance on Windows SSH host: %v", err)
	}

	wantTaskDir := `C:\Users\Kandev User\.kandev\tasks\task-task-win`
	wantSessionDir := wantTaskDir + `\.kandev\sessions\session-win`
	if instance.RuntimeName != executor.NameSSH {
		t.Fatalf("RuntimeName = %q", instance.RuntimeName)
	}
	if instance.WorkspacePath != wantTaskDir {
		t.Fatalf("WorkspacePath = %q, want %q", instance.WorkspacePath, wantTaskDir)
	}
	if got := getMetadataString(instance.Metadata, MetadataKeySSHRemoteSessionDir); got != wantSessionDir {
		t.Fatalf("remote session dir = %q, want %q", got, wantSessionDir)
	}
	if got := getMetadataString(instance.Metadata, MetadataKeySSHRemotePlatform); got != "windows/amd64" {
		t.Fatalf("ssh_remote_platform = %q, want windows/amd64", got)
	}

	harness.mu.Lock()
	defer harness.mu.Unlock()
	if len(harness.prepareScripts) != 1 || len(harness.prepareStdinJSON) != 1 {
		t.Fatalf("prepare script invocations = %d/%d, want 1", len(harness.prepareScripts), len(harness.prepareStdinJSON))
	}
	if strings.Contains(harness.prepareScripts[0], "ghs_secret_win_token") {
		t.Fatal("secret GitHub token leaked into PowerShell script command instead of stdin JSON")
	}
	for _, line := range harness.server.commands() {
		if strings.Contains(line, "ghs_secret_win_token") {
			t.Fatalf("secret GitHub token appeared on a remote command line: %.120s", line)
		}
	}
	if !strings.Contains(harness.prepareStdinJSON[0], `"GITHUB_TOKEN":"ghs_secret_win_token"`) {
		t.Fatalf("expected GITHUB_TOKEN in PowerShell stdin JSON, got: %s", harness.prepareStdinJSON[0])
	}
	if len(harness.startScripts) != 1 || !strings.Contains(harness.startScripts[0], `--workdir "`+wantTaskDir+`"`) {
		t.Fatalf("expected Win32-quoted --workdir in the WMI launch wrapper, got: %v", harness.startScripts)
	}
}
