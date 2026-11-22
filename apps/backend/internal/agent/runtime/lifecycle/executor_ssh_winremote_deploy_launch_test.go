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

// windowsDriveSFTPFS serves SFTP requests for /C:/... against a real local
// temp directory, faithfully emulating Windows OpenSSH sftp-server (including
// non-overwriting standard Rename and optional unsupported Chmod).
type windowsDriveSFTPFS struct {
	root             string
	allowPosixRename bool
	unsupportedChmod bool
}

func newWindowsDriveSFTPHandlers(root string, allowPosixRename, unsupportedChmod bool) *sftp.Handlers {
	fsImpl := &windowsDriveSFTPFS{
		root:             root,
		allowPosixRename: allowPosixRename,
		unsupportedChmod: unsupportedChmod,
	}
	var cmd sftp.FileCmder = fsImpl
	if allowPosixRename {
		cmd = &windowsDriveSFTPPosixFS{windowsDriveSFTPFS: fsImpl}
	}
	return &sftp.Handlers{
		FileGet:  fsImpl,
		FilePut:  fsImpl,
		FileCmd:  cmd,
		FileList: fsImpl,
	}
}

type windowsDriveSFTPPosixFS struct {
	*windowsDriveSFTPFS
}

func (f *windowsDriveSFTPPosixFS) PosixRename(r *sftp.Request) error {
	src, err := f.localPath(r.Filepath)
	if err != nil {
		return err
	}
	dst, err := f.localPath(r.Target)
	if err != nil {
		return err
	}
	return os.Rename(src, dst)
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
	return os.OpenFile(local, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
}

func (f *windowsDriveSFTPFS) Filecmd(r *sftp.Request) error {
	local, err := f.localPath(r.Filepath)
	if err != nil {
		return err
	}
	switch r.Method {
	case "Setstat":
		if f.unsupportedChmod {
			return sftp.ErrSSHFxOpUnsupported
		}
		return nil
	case "Rename":
		if !f.allowPosixRename {
			// Emulate standard Windows MoveFile without MOVEFILE_REPLACE_EXISTING.
			if _, statErr := os.Stat(local); statErr != nil {
				return sftp.ErrSSHFxNoSuchFile
			}
			dst, dstErr := f.localPath(r.Target)
			if dstErr != nil {
				return dstErr
			}
			if _, existsErr := os.Stat(dst); existsErr == nil {
				return sftp.ErrSSHFxFailure
			}
			return os.Rename(local, dst)
		}
		dst, dstErr := f.localPath(r.Target)
		if dstErr != nil {
			return dstErr
		}
		return os.Rename(local, dst)
	case "Mkdir":
		return os.Mkdir(local, 0o755)
	case "Rmdir":
		return os.Remove(local)
	case "Remove":
		return os.Remove(local)
	default:
		return sftp.ErrSSHFxOpUnsupported
	}
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
			info, infoErr := entry.Info()
			if infoErr == nil {
				infos = append(infos, info)
			}
		}
		return sftpStaticLister(infos), nil
	case "Stat":
		info, err := os.Stat(local)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, sftp.ErrSSHFxNoSuchFile
			}
			return nil, err
		}
		return sftpStaticLister{info}, nil
	case "Lstat":
		info, err := os.Lstat(local)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, sftp.ErrSSHFxNoSuchFile
			}
			return nil, err
		}
		return sftpStaticLister{info}, nil
	default:
		return nil, sftp.ErrSSHFxOpUnsupported
	}
}

func TestSSHWindowsDetectRemoteInfoFallsBackToPowerShell(t *testing.T) {
	for _, tc := range []struct {
		name          string
		winArch       string
		wantArch      string
		wantSupported bool
	}{
		{name: "windows amd64", winArch: "AMD64", wantArch: "amd64", wantSupported: true},
		{name: "windows arm64 is detected but unsupported", winArch: "ARM64", wantArch: "arm64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
				if strings.HasPrefix(command, "uname ") {
					return sshFail("'uname' is not recognized as an internal or external command")
				}
				script, ok := decodePowerShellEncodedCommand(command)
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
			err = SSHRequireSupportedExecutorPlatform(info.Platform)
			if tc.wantSupported != (err == nil) {
				t.Fatalf("SSHRequireSupportedExecutorPlatform(%s) = %v, want supported=%v", info.Platform, err, tc.wantSupported)
			}
		})
	}
}

