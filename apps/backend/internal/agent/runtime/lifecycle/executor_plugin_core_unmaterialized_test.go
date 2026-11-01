package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func repoBackedCoreFixture(t *testing.T) (*durableRecoveryFixture, *int) {
	t.Helper()
	fixture := newDurableRecoveryFixture(t, "provisioned")
	fixture.store.taskRepositories = []*models.TaskRepository{{ID: "tr-1", TaskID: "task-plugin-recovery", RepositoryID: "repo-1"}}
	destroys := 0
	fixture.operations.beforeDestroy = func() { destroys++ }
	return fixture, &destroys
}

// requireSettledAbsent checks that the destroy was confirmed and recorded: the
// row (kept because the session is live) is absent, so nothing is left to
// recover.
func requireSettledAbsent(t *testing.T, fixture *durableRecoveryFixture) {
	t.Helper()
	if fixture.store.record == nil {
		return
	}
	inventory, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	require.Equal(t, pluginExecutorPhaseAbsent, inventory.Phase)
}

// A durable resume of a core-prepared, repo-backed launch can never be valid:
// core materializes the repositories through agentctl after the instance exists
// and a recovery never does, so /workspace would be empty. The resource is
// destroyed once, the environment failed so a relaunch can re-elect, and nothing
// is left for the next recovery tick.
func TestDurableResumeOfACoreRepoBackedLaunchDestroysAndFailsTheEnvironment(t *testing.T) {
	fixture, destroys := repoBackedCoreFixture(t)

	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Empty(t, instances)
	require.Equal(t, 1, *destroys, "the resource must be destroyed exactly once")
	require.Equal(t, "core_workspace_not_materialized", fixture.operations.destroyRequest.GetCleanupReason())
	require.Equal(t, []string{"session-plugin-recovery"}, fixture.store.failedMaterializations)
	require.Empty(t, fixture.store.published, "a ready environment was published")
	require.Equal(t, -1, fixture.log.index("create_instance"), "an agentctl instance was created")
	requireSettledAbsent(t, fixture)

	next, err := fixture.store.ListExecutorsRunningPluginRemote(context.Background())
	require.NoError(t, err)
	_, err = fixture.runtime.RecoverInstances(context.Background(), next)
	require.NoError(t, err)
	require.Equal(t, 1, *destroys, "the next recovery tick retried")
	require.Len(t, fixture.store.failedMaterializations, 1)
}

func TestDurableResumeOfACoreRepoLessLaunchStillPublishes(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	defer instances[0].Client.Close()
	require.Len(t, fixture.store.published, 1)
	require.Empty(t, fixture.store.failedMaterializations)
}

// A repo-backed task on a provider-prepared workspace is not affected.
func TestDurableResumeOfAProviderPreparedRepoBackedLaunchIsNotAbandoned(t *testing.T) {
	fixture, destroys := repoBackedCoreFixture(t)
	inventory, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	inventory.WorkspacePreparation = pluginWorkspacePreparationProvider
	fixture.store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}

	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	defer instances[0].Client.Close()
	require.Zero(t, *destroys)
	require.Empty(t, fixture.store.failedMaterializations)
}

// A core-mode refusal on repositories or workspace cannot heal, so the resume
// ends it: destroy once, fail the environment, and leave nothing to retry.
func TestDurableResumeEndsACoreLaunchWhosePublicationIsRefusedForGood(t *testing.T) {
	for _, reason := range []string{models.PluginPublishReasonRepos, models.PluginPublishReasonWorkspace} {
		t.Run(reason, func(t *testing.T) {
			fixture := newDurableRecoveryFixture(t, "provisioned")
			destroys := 0
			fixture.operations.beforeDestroy = func() { destroys++ }
			fixture.store.publishErr = &models.PluginPublishMismatchError{Reason: reason}

			instances, err := fixture.recover()
			require.NoError(t, err)
			require.Empty(t, instances)
			require.Equal(t, 1, destroys)
			require.Equal(t, "publish_refused", fixture.operations.destroyRequest.GetCleanupReason())
			require.Equal(t, []string{"session-plugin-recovery"}, fixture.store.failedMaterializations)
			requireSettledAbsent(t, fixture)
		})
	}
}

// Provider-mode refusals keep their behaviour: the error is returned and the
// record stays for the next tick.
func TestDurableResumeLeavesAProviderModePublishRefusalRetryable(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	inventory, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	inventory.WorkspacePreparation = pluginWorkspacePreparationProvider
	fixture.store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	fixture.store.publishErr = &models.PluginPublishMismatchError{Reason: models.PluginPublishReasonRepos}

	_, _ = fixture.recover()
	require.Nil(t, fixture.operations.destroyRequest)
	require.Empty(t, fixture.store.failedMaterializations)
	require.NotNil(t, fixture.store.record)
}

// The production store must provide what the abandon path needs; otherwise the
// resume would defer for ever instead of failing the environment.
var (
	_ PluginExecutorMaterializationFailer = (*tasksqlite.Repository)(nil)
	_ pluginExecutorTaskRepositoryReader  = (*tasksqlite.Repository)(nil)
)
