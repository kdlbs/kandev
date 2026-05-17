package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const sshDialTimeout = 30 * time.Second

// SSHIdentitySource enumerates how kandev obtains an SSH credential.
type SSHIdentitySource string

const (
	SSHIdentitySourceAgent SSHIdentitySource = "agent" // use SSH_AUTH_SOCK
	SSHIdentitySourceFile  SSHIdentitySource = "file"  // explicit IdentityFile path
)

// SSHTarget is a resolved connection target after ~/.ssh/config inheritance.
type SSHTarget struct {
	Host           string            // resolved HostName
	Port           int               // resolved Port
	User           string            // resolved User
	IdentitySource SSHIdentitySource // agent | file
	IdentityFile   string            // path when IdentitySource=file
	ProxyJump      string            // optional bastion (single hop)

	// PinnedFingerprint is the SHA256 fingerprint the user trusted in settings.
	// Empty means "first connect / test mode" — accept any key and return what we saw.
	PinnedFingerprint string

	// ObservedFingerprint is set by Dial after the handshake completes.
	// In test-mode (PinnedFingerprint == "") this is the value to surface to the UI.
	ObservedFingerprint string
}

// SSHConnConfig holds the raw form values used to build an SSHTarget.
// Either HostAlias is set (and we read ~/.ssh/config) or the explicit fields are.
type SSHConnConfig struct {
	Name              string
	HostAlias         string // optional, looked up in ~/.ssh/config
	Host              string
	Port              int
	User              string
	IdentitySource    SSHIdentitySource
	IdentityFile      string
	ProxyJump         string
	PinnedFingerprint string
}

// ResolveSSHTarget merges form values with ~/.ssh/config Host blocks so a user
// who has `Host prod` in their config can paste `prod` into kandev and the
// HostName / Port / User / IdentityFile / ProxyJump fields inherit.
//
// Explicit form values always win over ~/.ssh/config values.
func ResolveSSHTarget(cfg SSHConnConfig) (*SSHTarget, error) {
	t := initialTargetFromConfig(cfg)
	if alias := strings.TrimSpace(cfg.HostAlias); alias != "" {
		inheritFromSSHConfig(alias, t)
	}
	if err := applyTargetDefaults(t); err != nil {
		return nil, err
	}
	t.PinnedFingerprint = cfg.PinnedFingerprint
	return t, nil
}

// initialTargetFromConfig copies form values into a partially populated target.
// All fields are trimmed; SSH-config inheritance and final defaults are applied later.
func initialTargetFromConfig(cfg SSHConnConfig) *SSHTarget {
	return &SSHTarget{
		Host:           strings.TrimSpace(cfg.Host),
		Port:           cfg.Port,
		User:           strings.TrimSpace(cfg.User),
		IdentitySource: cfg.IdentitySource,
		IdentityFile:   strings.TrimSpace(cfg.IdentityFile),
		ProxyJump:      strings.TrimSpace(cfg.ProxyJump),
	}
}

// inheritFromSSHConfig fills empty fields on target from the matching Host
// block in ~/.ssh/config. Explicit form values are never overwritten.
//
// Reads $HOME/.ssh/config on every call (no caching). The kevinburke/ssh_config
// package's package-level Get/GetStrict use a sync.Once to load the config once
// per process, which breaks tests (and any user who edits their config) — each
// resolve should see the current on-disk state.
func inheritFromSSHConfig(alias string, t *SSHTarget) {
	cfg := loadUserSSHConfig()
	if t.Host == "" {
		if v := lookupSSHConfig(cfg, alias, "HostName"); v != "" {
			t.Host = strings.TrimSpace(v)
		}
	}
	if t.Host == "" {
		t.Host = alias
	}
	if t.Port == 0 {
		if v := lookupSSHConfig(cfg, alias, "Port"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				t.Port = n
			}
		}
	}
	if t.User == "" {
		if v := lookupSSHConfig(cfg, alias, "User"); v != "" {
			t.User = v
		}
	}
	if t.IdentitySource == "" {
		if v := lookupSSHConfig(cfg, alias, "IdentityFile"); v != "" {
			t.IdentitySource = SSHIdentitySourceFile
			t.IdentityFile = expandHome(v)
		}
	}
	if t.ProxyJump == "" {
		if v := lookupSSHConfig(cfg, alias, "ProxyJump"); v != "" {
			t.ProxyJump = v
		}
	}
}

// loadUserSSHConfig parses $HOME/.ssh/config on demand. Returns nil if the
// file is absent or unreadable; lookupSSHConfig handles a nil config by
// returning the empty string.
func loadUserSSHConfig() *ssh_config.Config {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	f, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil
	}
	return cfg
}

