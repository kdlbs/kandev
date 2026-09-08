package lifecycle

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// newTestHomeDir points $HOME at a fresh temp dir so ~/.ssh lookups in this
// package are hermetic instead of reading the developer's real config.
func newTestHomeDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	previousSystemConfig := sshSystemConfigPath
	sshSystemConfigPath = filepath.Join(home, "missing-system-ssh-config")
	t.Cleanup(func() { sshSystemConfigPath = previousSystemConfig })
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatalf("create fake ~/.ssh: %v", err)
	}
	return home
}

func writeSSHConfig(t *testing.T, home, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write ssh config: %v", err)
	}
}

func TestInheritFromSSHConfig(t *testing.T) {
	t.Run("empty fields inherit from the matching Host block", func(t *testing.T) {
		home := newTestHomeDir(t)
		writeSSHConfig(t, home, `Host prod
  HostName prod.internal
  Port 2222
  User deploy
  IdentityFile ~/.ssh/prod_key
  ProxyJump bastion.example
`)
		target, err := ResolveSSHTarget(SSHConnConfig{HostAlias: "prod", PinnedFingerprint: "SHA256:x"})
		if err != nil {
			t.Fatalf("ResolveSSHTarget: %v", err)
		}
		if target.Host != "prod.internal" || target.Port != 2222 || target.User != "deploy" {
			t.Fatalf("target = %+v", target)
		}
		if target.IdentitySource != SSHIdentitySourceFile {
			t.Fatalf("IdentitySource = %q, want file", target.IdentitySource)
		}
		if target.IdentityFile != filepath.Join(home, ".ssh", "prod_key") {
			t.Fatalf("IdentityFile = %q, want the expanded ~ path", target.IdentityFile)
		}
		if target.ProxyJump != "bastion.example" {
			t.Fatalf("ProxyJump = %q", target.ProxyJump)
		}
	})

	t.Run("explicit form values always win", func(t *testing.T) {
		home := newTestHomeDir(t)
		writeSSHConfig(t, home, `Host prod
  HostName prod.internal
  Port 2222
  User deploy
  ProxyJump config-bastion
`)
		target, err := ResolveSSHTarget(SSHConnConfig{
			HostAlias:         "prod",
			Host:              "override.example",
			Port:              2200,
			User:              "override-user",
			IdentitySource:    SSHIdentitySourceAgent,
			ProxyJump:         "form-bastion",
			PinnedFingerprint: "SHA256:x",
		})
		if err != nil {
			t.Fatalf("ResolveSSHTarget: %v", err)
		}
		if target.Host != "override.example" || target.Port != 2200 || target.User != "override-user" {
			t.Fatalf("target = %+v, want the explicit values", target)
		}
		if target.ProxyJump != "form-bastion" || target.IdentitySource != SSHIdentitySourceAgent {
			t.Fatalf("target = %+v", target)
		}
	})

	t.Run("an unreadable config leaves the alias as the hostname", func(t *testing.T) {
		home := newTestHomeDir(t)
		writeSSHConfig(t, home, "this is not { a valid ssh config\n")
		target, err := ResolveSSHTarget(SSHConnConfig{
			HostAlias:         "prod",
			User:              "deploy",
			IdentitySource:    SSHIdentitySourceAgent,
			PinnedFingerprint: "SHA256:x",
		})
		if err != nil {
			t.Fatalf("ResolveSSHTarget: %v", err)
		}
		if target.Host != "prod" {
			t.Fatalf("Host = %q, want the alias fallback", target.Host)
		}
	})

	t.Run("a non-numeric config port is ignored and the default applies", func(t *testing.T) {
		home := newTestHomeDir(t)
		writeSSHConfig(t, home, "Host prod\n  HostName prod.internal\n  Port not-a-port\n")
		target, err := ResolveSSHTarget(SSHConnConfig{
			HostAlias:         "prod",
			User:              "deploy",
			IdentitySource:    SSHIdentitySourceAgent,
			PinnedFingerprint: "SHA256:x",
		})
		if err != nil {
			t.Fatalf("ResolveSSHTarget: %v", err)
		}
		if target.Port != 22 {
			t.Fatalf("Port = %d, want the default 22", target.Port)
		}
	})
}

