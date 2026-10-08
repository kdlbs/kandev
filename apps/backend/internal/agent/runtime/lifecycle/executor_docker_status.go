package lifecycle

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/task/models"
	"time"
)

func (r *DockerExecutor) GetRemoteStatus(ctx context.Context, instance *ExecutorInstance) (*RemoteStatus, error) {
	client, _, err := r.ensureClient()
	if err != nil {
		return nil, routingerr.SanitizeError(err)
	}
	return dockerExecutorRemoteStatus(ctx, client, instance, string(r.Name()))
}

func (r *RemoteDockerExecutor) GetRemoteStatus(ctx context.Context, instance *ExecutorInstance) (*RemoteStatus, error) {
	if instance == nil {
		return nil, fmt.Errorf("executor instance unavailable")
	}
	r.mu.Lock()
	session := r.sessions[instance.InstanceID]
	r.mu.Unlock()
	if session == nil || session.dockerClient == nil {
		return &RemoteStatus{RuntimeName: r.Name(), State: models.ExecutorOutcomeUnknown, LastCheckedAt: time.Now().UTC()}, nil
	}
	return dockerExecutorRemoteStatus(ctx, session.dockerClient, instance, string(r.Name()))
}

func dockerExecutorRemoteStatus(ctx context.Context, client *docker.Client, instance *ExecutorInstance, runtime string) (*RemoteStatus, error) {
	if instance == nil || instance.ContainerID == "" {
		return nil, fmt.Errorf("recorded container identity unavailable")
	}
	info, err := client.GetContainerInfo(ctx, instance.ContainerID)
	if err != nil {
		return nil, routingerr.SanitizeError(err)
	}
	if info.ID != instance.ContainerID || instance.TaskID == "" || info.Labels["kandev.task_id"] != instance.TaskID {
		return nil, fmt.Errorf("container identity differs from recorded executor")
	}
	now := time.Now().UTC()
	obs := &models.ExecutorObservation{Outcome: models.ExecutorOutcomeUnknown, Runtime: runtime, ResourceKey: info.ID, ObservedAt: now, Workspace: models.ExecutorOutcomeUnknown}
	evidence := models.ExecutorContainerEvidence{Name: "agent", State: info.State, StartedAt: nonZeroTimePtr(info.StartedAt), FinishedAt: nonZeroTimePtr(info.FinishedAt)}
	switch info.State {
	case string(PrepareStepRunning):
		obs.Outcome = models.ExecutorOutcomeHealthy
		evidence.Ready = true
	case "exited", "dead":
		obs.Outcome = models.ExecutorOutcomeTerminated
		obs.Reason = "ContainerExited"
		if info.OOMKilled {
			obs.Reason = "OOMKilled"
		}
		obs.OccurredAt = evidence.FinishedAt
		code := int32(info.ExitCode)
		evidence.ExitCode = &code
	}
	obs.ContainerReady = evidence.Ready
	obs.Containers = []models.ExecutorContainerEvidence{evidence}
	return &RemoteStatus{RuntimeName: instance.RuntimeName, State: info.State, LastCheckedAt: now, Observation: obs}, nil
}
