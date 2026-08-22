package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// remoteHomeCommand is the exact probe expandRemoteHome issues; matching on it
// keeps the fake-server rules honest about the real command string.
const remoteHomeCommand = `printf %s "$HOME"`

func TestDetectRemoteInfoNormalizesPlatform(t *testing.T) {
	tests := []struct {
		name       string
		unameOS    string
		unameArch  string
		wantGOOS   string
		wantGOARCH string
	}{
		{name: "linux x86_64", unameOS: "Linux", unameArch: "x86_64", wantGOOS: "linux", wantGOARCH: "amd64"},
		{name: "linux aarch64", unameOS: "Linux", unameArch: "aarch64", wantGOOS: "linux", wantGOARCH: "arm64"},
		{name: "darwin arm64", unameOS: "Darwin", unameArch: "arm64", wantGOOS: "darwin", wantGOARCH: "arm64"},
		{name: "darwin amd64", unameOS: "Darwin", unameArch: "x86_64", wantGOOS: "darwin", wantGOARCH: "amd64"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeSSHServer(t, newSSHScriptedHandler(t,
				sshScriptRule{match: "uname -a", result: sshOut("  " + tc.unameOS + " host 6.1.0  \n")},
				sshScriptRule{match: "uname -m", result: sshOut(tc.unameArch + "\n")},
				sshScriptRule{match: "uname -s", result: sshOut(tc.unameOS + "\n")},
				sshScriptRule{match: "git --version", result: sshOut("git version 2.44.0\n")},
			).handle)
			client := server.dial(t)

			info, err := SSHProbeRemote(context.Background(), client)
			if err != nil {
				t.Fatalf("SSHProbeRemote: %v", err)
			}
			if info.OS != tc.unameOS || info.Arch != tc.unameArch {
				t.Fatalf("raw uname = %q/%q, want %q/%q", info.OS, info.Arch, tc.unameOS, tc.unameArch)
			}
			if info.UnameAll != tc.unameOS+" host 6.1.0" {
				t.Fatalf("UnameAll = %q, want trimmed uname -a output", info.UnameAll)
			}
			if info.GitVer != "git version 2.44.0" {
				t.Fatalf("GitVer = %q", info.GitVer)
			}
			if info.Platform.GOOS != tc.wantGOOS || info.Platform.GOARCH != tc.wantGOARCH {
				t.Fatalf("platform = %s, want %s/%s", info.Platform, tc.wantGOOS, tc.wantGOARCH)
			}
			if info.Platform.UnameOS != tc.unameOS || info.Platform.UnameArch != tc.unameArch {
				t.Fatalf("platform retained raw = %q/%q", info.Platform.UnameOS, info.Platform.UnameArch)
			}
			if err := SSHRequireSupportedRemotePlatform(info.Platform); err != nil {
				t.Fatalf("normalized platform %s must be supported: %v", info.Platform, err)
			}
			wantCommands := []string{"uname -a", "uname -m", "uname -s", "git --version"}
			if got := server.commands(); !equalStrings(got, wantCommands) {
				t.Fatalf("probe commands = %v, want %v", got, wantCommands)
			}
		})
	}
}

