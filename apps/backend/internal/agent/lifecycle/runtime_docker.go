package lifecycle

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/common/logger"
)

// DockerRuntime implements Runtime for Docker-based agent execution.
type DockerRuntime struct {
	containerMgr *ContainerManager
	docker       *docker.Client
	logger       *logger.Logger
}

// NewDockerRuntime creates a new Docker runtime.
func NewDockerRuntime(dockerClient *docker.Client, log *logger.Logger) *DockerRuntime {
	return &DockerRuntime{
		containerMgr: NewContainerManager(dockerClient, "", log),
		docker:       dockerClient,
		logger:       log.WithFields(zap.String("runtime", "docker")),
	}
}

func (r *DockerRuntime) Name() string {
	return "docker"
}

func (r *DockerRuntime) HealthCheck(ctx context.Context) error {
	// Check Docker is reachable by listing containers
	_, err := r.docker.ListContainers(ctx, map[string]string{})
	if err != nil {
		return fmt.Errorf("docker not reachable: %w", err)
	}
	return nil
}

func (r *DockerRuntime) CreateInstance(ctx context.Context, req *RuntimeCreateRequest) (*RuntimeInstance, error) {
	// Convert RuntimeCreateRequest to ContainerConfig
	containerCfg := ContainerConfig{
		AgentConfig:    req.AgentConfig,
		WorkspacePath:  req.WorkspacePath,
		TaskID:         req.TaskID,
		InstanceID:     req.InstanceID,
		MainRepoGitDir: req.MainRepoGitDir,
		Credentials:    req.Env, // Env contains credentials from the caller
	}

	// Use ContainerManager to launch container
	containerID, client, err := r.containerMgr.LaunchContainer(ctx, containerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to launch container: %w", err)
	}

	// Get container IP for logging
	containerIP, _ := r.docker.GetContainerIP(ctx, containerID)

	// Build metadata
	metadata := make(map[string]interface{})
	if req.WorktreeID != "" {
		metadata["worktree_id"] = req.WorktreeID
		metadata["worktree_path"] = req.WorkspacePath
		metadata["worktree_branch"] = req.WorktreeBranch
	}

	r.logger.Info("docker instance created",
		zap.String("instance_id", req.InstanceID),
		zap.String("container_id", containerID),
		zap.String("container_ip", containerIP))

	return &RuntimeInstance{
		InstanceID:    req.InstanceID,
		TaskID:        req.TaskID,
		Client:        client,
		ContainerID:   containerID,
		ContainerIP:   containerIP,
		WorkspacePath: "/workspace", // Docker mounts workspace to /workspace
		Metadata:      metadata,
	}, nil
}

func (r *DockerRuntime) StopInstance(ctx context.Context, instance *RuntimeInstance, force bool) error {
	if instance.ContainerID == "" {
		return nil // No container to stop
	}

	var err error
	if force {
		err = r.docker.KillContainer(ctx, instance.ContainerID, "SIGKILL")
	} else {
		err = r.docker.StopContainer(ctx, instance.ContainerID, 30*time.Second)
	}

	if err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	return nil
}

func (r *DockerRuntime) RecoverInstances(ctx context.Context) ([]*RuntimeInstance, error) {
	// Find containers with kandev.managed label
	containers, err := r.docker.ListContainers(ctx, map[string]string{
		"kandev.managed": "true",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var recovered []*RuntimeInstance
	for _, ctr := range containers {
		// Only recover running containers
		if ctr.State != "running" {
			r.logger.Debug("skipping non-running container",
				zap.String("container_id", ctr.ID),
				zap.String("state", ctr.State))
			continue
		}

		// Get container labels to extract instance info
		labels, err := r.docker.GetContainerLabels(ctx, ctr.ID)
		if err != nil {
			r.logger.Warn("failed to get container labels",
				zap.String("container_id", ctr.ID),
				zap.Error(err))
			continue
		}

		instanceID := labels["kandev.instance_id"]
		taskID := labels["kandev.task_id"]
		agentProfileID := labels["kandev.agent_profile_id"]

		if instanceID == "" || taskID == "" {
			r.logger.Warn("container missing required labels",
				zap.String("container_id", ctr.ID))
			continue
		}

		// Get container IP
		containerIP, err := r.docker.GetContainerIP(ctx, ctr.ID)
		if err != nil {
			r.logger.Warn("failed to get container IP",
				zap.String("container_id", ctr.ID),
				zap.Error(err))
			containerIP = "127.0.0.1"
		}

		client := agentctl.NewClient(containerIP, AgentCtlPort, r.logger)

		recovered = append(recovered, &RuntimeInstance{
			InstanceID:    instanceID,
			TaskID:        taskID,
			Client:        client,
			ContainerID:   ctr.ID,
			ContainerIP:   containerIP,
			WorkspacePath: "/workspace",
			Metadata:      map[string]interface{}{"agent_profile_id": agentProfileID},
		})

		r.logger.Info("recovered docker instance",
			zap.String("instance_id", instanceID),
			zap.String("task_id", taskID),
			zap.String("container_id", ctr.ID))
	}

	return recovered, nil
}

