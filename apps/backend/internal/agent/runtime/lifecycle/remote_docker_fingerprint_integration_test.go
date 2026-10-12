package lifecycle

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// writeRemoteDockerTestIdentity generates a client key, writes it where
// SSHIdentitySourceFile can read it, and returns the path plus the authorized
// public key line for the sshd container.
func writeRemoteDockerTestIdentity(t *testing.T) (identityFile, authorizedKey string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	identityFile = filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(identityFile, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write identity: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	return identityFile, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
}

// TestRemoteDockerFingerprintPinningIsEnforced covers
// AC-EXECUTORS-REMOTE-DOCKER-001.5 and the mismatch cause named by
// AC-EXECUTORS-REMOTE-DOCKER-001.14, against real sshd hosts.
//
// This is the executor's security boundary: a saved profile reaches a daemon
// with effective root, so a host answering with a different key must be
// refused outright rather than silently re-pinned. Unit tests can cover the
// comparison; only a second real host proves the refusal happens on the wire.
//
// Opt-in, because it needs a Docker daemon and a prebuilt image.
func TestRemoteDockerFingerprintPinningIsEnforced(t *testing.T) {
	if os.Getenv("KANDEV_TEST_REMOTE_DOCKER") != "1" {
		t.Skip("set KANDEV_TEST_REMOTE_DOCKER=1 and build " + remoteDockerIntegrationImage)
	}

	identityFile, authorizedKey := writeRemoteDockerTestIdentity(t)

	// Two hosts, each with its own sshd host key. The first stands in for the
	// host trusted at save time; the second for that address later answering
	// with a different key.
	firstPort, stopFirst := startRemoteDockerTestHost(t, authorizedKey)
	t.Cleanup(stopFirst)
	secondPort, stopSecond := startRemoteDockerTestHost(t, authorizedKey)
	t.Cleanup(stopSecond)

	target := func(port int, pinned string) *SSHTarget {
		return &SSHTarget{
			Host:              "127.0.0.1",
			Port:              port,
			User:              "kandev",
			IdentitySource:    SSHIdentitySourceFile,
			IdentityFile:      identityFile,
			PinnedFingerprint: pinned,
		}
	}

	// An empty pin is test mode: accept the key and report what was seen.
	// This is the value the create flow asks the user to trust.
	firstSeen := observeUntilReady(t, target(firstPort, ""))
	if !strings.HasPrefix(firstSeen.ObservedFingerprint, "SHA256:") {
		t.Fatalf("ObservedFingerprint = %q, want a SHA256 fingerprint", firstSeen.ObservedFingerprint)
	}

	secondSeen := observeUntilReady(t, target(secondPort, ""))
	if secondSeen.ObservedFingerprint == firstSeen.ObservedFingerprint {
		t.Fatal("both hosts presented the same key; the mismatch case cannot be exercised")
	}

	t.Run("the trusted host connects with its pin", func(t *testing.T) {
		client, err := DialSSH(context.Background(), target(firstPort, firstSeen.ObservedFingerprint))
		if err != nil {
			t.Fatalf("DialSSH() error = %v, want nil for the pinned host", err)
		}
		_ = client.Close()
	})

	t.Run("a changed host key is refused", func(t *testing.T) {
		client, err := DialSSH(context.Background(), target(secondPort, firstSeen.ObservedFingerprint))
		if err == nil {
			_ = client.Close()
			t.Fatal("DialSSH() error = nil; a changed host key must never be accepted")
		}
		// "The host key changed" and "the host is unreachable" call for
		// different actions, so the cause has to be legible in the message.
		lowered := strings.ToLower(err.Error())
		if !strings.Contains(lowered, "fingerprint") && !strings.Contains(lowered, "host key") {
			t.Errorf("error = %v, want it to name the host-key mismatch", err)
		}
		t.Logf("refused as expected: %v", err)
	})

	t.Run("refusal was the pin, not an unreachable host", func(t *testing.T) {
		client, err := DialSSH(context.Background(), target(secondPort, secondSeen.ObservedFingerprint))
		if err != nil {
			t.Fatalf("DialSSH() error = %v, want nil once the second host's own key is pinned", err)
		}
		_ = client.Close()
	})
}

// observeUntilReady dials in test mode until the container's sshd accepts the
// connection, then returns the target carrying the observed fingerprint.
// DialSSH does not retry, and a freshly started container refuses connections
// for a moment; without this the test fails on startup timing rather than on
// the behavior it covers.
func observeUntilReady(t *testing.T, target *SSHTarget) *SSHTarget {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		client, err := DialSSH(context.Background(), target)
		if err == nil {
			_ = client.Close()
			return target
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("sshd on port %d never accepted a connection: %v", target.Port, lastErr)
	return nil
}