func TestApplyTargetDefaultsIdentityFallbacks(t *testing.T) {
	t.Run("ssh-agent is preferred when SSH_AUTH_SOCK is set", func(t *testing.T) {
		newTestHomeDir(t)
		t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
		target, err := ResolveSSHTarget(SSHConnConfig{
			Host: "build.example", User: "deploy", PinnedFingerprint: "SHA256:x",
		})
		if err != nil {
			t.Fatalf("ResolveSSHTarget: %v", err)
		}
		if target.IdentitySource != SSHIdentitySourceAgent {
			t.Fatalf("IdentitySource = %q, want agent", target.IdentitySource)
		}
	})

	t.Run("no agent falls back to ~/.ssh/id_ed25519", func(t *testing.T) {
		home := newTestHomeDir(t)
		t.Setenv("SSH_AUTH_SOCK", "")
		target, err := ResolveSSHTarget(SSHConnConfig{
			Host: "build.example", User: "deploy", PinnedFingerprint: "SHA256:x",
		})
		if err != nil {
			t.Fatalf("ResolveSSHTarget: %v", err)
		}
		if target.IdentitySource != SSHIdentitySourceFile {
			t.Fatalf("IdentitySource = %q, want file", target.IdentitySource)
		}
		if target.IdentityFile != filepath.Join(home, ".ssh", "id_ed25519") {
			t.Fatalf("IdentityFile = %q", target.IdentityFile)
		}
	})

	t.Run("user is required when $USER is unset", func(t *testing.T) {
		newTestHomeDir(t)
		t.Setenv("USER", "")
		_, err := ResolveSSHTarget(SSHConnConfig{Host: "build.example", IdentitySource: SSHIdentitySourceAgent})
		if err == nil || !strings.Contains(err.Error(), "user is required") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestBuildAuthMethods(t *testing.T) {
	t.Run("file identity loads the key", func(t *testing.T) {
		path := writeTestIdentityFile(t)
		methods, cleanup, err := buildAuthMethods(&SSHTarget{
			IdentitySource: SSHIdentitySourceFile,
			IdentityFile:   path,
		})
		if err != nil {
			t.Fatalf("buildAuthMethods: %v", err)
		}
		defer cleanup()
		if len(methods) != 1 {
			t.Fatalf("methods = %d, want 1", len(methods))
		}
	})

	t.Run("file identity requires a path", func(t *testing.T) {
		_, cleanup, err := buildAuthMethods(&SSHTarget{IdentitySource: SSHIdentitySourceFile})
		cleanup()
		if err == nil || !strings.Contains(err.Error(), "identity file path is required") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing identity file is reported with its path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "absent_key")
		_, cleanup, err := buildAuthMethods(&SSHTarget{
			IdentitySource: SSHIdentitySourceFile,
			IdentityFile:   missing,
		})
		cleanup()
		if err == nil || !strings.Contains(err.Error(), "failed to read identity file") {
			t.Fatalf("error = %v", err)
		}
		if !strings.Contains(err.Error(), missing) {
			t.Fatalf("error = %v, want the path", err)
		}
	})

	t.Run("an unparseable identity file is reported", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "garbage_key")
		if err := os.WriteFile(path, []byte("not a key"), 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}
		_, cleanup, err := buildAuthMethods(&SSHTarget{
			IdentitySource: SSHIdentitySourceFile,
			IdentityFile:   path,
		})
		cleanup()
		if err == nil || !strings.Contains(err.Error(), "failed to parse identity file") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a passphrase-protected key points the user at ssh-agent", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte("hunter2"))
		if err != nil {
			t.Fatalf("marshal encrypted key: %v", err)
		}
		path := filepath.Join(t.TempDir(), "encrypted_key")
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}
		_, cleanup, err := buildAuthMethods(&SSHTarget{
			IdentitySource: SSHIdentitySourceFile,
			IdentityFile:   path,
		})
		cleanup()
		if err == nil || !strings.Contains(err.Error(), "passphrase-protected") {
			t.Fatalf("error = %v", err)
		}
		if !strings.Contains(err.Error(), "ssh-add") {
			t.Fatalf("error = %v, want the ssh-agent remediation hint", err)
		}
	})

	t.Run("agent identity requires SSH_AUTH_SOCK", func(t *testing.T) {
		t.Setenv("SSH_AUTH_SOCK", "")
		_, cleanup, err := buildAuthMethods(&SSHTarget{IdentitySource: SSHIdentitySourceAgent})
		cleanup()
		if err == nil || !strings.Contains(err.Error(), "SSH_AUTH_SOCK is not set") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("agent identity connects to the socket", func(t *testing.T) {
		sock := filepath.Join(t.TempDir(), "agent.sock")
		listener, err := net.Listen("unix", sock)
		if err != nil {
			t.Skipf("unix sockets unavailable: %v", err)
		}
		t.Cleanup(func() { _ = listener.Close() })
		t.Setenv("SSH_AUTH_SOCK", sock)

		methods, cleanup, err := buildAuthMethods(&SSHTarget{IdentitySource: SSHIdentitySourceAgent})
		if err != nil {
			t.Fatalf("buildAuthMethods: %v", err)
		}
		if len(methods) != 1 {
			t.Fatalf("methods = %d, want 1", len(methods))
		}
		cleanup() // releases the agent socket
	})

	t.Run("an unreachable agent socket is an error", func(t *testing.T) {
		t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "absent.sock"))
		_, cleanup, err := buildAuthMethods(&SSHTarget{IdentitySource: SSHIdentitySourceAgent})
		cleanup()
		if err == nil || !strings.Contains(err.Error(), "failed to connect to ssh-agent") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("an unknown identity source is rejected", func(t *testing.T) {
		_, cleanup, err := buildAuthMethods(&SSHTarget{IdentitySource: "smartcard"})
		cleanup()
		if err == nil || !strings.Contains(err.Error(), `unsupported identity source: "smartcard"`) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestBuildClientConfigHostKeyCallback(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	hostKey := signer.PublicKey()
	fingerprint := SSHFingerprint(hostKey)
	if !strings.HasPrefix(fingerprint, "SHA256:") {
		t.Fatalf("SSHFingerprint = %q, want the OpenSSH SHA256 form", fingerprint)
	}

	identity := writeTestIdentityFile(t)

	t.Run("test mode records the observed fingerprint and accepts", func(t *testing.T) {
		target := &SSHTarget{
			User: "deploy", IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
		}
		cfg, cleanup, err := buildClientConfig(target)
		if err != nil {
			t.Fatalf("buildClientConfig: %v", err)
		}
		defer cleanup()
		if cfg.User != "deploy" {
			t.Fatalf("User = %q", cfg.User)
		}
		if err := cfg.HostKeyCallback("host:22", nil, hostKey); err != nil {
			t.Fatalf("unpinned host key must be accepted: %v", err)
		}
		if target.ObservedFingerprint != fingerprint {
			t.Fatalf("ObservedFingerprint = %q, want %q", target.ObservedFingerprint, fingerprint)
		}
	})

	t.Run("a pinned fingerprint accepts a match", func(t *testing.T) {
		target := &SSHTarget{
			User: "deploy", IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
			PinnedFingerprint: fingerprint,
		}
		cfg, cleanup, err := buildClientConfig(target)
		if err != nil {
			t.Fatalf("buildClientConfig: %v", err)
		}
		defer cleanup()
		if err := cfg.HostKeyCallback("host:22", nil, hostKey); err != nil {
			t.Fatalf("matching host key must be accepted: %v", err)
		}
	})

	t.Run("a pinned fingerprint rejects a mismatch", func(t *testing.T) {
		target := &SSHTarget{
			User: "deploy", IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
			PinnedFingerprint: "SHA256:something-else",
		}
		cfg, cleanup, err := buildClientConfig(target)
		if err != nil {
			t.Fatalf("buildClientConfig: %v", err)
		}
		defer cleanup()
		err = cfg.HostKeyCallback("host:22", nil, hostKey)
		var mismatch *errHostKeyMismatch
		if !errors.As(err, &mismatch) {
			t.Fatalf("error = %v, want errHostKeyMismatch", err)
		}
		if mismatch.Expected != "SHA256:something-else" || mismatch.Got != fingerprint {
			t.Fatalf("mismatch = %+v", mismatch)
		}
		if !strings.Contains(mismatch.Error(), "host key changed") {
			t.Fatalf("message = %q", mismatch.Error())
		}
	})

	t.Run("an auth failure surfaces before any dial", func(t *testing.T) {
		_, cleanup, err := buildClientConfig(&SSHTarget{IdentitySource: "smartcard"})
		cleanup()
		if err == nil {
			t.Fatal("expected the auth-method failure to abort config building")
		}
	})
}

func TestDialSSHConnectsAndEnforcesThePinnedFingerprint(t *testing.T) {
	server := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("hello\n") })
	newTestHomeDir(t)
	identity := writeTestIdentityFile(t)

	t.Run("matching fingerprint connects", func(t *testing.T) {
		target := &SSHTarget{
			Host: "127.0.0.1", Port: server.port(), User: "kandev",
			IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
			PinnedFingerprint: server.hostKeyFingerprint(),
		}
		client, err := DialSSH(context.Background(), target)
		if err != nil {
			t.Fatalf("DialSSH: %v", err)
		}
		defer func() { _ = client.Close() }()

		out, _, err := runSSHCommand(context.Background(), client, "echo hello")
		if err != nil {
			t.Fatalf("run over the dialled client: %v", err)
		}
		if out != "hello\n" {
			t.Fatalf("stdout = %q", out)
		}
		if target.ObservedFingerprint != server.hostKeyFingerprint() {
			t.Fatalf("ObservedFingerprint = %q", target.ObservedFingerprint)
		}
	})

	t.Run("a changed host key aborts the handshake", func(t *testing.T) {
		target := &SSHTarget{
			Host: "127.0.0.1", Port: server.port(), User: "kandev",
			IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
			PinnedFingerprint: "SHA256:not-the-server-key",
		}
		client, err := dialSSH(context.Background(), target)
		if err == nil {
			_ = client.Close()
			t.Fatal("expected the pinned-fingerprint mismatch to abort the dial")
		}
		if !strings.Contains(err.Error(), "host key changed") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("an unreachable host fails at the TCP dial", func(t *testing.T) {
		target := &SSHTarget{
			Host: "127.0.0.1", Port: 1, User: "kandev",
			IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
			PinnedFingerprint: server.hostKeyFingerprint(),
		}
		if _, err := dialSSH(context.Background(), target); err == nil ||
			!strings.Contains(err.Error(), "tcp dial") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a bad identity aborts before dialling", func(t *testing.T) {
		target := &SSHTarget{
			Host: "127.0.0.1", Port: server.port(), User: "kandev",
			IdentitySource: SSHIdentitySourceFile,
			IdentityFile:   filepath.Join(t.TempDir(), "absent"),
		}
		if _, err := dialSSH(context.Background(), target); err == nil ||
			!strings.Contains(err.Error(), "failed to read identity file") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestDialViaJumpTunnelsThroughTheBastion(t *testing.T) {
	newTestHomeDir(t)
	identity := writeTestIdentityFile(t)

	final := newFakeSSHServer(t, func(string, string) sshExecResult { return sshOut("behind-the-bastion\n") })
	bastion := newFakeSSHServer(t, nil)
	// The bastion forwards every direct-tcpip channel to the final host.
	bastion.forwardTo(final.addr())

	target := &SSHTarget{
		Host: "127.0.0.1", Port: final.port(), User: "kandev",
		IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
		PinnedFingerprint: final.hostKeyFingerprint(),
		ProxyJump:         "jump@127.0.0.1:" + strconv.Itoa(bastion.port()),
	}

	client, err := dialSSH(context.Background(), target)
	if err != nil {
		t.Fatalf("dialSSH via jump: %v", err)
	}
	defer func() { _ = client.Close() }()

	out, _, err := runSSHCommand(context.Background(), client, "echo hi")
	if err != nil {
		t.Fatalf("run through the tunnel: %v", err)
	}
	if out != "behind-the-bastion\n" {
		t.Fatalf("stdout = %q, want the final host's output", out)
	}
}

func TestDialViaJumpFailsWhenTheBastionIsUnreachable(t *testing.T) {
	newTestHomeDir(t)
	identity := writeTestIdentityFile(t)
	final := newFakeSSHServer(t, nil)

	target := &SSHTarget{
		Host: "127.0.0.1", Port: final.port(), User: "kandev",
		IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
		PinnedFingerprint: final.hostKeyFingerprint(),
		ProxyJump:         "jump@127.0.0.1:1",
	}
	_, err := dialSSH(context.Background(), target)
	if err == nil || !strings.Contains(err.Error(), "bastion dial") {
		t.Fatalf("error = %v, want a bastion dial failure", err)
	}
}

func TestDialViaJumpFailsWhenTheTunnelTargetIsClosed(t *testing.T) {
	newTestHomeDir(t)
	identity := writeTestIdentityFile(t)
	final := newFakeSSHServer(t, nil)
	bastion := newFakeSSHServer(t, nil) // no forward target configured

	target := &SSHTarget{
		Host: "127.0.0.1", Port: final.port(), User: "kandev",
		IdentitySource: SSHIdentitySourceFile, IdentityFile: identity,
		PinnedFingerprint: final.hostKeyFingerprint(),
		ProxyJump:         "jump@127.0.0.1:" + strconv.Itoa(bastion.port()),
	}
	_, err := dialSSH(context.Background(), target)
	if err == nil || !strings.Contains(err.Error(), "bastion tunnel") {
		t.Fatalf("error = %v, want a tunnel failure", err)
	}
}

func TestResolveBastion(t *testing.T) {
	home := newTestHomeDir(t)
	writeSSHConfig(t, home, `Host jump-alias
  HostName jump.internal
  Port 2022
  User jumper
`)

	t.Run("a literal user@host:port is parsed inline", func(t *testing.T) {
		bastion, err := resolveBastion(&SSHTarget{
			ProxyJump:      "jumper@bastion.example:2222",
			IdentitySource: SSHIdentitySourceFile,
			IdentityFile:   "/keys/id_ed25519",
		})
		if err != nil {
			t.Fatalf("resolveBastion: %v", err)
		}
		if bastion.Host != "bastion.example" || bastion.Port != 2222 || bastion.User != "jumper" {
			t.Fatalf("bastion = %+v", bastion)
		}
		if bastion.IdentityFile != "/keys/id_ed25519" {
			t.Fatalf("bastion identity = %q, want it inherited from the target", bastion.IdentityFile)
		}
	})

	t.Run("an alias is resolved through ~/.ssh/config", func(t *testing.T) {
		bastion, err := resolveBastion(&SSHTarget{
			ProxyJump:      "jump-alias",
			IdentitySource: SSHIdentitySourceAgent,
		})
		if err != nil {
			t.Fatalf("resolveBastion: %v", err)
		}
		if bastion.Host != "jump.internal" || bastion.Port != 2022 || bastion.User != "jumper" {
			t.Fatalf("bastion = %+v", bastion)
		}
	})

	t.Run("an IPv6 literal keeps its brackets off the hostname", func(t *testing.T) {
		bastion, err := resolveBastion(&SSHTarget{
			ProxyJump:      "jumper@[2001:db8::1]:2222",
			IdentitySource: SSHIdentitySourceAgent,
		})
		if err != nil {
			t.Fatalf("resolveBastion: %v", err)
		}
		if bastion.Host != "2001:db8::1" || bastion.Port != 2222 {
			t.Fatalf("bastion = %+v", bastion)
		}
	})
}

func TestBastionHostKeyCallback(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	otherSigner, err := ssh.NewSignerFromKey(otherPriv)
	if err != nil {
		t.Fatalf("other signer: %v", err)
	}
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}

	t.Run("absent known_hosts accepts on trust-on-first-use", func(t *testing.T) {
		newTestHomeDir(t)
		callback := bastionHostKeyCallback("bastion.example")
		if err := callback("bastion.example:2222", remote, signer.PublicKey()); err != nil {
			t.Fatalf("TOFU callback must accept: %v", err)
		}
	})

	t.Run("an unknown host in known_hosts is accepted", func(t *testing.T) {
		home := newTestHomeDir(t)
		writeKnownHosts(t, home, "other.example", otherSigner.PublicKey())
		callback := bastionHostKeyCallback("bastion.example")
		if err := callback("bastion.example:2222", remote, signer.PublicKey()); err != nil {
			t.Fatalf("unknown host must be accepted: %v", err)
		}
	})

	t.Run("a matching known_hosts entry is accepted", func(t *testing.T) {
		home := newTestHomeDir(t)
		writeKnownHosts(t, home, "[bastion.example]:2222", signer.PublicKey())
		callback := bastionHostKeyCallback("bastion.example")
		if err := callback("bastion.example:2222", remote, signer.PublicKey()); err != nil {
			t.Fatalf("matching key must be accepted: %v", err)
		}
	})

	t.Run("a changed known_hosts key is rejected", func(t *testing.T) {
		home := newTestHomeDir(t)
		writeKnownHosts(t, home, "[bastion.example]:2222", otherSigner.PublicKey())
		callback := bastionHostKeyCallback("bastion.example")
		err := callback("bastion.example:2222", remote, signer.PublicKey())
		if err == nil || !strings.Contains(err.Error(), "host key changed against known_hosts") {
			t.Fatalf("error = %v, want a loud mismatch", err)
		}
		if !strings.Contains(err.Error(), "bastion.example") {
			t.Fatalf("error = %v, want the ProxyJump value for context", err)
		}
	})

	t.Run("an unparseable known_hosts falls back to accepting", func(t *testing.T) {
		home := newTestHomeDir(t)
		if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"),
			[]byte("garbage line without a key\n"), 0o600); err != nil {
			t.Fatalf("write known_hosts: %v", err)
		}
		callback := bastionHostKeyCallback("bastion.example")
		if err := callback("bastion.example:2222", remote, signer.PublicKey()); err != nil {
			t.Fatalf("unparseable known_hosts must degrade to accept: %v", err)
		}
	})

	t.Run("the log-only callback accepts anything", func(t *testing.T) {
		callback := tofuLogOnlyCallback("bastion.example", "")
		if err := callback("bastion.example:2222", remote, signer.PublicKey()); err != nil {
			t.Fatalf("tofuLogOnlyCallback must accept: %v", err)
		}
	})
}

func writeKnownHosts(t *testing.T, home, host string, key ssh.PublicKey) {
	t.Helper()
	line := fmt.Sprintf("%s %s %s\n", host, key.Type(),
		strings.TrimSpace(strings.TrimPrefix(string(ssh.MarshalAuthorizedKey(key)), key.Type()+" ")))
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(line), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
}

func TestIdentityAgentInheritanceAndPrecedence(t *testing.T) {
	home := newTestHomeDir(t)
	writeSSHConfig(t, home, `Host agent-host
  HostName target.internal
  User deploy
  IdentityAgent ~/.agents/%n.sock
Host file-host
  HostName target.internal
  User deploy
  IdentityAgent /tmp/config-agent.sock
`)

	t.Run("explicit agent selection still inherits IdentityAgent", func(t *testing.T) {
		target, err := ResolveSSHTarget(SSHConnConfig{
			HostAlias: "agent-host", IdentitySource: SSHIdentitySourceAgent,
		})
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".agents", "agent-host.sock")
		if target.IdentityAgent != want {
			t.Fatalf("IdentityAgent = %q, want %q", target.IdentityAgent, want)
		}
	})

	t.Run("configured IdentityAgent selects agent without global socket", func(t *testing.T) {
		t.Setenv("SSH_AUTH_SOCK", "")
		target, err := ResolveSSHTarget(SSHConnConfig{HostAlias: "agent-host"})
		if err != nil {
			t.Fatal(err)
		}
		if target.IdentitySource != SSHIdentitySourceAgent {
			t.Fatalf("IdentitySource = %q, want agent", target.IdentitySource)
		}
	})

	t.Run("explicit file source ignores IdentityAgent", func(t *testing.T) {
		key := writeTestIdentityFile(t)
		target, err := ResolveSSHTarget(SSHConnConfig{
			HostAlias: "file-host", IdentitySource: SSHIdentitySourceFile, IdentityFile: key,
		})
		if err != nil {
			t.Fatal(err)
		}
		if target.IdentityAgent != "" || target.IdentityFile != key {
			t.Fatalf("target auth = %+v", target)
		}
	})
}

func TestBuildAuthMethodsIdentityAgentOverridesEnvironment(t *testing.T) {
	dir := t.TempDir()
	configuredPath := filepath.Join(dir, "configured.sock")
	envPath := filepath.Join(dir, "env.sock")
	configured, err := net.Listen("unix", configuredPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = configured.Close() })
	envListener, err := net.Listen("unix", envPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = envListener.Close() })
	t.Setenv("SSH_AUTH_SOCK", envPath)

	_, cleanup, err := buildAuthMethods(&SSHTarget{
		IdentitySource: SSHIdentitySourceAgent,
		IdentityAgent:  configuredPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanup()

	_ = configured.(*net.UnixListener).SetDeadline(time.Now().Add(time.Second))
	conn, err := configured.Accept()
	if err != nil {
		t.Fatalf("configured IdentityAgent was not dialed: %v", err)
	}
	_ = conn.Close()
	_ = envListener.(*net.UnixListener).SetDeadline(time.Now().Add(20 * time.Millisecond))
	if conn, err := envListener.Accept(); err == nil {
		_ = conn.Close()
		t.Fatal("SSH_AUTH_SOCK was dialed instead of IdentityAgent")
	}
}

func TestIdentityAgentNoneDisablesEnvironmentFallback(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/should-not-be-used.sock")
	_, cleanup, err := buildAuthMethods(&SSHTarget{
		IdentitySource: SSHIdentitySourceAgent,
		IdentityAgent:  "none",
	})
	cleanup()
	if err == nil || !strings.Contains(err.Error(), "IdentityAgent is set to none") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveBastionAuthPrecedence(t *testing.T) {
	home := newTestHomeDir(t)
	jumpKey := writeTestIdentityFile(t)
	writeSSHConfig(t, home, fmt.Sprintf(`Host jump-agent
  HostName jump.internal
  User jumper
  IdentityAgent ~/.agents/jump.sock
Host jump-file
  HostName jump.internal
  User jumper
  IdentityFile %s
Host jump-fallback
  HostName jump.internal
  User jumper
`, jumpKey))

	t.Run("jump alias IdentityAgent overrides target socket", func(t *testing.T) {
		bastion, err := resolveBastion(&SSHTarget{ProxyJump: "jump-agent", IdentitySource: SSHIdentitySourceAgent, IdentityAgent: "/target.sock"})
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".agents", "jump.sock")
		if bastion.IdentitySource != SSHIdentitySourceAgent || bastion.IdentityAgent != want {
			t.Fatalf("bastion auth = %+v, want agent %q", bastion, want)
		}
	})

	t.Run("jump alias IdentityFile overrides target agent", func(t *testing.T) {
		bastion, err := resolveBastion(&SSHTarget{ProxyJump: "jump-file", IdentitySource: SSHIdentitySourceAgent, IdentityAgent: "/target.sock"})
		if err != nil {
			t.Fatal(err)
		}
		if bastion.IdentitySource != SSHIdentitySourceFile || bastion.IdentityFile != jumpKey {
			t.Fatalf("bastion auth = %+v", bastion)
		}
	})

	t.Run("alias without auth falls back to target", func(t *testing.T) {
		bastion, err := resolveBastion(&SSHTarget{ProxyJump: "jump-fallback", IdentitySource: SSHIdentitySourceAgent, IdentityAgent: "/target.sock"})
		if err != nil {
			t.Fatal(err)
		}
		if bastion.IdentityAgent != "/target.sock" {
			t.Fatalf("bastion auth = %+v", bastion)
		}
	})

	t.Run("literal jump falls back to target", func(t *testing.T) {
		bastion, err := resolveBastion(&SSHTarget{ProxyJump: "jumper@jump.internal:22", IdentitySource: SSHIdentitySourceAgent, IdentityAgent: "/target.sock"})
		if err != nil {
			t.Fatal(err)
		}
		if bastion.IdentityAgent != "/target.sock" {
			t.Fatalf("bastion auth = %+v", bastion)
		}
	})
}

func TestEffectiveSSHConfigReadsUserAndSystemFresh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	systemPath := filepath.Join(t.TempDir(), "ssh_config")
	previousSystemConfig := sshSystemConfigPath
	sshSystemConfigPath = systemPath
	t.Cleanup(func() { sshSystemConfigPath = previousSystemConfig })
	if err := os.WriteFile(systemPath, []byte("Host system-only\n  HostName from-system.internal\n  User system-user\n  IdentityAgent /system-agent.sock\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeSSHConfig(t, home, "Host shared\n  HostName first-user.internal\n  User user-one\n")

	systemTarget, err := ResolveSSHTarget(SSHConnConfig{HostAlias: "system-only", IdentitySource: SSHIdentitySourceAgent})
	if err != nil {
		t.Fatal(err)
	}
	if systemTarget.Host != "from-system.internal" || systemTarget.User != "system-user" || systemTarget.IdentityAgent != "/system-agent.sock" {
		t.Fatalf("system target = %+v", systemTarget)
	}

	first, err := ResolveSSHTarget(SSHConnConfig{HostAlias: "shared", IdentitySource: SSHIdentitySourceAgent})
	if err != nil {
		t.Fatal(err)
	}
	writeSSHConfig(t, home, "Host shared\n  HostName second-user.internal\n  User user-two\n")
	second, err := ResolveSSHTarget(SSHConnConfig{HostAlias: "shared", IdentitySource: SSHIdentitySourceAgent})
	if err != nil {
		t.Fatal(err)
	}
	if first.Host != "first-user.internal" || second.Host != "second-user.internal" || second.User != "user-two" {
		t.Fatalf("fresh config results = first:%+v second:%+v", first, second)
	}
}
