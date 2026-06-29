package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
)

const (
	sshDefaultWorkdir       = "~/.kandev"
	sshRemoteAgentctlPath   = "~/.kandev/bin/agentctl"
	sshRemoteAgentctlSha256 = "~/.kandev/bin/agentctl.sha256"
	sshAgentctlReadyTimeout = 30 * time.Second
	sshAgentctlReadyPoll    = 500 * time.Millisecond

	sshRemoteGOOSLinux   = "linux"
	sshRemoteGOOSDarwin  = "darwin"
	sshRemoteGOARCHAMD64 = "amd64"
	sshRemoteGOARCHARM64 = "arm64"
)

// SSHRemotePlatform is the normalized remote OS/arch tuple used to choose the
// agentctl helper uploaded to SSH hosts.
type SSHRemotePlatform struct {
	GOOS      string
	GOARCH    string
	UnameOS   string
	UnameArch string
}

func (p SSHRemotePlatform) String() string {
	if p.GOOS == "" || p.GOARCH == "" {
		return "unknown"
	}
	return p.GOOS + "/" + p.GOARCH
}

// SSHRemoteInfo describes the remote host detected during connection.
type SSHRemoteInfo struct {
	UnameAll   string // `uname -a`
	OS         string // `uname -s`
	Arch       string // `uname -m`
	Platform   SSHRemotePlatform
	GitVer     string // `git --version`
	AgentctlOK bool   // true if the cached agentctl matches the local sha256
}

// SSHProbeRemote is the exported entry point for the test-connection endpoint
// to run a remote probe (uname / arch / git) over an already-dialed *ssh.Client.
func SSHProbeRemote(ctx context.Context, client *ssh.Client) (*SSHRemoteInfo, error) {
	return detectRemoteInfo(ctx, client)
}

// SSHRequireSupportedRemotePlatform is the exported platform support gate.
func SSHRequireSupportedRemotePlatform(platform SSHRemotePlatform) error {
	return requireSupportedRemotePlatform(platform)
}

// SSHCheckAgentctlCached reports whether the remote already has an agentctl
// binary whose sha256 matches the local one. Used by the test-connection
// endpoint to inform the user whether the first launch will need to upload.
//
// Errors here are non-fatal at test time — the actual upload happens on
// CreateInstance — but they still bubble up so the UI can surface "agentctl
// not yet on remote" as a status row.
func SSHCheckAgentctlCached(ctx context.Context, client *ssh.Client, resolver *AgentctlResolver, platform SSHRemotePlatform) (bool, error) {
	localSha, _, _, err := localAgentctlSha256(resolver, platform)
	if err != nil {
		return false, err
	}
	remoteShaFile, err := expandRemoteHome(ctx, client, sshRemoteAgentctlSha256)
	if err != nil {
		return false, err
	}
	// The `|| true` falls back to empty stdout if the sidecar is missing,
	// so a successful SSH session with no file is the "not cached" path —
	// distinct from a transport-level failure which we still want to bubble
	// up so the test endpoint can show a real error instead of "needs upload".
	out, _, err := runSSHCommand(ctx, client, "cat "+shellQuote(remoteShaFile)+" 2>/dev/null || true")
	if err != nil {
		return false, fmt.Errorf("ssh: read remote agentctl sha256: %w", err)
	}
	return strings.TrimSpace(out) == localSha, nil
}

// runSSHCommand executes a single command on the remote and returns its
// stdout, stderr, and any error. It is the workhorse for platform detection,
// remote mkdir, git clone, sha256 checks, and the like.
func runSSHCommand(ctx context.Context, client *ssh.Client, cmd string) (stdout, stderr string, err error) {
	return runSSHCommandStdin(ctx, client, cmd, nil)
}

// runSSHCommandStdin is like runSSHCommand but feeds stdin to the remote
// process. Used for the auth-setup path, where secret env vars are written
// to stdin (and sourced by the wrapped shell) instead of inlined into the
// command string — that keeps them out of the remote shell's argv and out
// of `ps aux` / `/proc/PID/cmdline` for the brief window the script runs.
func runSSHCommandStdin(ctx context.Context, client *ssh.Client, cmd string, stdin io.Reader) (stdout, stderr string, err error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("ssh: new session: %w", err)
	}
	defer func() { _ = session.Close() }()

	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf
	if stdin != nil {
		session.Stdin = stdin
	}

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