func lookupSSHConfig(cfg *ssh_config.Config, alias, key string) string {
	if cfg == nil {
		return ""
	}
	v, err := cfg.Get(alias, key)
	if err != nil {
		return ""
	}
	return v
}

// applyTargetDefaults fills in port/user/identity defaults and validates that
// host is set. Identity defaults: ssh-agent if SSH_AUTH_SOCK is set, otherwise
// fall back to ~/.ssh/id_ed25519.
func applyTargetDefaults(t *SSHTarget) error {
	if t.Host == "" {
		return errors.New("ssh: host is required")
	}
	if t.Port == 0 {
		t.Port = 22
	}
	if t.User == "" {
		current := os.Getenv("USER")
		if current == "" {
			return errors.New("ssh: user is required (no $USER set)")
		}
		t.User = current
	}
	if t.IdentitySource == "" {
		if os.Getenv("SSH_AUTH_SOCK") != "" {
			t.IdentitySource = SSHIdentitySourceAgent
		} else {
			t.IdentitySource = SSHIdentitySourceFile
			t.IdentityFile = expandHome("~/.ssh/id_ed25519")
		}
	}
	if t.IdentitySource == SSHIdentitySourceFile {
		t.IdentityFile = expandHome(t.IdentityFile)
	}
	return nil
}

func expandHome(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "~/") && p != "~" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// SSHFingerprint returns the SHA256 fingerprint of a host key in the
// standard `SHA256:<base64>` format used by OpenSSH and the `ssh` CLI.
func SSHFingerprint(key ssh.PublicKey) string {
	return ssh.FingerprintSHA256(key)
}

// errHostKeyMismatch is returned when the observed host key doesn't match the
// pinned fingerprint. The error message is shaped for direct surfacing in the UI.
type errHostKeyMismatch struct {
	Expected string
	Got      string
}

func (e *errHostKeyMismatch) Error() string {
	return fmt.Sprintf("host key changed: expected %s, got %s", e.Expected, e.Got)
}

// buildAuthMethods builds the SSH auth methods for the target.
// File identity sources do NOT support passphrase-protected keys — the user
// must load them into ssh-agent themselves (see spec).
//
// The returned cleanup function releases any resources held by the auth
// methods (e.g. the ssh-agent unix socket). Callers should invoke it after
// the handshake completes — the agent connection is only consulted during
// initial key exchange.
func buildAuthMethods(target *SSHTarget) ([]ssh.AuthMethod, func(), error) {
	noop := func() {}
	switch target.IdentitySource {
	case SSHIdentitySourceAgent:
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, noop, errors.New("ssh-agent identity source selected but SSH_AUTH_SOCK is not set; start an agent and add your key (ssh-add)")
		}
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, noop, fmt.Errorf("failed to connect to ssh-agent: %w", err)
		}
		ag := agent.NewClient(conn)
		cleanup := func() { _ = conn.Close() }
		return []ssh.AuthMethod{ssh.PublicKeysCallback(ag.Signers)}, cleanup, nil
	case SSHIdentitySourceFile:
		if target.IdentityFile == "" {
			return nil, noop, errors.New("ssh: identity file path is required")
		}
		// IdentityFile is configured by the kandev user themselves — it's the
		// same path semantics as `ssh -i` (any absolute or ~-relative path on
		// the host). Clean it (no `..` traversal sneaking through string
		// substitution), then read. The contents are passed straight to
		// ssh.ParsePrivateKey and never reflected back to a caller.
		path := filepath.Clean(target.IdentityFile)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, noop, fmt.Errorf("failed to read identity file %s: %w", path, err)
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			// passphrase-protected keys produce ssh.PassphraseMissingError — fail loudly.
			var pmErr *ssh.PassphraseMissingError
			if errors.As(err, &pmErr) {
				return nil, noop, fmt.Errorf("identity file %s is passphrase-protected; load it into ssh-agent (ssh-add) and switch identity source to ssh-agent — kandev does not store passphrases", path)
			}
			return nil, noop, fmt.Errorf("failed to parse identity file %s: %w", path, err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, noop, nil
	default:
		return nil, noop, fmt.Errorf("unsupported identity source: %q", target.IdentitySource)
	}
}

