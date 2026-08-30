// Package docker wraps the Docker SDK to provide container lifecycle operations.
package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

// ContainerConfig holds configuration for creating a container.
type ContainerConfig struct {
	Name         string
	Image        string
	Entrypoint   []string // Overrides the image ENTRYPOINT (nil = use image default)
	Cmd          []string
	Env          []string // Environment variables
	WorkingDir   string
	Mounts       []MountConfig
	NetworkMode  string
	Memory       int64 // Memory limit in bytes
	CPUQuota     int64 // CPU quota
	Labels       map[string]string
	AutoRemove   bool
	PortBindings []PortBindingConfig
}

// PortBindingConfig describes a container port to publish on the Docker host.
type PortBindingConfig struct {
	ContainerPort int
	HostIP        string
	HostPort      string
}

// MountConfig holds mount configuration.
type MountConfig struct {
	Source   string // Host path
	Target   string // Container path
	ReadOnly bool
}

// ContainerInfo holds information about a running container.
type ContainerInfo struct {
	ID         string
	Name       string
	Image      string
	State      string // created, running, paused, restarting, removing, exited, dead
	Status     string // Human-readable status
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
	Health     string
	Labels     map[string]string
}

// Client wraps the Docker client.
type containerRemover interface {
	ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

type imageBuilder interface {
	ImageBuild(context.Context, io.Reader, client.ImageBuildOptions) (client.ImageBuildResult, error)
}

type Client struct {
	cli      *client.Client
	storage  storageAPI
	remover  containerRemover
	builder  imageBuilder
	logger   *logger.Logger
	config   config.DockerConfig
	activity *activity.Coordinator
	mu       sync.RWMutex
}

// NewClient creates a new Docker client.
func NewClient(cfg config.DockerConfig, log *logger.Logger) (*Client, error) {
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	if cfg.Host != "" {
		opts = append(opts, client.WithHost(cfg.Host))
	}

	if cfg.APIVersion != "" {
		opts = append(opts, client.WithVersion(cfg.APIVersion))
	}

	cli, err := client.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	log.Info("Docker client created",
		zap.String("host", cfg.Host),
		zap.String("api_version", cfg.APIVersion),
	)

	return &Client{
		cli:     cli,
		storage: cli,
		remover: cli,
		builder: cli,
		logger:  log,
		config:  cfg,
	}, nil
}

// SetActivityCoordinator wires the optional install-wide host activity gate.
func (c *Client) SetActivityCoordinator(coordinator *activity.Coordinator) {
	c.mu.Lock()
	c.activity = coordinator
	c.mu.Unlock()
}

// Close closes the Docker client.
func (c *Client) Close() error {
	c.logger.Debug("Closing Docker client")
	return c.cli.Close()
}

// PullImage pulls a Docker image.
func (c *Client) PullImage(ctx context.Context, imageName string) error {
	c.logger.Info("Pulling image", zap.String("image", imageName))

	reader, err := c.cli.ImagePull(ctx, imageName, client.ImagePullOptions{})
	if err != nil {
		c.logger.Error("Failed to pull image", zap.String("image", imageName), zap.Error(err))
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			c.logger.Warn("Failed to close image pull reader", zap.Error(err))
		}
	}()

	// Read the output to ensure the image is fully pulled
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		c.logger.Error("Error reading image pull output", zap.String("image", imageName), zap.Error(err))
		return fmt.Errorf("error reading image pull output: %w", err)
	}

	c.logger.Info("Image pulled successfully", zap.String("image", imageName))
	return nil
}

// BuildImage builds a Docker image from a Dockerfile string.
// It returns the build output as an io.ReadCloser for streaming.
// The caller is responsible for closing the returned reader.
func (c *Client) BuildImage(ctx context.Context, dockerfile string, tag string, buildArgs map[string]*string) (io.ReadCloser, error) {
	c.logger.Info("Building image", zap.String("tag", tag))
	c.mu.RLock()
	coordinator := c.activity
	builder := c.builder
	c.mu.RUnlock()
	if builder == nil {
		builder = c.cli
	}
	var activityLease *activity.TaskLease
	var err error
	if coordinator != nil {
		activityLease, err = coordinator.AcquireTask(ctx, activity.KindDockerImageBuild)
		if err != nil {
			return nil, err
		}
	}

	buildContext, err := createDockerfileTar(dockerfile)
	if err != nil {
		activityLease.Release()
		return nil, fmt.Errorf("failed to create build context: %w", err)
	}

	resp, err := builder.ImageBuild(ctx, buildContext, client.ImageBuildOptions{
		Tags:       []string{tag},
		BuildArgs:  buildArgs,
		Dockerfile: "Dockerfile",
		Remove:     true,
	})
	if err != nil {
		activityLease.Release()
		c.logger.Error("Failed to build image", zap.String("tag", tag), zap.Error(err))
		return nil, fmt.Errorf("failed to build image %s: %w", tag, err)
	}

	return &activityReadCloser{ReadCloser: resp.Body, lease: activityLease}, nil
}

