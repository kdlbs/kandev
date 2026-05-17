package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/common/logger"
)

const (
	sshDefaultWorkdir       = "~/.kandev"
	sshRemoteAgentctlPath   = "~/.kandev/bin/agentctl"
	sshRemoteAgentctlSha256 = "~/.kandev/bin/agentctl.sha256"
	sshAgentctlReadyTimeout = 30 * time.Second
	sshAgentctlReadyPoll    = 500 * time.Millisecond
	sshSupportedArch        = "x86_64"
)

// SSHRemoteInfo describes the remote host detected during connection.
type SSHRemoteInfo struct {
	UnameAll   string // `uname -a`
	Arch       string // `uname -m`
	GitVer     string // `git --version`
	AgentctlOK bool   // true if the cached agentctl matches the local sha256
}

// SSHProbeRemote is the exported entry point for the test-connection endpoint
// to run a remote probe (uname / arch / git) over an already-dialed *ssh.Client.
func SSHProbeRemote(ctx context.Context, client *ssh.Client) (*SSHRemoteInfo, error) {
	return detectRemoteInfo(ctx, client)
}

// SSHRequireSupportedArch is the exported arm64-not-yet-supported gate.
// Returns nil for the supported arch (x86_64) and a user-facing error otherwise.
func SSHRequireSupportedArch(arch string) error {
	return requireSupportedArch(arch)
}

// SSHCheckAgentctlCached reports whether the remote already has an agentctl
// binary whose sha256 matches the local one. Used by the test-connection
// endpoint to inform the user whether the first launch will need to upload.
//
// Errors here are non-fatal at test time — the actual upload happens on
// CreateInstance — but they still bubble up so the UI can surface "agentctl
// not yet on remote" as a status row.
func SSHCheckAgentctlCached(ctx context.Context, client *ssh.Client, resolver *AgentctlResolver) (bool, error) {
	localSha, _, _, err := localAgentctlSha256(resolver)
	if err != nil {
		return false, err
	}
	remoteShaFile, err := expandRemoteHome(ctx, client, sshRemoteAgentctlSha256)
	if err != nil {
		return false, err
	}
	out, _, err := runSSHCommand(ctx, client, "cat "+shellQuote(remoteShaFile)+" 2>/dev/null")
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(out) == localSha, nil
}

// runSSHCommand executes a single command on the remote and returns its
// stdout, stderr, and any error. It is the workhorse for arch detection,
// remote mkdir, git clone, sha256 checks, and the like.
func runSSHCommand(ctx context.Context, client *ssh.Client, cmd string) (stdout, stderr string, err error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("ssh: new session: %w", err)
	}
	defer func() { _ = session.Close() }()

	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()
	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		return outBuf.String(), errBuf.String(), ctx.Err()
	case err := <-done:
		return outBuf.String(), errBuf.String(), err
	}
}

// detectRemoteInfo runs a tiny probe to learn about the host. amd64-only gate
// happens at the caller — this function only reports.
func detectRemoteInfo(ctx context.Context, client *ssh.Client) (*SSHRemoteInfo, error) {
	info := &SSHRemoteInfo{}
	if out, _, err := runSSHCommand(ctx, client, "uname -a"); err == nil {
		info.UnameAll = strings.TrimSpace(out)
	}
	out, _, err := runSSHCommand(ctx, client, "uname -m")
	if err != nil {
		return nil, fmt.Errorf("ssh: uname -m: %w", err)
	}
	info.Arch = strings.TrimSpace(out)

	if out, _, err := runSSHCommand(ctx, client, "git --version"); err == nil {
		info.GitVer = strings.TrimSpace(out)
	}
	return info, nil
}

// requireSupportedArch returns nil when the remote arch is supported, or a
// user-facing error otherwise. v1 = linux/amd64 only (mirrors Docker today).
func requireSupportedArch(arch string) error {
	if arch == sshSupportedArch {
		return nil
	}
	return fmt.Errorf(
		"unsupported remote architecture %q — SSH executor v1 supports linux/amd64 only (arm64 lands when agentctl-linux-arm64 is added to the build)",
		arch,
	)
}

// expandRemoteHome rewrites a leading ~/ to the home directory reported by the
// remote (`echo $HOME`). The result is an absolute path. Called once per
// connection and cached by the caller.
func expandRemoteHome(ctx context.Context, client *ssh.Client, path string) (string, error) {
	if !strings.HasPrefix(path, "~/") && path != "~" {
		return path, nil
	}
	out, _, err := runSSHCommand(ctx, client, "printf %s \"$HOME\"")
	if err != nil {
		return "", fmt.Errorf("ssh: resolve $HOME: %w", err)
	}
	home := strings.TrimSpace(out)
	if home == "" {
		return "", errors.New("ssh: remote $HOME is empty")
	}
	if path == "~" {
		return home, nil
	}
	return home + "/" + strings.TrimPrefix(path, "~/"), nil
}

