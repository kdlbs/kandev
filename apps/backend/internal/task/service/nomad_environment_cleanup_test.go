package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type nomadInventoryEnvRepo struct {
	stubEnvRepo
	retained bool
	readErr  error
}

func (r *nomadInventoryEnvRepo) HasNomadEnvironmentInventory(context.Context, string) (bool, error) {
	return r.retained, r.readErr
}

type recordingNomadDestroyer struct {
	calls []string
	err   error
}

func (d *recordingNomadDestroyer) DestroyNomadEnvironment(_ context.Context, environmentID string) error {
	d.calls = append(d.calls, environmentID)
	return d.err
}

func nomadResetFixture(t *testing.T, retained bool, destroyErr error) (*Service, *nomadInventoryEnvRepo, *recordingNomadDestroyer) {
	t.Helper()
	repo := &nomadInventoryEnvRepo{retained: retained, stubEnvRepo: stubEnvRepo{env: &models.TaskEnvironment{
		ID: "env-nomad", TaskID: "task-1", ExecutorType: string(models.ExecutorTypeNomad), OwnershipGeneration: 1,
	}}}
	destroyer := &recordingNomadDestroyer{err: destroyErr}
	svc := newResetTestService(t, &stubEnvRepo{})
	svc.taskEnvironments = repo
	svc.SetSessionRunningChecker(&stubRunningChecker{})
	svc.SetEnvironmentDestroyer(&stubDestroyer{})
	svc.SetNomadEnvironmentDestroyer(destroyer)
	return svc, repo, destroyer
}

func TestNomadCleanupEvidenceResetRetainsEnvOnFailure(t *testing.T) {
	failure := errors.New("allocation absence unproven")
	svc, repo, destroyer := nomadResetFixture(t, true, failure)
	err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	require.ErrorIs(t, err, failure)
	require.Equal(t, []string{"env-nomad"}, destroyer.calls)
	require.False(t, repo.deleted, "environment row must be retained when Nomad cleanup fails")

	svc.nomadDestroyer = nil
	err = svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	require.Error(t, err)
	require.False(t, repo.deleted, "a missing destroyer must not allow deletion")
}

func TestNomadCleanupEvidenceResetDeletesEnvAfterSuccess(t *testing.T) {
	svc, repo, destroyer := nomadResetFixture(t, true, nil)
	require.NoError(t, svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{}))
	require.Equal(t, []string{"env-nomad"}, destroyer.calls)
	require.True(t, repo.deleted)
}

func TestNomadCleanupEvidenceAbsentInventoryIsNotDestroyed(t *testing.T) {
	svc, repo, destroyer := nomadResetFixture(t, false, errors.New("must not be called"))
	require.NoError(t, svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{}))
	require.Empty(t, destroyer.calls)
	require.True(t, repo.deleted)
}

func TestNomadCleanupEvidenceInventoryReadFailureRetainsEnv(t *testing.T) {
	svc, repo, destroyer := nomadResetFixture(t, false, nil)
	repo.readErr = errors.New("inventory unreadable")
	err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	require.ErrorIs(t, err, repo.readErr)
	require.Empty(t, destroyer.calls)
	require.False(t, repo.deleted)
}

func TestNomadCleanupEvidenceTaskDeleteCascadeRetainsOwner(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	failure := errors.New("allocation absence unproven")
	destroyer := &recordingNomadDestroyer{err: failure}
	svc.SetNomadEnvironmentDestroyer(destroyer)

	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-nomad", Title: "Nomad"}))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-nomad", TaskID: "task-nomad", ExecutorType: string(models.ExecutorTypeNomad),
		OwnershipGeneration: 1, Status: models.TaskEnvironmentStatusCreating, MaterializationSessionID: "session-nomad",
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-nomad", TaskID: "task-nomad", TaskEnvironmentID: "env-nomad", State: models.TaskSessionStateCreated,
	}))
	workspacePath, err := nomad.EnvironmentWorkspacePath("/var/lib/kandev/workspaces/env", "env-nomad", 1)
	require.NoError(t, err)
	metadata := map[string]interface{}{}
	require.NoError(t, nomad.PutEnvironmentState(metadata, nomad.EnvironmentState{
		Version: 1, EnvironmentID: "env-nomad", Generation: 1, OperationID: "exec-1", InstallationID: "installation-one",
		ExecutionID: "exec-1", InputDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		JobID: "job-1", Namespace: "default", Region: "global", Datacenter: "dc1", NodeID: "node-b",
		Platform: "darwin/arm64", SSHHost: "worker-b.example", SSHUser: "kandev", SSHPort: 22,
		SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		WorkspaceRoot:      "/var/lib/kandev/workspaces/env", WorkspacePath: workspacePath,
		AllocationRoot: "/var/lib/kandev/allocations/env", CPUMHz: 1500, MemoryMB: 4096, Phase: "intent",
	}))
	require.NoError(t, repo.CheckpointNomadExecutorInventory(ctx, &models.ExecutorRunning{
		ID: "session-nomad", SessionID: "session-nomad", TaskID: "task-nomad", ExecutorID: "exec-nomad",
		Runtime: agentruntime.RuntimeNomad, Status: models.ExecutorRunningStatusStarting, AgentExecutionID: "exec-1",
		Metadata: metadata,
	}))

	err = svc.DeleteTask(ctx, "task-nomad")
	require.ErrorIs(t, err, failure)
	require.Equal(t, []string{"env-nomad"}, destroyer.calls)
	_, err = repo.GetTaskSession(ctx, "session-nomad")
	require.NoError(t, err, "owner session must survive failed Nomad cleanup")
	_, err = repo.GetTaskEnvironment(ctx, "env-nomad")
	require.NoError(t, err, "owner environment must survive failed Nomad cleanup")

	// Without the service-level step the repository guard still refuses.
	require.ErrorIs(t, repo.DeleteTask(ctx, "task-nomad"), sqliterepo.ErrNomadInventoryRetained)
}