type activityReadCloser struct {
	io.ReadCloser
	lease *activity.TaskLease
	once  sync.Once
}

func (r *activityReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if err == io.EOF {
		r.release()
	}
	return n, err
}

func (r *activityReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.release()
	return err
}

func (r *activityReadCloser) release() {
	r.once.Do(func() { r.lease.Release() })
}

// createDockerfileTar creates a tar archive containing a single Dockerfile.
func createDockerfileTar(dockerfile string) (io.Reader, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	header := &tar.Header{
		Name: "Dockerfile",
		Size: int64(len(dockerfile)),
		Mode: 0o644,
	}
	if err := tw.WriteHeader(header); err != nil {
		return nil, fmt.Errorf("failed to write tar header: %w", err)
	}
	if _, err := tw.Write([]byte(dockerfile)); err != nil {
		return nil, fmt.Errorf("failed to write Dockerfile to tar: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}

	return buf, nil
}

// CreateContainer creates a new container.
func (c *Client) CreateContainer(ctx context.Context, cfg ContainerConfig) (string, error) {
	c.logger.Info("Creating container",
		zap.String("name", cfg.Name),
		zap.String("image", cfg.Image),
	)

	// Build mounts
	mounts := make([]mount.Mount, 0, len(cfg.Mounts))
	for _, m := range cfg.Mounts {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	// Container configuration
	exposedPorts, portBindings, err := buildDockerPortBindings(cfg.PortBindings)
	if err != nil {
		return "", fmt.Errorf("failed to create container %s: %w", cfg.Name, err)
	}
	containerCfg := &container.Config{
		Image:        cfg.Image,
		Entrypoint:   cfg.Entrypoint,
		Cmd:          cfg.Cmd,
		Env:          cfg.Env,
		WorkingDir:   cfg.WorkingDir,
		Labels:       cfg.Labels,
		ExposedPorts: exposedPorts,
	}

	// Host configuration
	hostCfg := &container.HostConfig{
		Mounts:       mounts,
		NetworkMode:  container.NetworkMode(cfg.NetworkMode),
		AutoRemove:   cfg.AutoRemove,
		PortBindings: portBindings,
		Resources: container.Resources{
			Memory:   cfg.Memory,
			CPUQuota: cfg.CPUQuota,
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     containerCfg,
		HostConfig: hostCfg,
		Name:       cfg.Name,
	})
	if err != nil {
		c.logger.Error("Failed to create container",
			zap.String("name", cfg.Name),
			zap.Error(err),
		)
		return "", fmt.Errorf("failed to create container %s: %w", cfg.Name, err)
	}

	c.logger.Info("Container created", zap.String("id", resp.ID), zap.String("name", cfg.Name))
	return resp.ID, nil
}

func buildDockerPortBindings(bindings []PortBindingConfig) (network.PortSet, network.PortMap, error) {
	if len(bindings) == 0 {
		return nil, nil, nil
	}
	exposedPorts := network.PortSet{}
	portBindings := network.PortMap{}
	for _, binding := range bindings {
		containerPort, err := tcpPort(binding.ContainerPort)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid container port binding: %w", err)
		}
		hostIP, err := parseDockerHostIP(binding.HostIP)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid host IP for container port %d: %w", binding.ContainerPort, err)
		}
		exposedPorts[containerPort] = struct{}{}
		portBindings[containerPort] = append(portBindings[containerPort], network.PortBinding{
			HostIP:   hostIP,
			HostPort: binding.HostPort,
		})
	}
	return exposedPorts, portBindings, nil
}

// tcpPort converts a container port number into the API's opaque TCP port key.
func tcpPort(port int) (network.Port, error) {
	if port <= 0 || port > math.MaxUint16 {
		return network.Port{}, fmt.Errorf("port %d out of range", port)
	}
	parsed, ok := network.PortFrom(uint16(port), network.TCP)
	if !ok {
		return network.Port{}, fmt.Errorf("port %d is not a valid TCP port", port)
	}
	return parsed, nil
}

// parseDockerHostIP converts a configured host IP into the API's address type.
// An empty value keeps Docker's "publish on all interfaces" behavior.
func parseDockerHostIP(hostIP string) (netip.Addr, error) {
	if hostIP == "" {
		return netip.Addr{}, nil
	}
	addr, err := netip.ParseAddr(hostIP)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parse host IP %q: %w", hostIP, err)
	}
	return addr, nil
}