func TestDetectRemoteInfoFailsWhenUnameUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		failing string
		wantErr string
	}{
		{name: "arch probe fails", failing: "uname -m", wantErr: "ssh: uname -m"},
		{name: "os probe fails", failing: "uname -s", wantErr: "ssh: uname -s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
				if command == tc.failing {
					return sshFail("uname: not found")
				}
				return sshOut("Linux\n")
			})
			client := server.dial(t)

			info, err := detectRemoteInfo(context.Background(), client)
			if err == nil {
				t.Fatalf("expected failure, got info %+v", info)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestRequireSupportedRemotePlatformPreservesRawDetail(t *testing.T) {
	t.Run("supported matrix", func(t *testing.T) {
		for _, platform := range []SSHRemotePlatform{
			{GOOS: "linux", GOARCH: "amd64"},
			{GOOS: "linux", GOARCH: "arm64"},
			{GOOS: "darwin", GOARCH: "amd64"},
			{GOOS: "darwin", GOARCH: "arm64"},
		} {
			if err := requireSupportedRemotePlatform(platform); err != nil {
				t.Fatalf("%s must be supported: %v", platform, err)
			}
		}
	})

	t.Run("unsupported os keeps raw uname detail", func(t *testing.T) {
		platform, ok := normalizeSSHRemotePlatform("FreeBSD", "riscv64")
		if ok {
			t.Fatal("FreeBSD/riscv64 must not normalize as supported")
		}
		if platform.String() != "unknown" {
			t.Fatalf("normalized platform = %q, want unknown", platform.String())
		}
		err := requireSupportedRemotePlatform(platform)
		if err == nil {
			t.Fatal("expected unsupported-platform error")
		}
		if !strings.Contains(err.Error(), `"FreeBSD/riscv64"`) {
			t.Fatalf("error must preserve the raw reported platform: %v", err)
		}
		if !strings.Contains(err.Error(), "linux/{amd64,arm64}") ||
			!strings.Contains(err.Error(), "darwin/{amd64,arm64}") {
			t.Fatalf("error must list the supported matrix: %v", err)
		}
	})

	t.Run("half-unsupported keeps the normalized value", func(t *testing.T) {
		// Recognised OS, unrecognised arch: String() is "unknown" because
		// GOARCH is empty, so the raw pair is surfaced instead.
		platform, ok := normalizeSSHRemotePlatform("Linux", "ppc64le")
		if ok {
			t.Fatal("linux/ppc64le must not normalize as supported")
		}
		if platform.GOOS != "linux" || platform.GOARCH != "" {
			t.Fatalf("platform = %+v, want linux with empty arch", platform)
		}
		err := requireSupportedRemotePlatform(platform)
		if err == nil || !strings.Contains(err.Error(), `"Linux/ppc64le"`) {
			t.Fatalf("error = %v, want raw Linux/ppc64le detail", err)
		}
	})

	t.Run("fully unknown platform has no raw detail to add", func(t *testing.T) {
		err := requireSupportedRemotePlatform(SSHRemotePlatform{})
		if err == nil || !strings.Contains(err.Error(), `"unknown"`) {
			t.Fatalf("error = %v, want unknown", err)
		}
	})
}

func TestNormalizeSSHRemotePlatformAcceptsUnameAliases(t *testing.T) {
	tests := []struct {
		osName, arch       string
		wantGOOS, wantArch string
		wantSupported      bool
	}{
		{osName: "linux", arch: "amd64", wantGOOS: "linux", wantArch: "amd64", wantSupported: true},
		{osName: "  Linux  ", arch: "  X86_64 ", wantGOOS: "linux", wantArch: "amd64", wantSupported: true},
		{osName: "DARWIN", arch: "AARCH64", wantGOOS: "darwin", wantArch: "arm64", wantSupported: true},
		{osName: "Windows_NT", arch: "x86_64", wantGOOS: "", wantArch: "amd64", wantSupported: false},
		{osName: "Linux", arch: "i686", wantGOOS: "linux", wantArch: "", wantSupported: false},
	}
	for _, tc := range tests {
		t.Run(tc.osName+"/"+tc.arch, func(t *testing.T) {
			platform, ok := normalizeSSHRemotePlatform(tc.osName, tc.arch)
			if ok != tc.wantSupported {
				t.Fatalf("supported = %v, want %v", ok, tc.wantSupported)
			}
			if platform.GOOS != tc.wantGOOS || platform.GOARCH != tc.wantArch {
				t.Fatalf("normalized = %q/%q, want %q/%q",
					platform.GOOS, platform.GOARCH, tc.wantGOOS, tc.wantArch)
			}
		})
	}
}

func TestSSHDefaultShellForPlatformIsPlatformAware(t *testing.T) {
	// End-to-end contract: Darwin defaults to zsh, everything else to bash.
	if got := SSHDefaultShellForPlatform(SSHRemotePlatform{GOOS: "darwin", GOARCH: "arm64"}); got != "zsh" {
		t.Fatalf("darwin default shell = %q, want zsh", got)
	}
	if got := SSHDefaultShellForPlatform(SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}); got != "bash" {
		t.Fatalf("linux default shell = %q, want bash", got)
	}
	if got := SSHDefaultShellForPlatform(SSHRemotePlatform{}); got != "bash" {
		t.Fatalf("unknown-platform default shell = %q, want bash", got)
	}
}

