package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/sftp"
)

// windowsInUseSFTPFS adds Windows image locking to the drive emulation: the
// file of a running process cannot be removed, but it can be renamed, and the
// lock follows the renamed file.
type windowsInUseSFTPFS struct {
	*windowsDriveSFTPFS
	mu    sync.Mutex
	inUse map[string]bool
	// failTempRename fails the final rename of an uploaded temporary file.
	failTempRename bool
}

func (f *windowsInUseSFTPFS) Filecmd(r *sftp.Request) error {
	local, err := f.localPath(r.Filepath)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case "Remove":
		if f.inUse[local] {
			return sftp.ErrSSHFxFailure
		}
	case "Rmdir":
		// pkg/sftp retries a failed file removal as a directory removal; a
		// Windows rmdir never deletes a file.
		if info, statErr := os.Stat(local); statErr == nil && !info.IsDir() {
			return sftp.ErrSSHFxFailure
		}
	case "Rename":
		dst, dstErr := f.localPath(r.Target)
		if dstErr != nil {
			return dstErr
		}
		if f.failTempRename && strings.Contains(filepath.Base(local), ".tmp.") {
			return sftp.ErrSSHFxFailure
		}
		if err := f.windowsDriveSFTPFS.Filecmd(r); err != nil {
			return err
		}
		if f.inUse[local] {
			delete(f.inUse, local)
			f.inUse[dst] = true
		}
		return nil
	}
	return f.windowsDriveSFTPFS.Filecmd(r)
}

type windowsHelperHost struct {
	server *fakeSSHServer
	fs     *windowsInUseSFTPFS
	binDir string
	helper string
}

func newWindowsHelperHost(t *testing.T) *windowsHelperHost {
	t.Helper()
	driveRoot := t.TempDir()
	binDir := filepath.Join(driveRoot, "Users", "Kandev User", ".kandev", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	host := &windowsHelperHost{
		fs: &windowsInUseSFTPFS{
			windowsDriveSFTPFS: &windowsDriveSFTPFS{root: driveRoot, unsupportedChmod: true},
			inUse:              map[string]bool{},
		},
		binDir: binDir,
		helper: filepath.Join(t.TempDir(), "agentctl-windows-amd64.exe"),
	}
	t.Setenv("KANDEV_AGENTCTL_WINDOWS_AMD64_BINARY", host.helper)
	host.server = newFakeSSHServer(t, func(command, stdin string) sshExecResult {
		script, ok := decodeTestPowerShell(command, stdin)
		switch {
		case !ok:
			return sshFail("expected the PowerShell loader, got: " + command)
		case strings.Contains(script, "$env:USERPROFILE"):
			return sshOut(`C:\Users\Kandev User`)
		case strings.Contains(script, "ReadAllText"):
			data, err := os.ReadFile(filepath.Join(binDir, "agentctl.exe.sha256"))
			if err != nil {
				return sshOK
			}
			return sshOut(strings.TrimSpace(string(data)))
		default:
			return sshOK
		}
	})
	host.server.enableSFTP()
	host.server.sftpHandler = &sftp.Handlers{FileGet: host.fs, FilePut: host.fs, FileCmd: host.fs, FileList: host.fs}
	return host
}

func (h *windowsHelperHost) install(t *testing.T, data []byte, running bool) {
	t.Helper()
	bin := filepath.Join(h.binDir, "agentctl.exe")
	sum := sha256.Sum256(data)
	if err := os.WriteFile(bin, data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin+".sha256", []byte(hex.EncodeToString(sum[:])+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.fs.mu.Lock()
	h.fs.inUse[bin] = running
	h.fs.mu.Unlock()
}

func (h *windowsHelperHost) entries(t *testing.T, contains string) map[string]string {
	t.Helper()
	found := map[string]string{}
	dirEntries, err := os.ReadDir(h.binDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range dirEntries {
		if strings.Contains(entry.Name(), contains) {
			data, _ := os.ReadFile(filepath.Join(h.binDir, entry.Name()))
			found[entry.Name()] = string(data)
		}
	}
	return found
}

func (h *windowsHelperHost) ensure(t *testing.T, data []byte) error {
	t.Helper()
	if err := os.WriteFile(h.helper, data, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ensureAgentctlOnHost(context.Background(), h.server.dial(t), NewAgentctlResolver(newTestLogger()),
		SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}, newTestLogger())
	return err
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.6
func TestSSHWindowsReplacesRunningHelperByRenamingAside(t *testing.T) {
	host := newWindowsHelperHost(t)
	host.install(t, []byte("MZ-v1"), true)

	if err := host.ensure(t, []byte("MZ-v2")); err != nil {
		t.Fatalf("replacing a running helper: %v", err)
	}
	if got := host.entries(t, "agentctl.exe")["agentctl.exe"]; got != "MZ-v2" {
		t.Fatalf("agentctl.exe = %q, want MZ-v2", got)
	}
	aside := host.entries(t, "agentctl.exe.old-")
	if len(aside) != 1 {
		t.Fatalf("aside files = %v, want the running v1 image kept once", aside)
	}
	for _, data := range aside {
		if data != "MZ-v1" {
			t.Fatalf("aside file holds %q, want MZ-v1", data)
		}
	}
	sum := sha256.Sum256([]byte("MZ-v2"))
	if got := strings.TrimSpace(host.entries(t, ".sha256")["agentctl.exe.sha256"]); got != hex.EncodeToString(sum[:]) {
		t.Fatalf("sidecar = %q, want the v2 digest", got)
	}
	if tmp := host.entries(t, ".tmp."); len(tmp) != 0 {
		t.Fatalf("temporary upload files remain: %v", tmp)
	}

	// The v1 process exits and v2 starts running. The next replacement removes
	// the stale v1 image and keeps the running v2 image aside.
	host.fs.mu.Lock()
	for name := range host.fs.inUse {
		delete(host.fs.inUse, name)
	}
	host.fs.inUse[filepath.Join(host.binDir, "agentctl.exe")] = true
	host.fs.mu.Unlock()
	if err := host.ensure(t, []byte("MZ-v3")); err != nil {
		t.Fatalf("second replacement: %v", err)
	}
	aside = host.entries(t, "agentctl.exe.old-")
	if len(aside) != 1 {
		t.Fatalf("aside files after second replacement = %v, want only the running v2 image", aside)
	}
	for _, data := range aside {
		if data != "MZ-v2" {
			t.Fatalf("aside file holds %q, want MZ-v2", data)
		}
	}
}

// @covers AC-EXECUTORS-SSH-WINDOWS-001.6
func TestSSHWindowsHelperReplacementRestoresPreviousOnFailure(t *testing.T) {
	host := newWindowsHelperHost(t)
	host.install(t, []byte("MZ-v1"), true)
	host.fs.failTempRename = true

	if err := host.ensure(t, []byte("MZ-v2")); err == nil {
		t.Fatal("expected the failed final rename to fail the upload")
	}
	if got := host.entries(t, "agentctl.exe")["agentctl.exe"]; got != "MZ-v1" {
		t.Fatalf("agentctl.exe = %q after a failed replacement, want the restored MZ-v1", got)
	}
	if aside := host.entries(t, ".old-"); len(aside) != 0 {
		t.Fatalf("aside files remain after restore: %v", aside)
	}
	if tmp := host.entries(t, ".tmp."); len(tmp) != 0 {
		t.Fatalf("temporary upload files remain: %v", tmp)
	}
}