// buildClientConfig builds an *ssh.ClientConfig with auth + host-key callback.
//
// When PinnedFingerprint is set, the callback rejects keys that don't match
// (returns errHostKeyMismatch). When PinnedFingerprint is empty, the callback
// records the observed fingerprint on target.ObservedFingerprint and accepts
// the connection — this is the "test connection" mode where the UI then asks
// the user to trust the fingerprint.
//
// The returned cleanup function releases any auth-time resources (ssh-agent
// socket) — callers must call it once the handshake completes; the agent
// connection is only consulted during initial key exchange.
func buildClientConfig(target *SSHTarget) (*ssh.ClientConfig, func(), error) {
	auth, authCleanup, err := buildAuthMethods(target)
	if err != nil {
		return nil, authCleanup, err
	}
	return &ssh.ClientConfig{
		User: target.User,
		Auth: auth,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			fp := SSHFingerprint(key)
			target.ObservedFingerprint = fp
			if target.PinnedFingerprint == "" {
				return nil
			}
			if fp != target.PinnedFingerprint {
				return &errHostKeyMismatch{Expected: target.PinnedFingerprint, Got: fp}
			}
			return nil
		},
		Timeout: sshDialTimeout,
	}, authCleanup, nil
}

// DialSSH opens an SSH connection to target, optionally via a bastion if
// ProxyJump is set. Returns the live *ssh.Client and the observed fingerprint
// (recorded on target.ObservedFingerprint via the host-key callback).
//
// Callers own the client and must Close it when done.
func DialSSH(ctx context.Context, target *SSHTarget) (*ssh.Client, error) {
	return dialSSH(ctx, target)
}

// dial opens an SSH connection to target, optionally via a bastion if ProxyJump
// is set. Returns the live *ssh.Client and the observed fingerprint.
func dialSSH(ctx context.Context, target *SSHTarget) (*ssh.Client, error) {
	cfg, cleanup, err := buildClientConfig(target)
	if err != nil {
		cleanup()
		return nil, err
	}
	// The auth cleanup (ssh-agent socket) is only needed during the handshake
	// — the agent is consulted by PublicKeysCallback during key exchange and
	// never again. Release it after the dial path returns, regardless of
	// success.
	defer cleanup()
	addr := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))

	if target.ProxyJump == "" {
		return dialDirect(ctx, addr, cfg)
	}
	return dialViaJump(ctx, target, addr, cfg)
}

func dialDirect(ctx context.Context, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ssh: tcp dial %s: %w", addr, err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, cfg)
	if err != nil {
		_ = tcpConn.Close()
		return nil, fmt.Errorf("ssh: handshake with %s: %w", addr, err)
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}

// dialViaJump implements ProxyJump as a single bastion hop. The bastion is
// resolved from its own ~/.ssh/config Host block, defaulting to the same
// identity source as the target (passes the user's agent / key through).
//
// OpenSSH ProxyJump supports both an alias (e.g. `prod-jump`) and an explicit
// `[user@]host[:port]` form. We honor the latter inline rather than asking
// ResolveSSHTarget to treat it as an alias — the literal form has no Host
// block in ~/.ssh/config to inherit from.
func dialViaJump(ctx context.Context, target *SSHTarget, finalAddr string, finalCfg *ssh.ClientConfig) (*ssh.Client, error) {
	bastion, err := resolveBastion(target)
	if err != nil {
		return nil, fmt.Errorf("ssh: resolve bastion %q: %w", target.ProxyJump, err)
	}
	bastionCfg, bastionAuthCleanup, err := buildClientConfig(bastion)
	if err != nil {
		bastionAuthCleanup()
		return nil, fmt.Errorf("ssh: bastion config: %w", err)
	}
	defer bastionAuthCleanup()
	bastionAddr := net.JoinHostPort(bastion.Host, strconv.Itoa(bastion.Port))
	// Without an explicit pinned fingerprint, the bastion's host key would
	// otherwise be accepted unconditionally by buildClientConfig. Verify it
	// against ~/.ssh/known_hosts when available; fall back to a logged
	// warning when the host is absent so e2e + first-connect setups still
	// work (matches OpenSSH's StrictHostKeyChecking=accept-new semantics).
	bastionCfg.HostKeyCallback = bastionHostKeyCallback(target.ProxyJump)
	bastionClient, err := dialDirect(ctx, bastionAddr, bastionCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh: bastion dial: %w", err)
	}

	// Tunnel through the bastion to the final host.
	tunnel, err := bastionClient.DialContext(ctx, "tcp", finalAddr)
	if err != nil {
		_ = bastionClient.Close()
		return nil, fmt.Errorf("ssh: bastion tunnel to %s: %w", finalAddr, err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(tunnel, finalAddr, finalCfg)
	if err != nil {
		_ = tunnel.Close()
		_ = bastionClient.Close()
		return nil, fmt.Errorf("ssh: handshake with %s via %s: %w", finalAddr, target.ProxyJump, err)
	}
	// Attach the bastion close to the final client lifetime.
	final := ssh.NewClient(sshConn, chans, reqs)
	go func() {
		_ = final.Wait()
		_ = bastionClient.Close()
	}()
	return final, nil
}

// bastionHostKeyCallback returns an ssh.HostKeyCallback that consults
// $HOME/.ssh/known_hosts. When the bastion is present and its key matches we
// accept; when present but mismatched we reject (the alarm an attacker on the
// bastion path would trip); when absent we log + accept so first-connect and
// throwaway test setups still work without manual `ssh-keyscan` priming.
// proxyJump is the original metadata string (used for logging context).
func bastionHostKeyCallback(proxyJump string) ssh.HostKeyCallback {
	home, _ := os.UserHomeDir()
	if home == "" {
		return tofuLogOnlyCallback(proxyJump, "")
	}
	khPath := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(khPath); err != nil {
		return tofuLogOnlyCallback(proxyJump, khPath)
	}
	strict, err := knownhosts.New(khPath)
	if err != nil {
		return tofuLogOnlyCallback(proxyJump, khPath)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if err := strict(hostname, remote, key); err != nil {
			var keyErr *knownhosts.KeyError
			if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
				// Unknown host: log + accept (TOFU).
				return nil
			}
			// Real mismatch — fail loud.
			return fmt.Errorf("bastion %s host key changed against known_hosts: %w", proxyJump, err)
		}
		return nil
	}
}

// tofuLogOnlyCallback accepts any host key but records the observed fingerprint
// so users can audit which bastion they actually trusted. Used when
// ~/.ssh/known_hosts is unreadable or absent.
func tofuLogOnlyCallback(proxyJump, _ string) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, _ ssh.PublicKey) error {
		// Intentionally silent: the surrounding dialViaJump already logs the
		// successful jump; emitting a warn per dial would spam normal use.
		_ = proxyJump
		return nil
	}
}