func TestExpandRemoteHome(t *testing.T) {
	t.Run("rewrites tilde prefix", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: remoteHomeCommand, result: sshOut("/home/kandev\n")},
		).handle)
		client := server.dial(t)

		got, err := expandRemoteHome(context.Background(), client, "~/.kandev/bin/agentctl")
		if err != nil {
			t.Fatalf("expandRemoteHome: %v", err)
		}
		if got != "/home/kandev/.kandev/bin/agentctl" {
			t.Fatalf("expanded = %q", got)
		}
	})

	t.Run("bare tilde resolves to home", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: remoteHomeCommand, result: sshOut("/home/kandev")},
		).handle)
		got, err := expandRemoteHome(context.Background(), server.dial(t), "~")
		if err != nil {
			t.Fatalf("expandRemoteHome: %v", err)
		}
		if got != "/home/kandev" {
			t.Fatalf("expanded = %q", got)
		}
	})

	t.Run("absolute path issues no remote command", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		got, err := expandRemoteHome(context.Background(), server.dial(t), "/opt/agentctl")
		if err != nil {
			t.Fatalf("expandRemoteHome: %v", err)
		}
		if got != "/opt/agentctl" {
			t.Fatalf("expanded = %q, want unchanged", got)
		}
		if cmds := server.commands(); len(cmds) != 0 {
			t.Fatalf("absolute path must not probe the remote, got %v", cmds)
		}
	})

	t.Run("empty remote home is an error", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("  \n ") })
		_, err := expandRemoteHome(context.Background(), server.dial(t), "~/x")
		if err == nil || !strings.Contains(err.Error(), "remote $HOME is empty") {
			t.Fatalf("error = %v, want empty-$HOME error", err)
		}
	})

	t.Run("probe failure is wrapped", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("boom") })
		_, err := expandRemoteHome(context.Background(), server.dial(t), "~/x")
		if err == nil || !strings.Contains(err.Error(), "ssh: resolve $HOME") {
			t.Fatalf("error = %v, want wrapped resolve error", err)
		}
	})
}

func TestRunSSHCommandStdinFeedsRemoteStdin(t *testing.T) {
	server := newFakeSSHServer(t, func(_, stdin string) sshExecResult {
		return sshExecResult{Stdout: "received:" + stdin}
	})
	stdout, _, err := runSSHCommandStdin(context.Background(), server.dial(t),
		"cat", strings.NewReader("SECRET='x y'\n"))
	if err != nil {
		t.Fatalf("runSSHCommandStdin: %v", err)
	}
	if stdout != "received:SECRET='x y'\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if calls := server.execCalls(); len(calls) != 1 || calls[0].Stdin != "SECRET='x y'\n" {
		t.Fatalf("recorded stdin = %+v", calls)
	}
}

func TestRunSSHCommandAbandonsARunningCommandOnCancellation(t *testing.T) {
	// The remote command is held open across the cancellation for two reasons:
	// it makes the select land on ctx.Done() deterministically (cancelling up
	// front leaves both arms ready, and Go picks a ready arm at random), and it
	// keeps x/crypto/ssh's stdout/stderr copier goroutines writing while the
	// buffers are read — so under -race this doubles as the regression test for
	// the syncBuffer guarding those buffers.
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var releaseOnce, startOnce sync.Once
	releaseRemote := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseRemote)

	server := newFakeSSHServer(t, func(string, string) sshExecResult {
		startOnce.Do(func() { close(started) })
		<-release
		close(finished)
		return sshOut("output produced after the caller gave up")
	})
	client := server.dial(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		cancel()
	}()

	_, _, err := runSSHCommand(ctx, client, "sleep 60")

	// Returning while the command is still running is the whole point: the
	// caller must not be pinned to a remote command that outlives its context.
	select {
	case <-finished:
		t.Fatal("runSSHCommand waited for the remote command instead of abandoning it")
	default:
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}

	// Let the remote command finish and drain the server handler before the
	// test returns, so goleak sees no stragglers.
	releaseRemote()
	<-finished
}

func TestSyncBufferSnapshotsConcurrentWrites(t *testing.T) {
	var buf syncBuffer
	const writers, perWriter = 8, 50

	var wg sync.WaitGroup
	wg.Add(writers)
	for range writers {
		go func() {
			defer wg.Done()
			for range perWriter {
				_, _ = buf.Write([]byte("x"))
			}
		}()
	}
	// Read concurrently with the writes; under -race this is the assertion.
	for range 100 {
		_ = buf.String()
	}
	wg.Wait()

	if got := len(buf.String()); got != writers*perWriter {
		t.Fatalf("buffered bytes = %d, want %d", got, writers*perWriter)
	}
}

func TestRunSSHCommandSurfacesRemoteExitStatus(t *testing.T) {
	server := newFakeSSHServer(t, func(string, string) sshExecResult {
		return sshExecResult{Stdout: "partial", Stderr: "fatal: repo not found", ExitCode: 128}
	})
	stdout, stderr, err := runSSHCommand(context.Background(), server.dial(t), "git clone x")
	if err == nil {
		t.Fatal("expected non-zero exit to be an error")
	}
	if stdout != "partial" || stderr != "fatal: repo not found" {
		t.Fatalf("stdout/stderr = %q/%q, want both captured alongside the error", stdout, stderr)
	}
}

