package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
)

// Step names. Each one is a separate thing that can fail and a separate fix.
const (
	stepConnect    = "SSH connection"
	stepPlatform   = "Remote platform"
	stepDaemon     = "Docker daemon"
	stepAPIVersion = "Docker API version"
)

// Hints tell the user what to do about a failure. They are stable identifiers
// the frontend maps to translated copy, not user-facing English.
const (
	hintSocketAccess      = "remote_user_needs_docker_access"
	hintDockerCLIMissing  = "remote_host_needs_docker_cli"
	hintDaemonUnreachable = "remote_daemon_not_running"
)

// TestRequest is the unsaved profile connection under test.
type RemoteDockerTestRequest struct {
	Name         string `json:"name"`
	HostAlias    string `json:"host_alias,omitempty"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	User         string `json:"user,omitempty"`
	IdentitySrc  string `json:"identity_source,omitempty"`
	IdentityFile string `json:"identity_file,omitempty"`
	ProxyJump    string `json:"proxy_jump,omitempty"`
}

// TestStep is one probe result.
type RemoteDockerTestStep struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	// Hint is a stable identifier for the remediation the frontend shows.
	Hint string `json:"hint,omitempty"`
}

// TestResult is the probe response. The UI requires Success and Fingerprint
// before it lets the user trust the host and save the profile.
type RemoteDockerTestResult struct {
	Success          bool                   `json:"success"`
	Fingerprint      string                 `json:"fingerprint,omitempty"`
	Platform         string                 `json:"platform,omitempty"`
	DaemonAPIVersion string                 `json:"daemon_api_version,omitempty"`
	Steps            []RemoteDockerTestStep `json:"steps"`
	TotalDurationMs  int64                  `json:"total_duration_ms"`
	Error            string                 `json:"error,omitempty"`
}

// Prober runs the connection test. Its three operations are fields so the
// step and failure reporting can be tested without a host.
type RemoteDockerProber struct {
	// Dial connects and reports the observed host-key fingerprint. The test
	// records the fingerprint rather than pinning it: the user trusts it
	// explicitly before the profile is saved.
	Dial func(context.Context, *lifecycle.SSHTarget) (*ssh.Client, string, error)
	// Probe reads the remote OS and architecture.
	Probe func(context.Context, *ssh.Client) (*lifecycle.SSHRemoteInfo, error)
	// Daemon reaches the Docker daemon through the connection and returns its
	// API version.
	Daemon func(context.Context, *ssh.Client) (string, error)
}

// Run executes the probe, stopping at the first failure.
//
// Stopping matters: a later step's result is meaningless once an earlier one
// failed, and showing a green daemon under a red connection points the user at
// the wrong problem.
func (p *RemoteDockerProber) Run(ctx context.Context, req RemoteDockerTestRequest) RemoteDockerTestResult {
	start := time.Now()
	result := RemoteDockerTestResult{Steps: []RemoteDockerTestStep{}}

	target, err := resolveTarget(req)
	if err != nil {
		result.Error = err.Error()
		result.TotalDurationMs = time.Since(start).Milliseconds()
		return result
	}

	client, fingerprint, step := p.runConnect(ctx, target)
	result.Steps = append(result.Steps, step)
	result.Fingerprint = fingerprint
	if !step.Success {
		return p.finish(result, start, step.Error)
	}
	defer func() {
		if client != nil {
			_ = client.Close()
		}
	}()

	platform, step := p.runPlatform(ctx, client)
	result.Steps = append(result.Steps, step)
	result.Platform = platform
	if !step.Success {
		return p.finish(result, start, step.Error)
	}

	apiVersion, step := p.runDaemon(ctx, client)
	result.Steps = append(result.Steps, step)
	if !step.Success {
		return p.finish(result, start, step.Error)
	}

	result.DaemonAPIVersion = apiVersion
	result.Steps = append(result.Steps, RemoteDockerTestStep{
		Name:    stepAPIVersion,
		Success: true,
		Output:  apiVersion,
	})
	result.Success = true
	result.TotalDurationMs = time.Since(start).Milliseconds()
	return result
}

func (p *RemoteDockerProber) finish(result RemoteDockerTestResult, start time.Time, errText string) RemoteDockerTestResult {
	result.Error = errText
	result.TotalDurationMs = time.Since(start).Milliseconds()
	return result
}

func (p *RemoteDockerProber) runConnect(ctx context.Context, target *lifecycle.SSHTarget) (*ssh.Client, string, RemoteDockerTestStep) {
	begin := time.Now()
	client, fingerprint, err := p.Dial(ctx, target)
	step := RemoteDockerTestStep{Name: stepConnect, DurationMs: time.Since(begin).Milliseconds()}
	if err != nil {
		step.Error = err.Error()
		return nil, fingerprint, step
	}
	step.Success = true
	step.Output = target.Host
	return client, fingerprint, step
}

func (p *RemoteDockerProber) runPlatform(ctx context.Context, client *ssh.Client) (string, RemoteDockerTestStep) {
	begin := time.Now()
	info, err := p.Probe(ctx, client)
	step := RemoteDockerTestStep{Name: stepPlatform, DurationMs: time.Since(begin).Milliseconds()}
	if err != nil {
		step.Error = err.Error()
		return "", step
	}
	if err := lifecycle.SSHRequireSupportedRemotePlatform(info.Platform); err != nil {
		step.Error = err.Error()
		step.Output = info.Platform.String()
		return info.Platform.String(), step
	}
	step.Success = true
	step.Output = info.Platform.String()
	return info.Platform.String(), step
}

func (p *RemoteDockerProber) runDaemon(ctx context.Context, client *ssh.Client) (string, RemoteDockerTestStep) {
	begin := time.Now()
	apiVersion, err := p.Daemon(ctx, client)
	step := RemoteDockerTestStep{Name: stepDaemon, DurationMs: time.Since(begin).Milliseconds()}
	if err != nil {
		step.Error = err.Error()
		step.Hint = hintFor(err)
		return "", step
	}
	step.Success = true
	return apiVersion, step
}

// hintFor maps a daemon failure to the action that fixes it.
func hintFor(err error) string {
	switch {
	case errors.Is(err, docker.ErrRemoteSocketAccessDenied):
		return hintSocketAccess
	case errors.Is(err, docker.ErrRemoteDockerCLIMissing):
		return hintDockerCLIMissing
	default:
		return hintDaemonUnreachable
	}
}

// NewRemoteDockerProber wires the real dial, platform probe, and daemon check.
func NewRemoteDockerProber(log *logger.Logger) *RemoteDockerProber {
	return &RemoteDockerProber{
		Dial: func(ctx context.Context, target *lifecycle.SSHTarget) (*ssh.Client, string, error) {
			// The target carries no pinned fingerprint, so the host key is
			// observed and reported but not trusted. The user trusts it
			// explicitly before the profile is saved.
			client, err := lifecycle.DialSSH(ctx, target)
			return client, target.ObservedFingerprint, err
		},
		Probe: func(ctx context.Context, client *ssh.Client) (*lifecycle.SSHRemoteInfo, error) {
			probeCtx, cancel := context.WithTimeout(ctx, remoteDockerProbeTimeout)
			defer cancel()
			return lifecycle.SSHProbeRemote(probeCtx, client)
		},
		Daemon: func(ctx context.Context, client *ssh.Client) (string, error) {
			daemonCtx, cancel := context.WithTimeout(ctx, remoteDockerProbeTimeout)
			defer cancel()

			dockerClient, err := docker.NewRemoteClient(lifecycle.NewSSHDockerDialer(client, log), log)
			if err != nil {
				return "", err
			}
			defer func() { _ = dockerClient.Close() }()

			return dockerClient.PingVersion(daemonCtx)
		},
	}
}

// remoteDockerProbeTimeout bounds each remote step so a wedged host fails the
// test rather than hanging the settings page.
const remoteDockerProbeTimeout = 20 * time.Second

// resolveTarget validates the request and resolves it to an SSH target.
func resolveTarget(req RemoteDockerTestRequest) (*lifecycle.SSHTarget, error) {
	host := strings.TrimSpace(req.Host)
	alias := strings.TrimSpace(req.HostAlias)
	if host == "" && alias == "" {
		return nil, errors.New("a host or an OpenSSH alias is required")
	}
	for _, candidate := range []string{host, alias} {
		if candidate == "" {
			continue
		}
		if err := docker.ValidateRemoteDaemonAddress(candidate); err != nil {
			return nil, err
		}
	}
	if req.Port < 0 || req.Port > 65535 {
		return nil, fmt.Errorf("invalid port %d (must be 1-65535)", req.Port)
	}

	return lifecycle.ResolveSSHTarget(lifecycle.SSHConnConfig{
		HostAlias:      alias,
		Host:           host,
		Port:           req.Port,
		User:           req.User,
		IdentitySource: lifecycle.SSHIdentitySource(req.IdentitySrc),
		IdentityFile:   req.IdentityFile,
		ProxyJump:      req.ProxyJump,
	})
}