// detectRemoteInfo runs a tiny probe to learn about the host. The support gate
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
	out, _, err = runSSHCommand(ctx, client, "uname -s")
	if err != nil {
		return nil, fmt.Errorf("ssh: uname -s: %w", err)
	}
	info.OS = strings.TrimSpace(out)
	platform, _ := normalizeSSHRemotePlatform(info.OS, info.Arch)
	info.Platform = platform

	if out, _, err := runSSHCommand(ctx, client, "git --version"); err == nil {
		info.GitVer = strings.TrimSpace(out)
	}
	return info, nil
}

func normalizeSSHRemotePlatform(osName, arch string) (SSHRemotePlatform, bool) {
	goos := normalizeSSHRemoteOS(osName)
	goarch := normalizeSSHRemoteArch(arch)
	platform := SSHRemotePlatform{GOOS: goos, GOARCH: goarch, UnameOS: osName, UnameArch: arch}
	if err := requireSupportedRemotePlatform(platform); err != nil {
		return platform, false
	}
	return platform, true
}

func normalizeSSHRemoteOS(osName string) string {
	switch strings.ToLower(strings.TrimSpace(osName)) {
	case "linux":
		return sshRemoteGOOSLinux
	case "darwin":
		return sshRemoteGOOSDarwin
	default:
		return ""
	}
}

func normalizeSSHRemoteArch(arch string) string {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "x86_64", "amd64":
		return sshRemoteGOARCHAMD64
	case "arm64", "aarch64":
		return sshRemoteGOARCHARM64
	default:
		return ""
	}
}