// writeFakeAgentctl writes a stand-in agentctl binary and points the resolver
// at it via the platform env var, returning the path and its hex sha256.
func writeFakeAgentctl(t *testing.T, platform SSHRemotePlatform, content string) (path, sha string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "agentctl-"+platform.GOOS+"-"+platform.GOARCH)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake agentctl: %v", err)
	}
	sum := sha256.Sum256([]byte(content))
	envName := fmt.Sprintf("KANDEV_AGENTCTL_%s_%s_BINARY",
		strings.ToUpper(platform.GOOS), strings.ToUpper(platform.GOARCH))
	t.Setenv(envName, path)
	return path, hex.EncodeToString(sum[:])
}

func TestLocalAgentctlSha256(t *testing.T) {
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	path, wantSha := writeFakeAgentctl(t, platform, "agentctl-bytes")
	resolver := NewAgentctlResolver(newTestLogger())

	gotSha, data, gotPath, err := localAgentctlSha256(resolver, platform)
	if err != nil {
		t.Fatalf("localAgentctlSha256: %v", err)
	}
	if gotSha != wantSha {
		t.Fatalf("sha = %q, want %q", gotSha, wantSha)
	}
	if string(data) != "agentctl-bytes" {
		t.Fatalf("data = %q", data)
	}
	if gotPath != path {
		t.Fatalf("path = %q, want %q", gotPath, path)
	}

	t.Run("unresolvable platform bubbles up", func(t *testing.T) {
		if _, _, _, err := localAgentctlSha256(resolver, SSHRemotePlatform{GOOS: "plan9"}); err == nil {
			t.Fatal("expected unsupported-platform error")
		}
	})
}

func TestSSHCheckAgentctlCached(t *testing.T) {
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	_, sha := writeFakeAgentctl(t, platform, "agentctl-bytes")
	resolver := NewAgentctlResolver(newTestLogger())

	t.Run("matching remote sha reports cached", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: remoteHomeCommand, result: sshOut("/home/kandev")},
			sshScriptRule{match: "cat '/home/kandev/.kandev/bin/agentctl.sha256'", result: sshOut(sha + "\n")},
		).handle)
		cached, err := SSHCheckAgentctlCached(context.Background(), server.dial(t), resolver, platform)
		if err != nil {
			t.Fatalf("SSHCheckAgentctlCached: %v", err)
		}
		if !cached {
			t.Fatal("matching sha must report cached")
		}
	})

	t.Run("missing sidecar reports not cached", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: remoteHomeCommand, result: sshOut("/home/kandev")},
			sshScriptRule{match: "agentctl.sha256", result: sshOK},
		).handle)
		cached, err := SSHCheckAgentctlCached(context.Background(), server.dial(t), resolver, platform)
		if err != nil {
			t.Fatalf("SSHCheckAgentctlCached: %v", err)
		}
		if cached {
			t.Fatal("absent sidecar must report not cached")
		}
	})

	t.Run("an ssh-level sha256 read failure is an error, not 'needs upload'", func(t *testing.T) {
		// Production appends `|| true`, so a missing file exits 0 (covered
		// above). A non-zero exit here therefore means the SSH call itself
		// failed, which must surface rather than be read as "not cached".
		server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
			if strings.Contains(command, "agentctl.sha256") {
				return sshFail("connection reset")
			}
			return sshOut("/home/kandev")
		})
		if _, err := SSHCheckAgentctlCached(context.Background(), server.dial(t), resolver, platform); err == nil {
			t.Fatal("expected transport failure to bubble up")
		}
	})
}

func TestEnsureAgentctlOnHostSkipsUploadWhenShaMatches(t *testing.T) {
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "arm64"}
	_, sha := writeFakeAgentctl(t, platform, "cached-agentctl")
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: remoteHomeCommand, result: sshOut("/home/kandev")},
		sshScriptRule{match: "cat '/home/kandev/.kandev/bin/agentctl.sha256'", result: sshOut(sha + "\n")},
		sshScriptRule{match: "test -x '/home/kandev/.kandev/bin/agentctl'", result: sshOK},
	).handle)

	remoteBin, err := ensureAgentctlOnHost(context.Background(), server.dial(t),
		NewAgentctlResolver(newTestLogger()), platform, newTestLogger())
	if err != nil {
		t.Fatalf("ensureAgentctlOnHost: %v", err)
	}
	if remoteBin != "/home/kandev/.kandev/bin/agentctl" {
		t.Fatalf("remote binary = %q", remoteBin)
	}
	for _, command := range server.commands() {
		if strings.Contains(command, "mkdir -p") {
			t.Fatalf("cached agentctl must not re-create the bin dir, saw %q", command)
		}
	}
}

