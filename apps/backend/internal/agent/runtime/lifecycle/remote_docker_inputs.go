package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
)

// mockAgentExecutablePath is the E2E helper's in-container path, alongside the
// agentctl helper.
const mockAgentExecutablePath = "/usr/local/bin/mock-agent"

// containerArchiveWriter extracts a tar archive into a container. It is the
// only daemon capability the input delivery needs, narrow enough to drive
// without a daemon.
type containerArchiveWriter interface {
	CopyToContainer(ctx context.Context, containerID, dstPath string, archive []byte) error
}

// remoteContainerInputs delivers the files a remote container needs but its
// image does not carry: the agentctl helper, the E2E mock-agent helper, the
// per-instance agent session directory, and the credentials and configuration
// bundles seeded into it.
//
// Everything travels through the Docker Engine API into the container that
// consumes it. Nothing is written to the remote host, so the runtime works on a
// host that accepts no filesystem writes and exposes no SFTP subsystem, and the
// container's removal removes the agent's credentials with it.
type remoteContainerInputs struct {
	client         containerArchiveWriter
	platform       SSHRemotePlatform
	commandBuilder *CommandBuilder
	logger         *logger.Logger

	// resolveAgentctl returns the helper bytes matching the remote platform.
	resolveAgentctl func(SSHRemotePlatform) ([]byte, error)
	// resolveMockAgentBinary returns the backend-side E2E mock-agent path, or
	// "" in production.
	resolveMockAgentBinary func() (string, error)
}

func newRemoteContainerInputs(
	client containerArchiveWriter,
	platform SSHRemotePlatform,
	commandBuilder *CommandBuilder,
	log *logger.Logger,
) *remoteContainerInputs {
	resolver := NewAgentctlResolver(log)
	return &remoteContainerInputs{
		client:         client,
		platform:       platform,
		commandBuilder: commandBuilder,
		logger:         log,
		resolveAgentctl: func(p SSHRemotePlatform) ([]byte, error) {
			binaryPath, err := resolver.ResolveRemoteBinary(p)
			if err != nil {
				return nil, err
			}
			return os.ReadFile(binaryPath) //nolint:gosec // resolved from the backend's own build tree
		},
	}
}

// DeliverLaunchInputs delivers every input a fresh container needs, in one
// archive, into a container that has been created but not started.
//
// A helper failure is fatal: without agentctl the container starts and then
// fails with nothing useful named. A credential failure is a warning, matching
// the local Docker path, because some agents authenticate from the environment
// or their in-container setup script instead.
func (r *remoteContainerInputs) DeliverLaunchInputs(
	ctx context.Context, containerID string, config ContainerConfig,
) error {
	up := newTarFileUploader()
	if err := r.addHelpers(up); err != nil {
		return err
	}
	r.addAgentSession(ctx, up, config)
	return r.deliver(ctx, containerID, up)
}

// DeliverHelpers re-delivers only the helper binaries, for a preserved
// container about to be restarted.
//
// A container preserved across a backend upgrade would otherwise resume on the
// helper that was current when it was created. Its session directory is left
// alone: the container already holds what was seeded at launch.
func (r *remoteContainerInputs) DeliverHelpers(ctx context.Context, containerID string) error {
	up := newTarFileUploader()
	if err := r.addHelpers(up); err != nil {
		return err
	}
	return r.deliver(ctx, containerID, up)
}

func (r *remoteContainerInputs) addHelpers(up *tarFileUploader) error {
	if err := SSHRequireSupportedRemotePlatform(r.platform); err != nil {
		return fmt.Errorf("remote docker: %w", err)
	}
	if r.resolveAgentctl == nil {
		return fmt.Errorf("remote docker: no agentctl resolver configured")
	}
	data, err := r.resolveAgentctl(r.platform)
	if err != nil {
		return fmt.Errorf("remote docker: deliver agentctl: %w", err)
	}
	if err := up.WriteFile(context.Background(), remoteAgentctlExecutablePath, data, 0o755); err != nil {
		return fmt.Errorf("remote docker: deliver agentctl: %w", err)
	}

	mockPath, err := r.mockAgentPath()
	if err != nil {
		return err
	}
	if mockPath == "" {
		return nil
	}
	mockData, err := os.ReadFile(mockPath) //nolint:gosec // resolved from the backend's own build tree
	if err != nil {
		return fmt.Errorf("remote docker: read mock-agent: %w", err)
	}
	if err := up.WriteFile(context.Background(), mockAgentExecutablePath, mockData, 0o755); err != nil {
		return fmt.Errorf("remote docker: deliver mock-agent: %w", err)
	}
	return nil
}

