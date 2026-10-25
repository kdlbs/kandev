package lifecycle

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/docker"
)

// TestRemoteDockerLeavesNoHostFootprint is the end-to-end proof of
// REQ-EXECUTORS-REMOTE-DOCKER-002 against the same fixture the transport test
// uses: a real sshd host with a real Docker daemon behind it.
//
// It does what stubs cannot. It makes the remote account's home read-only
// before the launch, so any attempt to write there fails rather than passing
// unnoticed, and it inspects the account afterwards for the tree the previous
// design created.
//
// Opt-in, because it needs a Docker daemon and the prebuilt sshd image.
func TestRemoteDockerLeavesNoHostFootprint(t *testing.T) {
	if os.Getenv("KANDEV_TEST_REMOTE_DOCKER") != "1" {
		t.Skip("set KANDEV_TEST_REMOTE_DOCKER=1 and build " + remoteDockerIntegrationImage)
	}

	signer, authorizedKey := generateRemoteDockerTestKey(t)
	port, stop := startRemoteDockerTestHost(t, authorizedKey)
	t.Cleanup(stop)

	sshClient, err := dialTestSSH(port, signer, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sshClient.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	log := newTestLogger()
	dockerClient, err := docker.NewRemoteClient(NewSSHDockerDialer(sshClient, log), log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dockerClient.Close() })
	require.NoError(t, dockerClient.Ping(ctx))
	require.NoError(t, dockerClient.PullImage(ctx, archiveIntegrationImage))

	// A read-only home turns "the runtime still writes to the host" from a
	// silent leftover into a launch failure.
	remoteHome := remoteExec(t, ctx, sshClient, `printf %s "$HOME"`)
	remoteExec(t, ctx, sshClient, "chmod a-w "+shellQuote(remoteHome))
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _, _ = runSSHCommand(cleanupCtx, sshClient, "chmod u+w "+shellQuote(remoteHome))
	})

	// Prove the guard is live. Without this the test passes just as happily
	// against a home that is still writable, which is the regression it
	// exists to catch.
	_, _, writeErr := runSSHCommand(ctx, sshClient, "mkdir "+shellQuote(remoteHome+"/.kandev"))
	require.Error(t, writeErr, "the remote home is still writable, so this test proves nothing")

	inputs := newRemoteContainerInputs(dockerClient,
		SSHRemotePlatform{GOOS: "linux", GOARCH: "amd64"}, NewCommandBuilder(), log)
	inputs.resolveAgentctl = func(SSHRemotePlatform) ([]byte, error) {
		return []byte("#!/bin/sh\necho HELPER_RAN\n"), nil
	}
	inputs.resolveMockAgentBinary = func() (string, error) { return "", nil }

	sessionTarget := "/root/.agent-session"
	containerID, err := dockerClient.CreateContainer(ctx, docker.ContainerConfig{
		Name:       fmt.Sprintf("kandev-footprint-probe-%d", time.Now().UnixNano()),
		Image:      archiveIntegrationImage,
		Entrypoint: []string{"/bin/sh", "-c"},
		Cmd:        []string{remoteAgentctlExecutablePath + "; ls -d " + sessionTarget},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = dockerClient.RemoveContainer(context.Background(), containerID, true) })

	// Deliver the same way a launch does: into the created container, before
	// it is started.
	up := newTarFileUploader()
	require.NoError(t, up.WriteFile(ctx, remoteAgentctlExecutablePath,
		[]byte("#!/bin/sh\necho HELPER_RAN\n"), 0o755))
	require.NoError(t, up.EnsureDir(sessionTarget))
	require.NoError(t, up.WriteFile(ctx, sessionTarget+"/creds.json", []byte("seeded"), credentialFileMode))
	require.NoError(t, up.Err())
	require.NoError(t, dockerClient.CopyToContainer(ctx, containerID, "/", up.Archive()))

	require.NoError(t, dockerClient.StartContainer(ctx, containerID))
	code, err := dockerClient.WaitContainer(ctx, containerID)
	require.NoError(t, err)
	require.Zero(t, code, "the container could not use its delivered inputs")

	// The whole requirement, observed on the remote account itself.
	listing := remoteExec(t, ctx, sshClient, "ls -a "+shellQuote(remoteHome))
	require.NotContains(t, listing, ".kandev",
		"the launch created a Kandev tree in the remote home; listing = %q", listing)

	// And the credentials are gone once the container is.
	require.NoError(t, dockerClient.RemoveContainer(ctx, containerID, true))
	_, _, statErr := runSSHCommand(ctx, sshClient, "test -e "+shellQuote(remoteHome+"/.kandev"))
	require.Error(t, statErr, "a Kandev tree survives on the remote host after teardown")
}

// TestRemoteDockerTeardownRemovesAPreExistingSessionDir covers the containers
// provisioned before container-native delivery: their per-instance directory is
// a bind-mount source on the remote host, so the container's removal does not
// take the agent's credentials with it.
//
// Opt-in, because it needs the prebuilt sshd image.
func TestRemoteDockerTeardownRemovesAPreExistingSessionDir(t *testing.T) {
	if os.Getenv("KANDEV_TEST_REMOTE_DOCKER") != "1" {
		t.Skip("set KANDEV_TEST_REMOTE_DOCKER=1 and build " + remoteDockerIntegrationImage)
	}

	signer, authorizedKey := generateRemoteDockerTestKey(t)
	port, stop := startRemoteDockerTestHost(t, authorizedKey)
	t.Cleanup(stop)

	sshClient, err := dialTestSSH(port, signer, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sshClient.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	remoteHome := remoteExec(t, ctx, sshClient, `printf %s "$HOME"`)
	legacyDir := remoteHome + "/.kandev/agent-sessions/instance-legacy"
	remoteExec(t, ctx, sshClient, "mkdir -p "+shellQuote(legacyDir))
	remoteExec(t, ctx, sshClient, "printf secret > "+shellQuote(legacyDir+"/creds.json"))

	exec := NewRemoteDockerExecutor(newTestLogger())
	exec.sessions["instance-legacy"] = &remoteDockerSession{sshClient: sshClient}

	require.NoError(t, exec.StopInstance(ctx, &ExecutorInstance{
		InstanceID: "instance-legacy",
		TaskID:     "task-1",
		StopReason: StopReasonTaskDeleted,
	}, false))

	_, _, statErr := runSSHCommand(ctx, sshClient, "test -e "+shellQuote(legacyDir))
	require.Error(t, statErr, "the pre-existing session dir and its credentials survived a delete")
}

func remoteExec(t *testing.T, ctx context.Context, client *ssh.Client, command string) string {
	t.Helper()
	out, stderr, err := runSSHCommand(ctx, client, command)
	require.NoErrorf(t, err, "remote command %q failed: %s", command, stderr)
	return strings.TrimSpace(out)
}