func TestEnsureAgentctlOnHostUploadsWhenShaDiffers(t *testing.T) {
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	_, sha := writeFakeAgentctl(t, platform, "fresh-agentctl")
	// The SFTP subsystem serves the real filesystem, so the "remote home" is a
	// temp dir and the uploaded bytes are assertable on disk.
	remoteHome := t.TempDir()
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: remoteHomeCommand, result: sshOut(remoteHome)},
		sshScriptRule{match: "agentctl.sha256", result: sshOut("stale-sha\n")},
		sshScriptRule{match: "mkdir -p", result: sshOK},
		sshScriptRule{match: "test -x", result: sshOK},
	).handle)
	server.enableSFTP()
	if err := os.MkdirAll(filepath.Join(remoteHome, ".kandev", "bin"), 0o755); err != nil {
		t.Fatalf("seed remote bin dir: %v", err)
	}

	remoteBin, err := ensureAgentctlOnHost(context.Background(), server.dial(t),
		NewAgentctlResolver(newTestLogger()), platform, newTestLogger())
	if err != nil {
		t.Fatalf("ensureAgentctlOnHost: %v", err)
	}
	wantBin := filepath.Join(remoteHome, ".kandev", "bin", "agentctl")
	if remoteBin != wantBin {
		t.Fatalf("remote binary = %q, want %q", remoteBin, wantBin)
	}
	uploaded, err := os.ReadFile(wantBin)
	if err != nil {
		t.Fatalf("read uploaded agentctl: %v", err)
	}
	if string(uploaded) != "fresh-agentctl" {
		t.Fatalf("uploaded bytes = %q", uploaded)
	}
	info, err := os.Stat(wantBin)
	if err != nil {
		t.Fatalf("stat uploaded agentctl: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("uploaded mode = %v, want 0755", info.Mode().Perm())
	}
	sidecar, err := os.ReadFile(wantBin + ".sha256")
	if err != nil {
		t.Fatalf("read sha sidecar: %v", err)
	}
	if string(sidecar) != sha+"\n" {
		t.Fatalf("sidecar = %q, want %q", sidecar, sha+"\n")
	}
	if _, ok := server.lastCommandContaining("mkdir -p '" + filepath.Join(remoteHome, ".kandev", "bin") + "'"); !ok {
		t.Fatalf("expected the bin dir to be created before upload, commands: %v", server.commands())
	}
}

func TestEnsureAgentctlOnHostFailsWhenBinaryNotExecutable(t *testing.T) {
	platform := SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}
	writeFakeAgentctl(t, platform, "fresh-agentctl")
	remoteHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(remoteHome, ".kandev", "bin"), 0o755); err != nil {
		t.Fatalf("seed remote bin dir: %v", err)
	}
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch {
		case strings.Contains(command, remoteHomeCommand):
			return sshOut(remoteHome)
		case strings.Contains(command, "test -x"):
			return sshFail("not executable")
		default:
			return sshOK
		}
	})
	server.enableSFTP()

	_, err := ensureAgentctlOnHost(context.Background(), server.dial(t),
		NewAgentctlResolver(newTestLogger()), platform, newTestLogger())
	if err == nil || !strings.Contains(err.Error(), "not executable after upload") {
		t.Fatalf("error = %v, want post-upload executability failure", err)
	}
}