func TestSSHWindowsEnsureAgentctlUploadsAndReplacesExeViaSFTP(t *testing.T) {
	driveRoot := t.TempDir()
	winHomeRel := filepath.Join("Users", "Kandev User")
	if err := os.MkdirAll(filepath.Join(driveRoot, winHomeRel), 0o755); err != nil {
		t.Fatal(err)
	}
	winHome := `C:\Users\Kandev User`

	v1Bytes := []byte("MZ-agentctl-v1")
	v1Sum := sha256.Sum256(v1Bytes)
	v1Sha := hex.EncodeToString(v1Sum[:])
	localHelper := filepath.Join(t.TempDir(), "agentctl-windows-amd64.exe")
	if err := os.WriteFile(localHelper, v1Bytes, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KANDEV_AGENTCTL_WINDOWS_AMD64_BINARY", localHelper)

	var mkdirCalls atomic.Int32
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		script, ok := decodePowerShellEncodedCommand(command)
		if !ok {
			return sshFail("expected PowerShell command, got: " + command)
		}
		switch {
		case strings.Contains(script, "$env:USERPROFILE"):
			return sshOut(winHome)
		case strings.Contains(script, "ReadAllText"):
			shaPath := filepath.Join(driveRoot, winHomeRel, ".kandev", "bin", "agentctl.exe.sha256")
			binPath := filepath.Join(driveRoot, winHomeRel, ".kandev", "bin", "agentctl.exe")
			info, statErr := os.Stat(binPath)
			if statErr != nil || info.Size() == 0 {
				return sshOK
			}
			data, readErr := os.ReadFile(shaPath)
			if readErr != nil {
				return sshOK
			}
			return sshOut(strings.TrimSpace(string(data)))
		case strings.Contains(script, "CreateDirectory"):
			mkdirCalls.Add(1)
			if err := os.MkdirAll(filepath.Join(driveRoot, winHomeRel, ".kandev", "bin"), 0o755); err != nil {
				return sshFail(err.Error())
			}
			return sshOK
		case strings.Contains(script, "agentctl.exe missing after upload"):
			binPath := filepath.Join(driveRoot, winHomeRel, ".kandev", "bin", "agentctl.exe")
			info, err := os.Stat(binPath)
			if err != nil || info.Size() == 0 {
				return sshFail("binary missing on disk")
			}
			return sshOK
		default:
			return sshFail("unexpected PowerShell script: " + script)
		}
	})
	server.enableSFTP()
	server.sftpHandler = newWindowsDriveSFTPHandlers(driveRoot, false, true)

	client := server.dial(t)
	platform := SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}
	resolver := NewAgentctlResolver(newTestLogger())

	cachedBefore, err := SSHCheckAgentctlCached(context.Background(), client, resolver, platform)
	if err != nil || cachedBefore {
		t.Fatalf("SSHCheckAgentctlCached before upload = (%v, %v), want (false, nil)", cachedBefore, err)
	}

	remoteBin, err := ensureAgentctlOnHost(context.Background(), client, resolver, platform, newTestLogger())
	if err != nil {
		t.Fatalf("first ensureAgentctlOnHost: %v", err)
	}
	wantRemoteBin := `C:\Users\Kandev User\.kandev\bin\agentctl.exe`
	if remoteBin != wantRemoteBin {
		t.Fatalf("remoteBin = %q, want %q", remoteBin, wantRemoteBin)
	}
	diskBytes, err := os.ReadFile(filepath.Join(driveRoot, winHomeRel, ".kandev", "bin", "agentctl.exe"))
	if err != nil || string(diskBytes) != string(v1Bytes) {
		t.Fatalf("uploaded agentctl.exe bytes = %q, err=%v", diskBytes, err)
	}
	diskSha, err := os.ReadFile(filepath.Join(driveRoot, winHomeRel, ".kandev", "bin", "agentctl.exe.sha256"))
	if err != nil || strings.TrimSpace(string(diskSha)) != v1Sha {
		t.Fatalf("uploaded sha256 = %q, want %q (err=%v)", diskSha, v1Sha, err)
	}

	cachedAfter, err := SSHCheckAgentctlCached(context.Background(), client, resolver, platform)
	if err != nil || !cachedAfter {
		t.Fatalf("SSHCheckAgentctlCached after upload = (%v, %v), want (true, nil)", cachedAfter, err)
	}

	// Second call with matching sha256 must skip re-upload.
	beforeSecond := mkdirCalls.Load()
	if _, err := ensureAgentctlOnHost(context.Background(), client, resolver, platform, newTestLogger()); err != nil {
		t.Fatalf("second ensureAgentctlOnHost: %v", err)
	}
	if mkdirCalls.Load() != beforeSecond {
		t.Fatal("matching sha256 should skip uploading agentctl.exe")
	}

	// Updating local helper bytes must replace the existing Windows file even
	// when PosixRename is unsupported and Chmod returns OpUnsupported.
	v2Bytes := []byte("MZ-agentctl-v2-updated")
	if err := os.WriteFile(localHelper, v2Bytes, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureAgentctlOnHost(context.Background(), client, resolver, platform, newTestLogger()); err != nil {
		t.Fatalf("upgrade ensureAgentctlOnHost: %v", err)
	}
	updatedBytes, err := os.ReadFile(filepath.Join(driveRoot, winHomeRel, ".kandev", "bin", "agentctl.exe"))
	if err != nil || string(updatedBytes) != string(v2Bytes) {
		t.Fatalf("replaced agentctl.exe bytes = %q, want %q (err=%v)", updatedBytes, v2Bytes, err)
	}
}