// StartContainer starts a container.
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	c.logger.Info("Starting container", zap.String("container_id", containerID))

	_, err := c.cli.ContainerStart(ctx, containerID, client.ContainerStartOptions{})
	if err != nil {
		c.logger.Error("Failed to start container", zap.String("container_id", containerID), zap.Error(err))
		return fmt.Errorf("failed to start container %s: %w", containerID, err)
	}

	c.logger.Info("Container started", zap.String("container_id", containerID))
	return nil
}

// StopContainer stops a container with a timeout.
func (c *Client) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	c.logger.Info("Stopping container",
		zap.String("container_id", containerID),
		zap.Duration("timeout", timeout),
	)

	timeoutSeconds := int(timeout.Seconds())
	_, err := c.cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{
		Timeout: &timeoutSeconds,
	})
	if err != nil {
		if errdefs.IsNotFound(err) {
			// Container teardown is intentionally idempotent. A container can
			// disappear between the task resource snapshot and this stop call
			// (for example, after an explicit environment reset).
			return nil
		}
		c.logger.Error("Failed to stop container", zap.String("container_id", containerID), zap.Error(err))
		return fmt.Errorf("failed to stop container %s: %w", containerID, err)
	}

	c.logger.Info("Container stopped", zap.String("container_id", containerID))
	return nil
}

// RemoveContainer removes a container.
func (c *Client) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	c.logger.Info("Removing container",
		zap.String("container_id", containerID),
		zap.Bool("force", force),
	)

	_, err := c.containerRemover().ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{
		Force:         force,
		RemoveVolumes: true,
	})
	if err != nil {
		if errdefs.IsNotFound(err) {
			// Remove is a convergence operation. Another cleanup owner may have
			// removed the container already, which is the desired end state.
			return nil
		}
		if containerRemovalInProgress(err) {
			// A concurrent cleanup owner is already removing the container. Treat
			// this conflict as success because both callers converge on removal.
			return nil
		}
		c.logger.Error("Failed to remove container", zap.String("container_id", containerID), zap.Error(err))
		return fmt.Errorf("failed to remove container %s: %w", containerID, err)
	}

	c.logger.Info("Container removed", zap.String("container_id", containerID))
	return nil
}

func containerRemovalInProgress(err error) bool {
	if !errdefs.IsConflict(err) {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "removal of container") &&
		strings.Contains(message, "already in progress")
}

func (c *Client) containerRemover() containerRemover {
	if c.remover != nil {
		return c.remover
	}
	return c.cli
}

// KillContainer kills a container.
func (c *Client) KillContainer(ctx context.Context, containerID string, signal string) error {
	c.logger.Info("Killing container",
		zap.String("container_id", containerID),
		zap.String("signal", signal),
	)

	_, err := c.cli.ContainerKill(ctx, containerID, client.ContainerKillOptions{Signal: signal})
	if err != nil {
		if errdefs.IsNotFound(err) {
			// A forced cleanup may race with Docker auto-removal or another
			// teardown pass. Missing already means the container is stopped.
			return nil
		}
		c.logger.Error("Failed to kill container", zap.String("container_id", containerID), zap.Error(err))
		return fmt.Errorf("failed to kill container %s: %w", containerID, err)
	}

	c.logger.Info("Container killed", zap.String("container_id", containerID))
	return nil
}

// inspectContainer inspects a container and unwraps the typed response.
func (c *Client) inspectContainer(ctx context.Context, containerID string) (container.InspectResponse, error) {
	result, err := c.cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, err
	}
	return result.Container, nil
}

// GetContainerInfo returns information about a container.
func (c *Client) GetContainerInfo(ctx context.Context, containerID string) (*ContainerInfo, error) {
	c.logger.Debug("Getting container info", zap.String("container_id", containerID))

	inspect, err := c.inspectContainer(ctx, containerID)
	if err != nil {
		c.logger.Error("Failed to inspect container", zap.String("container_id", containerID), zap.Error(err))
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerID, err)
	}

	info := &ContainerInfo{
		ID:   inspect.ID,
		Name: inspect.Name,
	}
	if inspect.Config != nil {
		info.Image = inspect.Config.Image
		info.Labels = inspect.Config.Labels
	}
	applyContainerState(info, inspect.State)

	return info, nil
}

