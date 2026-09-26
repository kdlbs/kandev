package runtime

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
)

// RemoteDockerBuilder builds an image on the daemon named by a remote Docker
// executor's stored connection.
//
// The build has to target that executor's daemon, not the install-wide one:
// the image is what the task container runs from, and it only exists where it
// was built.
type RemoteDockerBuilder struct {
	// Connect opens the SSH connection for a resolved target. Indirected so
	// the resolution and failure paths can be tested without a host.
	Connect func(context.Context, *lifecycle.SSHTarget) (RemoteDockerBuildClient, func(), error)
}

// RemoteDockerBuildClient is the daemon surface a build needs.
type RemoteDockerBuildClient interface {
	BuildImage(ctx context.Context, dockerfile, tag string, buildArgs map[string]*string) (io.ReadCloser, error)
}

// NewRemoteDockerBuilder wires the production connection.
func NewRemoteDockerBuilder(log *logger.Logger) *RemoteDockerBuilder {
	return &RemoteDockerBuilder{
		Connect: func(ctx context.Context, target *lifecycle.SSHTarget) (RemoteDockerBuildClient, func(), error) {
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

// Build streams the image build from the executor's own daemon. The caller
// closes the returned reader, which also releases the connection.
func (b *RemoteDockerBuilder) Build(
	ctx context.Context,
	executorConfig map[string]string,
	dockerfile, tag string,
	buildArgs map[string]*string,
) (io.ReadCloser, error) {
	target, err := RemoteDockerTargetFromConfig(executorConfig)
	if err != nil {
		return nil, err
	}

	client, release, err := b.Connect(ctx, target)
	if err != nil {
		return nil, err
	}

	stream, err := client.BuildImage(ctx, dockerfile, tag, buildArgs)
	if err != nil {
		release()
		return nil, fmt.Errorf("remote docker: build %s: %w", tag, err)
	}
	return &releasingReader{ReadCloser: stream, release: release}, nil
}

// releasingReader ties the connection's lifetime to the build stream, so a
// caller that reads the build log to completion also closes the SSH session.
type releasingReader struct {
	io.ReadCloser
	release func()
}

func (r *releasingReader) Close() error {
	err := r.ReadCloser.Close()
	r.release()
	return err
}

// RemoteDockerTargetFromConfig resolves a saved remote Docker executor's
// connection. It requires the pinned fingerprint: a build runs arbitrary
// Dockerfile instructions with the daemon's authority, so it must not reach an
// unverified host.
func RemoteDockerTargetFromConfig(cfg map[string]string) (*lifecycle.SSHTarget, error) {
	if cfg == nil {
		return nil, fmt.Errorf("remote docker: executor has no config")
	}
	host := strings.TrimSpace(cfg["ssh_host"])
	alias := strings.TrimSpace(cfg["ssh_host_alias"])
	if host == "" && alias == "" {
		return nil, fmt.Errorf("remote docker: executor config has no host")
	}
	for _, candidate := range []string{host, alias} {
		if candidate == "" {
			continue
		}
		if err := docker.ValidateRemoteDaemonAddress(candidate); err != nil {
			return nil, err
		}
	}
	fingerprint := strings.TrimSpace(cfg["ssh_host_fingerprint"])
	if fingerprint == "" {
		return nil, fmt.Errorf("remote docker: executor has no trusted host fingerprint")
	}

	port := 0
	if p := strings.TrimSpace(cfg["ssh_port"]); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("remote docker: invalid ssh_port %q", p)
		}
		port = n
	}

	return lifecycle.ResolveSSHTarget(lifecycle.SSHConnConfig{
		HostAlias:         alias,
		Host:              host,
		Port:              port,
		User:              cfg["ssh_user"],
		IdentitySource:    lifecycle.SSHIdentitySource(cfg["ssh_identity_source"]),
		IdentityFile:      cfg["ssh_identity_file"],
		ProxyJump:         cfg["ssh_proxy_jump"],
		PinnedFingerprint: fingerprint,
	})
}