func requireSupportedRemotePlatform(platform SSHRemotePlatform) error {
	switch platform.String() {
	case sshRemoteGOOSLinux + "/" + sshRemoteGOARCHAMD64,
		sshRemoteGOOSDarwin + "/" + sshRemoteGOARCHARM64,
		sshRemoteGOOSDarwin + "/" + sshRemoteGOARCHAMD64:
		return nil
	default:
		reported := platform.String()
		if reported == "unknown" && (platform.UnameOS != "" || platform.UnameArch != "") {
			reported = fmt.Sprintf("%s/%s", platform.UnameOS, platform.UnameArch)
		}
		return fmt.Errorf(
			"unsupported remote platform %q — SSH executor supports linux/amd64, darwin/arm64, and darwin/amd64",
			reported,
		)
	}
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
func localAgentctlSha256(resolver *AgentctlResolver, platform SSHRemotePlatform) (string, []byte, string, error) {
	path, err := resolver.ResolveRemoteBinary(platform)
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
func ensureAgentctlOnHost(ctx context.Context, client *ssh.Client, resolver *AgentctlResolver, platform SSHRemotePlatform, log *logger.Logger) (string, error) {
	localSha, localData, localPath, err := localAgentctlSha256(resolver, platform)
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

	// Compare existing remote sha256, if any. Every path that lands in a
	// shell-interpreted command goes through shellQuote so a remote $HOME
	// (or anything else with metacharacters) can't break the parse — even
	// though the path was supplied by the remote, not the kandev user.
	if out, _, err := runSSHCommand(ctx, client, "cat "+shellQuote(remoteShaFile)+" 2>/dev/null"); err == nil {
		if strings.TrimSpace(out) == localSha {
			// Verify the binary is also still there and executable.
			if _, _, terr := runSSHCommand(ctx, client, "test -x "+shellQuote(remoteBin)); terr == nil {
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

	if _, _, err := runSSHCommand(ctx, client, "mkdir -p "+shellQuote(filepath.Dir(remoteBin))); err != nil {
		return "", fmt.Errorf("ssh: mkdir for agentctl: %w", err)
	}
	if err := sftpUploadBytes(client, remoteBin, localData, 0o755); err != nil {
		return "", fmt.Errorf("ssh: upload agentctl: %w", err)
	}
	if err := sftpUploadBytes(client, remoteShaFile, []byte(localSha+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("ssh: upload agentctl sha256: %w", err)
	}
	// Sanity check.
	if _, _, err := runSSHCommand(ctx, client, "test -x "+shellQuote(remoteBin)); err != nil {
		return "", fmt.Errorf("ssh: agentctl not executable after upload: %w", err)
	}
	return remoteBin, nil
}

// sftpUploadBytes writes data to remotePath via SFTP with the given mode.
// Intermediate directories must already exist.
//
// The temp filename includes a random suffix so two concurrent uploaders
// targeting the same remotePath don't collide: with a shared `.tmp` name
// the second rename would error with "file does not exist" once the first
// uploader's rename consumed the temp file. Each uploader writes to its
// own temp, then races on the rename — last-writer-wins is safe because
// callers always upload identical content (sha256-keyed binary) or
// content that doesn't matter if it loses the race (the sha256 sidecar).
func sftpUploadBytes(client *ssh.Client, remotePath string, data []byte, mode os.FileMode) error {
	c, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("sftp: new client: %w", err)
	}
	defer func() { _ = c.Close() }()

	tmp := fmt.Sprintf("%s.tmp.%d.%d", remotePath, os.Getpid(), mrand.Uint64())
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
		_ = c.Remove(tmp)
		return fmt.Errorf("sftp: chmod %s: %w", tmp, err)
	}
	if err := c.PosixRename(tmp, remotePath); err != nil {
		// Some servers don't support POSIX rename; fall back to a non-atomic rename.
		if rerr := c.Rename(tmp, remotePath); rerr != nil {
			_ = c.Remove(tmp)
			return fmt.Errorf("sftp: rename %s -> %s: %w", tmp, remotePath, rerr)
		}
	}
	return nil
}

// shellQuote is a minimal POSIX shell-safe single-quote wrapper.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// defaultLoginShell is the shell used when the profile didn't pick one
// explicitly. bash is on virtually every Linux distro (including the
// e2e Alpine image, which symlinks /bin/sh → bash via the bash package)
// and is what nvm/asdf/brew assume — so it's the right default for the
// "agent isn't on PATH" diagnosis.
const defaultLoginShell = "bash"

// WrapLoginShell wraps cmd in `${shell} -lc '<cmd>'` so commands run under
// a login shell that has sourced the user's profile (~/.profile,
// ~/.bash_profile, etc.). This is the canonical fix for "I have nvm
// installed but kandev can't find npx" — sshd's default exec channel
// runs a non-interactive non-login shell which doesn't pick up shell-init
// PATH additions.
//
// Empty shell falls back to defaultLoginShell. The inner cmd is
// single-quote-escaped so embedded quotes don't break the wrapper.
func WrapLoginShell(shell, cmd string) string {
	if shell == "" {
		shell = defaultLoginShell
	}
	return shell + " -lc " + shellQuote(cmd)
}

// ProbeRemoteBinary runs `command -v <binary>` over the existing SSH client
// and reports whether the binary resolves on the remote's $PATH. Returns
// the resolved absolute path on success (the `command -v` stdout), or an
// empty string when missing. err is non-nil only when the SSH call itself
// fails — a missing binary is not an error.
//
// shell is the login shell to run the probe under (e.g. "bash", "zsh");
// empty defaults to bash. Running through a login shell is what makes the
// probe pick up nvm/asdf/brew PATH setup — without it, every node-based
// agent would show as "missing" on dev machines.
//
// Exported for the SSH agent-readiness probe in package ssh; callers
// outside lifecycle would otherwise have to copy the shellQuote + run
// dance and risk drifting from the launch-time pre-flight semantics.
func ProbeRemoteBinary(ctx context.Context, client *ssh.Client, shell, binary string) (string, error) {
	probe := "command -v " + shellQuote(binary) + " 2>/dev/null || true"
	out, _, err := runSSHCommand(ctx, client, WrapLoginShell(shell, probe))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
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

// startRemoteAgentctl launches an agentctl process on the remote with a
// kandev-chosen port and waits for the agentctl log to confirm a successful
// bind. Returns the chosen port and the process PID.
//
// On-remote layout written by the launch wrapper:
//
//	<sessionDir>/agentctl.pid   — written by the wrapper script ($!)
//	<sessionDir>/agentctl.log   — agentctl's own stdout+stderr
//
// agentctl honors AGENTCTL_PORT from its environment (default 39429). We pick
// a per-session port from a wide ephemeral range; collisions on the remote are
// vanishingly unlikely and would surface as a clear bind failure that the
// caller can retry.
func startRemoteAgentctl(
	ctx context.Context,
	client *ssh.Client,
	shell, agentctlBin, workspacePath, sessionDir string,
	log *logger.Logger,
) (port int, pid int, err error) {
	port = pickRemoteAgentctlPort()

	// Wrap the agentctl exec in a login shell so the spawned process
	// inherits the user's $PATH (nvm/asdf/brew etc.). Without this, even
	// if `npx` is installed via nvm, agentctl's child processes won't
	// find it because the SSH-exec channel runs a non-interactive non-
	// login shell and `nohup` inherits whatever that shell's PATH was.
	innerScript := fmt.Sprintf(
		`set -e
mkdir -p %[1]s
: > %[1]s/agentctl.log
AGENTCTL_PORT=%[4]d nohup %[2]s --workdir %[3]s \
  >> %[1]s/agentctl.log 2>&1 < /dev/null &
AGENTCTL_PID=$!
disown "$AGENTCTL_PID" 2>/dev/null || true
echo "$AGENTCTL_PID" > %[1]s/agentctl.pid
echo "$AGENTCTL_PID"
`,
		shellQuote(sessionDir),
		shellQuote(agentctlBin),
		shellQuote(workspacePath),
		port,
	)
	out, stderr, err := runSSHCommand(ctx, client, WrapLoginShell(shell, innerScript))
	if err != nil {
		return 0, 0, fmt.Errorf("ssh: launch agentctl: %w (stderr: %s)", err, strings.TrimSpace(stderr))
	}
	pid, err = strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, 0, fmt.Errorf("ssh: agentctl wrapper returned non-numeric pid %q", out)
	}

	// Poll the on-disk log for the "bound successfully" line; until then the
	// process is starting up and a port-forward connect would race the bind.
	deadline := time.Now().Add(sshAgentctlReadyTimeout)
	for time.Now().Before(deadline) {
		logOut, _, _ := runSSHCommand(ctx, client,
			"cat "+shellQuote(sessionDir+"/agentctl.log")+" 2>/dev/null")
		if strings.Contains(logOut, "HTTP server bound successfully") {
			log.Info("agentctl started on remote",
				zap.Int("port", port),
				zap.Int("pid", pid),
				zap.String("session_dir", sessionDir))
			return port, pid, nil
		}
		if strings.Contains(logOut, "HTTP server failed to bind") {
			return 0, 0, fmt.Errorf(
				"ssh: agentctl failed to bind port %d on remote; log:\n%s", port,
				lastLines(logOut, sshAgentctlLogTailLines))
		}
		// Also catch "exited without binding" via pid check — if the wrapper
		// exited before logging, kill -0 fails and we fail fast.
		if !isRemoteAgentctlAlive(ctx, client, pid) {
			return 0, 0, fmt.Errorf(
				"ssh: agentctl exited before becoming ready; log tail:\n%s",
				lastLines(logOut, sshAgentctlLogTailLines))
		}
		time.Sleep(sshAgentctlReadyPoll)
	}
	tail, _, _ := runSSHCommand(ctx, client,
		"tail -n 50 "+shellQuote(sessionDir+"/agentctl.log")+" 2>/dev/null")
	return 0, 0, fmt.Errorf("ssh: agentctl did not become ready within %v; log tail:\n%s",
		sshAgentctlReadyTimeout, tail)
}

const sshAgentctlLogTailLines = 25

// createRemoteAgentInstance creates a per-session agent instance on the
// remote agentctl control server by POSTing to /api/v1/instances over a
// direct-tcpip channel through the existing SSH client — no second port
// forward, and no dependency on remote curl. Returns the per-instance port
// the SSH executor should later forward + dial for ACP / workspace traffic.
// Mirrors what executor_sprites.go does inside its sprite.
func createRemoteAgentInstance(
	ctx context.Context,
	client *ssh.Client,
	controlPort int,
	workspacePath string,
	req *ExecutorCreateRequest,
	log *logger.Logger,
) (int, error) {
	body, err := json.Marshal(agentctl.CreateInstanceRequest{
		ID:            req.InstanceID,
		WorkspacePath: workspacePath,
		SessionID:     req.SessionID,
		TaskID:        req.TaskID,
		Protocol:      req.Protocol,
		AgentType:     sshAgentTypeFromReq(req),
		AutoApprovePermissions: autoApprovePermissionsOverride(
			req.AutoApprovePermissions,
			req.AutoApprovePermissionsOverride,
		),
		McpServers:          req.McpServers,
		McpMode:             req.McpMode,
		RequiresProcessKill: requiresProcessKillFromReq(req),
		BaseBranches:        getMetadataStringMap(req.Metadata, MetadataKeyBaseBranches),
	})
	if err != nil {
		return 0, fmt.Errorf("ssh: marshal create-instance: %w", err)
	}

	// HTTP-over-direct-tcpip: every request dials a fresh SSH channel to the
	// remote control port. Keep-alives are disabled so the channel closes
	// after the response.
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return client.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)))
			},
			DisableKeepAlives: true,
		},
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/instances", controlPort)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("ssh: build create-instance request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("ssh: create-instance dial: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return 0, fmt.Errorf("ssh: read create-instance response: %w", err)
	}
	if httpResp.StatusCode >= http.StatusBadRequest {
		return 0, fmt.Errorf("ssh: create-instance returned %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var resp agentctl.CreateInstanceResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return 0, fmt.Errorf("ssh: parse create-instance response: %w (body: %s)", err, string(respBody))
	}
	if resp.Port == 0 {
		return 0, fmt.Errorf("ssh: create-instance returned port 0 (body: %s)", string(respBody))
	}
	log.Info("created remote agent instance",
		zap.Int("control_port", controlPort),
		zap.Int("instance_port", resp.Port),
		zap.String("instance_id", resp.ID))
	return resp.Port, nil
}

