package lifecycle

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/docker"
	commonconfig "github.com/kandev/kandev/internal/common/config"
)

// archiveIntegrationImage is a base image every daemon can pull quickly. The
// test only needs a shell and `ls`.
const archiveIntegrationImage = "alpine:3"

// TestArchiveDeliveryIntoACreatedContainer is the load-bearing proof for the
// whole remote Docker container-input design: the daemon extracts a tar into a
// container that has been created but never started, and the container's
// entrypoint then sees those files.
//
// Stubs cannot establish this. If it were false, a remote launch would have to
// seed through a second, short-lived container instead.
//
// Opt-in, because it needs a Docker daemon.
func TestArchiveDeliveryIntoACreatedContainer(t *testing.T) {
	if os.Getenv("KANDEV_TEST_DOCKER") != "1" {
		t.Skip("set KANDEV_TEST_DOCKER=1 and provide a reachable Docker daemon")
	}

	log := newTestLogger()
	client, err := docker.NewClient(commonconfig.DockerConfig{}, log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	require.NoError(t, client.Ping(ctx))
	require.NoError(t, client.PullImage(ctx, archiveIntegrationImage))

	// The container reports what it can see: the helper's own output, the
	// credential file's mode, and its contents.
	containerID, err := client.CreateContainer(ctx, docker.ContainerConfig{
		Name:       fmt.Sprintf("kandev-archive-probe-%d", time.Now().UnixNano()),
		Image:      archiveIntegrationImage,
		Entrypoint: []string{"/bin/sh", "-c"},
		Cmd: []string{
			remoteAgentctlExecutablePath + "; " +
				"stat -c '%a' /root/.agent/deep/creds.json; " +
				"cat /root/.agent/deep/creds.json",
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.RemoveContainer(context.Background(), containerID, true) })

	info, err := client.GetContainerInfo(ctx, containerID)
	require.NoError(t, err)
	require.Equal(t, "created", info.State, "the archive must land before the container ever runs")

	up := newTarFileUploader()
	require.NoError(t, up.WriteFile(ctx, remoteAgentctlExecutablePath,
		[]byte("#!/bin/sh\necho HELPER_RAN\n"), 0o755))
	require.NoError(t, up.WriteFile(ctx, "/root/.agent/deep/creds.json",
		[]byte("seeded-token"), credentialFileMode))
	require.NoError(t, up.Err())

	require.NoError(t, client.CopyToContainer(ctx, containerID, "/", up.Archive()))
	require.NoError(t, client.StartContainer(ctx, containerID))

	code, err := client.WaitContainer(ctx, containerID)
	require.NoError(t, err)
	require.Zero(t, code, "the seeded helper must run")

	logs := readContainerLogs(t, client, containerID)
	require.Contains(t, logs, "HELPER_RAN", "the delivered helper must be executable")
	require.Contains(t, logs, "600", "the credential file keeps its restrictive mode")
	require.Contains(t, logs, "seeded-token")
}

func readContainerLogs(t *testing.T, client *docker.Client, containerID string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reader, err := client.GetContainerLogs(ctx, containerID, false, "all")
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()

	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}
	return sb.String()
}
