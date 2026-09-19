package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoteDockerSeedsIntoTheMountedDir is the whole point of remote seeding:
// the agent's login files must land in the per-instance directory the
// container mounts, not in the remote user's home.
//
// The SSH executor writes into the remote home because its agent runs there as
// a plain process. A remote Docker agent runs inside a container and only sees
// the mounted directory, so the same destination would leave it logged out.
func TestRemoteDockerSeedsIntoTheMountedDir(t *testing.T) {
	localHome := t.TempDir()
	t.Setenv("HOME", localHome)
	localCred := filepath.Join(localHome, ".credential-agent", "creds.json")
	if err := os.MkdirAll(filepath.Dir(localCred), 0o755); err != nil {
		t.Fatalf("seed local credential dir: %v", err)
	}
	if err := os.WriteFile(localCred, []byte(`{"token":"local"}`), 0o600); err != nil {
		t.Fatalf("seed local credential file: %v", err)
	}

	remoteHome := t.TempDir()
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		if strings.Contains(command, remoteHomeCommand) {
			return sshOut(remoteHome)
		}
		return sshOK
	})
	defer server.Close()
	server.enableSFTP()

	client := server.dial(t)
	defer func() { _ = client.Close() }()

	agent := newCredentialAgent(".credential-agent/creds.json")
	sessionDir := filepath.Join(remoteHome, "agent-sessions", "instance-1")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("create remote session dir: %v", err)
	}

	err := seedRemoteAgentSessionDir(
		context.Background(),
		client,
		agent,
		sessionDir,
		nil,
		newTestLogger(),
		nil,
	)
	if err != nil {
		t.Fatalf("seedRemoteAgentSessionDir: %v", err)
	}

	seeded := filepath.Join(sessionDir, ".credential-agent", "creds.json")
	data, readErr := os.ReadFile(seeded)
	if readErr != nil {
		t.Fatalf("credential not seeded into the mounted dir: %v", readErr)
	}
	if string(data) != `{"token":"local"}` {
		t.Fatalf("seeded credential = %q, want the host copy", data)
	}

	// The remote home is where the SSH executor writes. A remote Docker
	// container never sees it, so a copy landing there is the bug this test
	// exists to catch.
	strayPath := filepath.Join(remoteHome, ".credential-agent", "creds.json")
	if _, err := os.Stat(strayPath); err == nil {
		t.Fatal("credential also written to the remote home, which the container does not mount")
	}
}

// TestRemoteDockerSeedRequiresADestination fails closed rather than writing an
// agent's credentials to an unintended path.
func TestRemoteDockerSeedRequiresADestination(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	defer server.Close()
	server.enableSFTP()
	client := server.dial(t)
	defer func() { _ = client.Close() }()

	agent := newCredentialAgent(".credential-agent/creds.json")
	err := seedRemoteAgentSessionDir(
		context.Background(), client, agent, "", nil, newTestLogger(), nil,
	)
	if err == nil {
		t.Fatal("seedRemoteAgentSessionDir with no destination = nil error, want error")
	}
}

// TestRemoteDockerSeedWithoutAgentIsANoop keeps a repository-only launch from
// failing on a step that has nothing to do.
func TestRemoteDockerSeedWithoutAgentIsANoop(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	defer server.Close()
	server.enableSFTP()
	client := server.dial(t)
	defer func() { _ = client.Close() }()

	err := seedRemoteAgentSessionDir(
		context.Background(), client, nil, t.TempDir(), nil, newTestLogger(), nil,
	)
	if err != nil {
		t.Fatalf("seedRemoteAgentSessionDir with no agent = %v, want nil", err)
	}
}
