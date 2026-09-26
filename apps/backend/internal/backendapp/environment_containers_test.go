package backendapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type recordingLocalOps struct {
	statusCalls  []string
	destroyCalls []string
	status       *taskservice.ContainerLiveStatus
	err          error
}

func (r *recordingLocalOps) GetContainerLiveStatus(_ context.Context, id string) (*taskservice.ContainerLiveStatus, error) {
	r.statusCalls = append(r.statusCalls, id)
	return r.status, r.err
}

func (r *recordingLocalOps) DestroyContainer(_ context.Context, id string) error {
	r.destroyCalls = append(r.destroyCalls, id)
	return r.err
}

type recordingRemoteOps struct {
	inspectCalls []string
	removeCalls  []string
	seenConfig   map[string]string
	info         *docker.ContainerInfo
	inspectErr   error
	removeErr    error
}

func (r *recordingRemoteOps) Inspect(_ context.Context, cfg map[string]string, id string) (*docker.ContainerInfo, error) {
	r.inspectCalls = append(r.inspectCalls, id)
	r.seenConfig = cfg
	return r.info, r.inspectErr
}

func (r *recordingRemoteOps) Remove(_ context.Context, cfg map[string]string, id string) error {
	r.removeCalls = append(r.removeCalls, id)
	r.seenConfig = cfg
	return r.removeErr
}

type stubExecutors struct {
	executor *models.Executor
	err      error
}

func (s *stubExecutors) GetExecutor(context.Context, string) (*models.Executor, error) {
	return s.executor, s.err
}

const remoteExecutorID = "exec-remote"

func remoteEnv() *models.TaskEnvironment {
	return &models.TaskEnvironment{
		ExecutorType: string(models.ExecutorTypeRemoteDocker),
		ExecutorID:   remoteExecutorID,
		ContainerID:  "abc123",
	}
}

func localEnv() *models.TaskEnvironment {
	return &models.TaskEnvironment{
		ExecutorType: string(models.ExecutorTypeLocalDocker),
		ExecutorID:   "exec-local-docker",
		ContainerID:  "def456",
	}
}

func newDispatch(local *recordingLocalOps, remote *recordingRemoteOps) *containerOpsDispatch {
	return &containerOpsDispatch{
		local:  local,
		remote: remote,
		executors: &stubExecutors{executor: &models.Executor{
			ID:     remoteExecutorID,
			Type:   models.ExecutorTypeRemoteDocker,
			Config: map[string]string{"ssh_host": "10.0.0.5"},
		}},
	}
}

// TestContainerOpsDispatchRoutesByExecutorType is the defect this fix exists
// for: a remote Docker container was inspected on the install-wide daemon,
// which never knows it. AC-EXECUTORS-REMOTE-DOCKER-001.12.
func TestContainerOpsDispatchRoutesByExecutorType(t *testing.T) {
	t.Run("remote docker never reaches the local daemon", func(t *testing.T) {
		local := &recordingLocalOps{}
		remote := &recordingRemoteOps{info: &docker.ContainerInfo{ID: "abc123", State: "running", Status: "Up 21 minutes"}}
		d := newDispatch(local, remote)

		status, err := d.GetContainerLiveStatus(context.Background(), remoteEnv())
		if err != nil {
			t.Fatalf("GetContainerLiveStatus() error = %v, want nil", err)
		}
		if err := d.DestroyContainer(context.Background(), remoteEnv()); err != nil {
			t.Fatalf("DestroyContainer() error = %v, want nil", err)
		}

		if len(local.statusCalls) != 0 || len(local.destroyCalls) != 0 {
			t.Errorf("local daemon was consulted for a remote environment: status=%v destroy=%v",
				local.statusCalls, local.destroyCalls)
		}
		if len(remote.inspectCalls) != 1 || len(remote.removeCalls) != 1 {
			t.Errorf("remote daemon calls = inspect %v, remove %v; want one each",
				remote.inspectCalls, remote.removeCalls)
		}
		if remote.seenConfig["ssh_host"] != "10.0.0.5" {
			t.Errorf("remote ops got config %v, want the executor's saved connection", remote.seenConfig)
		}
		// The whole point: a live container reads as running, not missing.
		if status == nil || status.State != "running" || status.Missing {
			t.Errorf("status = %+v, want a running, non-missing container", status)
		}
	})

	t.Run("local docker keeps using the local daemon", func(t *testing.T) {
		local := &recordingLocalOps{status: &taskservice.ContainerLiveStatus{ContainerID: "def456", State: "running"}}
		remote := &recordingRemoteOps{}
		d := newDispatch(local, remote)

		if _, err := d.GetContainerLiveStatus(context.Background(), localEnv()); err != nil {
			t.Fatalf("GetContainerLiveStatus() error = %v, want nil", err)
		}
		if err := d.DestroyContainer(context.Background(), localEnv()); err != nil {
			t.Fatalf("DestroyContainer() error = %v, want nil", err)
		}

		if len(local.statusCalls) != 1 || len(local.destroyCalls) != 1 {
			t.Errorf("local calls = status %v, destroy %v; want one each", local.statusCalls, local.destroyCalls)
		}
		if len(remote.inspectCalls) != 0 || len(remote.removeCalls) != 0 {
			t.Error("a local Docker environment was routed to the remote path")
		}
	})
}