// sshAgentTypeFromReq returns the agent type ID for the create-instance call,
// or empty when the request didn't carry an agent config.
func sshAgentTypeFromReq(req *ExecutorCreateRequest) string {
	if req == nil || req.AgentConfig == nil {
		return ""
	}
	return req.AgentConfig.ID()
}

// pickRemoteAgentctlPort returns a port in [40000, 60000). The kandev backend
// picks per session; agentctl honors AGENTCTL_PORT. Uses math/rand/v2 so two
// concurrent CreateInstance calls don't collide (UnixNano%20000 cycles in
// ~20µs, which is well within the window between back-to-back launches on a
// fast machine). A residual collision still surfaces as a clear bind failure
// and the caller can retry.
func pickRemoteAgentctlPort() int {
	return 40000 + mrand.IntN(20000)
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
	// dialMu serializes client.Dial calls. golang.org/x/crypto/ssh's
	// Client.Dial is documented as safe to call concurrently, but in practice
	// the kandev stream-manager opens its workspace + agent streams in
	// parallel — and the second Dial occasionally returns io.EOF as if the
	// channel-open response never came back. Serializing the opens makes
	// the long-lived WS forward reliable; the throughput cost is negligible
	// because channel-open completes in ~1ms.
	dialMu sync.Mutex
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
			}
			// Distinguish a permanently-broken listener (closed FD,
			// underlying socket dead) from transient per-accept errors
			// like EMFILE / ECONNABORTED that would orphan the
			// forwarder if we returned. net.ErrClosed is the only
			// error the listener emits after Close, and we already
			// matched <-f.closed for the orderly-close path, so
			// anything else here is a genuine "accept failed once"
			// that we want to log and try again.
			if errors.Is(err, net.ErrClosed) {
				f.logger.Debug("ssh forwarder accept on closed listener", zap.Error(err))
				return
			}
			f.logger.Warn("ssh forwarder accept failed; continuing",
				zap.Int("remote_port", f.remotePort),
				zap.Error(err))
			continue
		}
		go f.handleLocal(client, local)
	}
}