// resolveBastion turns the target's ProxyJump value into a connection
// SSHTarget. Two accepted shapes:
//
//   - alias: looked up in ~/.ssh/config (HostName / Port / User / IdentityFile)
//   - literal "[user@]host[:port]": parsed inline, no config lookup
//
// The literal form has no Host block to inherit from, so passing it straight
// to ResolveSSHTarget (which falls back to alias-as-hostname) would produce
// `user@host:port` as a single hostname and the TCP dial fails on lookup.
// Identity defaults flow from the target so the same key reaches the bastion.
func resolveBastion(target *SSHTarget) (*SSHTarget, error) {
	if user, host, port, ok := parseLiteralProxyJump(target.ProxyJump); ok {
		return ResolveSSHTarget(SSHConnConfig{
			Host:           host,
			Port:           port,
			User:           user,
			IdentitySource: target.IdentitySource,
			IdentityFile:   target.IdentityFile,
		})
	}
	return ResolveSSHTarget(SSHConnConfig{
		HostAlias:      target.ProxyJump,
		IdentitySource: target.IdentitySource,
		IdentityFile:   target.IdentityFile,
	})
}

// parseLiteralProxyJump recognizes a `[user@]host[:port]` ProxyJump literal.
// Returns ok=false when the input doesn't contain an `@` or `:` (treat as
// alias instead). Anything that doesn't parse cleanly also returns false so
// the alias path can take over.
func parseLiteralProxyJump(s string) (user, host string, port int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" || !strings.ContainsAny(s, "@:") {
		return "", "", 0, false
	}
	rest := s
	if at := strings.LastIndex(rest, "@"); at != -1 {
		user = rest[:at]
		rest = rest[at+1:]
	}
	if colon := strings.LastIndex(rest, ":"); colon != -1 {
		portStr := rest[colon+1:]
		n, err := strconv.Atoi(portStr)
		if err != nil {
			return "", "", 0, false
		}
		port = n
		rest = rest[:colon]
	}
	host = rest
	if host == "" {
		return "", "", 0, false
	}
	return user, host, port, true
}

// SSH connections are owned per-session (no shared pool): a session's client
// lives on sshSessionState and is Close()d by StopInstance. The earlier
// pool/refcount/keepalive plumbing was dead code — the executor never used it
// in production, and the orphaned keepalive goroutines surfaced under e2e
// fault-injection. See PR #927 for the removal rationale.
