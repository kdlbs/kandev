package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestRemoteDockerRequiresAnSSHTarget fails closed when the profile carries no
// usable target, rather than falling through to a local daemon. A remote
// profile that silently ran locally would put the task on the wrong machine.
func TestRemoteDockerRequiresAnSSHTarget(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))

	_, err := exec.CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		Metadata:   map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("CreateInstance with no SSH target = nil error, want error")
	}
	if strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("runtime still reports itself unimplemented: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "host") {
		t.Fatalf("error %q does not name the missing host", err)
	}
}

// TestRemoteDockerRejectsDaemonURL keeps a Docker host URL out of the profile's
// host field. Accepting tcp:// here is what the requirement excludes, and
// accepting ssh:// would imply a second SSH implementation.
func TestRemoteDockerRejectsDaemonURL(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))

	for _, host := range []string{"tcp://build-box:2375", "ssh://dev@build-box", "unix:///var/run/docker.sock"} {
		_, err := exec.CreateInstance(context.Background(), &ExecutorCreateRequest{
			InstanceID: "instance-1",
			TaskID:     "task-1",
			Metadata:   map[string]interface{}{MetadataKeySSHHost: host},
		})
		if err == nil {
			t.Errorf("CreateInstance with host %q = nil error, want error", host)
		}
	}
}

// TestRemoteDockerReportsRuntimeIdentity keeps the runtime's advertised
// identity stable: the executor type maps to this runtime, it needs a clone
// URL because the remote cannot see host paths, and it is containerized.
func TestRemoteDockerReportsRuntimeIdentity(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))

	if got := exec.Name(); got != executorNameRemoteDocker() {
		t.Fatalf("Name() = %q, want the remote docker runtime", got)
	}
	if !exec.RequiresCloneURL() {
		t.Fatal("RequiresCloneURL() = false; a remote daemon cannot read host paths")
	}
	if !models.IsContainerizedExecutorType(models.ExecutorTypeRemoteDocker) {
		t.Fatal("remote_docker is not reported as containerized")
	}
	if !models.IsRemoteExecutorType(models.ExecutorTypeRemoteDocker) {
		t.Fatal("remote_docker is not reported as remote")
	}
}

// TestRemoteDockerStopWithoutContainerIsNotAnError keeps teardown idempotent:
// a stop for an instance that never got a container should not fail the
// archive/delete path it runs inside.
func TestRemoteDockerStopWithoutContainerIsNotAnError(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))

	err := exec.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: "instance-1",
		TaskID:     "task-1",
	}, false)
	if err != nil && strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("StopInstance still reports itself unimplemented: %v", err)
	}
	if err != nil {
		t.Fatalf("StopInstance with no container = %v, want nil", err)
	}
}

// TestRemoteDockerHealthCheckDoesNotClaimAvailability stops the runtime from
// reporting healthy without any daemon behind it. The old stub's no-op health
// check made an unimplemented runtime look ready.
func TestRemoteDockerHealthCheckDoesNotClaimAvailability(t *testing.T) {
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	if err := exec.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck = %v; the runtime itself is a capability and should not fail startup", err)
	}
}

// TestRemoteDockerSurfacesTransportCause proves a transport failure keeps its
// cause through CreateInstance instead of collapsing into a generic error.
func TestRemoteDockerSurfacesTransportCause(t *testing.T) {
	wantErr := errors.New("ssh: handshake failed")
	exec := NewRemoteDockerExecutor(dialerTestLogger(t))
	exec.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
		return nil, wantErr
	}

	_, err := exec.CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		Metadata:   map[string]interface{}{MetadataKeySSHHost: "build-box"},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("CreateInstance error = %v, want it to wrap %v", err, wantErr)
	}
}
