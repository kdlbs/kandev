package service

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

func remoteDockerAdminContext() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "admin-1",
		Role:   authn.RoleAdmin,
	})
}

func validRemoteDockerExecutorConfig() map[string]string {
	return map[string]string{
		sshMetaHost:            "10.0.0.5",
		sshMetaUser:            "root",
		sshMetaPort:            "22",
		sshMetaIdentitySource:  "file",
		sshMetaIdentityFile:    "~/.ssh/id_ed25519",
		sshMetaHostFingerprint: "SHA256:abc",
	}
}

// createRetainedRemoteDockerExecutor stores a remote Docker executor and a
// running row that stands for a container preserved by an ordinary stop.
func createRetainedRemoteDockerExecutor(t *testing.T, svc *Service, repo interface {
	UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error
}) *models.Executor {
	t.Helper()
	executor, err := svc.CreateExecutor(remoteDockerAdminContext(), &CreateExecutorRequest{
		Name: "prod-box", Type: models.ExecutorTypeRemoteDocker,
		Status: models.ExecutorStatusActive, Config: validRemoteDockerExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		SessionID: "session-retained", TaskID: "task-retained", ExecutorID: executor.ID,
		Runtime: agentruntime.RuntimeRemoteDocker,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	return executor
}

// TestUpdateRemoteDockerExecutorRejectsConnectionChangeWhileContainersAreRetained
// covers a review finding. An ordinary stop preserves the remote container,
// and every later inspect, resume, and teardown reaches it through the
// executor's *current* connection. Repointing that connection strands the
// container on the original host with nothing left pointing at it.
func TestUpdateRemoteDockerExecutorRejectsConnectionChangeWhileContainersAreRetained(t *testing.T) {
	svc, _, repo := createTestService(t)
	executor := createRetainedRemoteDockerExecutor(t, svc, repo)

	moved := validRemoteDockerExecutorConfig()
	moved[sshMetaHost] = "10.0.0.99"

	_, err := svc.UpdateExecutor(remoteDockerAdminContext(), executor.ID, &UpdateExecutorRequest{
		Config: moved,
	})

	if !errors.Is(err, ErrActiveTaskSessions) {
		t.Fatalf("UpdateExecutor() error = %v, want ErrActiveTaskSessions", err)
	}
	stored, getErr := repo.GetExecutor(context.Background(), executor.ID)
	if getErr != nil {
		t.Fatalf("GetExecutor: %v", getErr)
	}
	if stored.Config[sshMetaHost] != "10.0.0.5" {
		t.Fatalf("blocked update still repointed the daemon to %q",
			stored.Config[sshMetaHost])
	}
}

// A retained container must not make a legitimate host-key rotation
// unrecoverable. The daemon target stays the same, while an administrator
// explicitly replaces the trusted fingerprint after re-testing the host.
func TestUpdateRemoteDockerExecutorAllowsFingerprintChangeWhileContainersAreRetained(t *testing.T) {
	svc, _, repo := createTestService(t)
	executor := createRetainedRemoteDockerExecutor(t, svc, repo)

	rotated := validRemoteDockerExecutorConfig()
	rotated[sshMetaHostFingerprint] = "SHA256:def"

	updated, err := svc.UpdateExecutor(remoteDockerAdminContext(), executor.ID, &UpdateExecutorRequest{
		Config: rotated,
	})
	if err != nil {
		t.Fatalf("UpdateExecutor() error = %v, want nil for an explicit fingerprint rotation", err)
	}
	if got := updated.Config[sshMetaHostFingerprint]; got != "SHA256:def" {
		t.Fatalf("updated fingerprint = %q, want SHA256:def", got)
	}
}

// A rename touches no connection field, so it must stay allowed: the guard
// exists to protect reachability, not to freeze the row.
func TestUpdateRemoteDockerExecutorAllowsNonConnectionChangeWhileContainersAreRetained(t *testing.T) {
	svc, _, repo := createTestService(t)
	executor := createRetainedRemoteDockerExecutor(t, svc, repo)

	renamed := "prod-box (eu-west)"
	updated, err := svc.UpdateExecutor(remoteDockerAdminContext(), executor.ID, &UpdateExecutorRequest{
		Name: &renamed,
	})
	if err != nil {
		t.Fatalf("UpdateExecutor() error = %v, want nil for a rename", err)
	}
	if updated.Name != renamed {
		t.Fatalf("name = %q, want %q", updated.Name, renamed)
	}
}

// Deletion soft-deletes the row, so the lookup that resume and Reset
// Environment depend on fails afterwards and the container becomes
// unreachable by any Kandev path.
func TestDeleteRemoteDockerExecutorRejectsWhileContainersAreRetained(t *testing.T) {
	svc, _, repo := createTestService(t)
	executor := createRetainedRemoteDockerExecutor(t, svc, repo)

	err := svc.DeleteExecutor(remoteDockerAdminContext(), executor.ID)

	if !errors.Is(err, ErrActiveTaskSessions) {
		t.Fatalf("DeleteExecutor() error = %v, want ErrActiveTaskSessions", err)
	}
	if _, getErr := repo.GetExecutor(context.Background(), executor.ID); getErr != nil {
		t.Fatalf("executor was deleted despite a retained container: %v", getErr)
	}
}