type windowsSSHLaunchHarness struct {
	server           *fakeSSHServer
	driveRoot        string
	winHome          string
	createRequests   []string
	prepareScripts   []string
	prepareStdinJSON []string
	startScripts     []string
	stopScripts      []string
	cleanupScripts   []string
	alive            atomic.Bool
	pid              int
	mu               sync.Mutex
}

func newWindowsSSHLaunchHarness(t *testing.T, pid int) *windowsSSHLaunchHarness {
	t.Helper()
	driveRoot := t.TempDir()
	winHomeRel := filepath.Join("Users", "Kandev User")
	if err := os.MkdirAll(filepath.Join(driveRoot, winHomeRel), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &windowsSSHLaunchHarness{
		driveRoot: driveRoot,
		winHome:   `C:\Users\Kandev User`,
		pid:       pid,
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
			body, _ := io.ReadAll(r.Body)
			h.createRequests = append(h.createRequests, string(body))
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
		script, ok := decodePowerShellEncodedCommand(command)
		if !ok {
			return sshFail("expected PowerShell -EncodedCommand, got: " + command)
		}
		return h.handlePowerShell(script, stdin)
	})
	h.server.enableSFTP()
	h.server.sftpHandler = newWindowsDriveSFTPHandlers(driveRoot, false, true)
	h.server.forwardTo(strings.TrimPrefix(control.URL, "http://"))
	return h
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
	case strings.Contains(script, "Start-Process -FilePath $agentctlBin"):
		h.mu.Lock()
		h.startScripts = append(h.startScripts, script)
		h.mu.Unlock()
		h.alive.Store(true)
		sessionHostDir := h.toHostPath(`C:\Users\Kandev User\.kandev\tasks\task-task-win\.kandev\sessions\session-win`)
		_ = os.MkdirAll(sessionHostDir, 0o755)
		_ = os.WriteFile(filepath.Join(sessionHostDir, "agentctl.pid"), []byte("4242\r\n"), 0o644)
		_ = os.WriteFile(filepath.Join(sessionHostDir, "agentctl.log"), []byte(`{"msg":"HTTP server bound successfully","address":"127.0.0.1:45124"}`+"\r\n"), 0o644)
		return sshOut("4242\r\n")
	case strings.Contains(script, "foreach ($name in @('agentctl.log', 'agentctl.err.log'))") || strings.Contains(script, "Join-Path") && strings.Contains(script, "'agentctl.log'"):
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
	case strings.Contains(script, "Get-Command -Name"):
		return sshOut(`C:\Program Files\nodejs\npx.cmd`)
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
	case strings.Contains(script, "Win32_Process"):
		if !h.alive.Load() {
			return sshOut("ABSENT\r\n")
		}
		return sshOut(`CMDLINE:"C:\Users\Kandev User\.kandev\bin\agentctl.exe" --workdir "C:\Users\Kandev User\.kandev\tasks\task-task-win"` + "\r\n")
	case strings.Contains(script, "DIR_MISSING") && strings.Contains(script, "agentctl.pid"):
		sessionHostDir := h.toHostPath(`C:\Users\Kandev User\.kandev\tasks\task-task-win\.kandev\sessions\session-win`)
		if _, err := os.Stat(sessionHostDir); os.IsNotExist(err) {
			return sshOut("DIR_MISSING\r\n")
		}
		data, err := os.ReadFile(filepath.Join(sessionHostDir, "agentctl.pid"))
		if err != nil {
			return sshOut("PIDFILE_MISSING\r\n")
		}
		return sshOut("PID:" + strings.TrimSpace(string(data)) + "\r\n")
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
			`C:\Users\Kandev User\.kandev\bin`,
			`C:\Users\Kandev User\.kandev\tasks\task-task-win`,
			`C:\Users\Kandev User\.kandev\tasks\task-task-win\.kandev\sessions\session-win`,
		} {
			if strings.Contains(script, winPath) {
				_ = os.MkdirAll(h.toHostPath(winPath), 0o755)
			}
		}
		return sshOK
	case strings.Contains(script, "agentctl.exe missing after upload"):
		if _, err := os.Stat(h.toHostPath(`C:\Users\Kandev User\.kandev\bin\agentctl.exe`)); err != nil {
			return sshFail(err.Error())
		}
		return sshOK
	default:
		return sshOK
	}
}