func (r *remoteContainerInputs) mockAgentPath() (string, error) {
	if r.resolveMockAgentBinary == nil {
		return "", nil
	}
	mockPath, err := r.resolveMockAgentBinary()
	if err != nil {
		return "", fmt.Errorf("remote docker: mock-agent binary lookup: %w", err)
	}
	return mockPath, nil
}

// addAgentSession creates the agent's session directory in the archive and
// seeds it. Failures are recorded as warnings; the launch continues.
func (r *remoteContainerInputs) addAgentSession(ctx context.Context, up *tarFileUploader, config ContainerConfig) {
	if config.AgentConfig == nil || r.commandBuilder == nil {
		return
	}
	target := r.commandBuilder.GetSessionDirTarget(config.AgentConfig)
	if target == "" {
		// The agent relies on its in-container setup script instead.
		return
	}
	if err := up.EnsureDir(target); err != nil {
		r.logger.Warn("remote docker: could not create the agent session dir (continuing)",
			zap.String("instance_id", config.InstanceID), zap.Error(err))
		return
	}

	home, err := containerHomeForSession(config.AgentConfig, target)
	if err != nil {
		r.logger.Warn("remote docker: could not resolve the container home for agent credentials (continuing)",
			zap.String("instance_id", config.InstanceID), zap.Error(err))
		return
	}
	if err := seedAgentSessionArchive(ctx, up, config.AgentConfig, home,
		selectedPortableConfigBundleIDs(config.Metadata), r.logger,
		func(warnings []PortableConfigWarning) {
			reportPortableConfigWarnings(config.OnProgress, warnings)
		}); err != nil {
		r.logger.Warn("remote docker: failed to seed agent credentials (continuing)",
			zap.String("instance_id", config.InstanceID), zap.Error(err))
	}
}

// containerHomeForSession reverses the agent's {home}-based session template
// against its in-container mount target. Credential and portable-config paths
// are home-relative even when the mounted session directory is a nested
// dot-directory such as /root/.codex.
func containerHomeForSession(ag agents.Agent, sessionDirTarget string) (string, error) {
	if ag == nil || ag.Runtime() == nil {
		return "", fmt.Errorf("remote docker: agent runtime is required")
	}
	template := path.Clean(ag.Runtime().SessionConfig.SessionDirTemplate)
	target := path.Clean(sessionDirTarget)
	if template == "{home}" {
		return target, nil
	}
	const homePrefix = "{home}/"
	if !strings.HasPrefix(template, homePrefix) {
		return "", fmt.Errorf("remote docker: unsupported session dir template %q", template)
	}
	relative := strings.TrimPrefix(template, homePrefix)
	suffix := "/" + relative
	if !strings.HasSuffix(target, suffix) {
		return "", fmt.Errorf("remote docker: session target %q does not match template %q", target, template)
	}
	home := strings.TrimSuffix(target, suffix)
	if home == "" {
		home = "/"
	}
	return home, nil
}

func (r *remoteContainerInputs) deliver(ctx context.Context, containerID string, up *tarFileUploader) error {
	if up.IsEmpty() {
		return nil
	}
	archive := up.Archive()
	if err := up.Err(); err != nil {
		return err
	}
	return r.client.CopyToContainer(ctx, containerID, "/", archive)
}

// seedAgentSessionArchive copies the agent's login files and selected
// configuration bundles into the archive, at the directory the container
// mounts nothing for and sees as its own.
//
// This is the local Docker seeder with one substitution: the writer collects
// tar entries rather than writing a filesystem. Both paths call the same
// UploadCredentialFiles and UploadPortableConfigBundles, so what an agent
// receives does not depend on which daemon runs it.
func seedAgentSessionArchive(
	ctx context.Context,
	up *tarFileUploader,
	ag agents.Agent,
	targetHomeDir string,
	selectedBundleIDs []string,
	log *logger.Logger,
	onWarnings func([]PortableConfigWarning),
) error {
	if ag == nil {
		return nil
	}
	if targetHomeDir == "" {
		return fmt.Errorf("remote docker: target home is required to seed agent credentials")
	}

	var authErr error
	if auth := ag.RemoteAuth(); auth != nil {
		// Source files are read from the backend host, so the host OS selects
		// which of the agent's declared source lists applies.
		methods := authMethodsForHost(auth.Methods, runtime.GOOS)
		if len(methods) > 0 {
			authErr = UploadCredentialFiles(ctx, up, methods, targetHomeDir, log)
		}
	}

	if len(selectedBundleIDs) > 0 {
		warnings := UploadPortableConfigBundles(ctx, up, ag, selectedBundleIDs, targetHomeDir, log)
		if onWarnings != nil {
			onWarnings(warnings)
		}
	}

	return authErr
}