// applyContainerState copies the inspected runtime state onto info. The API
// models State as a pointer, so a missing state leaves the zero values.
func applyContainerState(info *ContainerInfo, state *container.State) {
	if state == nil {
		return
	}
	info.State = string(state.Status)
	info.Status = string(state.Status)
	info.ExitCode = state.ExitCode

	if startedAt, err := time.Parse(time.RFC3339Nano, state.StartedAt); err == nil {
		info.StartedAt = startedAt
	}
	if finishedAt, err := time.Parse(time.RFC3339Nano, state.FinishedAt); err == nil {
		info.FinishedAt = finishedAt
	}
	if state.Health != nil {
		info.Health = string(state.Health.Status)
	}
}

// IsContainerRunning returns true if the container exists and is in "running" state
func (c *Client) IsContainerRunning(ctx context.Context, containerID string) (bool, error) {
	info, err := c.GetContainerInfo(ctx, containerID)
	if err != nil {
		return false, err
	}
	return info.State == "running", nil
}

// GetContainerIP returns the IP address of a container
func (c *Client) GetContainerIP(ctx context.Context, containerID string) (string, error) {
	c.logger.Debug("Getting container IP", zap.String("container_id", containerID))

	inspect, err := c.inspectContainer(ctx, containerID)
	if err != nil {
		c.logger.Error("Failed to inspect container for IP", zap.String("container_id", containerID), zap.Error(err))
		return "", err
	}

	if inspect.NetworkSettings != nil {
		// Check available networks for an IP address.
		for netName, netSettings := range inspect.NetworkSettings.Networks {
			if netSettings == nil || !netSettings.IPAddress.IsValid() {
				continue
			}
			ip := netSettings.IPAddress.String()
			c.logger.Debug("Found container IP",
				zap.String("container_id", containerID),
				zap.String("network", netName),
				zap.String("ip", ip))
			return ip, nil
		}
	}

	return "", fmt.Errorf("no IP address found for container %s", containerID)
}

// GetContainerHostPort returns the Docker host endpoint for a published TCP port.
func (c *Client) GetContainerHostPort(ctx context.Context, containerID string, containerPort int) (string, int, error) {
	c.logger.Debug("Getting container host port",
		zap.String("container_id", containerID),
		zap.Int("container_port", containerPort))

	inspect, err := c.inspectContainer(ctx, containerID)
	if err != nil {
		return "", 0, fmt.Errorf("failed to inspect container %s: %w", containerID, err)
	}
	if inspect.NetworkSettings == nil || inspect.NetworkSettings.Ports == nil {
		return "", 0, fmt.Errorf("container %s has no published ports", containerID)
	}

	port, err := tcpPort(containerPort)
	if err != nil {
		return "", 0, fmt.Errorf("container %s: %w", containerID, err)
	}
	bindings := inspect.NetworkSettings.Ports[port]
	if len(bindings) == 0 {
		return "", 0, fmt.Errorf("container %s port %d/tcp is not published", containerID, containerPort)
	}
	hostPort, err := parseHostPort(bindings[0].HostPort)
	if err != nil {
		return "", 0, fmt.Errorf("invalid host port for container %s port %d/tcp: %w", containerID, containerPort, err)
	}
	hostIP := normalizeDockerHostIP(bindings[0].HostIP)
	return hostIP, hostPort, nil
}

// parseHostPort validates a host-side port number reported by the daemon.
func parseHostPort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse port %q: %w", value, err)
	}
	if port < 0 || port > math.MaxUint16 {
		return 0, fmt.Errorf("port %d out of range", port)
	}
	return port, nil
}

// normalizeDockerHostIP maps an unset or wildcard publish address onto loopback
// so callers get an address they can actually dial.
func normalizeDockerHostIP(hostIP netip.Addr) string {
	if !hostIP.IsValid() || hostIP.IsUnspecified() {
		return "127.0.0.1"
	}
	return hostIP.String()
}

// GetContainerLabels returns the labels of a container
func (c *Client) GetContainerLabels(ctx context.Context, containerID string) (map[string]string, error) {
	c.logger.Debug("Getting container labels", zap.String("container_id", containerID))

	inspect, err := c.inspectContainer(ctx, containerID)
	if err != nil {
		c.logger.Error("Failed to inspect container for labels", zap.String("container_id", containerID), zap.Error(err))
		return nil, err
	}

	if inspect.Config != nil && inspect.Config.Labels != nil {
		return inspect.Config.Labels, nil
	}

	return make(map[string]string), nil
}

