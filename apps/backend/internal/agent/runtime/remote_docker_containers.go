package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
)

// ErrRemoteContainerMissing reports that the remote daemon answered and does
// not know the container.
//
// This is deliberately distinct from a transport failure. Treating "cannot
// reach the daemon" as "the container is gone" is what let a live remote
// session report itself missing and become unresumable, so the two outcomes
// stay separable by callers.
var ErrRemoteContainerMissing = errors.New("remote docker: container not found on the remote daemon")

// RemoteDockerContainerClient is the daemon surface a status poll or a
// teardown needs.
type RemoteDockerContainerClient interface {
	GetContainerInfo(ctx context.Context, containerID string) (*docker.ContainerInfo, error)
	RemoveContainer(ctx context.Context, containerID string, force bool) error
}

// RemoteDockerContainers inspects and removes containers on the daemon named
// by a remote Docker executor's stored connection.
//
// A task container lives on the executor's own host, so the install-wide
// daemon cannot answer for it. This mirrors RemoteDockerBuilder rather than
// adding a second connection shape: the same resolved target, the same pinned
// fingerprint requirement, the same per-operation connection.
type RemoteDockerContainers struct {
	// Connect opens the SSH connection for a resolved target. Indirected so
	// the resolution and failure paths can be tested without a host.
	Connect func(context.Context, *lifecycle.SSHTarget) (RemoteDockerContainerClient, func(), error)
}

// NewRemoteDockerContainers wires the production connection.
func NewRemoteDockerContainers(log *logger.Logger) *RemoteDockerContainers {
	return &RemoteDockerContainers{
		Connect: func(ctx context.Context, target *lifecycle.SSHTarget) (RemoteDockerContainerClient, func(), error) {
			sshClient, err := lifecycle.DialSSH(ctx, target)
			if err != nil {
				return nil, nil, fmt.Errorf("remote docker: connect to %s: %w", target.Host, err)
			}
			dockerClient, err := docker.NewRemoteClient(lifecycle.NewSSHDockerDialer(sshClient, log), log)
			if err != nil {
				_ = sshClient.Close()
				return nil, nil, fmt.Errorf("remote docker: create client for %s: %w", target.Host, err)
			}
			release := func() {
				_ = dockerClient.Close()
				_ = sshClient.Close()
			}
			return dockerClient, release, nil
		},
	}
}

// Inspect returns the container's state as reported by the executor's own
// daemon. A daemon that cannot be reached returns a transport error; a daemon
// that answers without knowing the container returns ErrRemoteContainerMissing.
func (c *RemoteDockerContainers) Inspect(
	ctx context.Context,
	executorConfig map[string]string,
	containerID string,
) (*docker.ContainerInfo, error) {
	client, release, err := c.connectFor(ctx, executorConfig)
	if err != nil {
		return nil, err
	}
	defer release()

	info, err := client.GetContainerInfo(ctx, containerID)
	if err != nil {
		if isNoSuchContainer(err) {
			return nil, fmt.Errorf("%w: %s", ErrRemoteContainerMissing, containerID)
		}
		return nil, fmt.Errorf("remote docker: inspect %s: %w", containerID, err)
	}
	return info, nil
}

// Remove tears the container down on the executor's own daemon. A container
// the daemon has already forgotten is the desired end state and is not an
// error; an unreachable daemon is, because reporting success there would
// silently orphan a running container on the remote host.
func (c *RemoteDockerContainers) Remove(
	ctx context.Context,
	executorConfig map[string]string,
	containerID string,
) error {
	client, release, err := c.connectFor(ctx, executorConfig)
	if err != nil {
		return err
	}
	defer release()

	if err := client.RemoveContainer(ctx, containerID, true); err != nil {
		if isNoSuchContainer(err) {
			return nil
		}
		return fmt.Errorf("remote docker: remove %s: %w", containerID, err)
	}
	return nil
}

// connectFor resolves the executor's target and opens a connection to it.
// Resolution runs first so an executor with no pinned fingerprint is refused
// before any host is dialed.
func (c *RemoteDockerContainers) connectFor(
	ctx context.Context,
	executorConfig map[string]string,
) (RemoteDockerContainerClient, func(), error) {
	target, err := RemoteDockerTargetFromConfig(executorConfig)
	if err != nil {
		return nil, nil, err
	}
	return c.Connect(ctx, target)
}

// isNoSuchContainer recognizes the daemon's own answer for an unknown
// container. The Docker API reports it as a message rather than a typed error
// once it has crossed the transport.
func isNoSuchContainer(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such container")
}