func TestSftpUploadBytesWritesAtomically(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	client := server.dial(t)
	root := t.TempDir()
	target := filepath.Join(root, "creds.json")

	if err := os.WriteFile(target, []byte("previous"), 0o600); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := sftpUploadBytes(client, target, []byte("replacement"), 0o640); err != nil {
		t.Fatalf("sftpUploadBytes: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "replacement" {
		t.Fatalf("content = %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", info.Mode().Perm())
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %d entries", len(entries))
	}
}

func TestSftpUploadBytesFailsOnMissingParent(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	target := filepath.Join(t.TempDir(), "absent", "file")
	if err := sftpUploadBytes(server.dial(t), target, []byte("x"), 0o644); err == nil {
		t.Fatal("expected create to fail when the parent directory is missing")
	}
}

func TestEnsureRemoteTaskDir(t *testing.T) {
	t.Run("creates the dir under the expanded workdir root", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: remoteHomeCommand, result: sshOut("/home/kandev")},
			sshScriptRule{match: "mkdir -p", result: sshOK},
		).handle)
		taskDir, err := ensureRemoteTaskDir(context.Background(), server.dial(t), "~/.kandev", "task-42")
		if err != nil {
			t.Fatalf("ensureRemoteTaskDir: %v", err)
		}
		if taskDir != "/home/kandev/.kandev/tasks/task-42" {
			t.Fatalf("taskDir = %q", taskDir)
		}
		call, ok := server.lastCommandContaining("mkdir -p")
		if !ok || call.Command != "mkdir -p '/home/kandev/.kandev/tasks/task-42'" {
			t.Fatalf("mkdir command = %q", call.Command)
		}
	})

	t.Run("empty task dir name is rejected before dialing", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		_, err := ensureRemoteTaskDir(context.Background(), server.dial(t), "~/.kandev", "")
		if err == nil || !strings.Contains(err.Error(), "task dir name is empty") {
			t.Fatalf("error = %v", err)
		}
		if cmds := server.commands(); len(cmds) != 0 {
			t.Fatalf("no remote command should run, got %v", cmds)
		}
	})

	t.Run("mkdir failure is wrapped with the path", func(t *testing.T) {
		server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
			if strings.HasPrefix(command, "mkdir") {
				return sshFail("permission denied")
			}
			return sshOut("/home/kandev")
		})
		_, err := ensureRemoteTaskDir(context.Background(), server.dial(t), "~/.kandev", "task-42")
		if err == nil || !strings.Contains(err.Error(), "/home/kandev/.kandev/tasks/task-42") {
			t.Fatalf("error = %v, want the task dir path in the message", err)
		}
	})
}

func TestEnsureRemoteSessionDir(t *testing.T) {
	t.Run("creates the per-session runtime dir", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "mkdir -p", result: sshOK},
		).handle)
		sessionDir, err := ensureRemoteSessionDir(context.Background(), server.dial(t), "/remote/task", "sess-7")
		if err != nil {
			t.Fatalf("ensureRemoteSessionDir: %v", err)
		}
		if sessionDir != "/remote/task/.kandev/sessions/sess-7" {
			t.Fatalf("sessionDir = %q", sessionDir)
		}
		if got := server.commands(); len(got) != 1 ||
			got[0] != "cd -- '/remote/task' && mkdir -p -- '.kandev/sessions/sess-7'" {
			t.Fatalf("commands = %v", got)
		}
	})

	t.Run("does not recreate a task directory deleted after the reuse probe", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "test -d", result: sshOK},
			sshScriptRule{match: "cd --", result: sshFail("no such file or directory")},
		).handle)
		client := server.dial(t)
		if err := ensureReuseRequiredRemoteTaskDirExists(context.Background(), client, "/remote/task"); err != nil {
			t.Fatalf("ensureReuseRequiredRemoteTaskDirExists: %v", err)
		}
		if _, err := ensureRemoteSessionDir(context.Background(), client, "/remote/task", "sess-7"); err == nil {
			t.Fatal("ensureRemoteSessionDir succeeded after the canonical task directory disappeared")
		}
		for _, command := range server.commands() {
			if strings.Contains(command, "mkdir -p '/remote/task") {
				t.Fatalf("session creation could recreate the canonical directory: %q", command)
			}
		}
	})

	t.Run("empty session ID is rejected", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		_, err := ensureRemoteSessionDir(context.Background(), server.dial(t), "/remote/task", "")
		if err == nil || !strings.Contains(err.Error(), "session ID is empty") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("mkdir failure is wrapped", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("read-only fs") })
		_, err := ensureRemoteSessionDir(context.Background(), server.dial(t), "/remote/task", "sess-7")
		if err == nil || !strings.Contains(err.Error(), "mkdir session dir") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestEnsureReuseRequiredRemoteTaskDirExists(t *testing.T) {
	t.Run("existing canonical directory is accepted without creating it", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "test -d", result: sshOK},
		).handle)
		if err := ensureReuseRequiredRemoteTaskDirExists(context.Background(), server.dial(t), "/remote/task"); err != nil {
			t.Fatalf("ensureReuseRequiredRemoteTaskDirExists: %v", err)
		}
		if got := server.commands(); len(got) != 1 || got[0] != "test -d '/remote/task'" {
			t.Fatalf("commands = %v, want only the non-mutating directory probe", got)
		}
	})

	t.Run("missing canonical directory fails closed", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "test -d", result: sshFail("missing")},
		).handle)
		err := ensureReuseRequiredRemoteTaskDirExists(context.Background(), server.dial(t), "/remote/task")
		if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
			t.Fatalf("error = %v, want workspace reuse unsafe", err)
		}
		for _, command := range server.commands() {
			if strings.Contains(command, "mkdir -p") {
				t.Fatalf("reuse probe created remote state: %q", command)
			}
		}
	})
}