func TestSSHWindowsCreateInstancePowerShellEndToEnd(t *testing.T) {
	withSSHKeepaliveTuning(t, 5*time.Second, 20*time.Second)
	harness := newWindowsSSHLaunchHarness(t, 4242)
	exec := NewSSHExecutor(nil, nil, NewAgentctlResolver(newTestLogger()), newTestLogger())
	t.Cleanup(func() { _ = exec.Close() })

	metadata := sshConnectionMetadata(t, harness.server)
	// Simulate a profile that snapshotted the built-in POSIX prepare script on
	// creation; Windows PowerShell execution must translate it automatically.
	metadata[MetadataKeySetupScript] = defaultSSHPrepareScript
	metadata["repository_clone_url"] = "https://github.com/example/demo.git"
	metadata[MetadataKeyBaseBranch] = "main"
	metadata[MetadataKeyWorktreeBranch] = "feature/win-ssh"

	req := &ExecutorCreateRequest{
		InstanceID: "instance-win",
		TaskID:     "task-win",
		SessionID:  "session-win",
		Protocol:   "acp",
		Metadata:   metadata,
		Env: map[string]string{
			envKeyGitHubToken: "ghs_secret_win_token",
		},
	}

	instance, err := exec.CreateInstance(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateInstance on Windows SSH host: %v", err)
	}

	wantTaskDir := `C:\Users\Kandev User\.kandev\tasks\task-task-win`
	wantSessionDir := `C:\Users\Kandev User\.kandev\tasks\task-task-win\.kandev\sessions\session-win`
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
	prepScripts := append([]string(nil), harness.prepareScripts...)
	prepStdin := append([]string(nil), harness.prepareStdinJSON...)
	startScripts := append([]string(nil), harness.startScripts...)
	harness.mu.Unlock()

	if len(prepScripts) != 1 || len(prepStdin) != 1 {
		t.Fatalf("prepare script invocations = %d/%d, want 1", len(prepScripts), len(prepStdin))
	}
	if strings.Contains(prepScripts[0], "ghs_secret_win_token") {
		t.Fatal("secret GitHub token leaked into PowerShell script command instead of stdin JSON")
	}
	if !strings.Contains(prepStdin[0], `"GITHUB_TOKEN":"ghs_secret_win_token"`) {
		t.Fatalf("expected GITHUB_TOKEN in PowerShell stdin JSON, got: %s", prepStdin[0])
	}
	if len(startScripts) != 1 || !strings.Contains(startScripts[0], `--workdir "C:\Users\Kandev User\.kandev\tasks\task-task-win"`) {
		t.Fatalf("expected Win32-quoted --workdir in PowerShell Start-Process script, got: %v", startScripts)
	}
}