// TestContainerOpsDispatchSeparatesMissingFromUnreachable covers
// AC-EXECUTORS-REMOTE-DOCKER-001.13. Reporting an unreachable daemon as
// "missing" is what made a live session unresumable, so the two must not
// collapse back into one state.
func TestContainerOpsDispatchSeparatesMissingFromUnreachable(t *testing.T) {
	t.Run("daemon says no such container", func(t *testing.T) {
		remote := &recordingRemoteOps{inspectErr: runtime.ErrRemoteContainerMissing}
		d := newDispatch(&recordingLocalOps{}, remote)

		status, err := d.GetContainerLiveStatus(context.Background(), remoteEnv())

		if err != nil {
			t.Fatalf("GetContainerLiveStatus() error = %v, want nil for a known-absent container", err)
		}
		if status == nil || !status.Missing {
			t.Errorf("status = %+v, want Missing=true", status)
		}
	})

	t.Run("daemon unreachable is an error, not missing", func(t *testing.T) {
		remote := &recordingRemoteOps{inspectErr: errors.New("dial tcp: connection refused")}
		d := newDispatch(&recordingLocalOps{}, remote)

		status, err := d.GetContainerLiveStatus(context.Background(), remoteEnv())

		if err == nil {
			t.Fatal("GetContainerLiveStatus() error = nil; an unreachable host must not read as a missing container")
		}
		if status != nil && status.Missing {
			t.Error("an unreachable daemon was reported as a missing container")
		}
	})

	t.Run("reset reports an unreachable daemon rather than orphaning", func(t *testing.T) {
		remote := &recordingRemoteOps{removeErr: errors.New("dial tcp: connection refused")}
		d := newDispatch(&recordingLocalOps{}, remote)

		err := d.DestroyContainer(context.Background(), remoteEnv())

		if err == nil {
			t.Fatal("DestroyContainer() error = nil; the remote container would be silently orphaned")
		}
	})
}

// TestContainerOpsDispatchUnresolvableExecutor keeps the failure honest when
// the executor row cannot be read: without its connection there is no daemon
// to ask, and falling back to the local one is the original bug.
func TestContainerOpsDispatchUnresolvableExecutor(t *testing.T) {
	local := &recordingLocalOps{}
	d := &containerOpsDispatch{
		local:     local,
		remote:    &recordingRemoteOps{},
		executors: &stubExecutors{err: errors.New("executor not found")},
	}

	_, err := d.GetContainerLiveStatus(context.Background(), remoteEnv())

	if err == nil || !strings.Contains(err.Error(), "executor not found") {
		t.Fatalf("error = %v, want the executor lookup failure surfaced", err)
	}
	if len(local.statusCalls) != 0 {
		t.Error("fell back to the local daemon when the executor row was unreadable")
	}
}