// GetContainerLogs returns logs from a container.
func (c *Client) GetContainerLogs(ctx context.Context, containerID string, follow bool, tail string) (io.ReadCloser, error) {
	c.logger.Debug("Getting container logs",
		zap.String("container_id", containerID),
		zap.Bool("follow", follow),
		zap.String("tail", tail),
	)

	opts := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tail,
		Timestamps: false,
	}

	reader, err := c.cli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		c.logger.Error("Failed to get container logs", zap.String("container_id", containerID), zap.Error(err))
		return nil, fmt.Errorf("failed to get container logs for %s: %w", containerID, err)
	}

	return reader, nil
}

// WaitContainer waits for a container to stop and returns the exit code.
func (c *Client) WaitContainer(ctx context.Context, containerID string) (int64, error) {
	c.logger.Info("Waiting for container", zap.String("container_id", containerID))

	wait := c.cli.ContainerWait(ctx, containerID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})

	select {
	case err := <-wait.Error:
		if err != nil {
			c.logger.Error("Error waiting for container", zap.String("container_id", containerID), zap.Error(err))
			return -1, fmt.Errorf("error waiting for container %s: %w", containerID, err)
		}
	case status := <-wait.Result:
		c.logger.Info("Container exited",
			zap.String("container_id", containerID),
			zap.Int64("exit_code", status.StatusCode),
		)
		return status.StatusCode, nil
	case <-ctx.Done():
		c.logger.Warn("Context cancelled while waiting for container", zap.String("container_id", containerID))
		return -1, ctx.Err()
	}

	return -1, nil
}

