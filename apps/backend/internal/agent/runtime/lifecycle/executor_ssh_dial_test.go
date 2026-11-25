package lifecycle

import (
	"context"
	"fmt"
	"golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func newTestHomeDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatalf("create fake ~/.ssh: %v", err)
	}
	return home
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

func writeKnownHosts(t *testing.T, home, host string, key ssh.PublicKey) {
	t.Helper()
	line := fmt.Sprintf("%s %s %s\n", host, key.Type(),
		strings.TrimSpace(strings.TrimPrefix(string(ssh.MarshalAuthorizedKey(key)), key.Type()+" ")))
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte(line), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
}