func (f *SSHPortForwarder) handleLocal(client *ssh.Client, local net.Conn) {
	f.dialMu.Lock()
	remote, err := client.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(f.remotePort)))
	f.dialMu.Unlock()
	if err != nil {
		_ = local.Close()
		f.logger.Warn("ssh forwarder dial remote failed",
			zap.Int("remote_port", f.remotePort),
			zap.String("local_addr", local.LocalAddr().String()),
			zap.String("remote_addr", local.RemoteAddr().String()),
			zap.String("error_type", fmt.Sprintf("%T", err)),
			zap.Error(err))
		return
	}

	// Bidirectional copy. Each io.Copy reads until its source EOFs (kandev
	// or agentctl sends FIN at the application layer) or errors. We use
	// CloseWrite to propagate the half-close cleanly: when local->remote
	// finishes, we tell agentctl "no more data from us" via the SSH channel's
	// EOF without slamming the whole channel shut — that lets agentctl's WS
	// writer finish flushing its pending frames before naturally tearing
	// down its side. Symmetric for the other direction. The final full Close
	// happens via the deferred handler when both goroutines have returned.
	type halfCloser interface{ CloseWrite() error }
	closeWriteHalf := func(c net.Conn) {
		if hc, ok := c.(halfCloser); ok {
			_ = hc.CloseWrite()
		} else {
			_ = c.Close()
		}
	}
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(remote, local)
		closeWriteHalf(remote)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(local, remote)
		closeWriteHalf(local)
		errc <- err
	}()
	<-errc
	<-errc
	_ = remote.Close()
	_ = local.Close()
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
// recovery. An open TCP socket isn't enough — the local port is owned by the
// SSH forwarder, which accepts then dials direct-tcpip to the remote; a TCP
// connect can succeed before the forwarder actually establishes the channel.
// Probe with a real HTTP request and require a 2xx response so a broken
// channel surfaces here instead of at the first agent operation.
func waitAgentctlHealthy(ctx context.Context, localPort int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/health", localPort)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	defer httpClient.CloseIdleConnections()

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("agentctl health probe: build request: %w", err)
		}
		resp, err := httpClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("agentctl on local port %d not reachable", localPort)
}
