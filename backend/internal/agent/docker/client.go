// Package docker wraps the Docker SDK to provide container lifecycle operations.
package docker

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// ContainerConfig holds configuration for creating a container.
type ContainerConfig struct {
	Name        string
	Image       string
	Cmd         []string
	Env         []string // Environment variables
	WorkingDir  string
	Mounts      []MountConfig
	NetworkMode string
	Memory      int64 // Memory limit in bytes
	CPUQuota    int64 // CPU quota
	Labels      map[string]string
	AutoRemove  bool
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
}

// Client wraps the Docker client.
type Client struct {
	cli    *client.Client
	logger *logger.Logger
	config config.DockerConfig
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

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	log.Info("Docker client created",
		zap.String("host", cfg.Host),
		zap.String("api_version", cfg.APIVersion),
	)

	return &Client{
		cli:    cli,
		logger: log,
		config: cfg,
	}, nil
}

// Close closes the Docker client.
func (c *Client) Close() error {
	c.logger.Debug("Closing Docker client")
	return c.cli.Close()
}

// PullImage pulls a Docker image.
func (c *Client) PullImage(ctx context.Context, imageName string) error {
	c.logger.Info("Pulling image", zap.String("image", imageName))

	reader, err := c.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		c.logger.Error("Failed to pull image", zap.String("image", imageName), zap.Error(err))
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer reader.Close()

	// Read the output to ensure the image is fully pulled
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		c.logger.Error("Error reading image pull output", zap.String("image", imageName), zap.Error(err))
		return fmt.Errorf("error reading image pull output: %w", err)
	}

	c.logger.Info("Image pulled successfully", zap.String("image", imageName))
	return nil
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
	containerCfg := &container.Config{
		Image:      cfg.Image,
		Cmd:        cfg.Cmd,
		Env:        cfg.Env,
		WorkingDir: cfg.WorkingDir,
		Labels:     cfg.Labels,
	}

	// Host configuration
	hostCfg := &container.HostConfig{
		Mounts:      mounts,
		NetworkMode: container.NetworkMode(cfg.NetworkMode),
		AutoRemove:  cfg.AutoRemove,
		Resources: container.Resources{
			Memory:   cfg.Memory,
			CPUQuota: cfg.CPUQuota,
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, cfg.Name)
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

// StartContainer starts a container.
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	c.logger.Info("Starting container", zap.String("container_id", containerID))

	err := c.cli.ContainerStart(ctx, containerID, container.StartOptions{})
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
	err := c.cli.ContainerStop(ctx, containerID, container.StopOptions{
		Timeout: &timeoutSeconds,
	})
	if err != nil {
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

	err := c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: true,
	})
	if err != nil {
		c.logger.Error("Failed to remove container", zap.String("container_id", containerID), zap.Error(err))
		return fmt.Errorf("failed to remove container %s: %w", containerID, err)
	}

	c.logger.Info("Container removed", zap.String("container_id", containerID))
	return nil
}

// KillContainer kills a container.
func (c *Client) KillContainer(ctx context.Context, containerID string, signal string) error {
	c.logger.Info("Killing container",
		zap.String("container_id", containerID),
		zap.String("signal", signal),
	)

	err := c.cli.ContainerKill(ctx, containerID, signal)
	if err != nil {
		c.logger.Error("Failed to kill container", zap.String("container_id", containerID), zap.Error(err))
		return fmt.Errorf("failed to kill container %s: %w", containerID, err)
	}

	c.logger.Info("Container killed", zap.String("container_id", containerID))
	return nil
}

// GetContainerInfo returns information about a container.
func (c *Client) GetContainerInfo(ctx context.Context, containerID string) (*ContainerInfo, error) {
	c.logger.Debug("Getting container info", zap.String("container_id", containerID))

	inspect, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		c.logger.Error("Failed to inspect container", zap.String("container_id", containerID), zap.Error(err))
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerID, err)
	}

	info := &ContainerInfo{
		ID:       inspect.ID,
		Name:     inspect.Name,
		Image:    inspect.Config.Image,
		State:    inspect.State.Status,
		Status:   inspect.State.Status,
		ExitCode: inspect.State.ExitCode,
	}

	// Parse timestamps
	if inspect.State.StartedAt != "" {
		startedAt, err := time.Parse(time.RFC3339Nano, inspect.State.StartedAt)
		if err == nil {
			info.StartedAt = startedAt
		}
	}

	if inspect.State.FinishedAt != "" {
		finishedAt, err := time.Parse(time.RFC3339Nano, inspect.State.FinishedAt)
		if err == nil {
			info.FinishedAt = finishedAt
		}
	}

	// Get health status if available
	if inspect.State.Health != nil {
		info.Health = inspect.State.Health.Status
	}

	return info, nil
}

// GetContainerLogs returns logs from a container.
func (c *Client) GetContainerLogs(ctx context.Context, containerID string, follow bool, tail string) (io.ReadCloser, error) {
	c.logger.Debug("Getting container logs",
		zap.String("container_id", containerID),
		zap.Bool("follow", follow),
		zap.String("tail", tail),
	)

	opts := container.LogsOptions{
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

	statusCh, errCh := c.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	select {
	case err := <-errCh:
		if err != nil {
			c.logger.Error("Error waiting for container", zap.String("container_id", containerID), zap.Error(err))
			return -1, fmt.Errorf("error waiting for container %s: %w", containerID, err)
		}
	case status := <-statusCh:
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
	filterArgs := filters.NewArgs()
	for key, value := range labels {
		filterArgs.Add("label", fmt.Sprintf("%s=%s", key, value))
	}

	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		c.logger.Error("Failed to list containers", zap.Error(err))
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	infos := make([]ContainerInfo, 0, len(containers))
	for _, ctr := range containers {
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
			State:  ctr.State,
			Status: ctr.Status,
		}
		infos = append(infos, info)
	}

	c.logger.Debug("Listed containers", zap.Int("count", len(infos)))
	return infos, nil
}

// Ping checks if Docker is available.
func (c *Client) Ping(ctx context.Context) error {
	c.logger.Debug("Pinging Docker daemon")

	_, err := c.cli.Ping(ctx)
	if err != nil {
		c.logger.Error("Docker ping failed", zap.Error(err))
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

	// Container configuration with stdin attached
	containerCfg := &container.Config{
		Image:        cfg.Image,
		Cmd:          cfg.Cmd,
		Env:          cfg.Env,
		WorkingDir:   cfg.WorkingDir,
		Labels:       cfg.Labels,
		OpenStdin:    true,
		StdinOnce:    false,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false, // Important: no TTY for JSON-RPC
	}

	// Host configuration
	hostCfg := &container.HostConfig{
		Mounts:      mounts,
		NetworkMode: container.NetworkMode(cfg.NetworkMode),
		AutoRemove:  cfg.AutoRemove,
		Resources: container.Resources{
			Memory:   cfg.Memory,
			CPUQuota: cfg.CPUQuota,
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, cfg.Name)
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

	opts := container.AttachOptions{
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
		io.Copy(resp.Conn, stdinReader)
	}()

	c.logger.Info("Attached to container", zap.String("container_id", containerID))

	return &AttachResult{
		Stdin:  stdinWriter,
		Stdout: resp.Reader, // This is a multiplexed stream (stdout + stderr)
		Conn:   resp.Conn,
	}, nil
}

// Close closes the attach result
func (a *AttachResult) Close() error {
	if a.Stdin != nil {
		a.Stdin.Close()
	}
	if a.Conn != nil {
		a.Conn.Close()
	}
	return nil
}