// localAgentctlSha256 returns the hex sha256 of the local agentctl binary
// resolved via AgentctlResolver. Used to decide whether to re-upload.
func localAgentctlSha256(resolver *AgentctlResolver) (string, []byte, string, error) {
	path, err := resolver.ResolveLinuxBinary()
	if err != nil {
		return "", nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, "", fmt.Errorf("read agentctl: %w", err)
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), data, path, nil
}

// ensureAgentctlOnHost uploads the agentctl binary if the remote's cached sha256
// differs from the local binary's sha256. Returns the absolute remote path.
func ensureAgentctlOnHost(ctx context.Context, client *ssh.Client, resolver *AgentctlResolver, log *logger.Logger) (string, error) {
	localSha, localData, localPath, err := localAgentctlSha256(resolver)
	if err != nil {
		return "", err
	}

	remoteBin, err := expandRemoteHome(ctx, client, sshRemoteAgentctlPath)
	if err != nil {
		return "", err
	}
	remoteShaFile, err := expandRemoteHome(ctx, client, sshRemoteAgentctlSha256)
	if err != nil {
		return "", err
	}

	// Compare existing remote sha256, if any.
	if out, _, err := runSSHCommand(ctx, client, "cat "+remoteShaFile+" 2>/dev/null"); err == nil {
		if strings.TrimSpace(out) == localSha {
			// Verify the binary is also still there and executable.
			if _, _, terr := runSSHCommand(ctx, client, "test -x "+remoteBin); terr == nil {
				log.Debug("agentctl already up-to-date on remote", zap.String("sha256", localSha))
				return remoteBin, nil
			}
		}
	}

	log.Info("uploading agentctl to remote",
		zap.String("local_path", localPath),
		zap.String("remote_path", remoteBin),
		zap.String("sha256", localSha),
		zap.Int("bytes", len(localData)))

	if _, _, err := runSSHCommand(ctx, client, "mkdir -p "+filepath.Dir(remoteBin)); err != nil {
		return "", fmt.Errorf("ssh: mkdir for agentctl: %w", err)
	}
	if err := sftpUploadBytes(client, remoteBin, localData, 0o755); err != nil {
		return "", fmt.Errorf("ssh: upload agentctl: %w", err)
	}
	if err := sftpUploadBytes(client, remoteShaFile, []byte(localSha+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("ssh: upload agentctl sha256: %w", err)
	}
	// Sanity check.
	if _, _, err := runSSHCommand(ctx, client, "test -x "+remoteBin); err != nil {
		return "", fmt.Errorf("ssh: agentctl not executable after upload: %w", err)
	}
	return remoteBin, nil
}

// sftpUploadBytes writes data to remotePath via SFTP with the given mode.
// Intermediate directories must already exist.
func sftpUploadBytes(client *ssh.Client, remotePath string, data []byte, mode os.FileMode) error {
	c, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("sftp: new client: %w", err)
	}
	defer func() { _ = c.Close() }()

	// Stream to a temp path then rename, so a half-uploaded file never appears
	// as the final name (matters most for the agentctl binary).
	tmp := remotePath + ".tmp"
	f, err := c.Create(tmp)
	if err != nil {
		return fmt.Errorf("sftp: create %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = c.Remove(tmp)
		return fmt.Errorf("sftp: write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = c.Remove(tmp)
		return fmt.Errorf("sftp: close %s: %w", tmp, err)
	}
	if err := c.Chmod(tmp, mode); err != nil {
		return fmt.Errorf("sftp: chmod %s: %w", tmp, err)
	}
	if err := c.PosixRename(tmp, remotePath); err != nil {
		// Some servers don't support POSIX rename; fall back to a non-atomic rename.
		if rerr := c.Rename(tmp, remotePath); rerr != nil {
			return fmt.Errorf("sftp: rename %s -> %s: %w", tmp, remotePath, rerr)
		}
	}
	return nil
}

// sftpWriteFile is the FileUploader.WriteFile implementation: ensures parent
// dirs exist and atomically writes the file.
func sftpWriteFile(client *ssh.Client, remotePath string, data []byte, mode os.FileMode) error {
	if dir := filepath.Dir(remotePath); dir != "" && dir != "." {
		if _, _, err := runSSHCommand(context.Background(), client, "mkdir -p "+shellQuote(dir)); err != nil {
			return fmt.Errorf("ssh: mkdir %s: %w", dir, err)
		}
	}
	return sftpUploadBytes(client, remotePath, data, mode)
}

// shellQuote is a minimal POSIX shell-safe single-quote wrapper.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ensureRemoteTaskDir creates <workdirRoot>/<taskDirName> if missing and
// returns the absolute remote path. Repo clones happen via the prepare-script
// path (scriptengine), not here; this is just the parent dir.
func ensureRemoteTaskDir(ctx context.Context, client *ssh.Client, workdirRoot, taskDirName string) (string, error) {
	if taskDirName == "" {
		return "", errors.New("ssh: task dir name is empty")
	}
	root, err := expandRemoteHome(ctx, client, workdirRoot)
	if err != nil {
		return "", err
	}
	taskDir := root + "/tasks/" + taskDirName
	if _, _, err := runSSHCommand(ctx, client, "mkdir -p "+shellQuote(taskDir)); err != nil {
		return "", fmt.Errorf("ssh: mkdir task dir %s: %w", taskDir, err)
	}
	return taskDir, nil
}

// ensureRemoteSessionDir creates <taskDir>/.kandev/sessions/<sessionID>/ and
// returns the absolute remote path. Per-session runtime data (PID file, logs,
// agentctl socket) lives here.
func ensureRemoteSessionDir(ctx context.Context, client *ssh.Client, taskDir, sessionID string) (string, error) {
	if sessionID == "" {
		return "", errors.New("ssh: session ID is empty")
	}
	sessionDir := taskDir + "/.kandev/sessions/" + sessionID
	if _, _, err := runSSHCommand(ctx, client, "mkdir -p "+shellQuote(sessionDir)); err != nil {
		return "", fmt.Errorf("ssh: mkdir session dir: %w", err)
	}
	return sessionDir, nil
}

// startRemoteAgentctl launches an agentctl process on the remote bound to
// 127.0.0.1:0 and waits for it to write its chosen port + PID into the session
// runtime dir. Returns the chosen port and the process PID.
//
// The on-remote layout written by agentctl on startup:
//
//	<sessionDir>/agentctl.port
//	<sessionDir>/agentctl.pid
//	<sessionDir>/agentctl.log
func startRemoteAgentctl(
	ctx context.Context,
	client *ssh.Client,
	agentctlBin, workspacePath, sessionDir string,
	log *logger.Logger,
) (port int, pid int, err error) {
	// Build the launch command. We:
	//   - export the workspace
	//   - launch agentctl as a backgrounded, disowned process
	//   - have it bind to a free port; on startup we expect agentctl to either
	//     accept --port-file / --pid-file flags OR we fall back to grabbing a
	//     port ourselves and passing --port. To stay portable we always grab
	//     a port ourselves here using `python3 -c` or `ss -tln` is brittle, so
	//     we use a small inline shell helper.
	//
	// agentctl is invoked with KANDEV_AGENTCTL_PORT honored. We pick a port by
	// asking the kernel to allocate one (via `bash` reading from /dev/tcp is
	// unreliable for binding), so we use a tiny `python3` fallback if available,
	// else awk-on-ss. To keep the dependency surface minimal we instead just
	// rely on agentctl itself printing its bound port to a port-file when
	// invoked with KANDEV_AGENTCTL_PORT=0 — the standalone agentctl already
	// supports binding to :0 and printing the chosen port to stdout.
	cmd := fmt.Sprintf(
		`set -e
		mkdir -p %[1]s
		: > %[1]s/agentctl.log
		nohup %[2]s serve \
		  --workspace %[3]s \
		  --port 0 \
		  --port-file %[1]s/agentctl.port \
		  --pid-file %[1]s/agentctl.pid \
		  >> %[1]s/agentctl.log 2>&1 < /dev/null &
		disown $! 2>/dev/null || true
		echo $!
		`,
		shellQuote(sessionDir),
		shellQuote(agentctlBin),
		shellQuote(workspacePath),
	)
	out, stderr, err := runSSHCommand(ctx, client, cmd)
	if err != nil {
		return 0, 0, fmt.Errorf("ssh: launch agentctl: %w (stderr: %s)", err, strings.TrimSpace(stderr))
	}
	_ = strings.TrimSpace(out) // immediate $! is informational; the on-disk pidfile is authoritative

	// Poll for the port file + pid file to appear and be readable.
	deadline := time.Now().Add(sshAgentctlReadyTimeout)
	for time.Now().Before(deadline) {
		portStr, _, perr := runSSHCommand(ctx, client, "cat "+shellQuote(sessionDir+"/agentctl.port")+" 2>/dev/null")
		pidStr, _, _ := runSSHCommand(ctx, client, "cat "+shellQuote(sessionDir+"/agentctl.pid")+" 2>/dev/null")
		portStr = strings.TrimSpace(portStr)
		pidStr = strings.TrimSpace(pidStr)
		if portStr != "" && pidStr != "" && perr == nil {
			port, err = strconv.Atoi(portStr)
			if err != nil {
				return 0, 0, fmt.Errorf("ssh: agentctl wrote invalid port %q", portStr)
			}
			pid, err = strconv.Atoi(pidStr)
			if err != nil {
				return 0, 0, fmt.Errorf("ssh: agentctl wrote invalid pid %q", pidStr)
			}
			log.Info("agentctl started on remote",
				zap.Int("port", port),
				zap.Int("pid", pid),
				zap.String("session_dir", sessionDir))
			return port, pid, nil
		}
		time.Sleep(sshAgentctlReadyPoll)
	}
	tail, _, _ := runSSHCommand(ctx, client, "tail -n 50 "+shellQuote(sessionDir+"/agentctl.log")+" 2>/dev/null")
	return 0, 0, fmt.Errorf("ssh: agentctl did not become ready within %v; log tail:\n%s", sshAgentctlReadyTimeout, tail)
}

// stopRemoteAgentctl best-effort kills a remote agentctl by PID and removes
// the session runtime dir.
func stopRemoteAgentctl(ctx context.Context, client *ssh.Client, sessionDir string, pid int) error {
	if pid > 0 {
		if _, _, err := runSSHCommand(ctx, client, fmt.Sprintf("kill %d 2>/dev/null || true", pid)); err != nil {
			return err
		}
	}
	// Leave the task dir intact (mirrors spec); only wipe session runtime.
	_, _, _ = runSSHCommand(ctx, client, "rm -rf "+shellQuote(sessionDir))
	return nil
}

// isRemoteAgentctlAlive returns true when a kill -0 on the pid succeeds.
func isRemoteAgentctlAlive(ctx context.Context, client *ssh.Client, pid int) bool {
	if pid <= 0 {
		return false
	}
	_, _, err := runSSHCommand(ctx, client, fmt.Sprintf("kill -0 %d", pid))
	return err == nil
}

// SSHPortForwarder fans out incoming local-port connections to a remote port
// over the shared SSH connection using direct-tcpip channels. Each Forwarder
// owns its local listener; closing the Forwarder closes the listener and any
// outstanding channels.
type SSHPortForwarder struct {
	listener   net.Listener
	localPort  int
	remotePort int
	logger     *logger.Logger
	closed     chan struct{}
}

// StartPortForward opens a fresh 127.0.0.1:<random> listener and tunnels each
// accept to the given remote port over client. Caller MUST call Close when the
// session ends, otherwise both the listener and the SSH channels leak.
func StartPortForward(client *ssh.Client, remotePort int, log *logger.Logger) (*SSHPortForwarder, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("ssh: local listen: %w", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	fwd := &SSHPortForwarder{
		listener:   listener,
		localPort:  addr.Port,
		remotePort: remotePort,
		logger:     log,
		closed:     make(chan struct{}),
	}
	go fwd.serve(client)
	return fwd, nil
}

func (f *SSHPortForwarder) serve(client *ssh.Client) {
	for {
		local, err := f.listener.Accept()
		if err != nil {
			select {
			case <-f.closed:
				return
			default:
				f.logger.Debug("ssh forwarder accept failed", zap.Error(err))
				return
			}
		}
		go f.handleLocal(client, local)
	}
}

func (f *SSHPortForwarder) handleLocal(client *ssh.Client, local net.Conn) {
	defer func() { _ = local.Close() }()
	remote, err := client.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(f.remotePort)))
	if err != nil {
		f.logger.Warn("ssh forwarder dial remote failed",
			zap.Int("remote_port", f.remotePort),
			zap.Error(err))
		return
	}
	defer func() { _ = remote.Close() }()
	go func() { _, _ = io.Copy(remote, local) }()
	_, _ = io.Copy(local, remote)
}

// LocalPort returns the local TCP port the forwarder is listening on.
func (f *SSHPortForwarder) LocalPort() int { return f.localPort }

// Close terminates the forwarder. Idempotent.
func (f *SSHPortForwarder) Close() error {
	select {
	case <-f.closed:
		return nil
	default:
		close(f.closed)
	}
	return f.listener.Close()
}

// waitAgentctlHealthy polls http://127.0.0.1:<localPort>/health for up to
// timeout. Used to confirm the forwarded tunnel is wired up after start /
// recovery.
func waitAgentctlHealthy(ctx context.Context, localPort int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)))
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("agentctl on local port %d not reachable", localPort)
}