// ListContainers lists containers with optional filters.
func (c *Client) ListContainers(ctx context.Context, labels map[string]string) ([]ContainerInfo, error) {
	c.logger.Debug("Listing containers", zap.Any("labels", labels))

	// Build filters from labels
	listFilters := make(client.Filters)
	for key, value := range labels {
		listFilters.Add("label", fmt.Sprintf("%s=%s", key, value))
	}

	result, err := c.cli.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: listFilters,
	})
	if err != nil {
		c.logger.Error("Failed to list containers", zap.Error(err))
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	infos := make([]ContainerInfo, 0, len(result.Items))
	for _, ctr := range result.Items {
		name := ""
		if len(ctr.Names) > 0 {
			name = ctr.Names[0]
			// Remove leading slash from container name
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		info := ContainerInfo{
			ID:     ctr.ID,
			Name:   name,
			Image:  ctr.Image,
			State:  string(ctr.State),
			Status: ctr.Status,
			Labels: ctr.Labels,
		}
		infos = append(infos, info)
	}

	c.logger.Debug("Listed containers", zap.Int("count", len(infos)))
	return infos, nil
}

// Ping checks if Docker is available.
func (c *Client) Ping(ctx context.Context) error {
	c.logger.Debug("Pinging Docker daemon")

	_, err := c.cli.Ping(ctx, client.PingOptions{})
	if err != nil {
		c.logger.Debug("Docker ping failed", zap.Error(err))
		return fmt.Errorf("docker ping failed: %w", err)
	}

	c.logger.Debug("Docker daemon is available")
	return nil
}

// AttachResult contains the streams for container I/O
type AttachResult struct {
	Stdin  io.WriteCloser
	Stdout io.Reader
	Stderr io.Reader
	Conn   net.Conn
}

// CreateContainerInteractive creates a container with stdin attached for interactive use
func (c *Client) CreateContainerInteractive(ctx context.Context, cfg ContainerConfig) (string, error) {
	c.logger.Info("Creating interactive container",
		zap.String("name", cfg.Name),
		zap.String("image", cfg.Image),
	)

	// Build mounts
	mounts := make([]mount.Mount, 0, len(cfg.Mounts))
	for _, m := range cfg.Mounts {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	// Container configuration with stdin attached. Mirror CreateContainer's
	// handling of ContainerConfig fields — including Entrypoint — so the same
	// config struct produces consistent container behavior on both paths.
	exposedPorts, portBindings, err := buildDockerPortBindings(cfg.PortBindings)
	if err != nil {
		return "", fmt.Errorf("failed to create interactive container %s: %w", cfg.Name, err)
	}
	containerCfg := &container.Config{
		Image:        cfg.Image,
		Entrypoint:   cfg.Entrypoint,
		Cmd:          cfg.Cmd,
		Env:          cfg.Env,
		WorkingDir:   cfg.WorkingDir,
		Labels:       cfg.Labels,
		OpenStdin:    true,
		StdinOnce:    false,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		ExposedPorts: exposedPorts,
		Tty:          false, // Important: no TTY for JSON-RPC
	}

	// Host configuration
	hostCfg := &container.HostConfig{
		Mounts:       mounts,
		NetworkMode:  container.NetworkMode(cfg.NetworkMode),
		AutoRemove:   cfg.AutoRemove,
		PortBindings: portBindings,
		Resources: container.Resources{
			Memory:   cfg.Memory,
			CPUQuota: cfg.CPUQuota,
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     containerCfg,
		HostConfig: hostCfg,
		Name:       cfg.Name,
	})
	if err != nil {
		c.logger.Error("Failed to create interactive container",
			zap.String("name", cfg.Name),
			zap.Error(err),
		)
		return "", fmt.Errorf("failed to create interactive container %s: %w", cfg.Name, err)
	}

	c.logger.Info("Interactive container created", zap.String("id", resp.ID), zap.String("name", cfg.Name))
	return resp.ID, nil
}

// AttachContainer attaches to a container's stdin, stdout, and stderr
func (c *Client) AttachContainer(ctx context.Context, containerID string) (*AttachResult, error) {
	c.logger.Info("Attaching to container", zap.String("container_id", containerID))

	opts := client.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	}

	resp, err := c.cli.ContainerAttach(ctx, containerID, opts)
	if err != nil {
		c.logger.Error("Failed to attach to container", zap.String("container_id", containerID), zap.Error(err))
		return nil, fmt.Errorf("failed to attach to container %s: %w", containerID, err)
	}

	// Create a pipe for stdin
	stdinReader, stdinWriter := io.Pipe()

	// Start goroutine to copy from pipe to container
	go func() {
		if _, err := io.Copy(resp.Conn, stdinReader); err != nil {
			c.logger.Debug("failed to copy stdin to container", zap.Error(err))
		}
	}()

	// Create a pipe for demultiplexed stdout
	// Docker sends multiplexed streams when Tty=false with 8-byte headers
	stdoutReader, stdoutWriter := io.Pipe()

	// Start goroutine to demultiplex stdout/stderr
	go func() {
		defer func() {
			if err := stdoutWriter.Close(); err != nil {
				c.logger.Debug("failed to close stdout writer", zap.Error(err))
			}
		}()
		c.demultiplexStream(resp.Reader, stdoutWriter)
	}()

	c.logger.Info("Attached to container", zap.String("container_id", containerID))

	return &AttachResult{
		Stdin:  stdinWriter,
		Stdout: stdoutReader, // Demultiplexed stdout stream
		Conn:   resp.Conn,
	}, nil
}

// demultiplexStream reads Docker's multiplexed stream format and writes only stdout to the writer.
// Docker stream format when Tty=false:
// - Byte 0: Stream type (0=stdin, 1=stdout, 2=stderr)
// - Bytes 1-3: Reserved
// - Bytes 4-7: Frame size (big endian uint32)
// - Bytes 8+: Frame data
func (c *Client) demultiplexStream(reader io.Reader, writer io.Writer) {
	header := make([]byte, 8)
	for {
		// Read the 8-byte header
		_, err := io.ReadFull(reader, header)
		if err != nil {
			if err != io.EOF {
				c.logger.Debug("demultiplex stream ended", zap.Error(err))
			}
			return
		}

		// Parse header
		streamType := header[0]
		size := binary.BigEndian.Uint32(header[4:8])

		// Read the frame data
		if size > 0 {
			data := make([]byte, size)
			_, err := io.ReadFull(reader, data)
			if err != nil {
				c.logger.Debug("failed to read frame data", zap.Error(err))
				return
			}

			// Write stdout (type 1) and stderr (type 2) to writer
			// We want both for ACP since errors should be visible
			if streamType == 1 || streamType == 2 {
				if _, err := writer.Write(data); err != nil {
					c.logger.Debug("failed to write demultiplexed data", zap.Error(err))
				}
			}
		}
	}
}

// Close closes the attach result
func (a *AttachResult) Close() error {
	if a.Stdin != nil {
		if err := a.Stdin.Close(); err != nil {
			return err
		}
	}
	if a.Conn != nil {
		if err := a.Conn.Close(); err != nil {
			return err
		}
	}
	return nil
}