func TestProbeRemoteBinary(t *testing.T) {
	t.Run("resolves through a login shell and trims output", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "command -v", result: sshOut("/usr/bin/npx\n")},
		).handle)
		got, err := ProbeRemoteBinary(context.Background(), server.dial(t), "zsh", "npx")
		if err != nil {
			t.Fatalf("ProbeRemoteBinary: %v", err)
		}
		if got != "/usr/bin/npx" {
			t.Fatalf("resolved path = %q", got)
		}
		want := `zsh -lc 'command -v '\''npx'\'' 2>/dev/null || true'`
		if cmds := server.commands(); len(cmds) != 1 || cmds[0] != want {
			t.Fatalf("probe command = %v, want %q", cmds, want)
		}
	})

	t.Run("missing binary is an empty string, not an error", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("\n") })
		got, err := ProbeRemoteBinary(context.Background(), server.dial(t), "", "npx")
		if err != nil {
			t.Fatalf("ProbeRemoteBinary: %v", err)
		}
		if got != "" {
			t.Fatalf("resolved path = %q, want empty", got)
		}
	})

	t.Run("transport failure is an error", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("closed") })
		if _, err := ProbeRemoteBinary(context.Background(), server.dial(t), "bash", "npx"); err == nil {
			t.Fatal("expected transport error")
		}
	})
}

func TestIsRemoteAgentctlAlive(t *testing.T) {
	t.Run("live pid", func(t *testing.T) {
		server := newFakeSSHServer(t, newSSHScriptedHandler(t,
			sshScriptRule{match: "kill -0 4242", result: sshOK},
		).handle)
		if !isRemoteAgentctlAlive(context.Background(), server.dial(t), 4242) {
			t.Fatal("expected pid to be reported alive")
		}
	})

	t.Run("dead pid", func(t *testing.T) {
		server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshFail("no such process") })
		if isRemoteAgentctlAlive(context.Background(), server.dial(t), 4242) {
			t.Fatal("failed kill -0 must report not alive")
		}
	})

	t.Run("non-positive pid short-circuits", func(t *testing.T) {
		server := newFakeSSHServer(t, nil)
		client := server.dial(t)
		if isRemoteAgentctlAlive(context.Background(), client, 0) ||
			isRemoteAgentctlAlive(context.Background(), client, -1) {
			t.Fatal("non-positive pids must report not alive")
		}
		if cmds := server.commands(); len(cmds) != 0 {
			t.Fatalf("no remote command expected, got %v", cmds)
		}
	})
}

func TestStopRemoteAgentctlSendsTheStopScript(t *testing.T) {
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "rm -rf", result: sshOK},
	).handle)
	if err := stopRemoteAgentctl(context.Background(), server.dial(t), "/remote/session", 4242); err != nil {
		t.Fatalf("stopRemoteAgentctl: %v", err)
	}
	cmds := server.commands()
	if len(cmds) != 1 || cmds[0] != remoteAgentctlStopCommand("/remote/session", 4242) {
		t.Fatalf("stop command = %v", cmds)
	}
}

func TestRemoteAgentctlStopCommandWithoutPidOnlyRemovesSessionDir(t *testing.T) {
	got := remoteAgentctlStopCommand("/remote/session", 0)
	if got != "rm -rf '/remote/session'" {
		t.Fatalf("stop command = %q, want session-dir removal only", got)
	}
	if strings.Contains(got, "kill") {
		t.Fatalf("no pid must mean no kill: %q", got)
	}
}

func TestStartRemoteAgentctlOnPortBuildsLaunchScript(t *testing.T) {
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch {
		case strings.Contains(command, "nohup"):
			return sshOut("4242\n")
		case strings.Contains(command, "cat "):
			return sshOut("... HTTP server bound successfully on 127.0.0.1:45000\n")
		default:
			return sshOK
		}
	})

	pid, err := startRemoteAgentctlOnPort(context.Background(), server.dial(t),
		"zsh", "/home/kandev/.kandev/bin/agentctl", "/remote/task", "/remote/session",
		"GITHUB_TOKEN='tok'\n", 45000, newTestLogger())
	if err != nil {
		t.Fatalf("startRemoteAgentctlOnPort: %v", err)
	}
	if pid != 4242 {
		t.Fatalf("pid = %d, want 4242", pid)
	}

	launch, ok := server.lastCommandContaining("nohup")
	if !ok {
		t.Fatalf("no launch command recorded: %v", server.commands())
	}
	script := unwrapLoginShell(t, "zsh", launch.Command)
	for _, want := range []string{
		"AGENTCTL_PORT=45000 nohup '/home/kandev/.kandev/bin/agentctl' --workdir '/remote/task'",
		">> '/remote/session'/agentctl.log 2>&1 < /dev/null &",
		`echo "$AGENTCTL_PID" > '/remote/session'/agentctl.pid`,
		"mkdir -p '/remote/session'",
		". /dev/stdin",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("launch script missing %q:\n%s", want, script)
		}
	}
	if launch.Stdin != "GITHUB_TOKEN='tok'\n" {
		t.Fatalf("launch stdin = %q, want the env init script (never argv)", launch.Stdin)
	}
	if strings.Contains(launch.Command, "GITHUB_TOKEN") {
		t.Fatalf("secrets must never reach the remote argv:\n%s", launch.Command)
	}
}

func TestStartRemoteAgentctlOnPortDetectsBindCollision(t *testing.T) {
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		if strings.Contains(command, "nohup") {
			return sshOut("4242\n")
		}
		return sshOut("HTTP server failed to bind: address already in use\n")
	})

	_, err := startRemoteAgentctlOnPort(context.Background(), server.dial(t),
		"bash", "/bin/agentctl", "/remote/task", "/remote/session", "", 45000, newTestLogger())
	if !errors.Is(err, errSSHAgentctlPortInUse) {
		t.Fatalf("error = %v, want errSSHAgentctlPortInUse", err)
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("error must carry the remote log tail: %v", err)
	}
}

func TestStartRemoteAgentctlOnPortFailsWhenProcessDies(t *testing.T) {
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		switch {
		case strings.Contains(command, "nohup"):
			return sshOut("4242\n")
		case strings.Contains(command, "kill -0"):
			return sshFail("no such process")
		default:
			return sshOut("panic: bad config\n")
		}
	})

	_, err := startRemoteAgentctlOnPort(context.Background(), server.dial(t),
		"bash", "/bin/agentctl", "/remote/task", "/remote/session", "", 45000, newTestLogger())
	if err == nil || !strings.Contains(err.Error(), "exited before becoming ready") {
		t.Fatalf("error = %v, want early-exit detection", err)
	}
	if !strings.Contains(err.Error(), "panic: bad config") {
		t.Fatalf("error must include the log tail: %v", err)
	}
}

func TestStartRemoteAgentctlOnPortRejectsNonNumericPid(t *testing.T) {
	server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("not-a-pid\n") })
	_, err := startRemoteAgentctlOnPort(context.Background(), server.dial(t),
		"bash", "/bin/agentctl", "/remote/task", "/remote/session", "", 45000, newTestLogger())
	if err == nil || !strings.Contains(err.Error(), "non-numeric pid") {
		t.Fatalf("error = %v, want non-numeric pid rejection", err)
	}
}

func TestStartRemoteAgentctlOnPortWrapsLaunchFailure(t *testing.T) {
	server := newFakeSSHServer(t, func(string, string) sshExecResult {
		return sshExecResult{Stderr: "zsh: command not found", ExitCode: 127}
	})
	_, err := startRemoteAgentctlOnPort(context.Background(), server.dial(t),
		"zsh", "/bin/agentctl", "/remote/task", "/remote/session", "", 45000, newTestLogger())
	if err == nil || !strings.Contains(err.Error(), "ssh: launch agentctl") {
		t.Fatalf("error = %v, want wrapped launch failure", err)
	}
	if !strings.Contains(err.Error(), "zsh: command not found") {
		t.Fatalf("error must carry remote stderr: %v", err)
	}
}

func TestPickRemoteAgentctlPortStaysInRange(t *testing.T) {
	for range 200 {
		port := pickRemoteAgentctlPort()
		if port < 40000 || port >= 60000 {
			t.Fatalf("pickRemoteAgentctlPort() = %d, want [40000, 60000)", port)
		}
	}
}
