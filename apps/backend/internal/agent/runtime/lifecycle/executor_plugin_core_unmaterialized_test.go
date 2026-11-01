package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
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

	_, err = fixture.recoverOne()
	require.ErrorIs(t, err, models.ErrExecutorPublishMismatch)
	require.Nil(t, fixture.operations.destroyRequest)
	require.Empty(t, fixture.store.failedMaterializations)
	require.NotNil(t, fixture.store.record)
}

// The production store must provide what the abandon path needs; otherwise the
// resume would defer for ever instead of failing the environment.
var (
	_ PluginExecutorMaterializationFailer = (*tasksqlite.Repository)(nil)
	_ pluginExecutorTaskRepositoryReader  = (*tasksqlite.Repository)(nil)
	_ pluginExecutorEnvironmentReader     = (*tasksqlite.Repository)(nil)
)

// recoverOne recovers the stored record and returns the error RecoverInstances
// would only log.
func (f *durableRecoveryFixture) recoverOne() (*ExecutorInstance, error) {
	return f.runtime.recoverPluginExecutor(context.Background(), f.store.record)
}

func storedInventory(t *testing.T, fixture *durableRecoveryFixture) pluginExecutorInventory {
	t.Helper()
	inventory, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	return inventory
}

func trackDestroys(fixture *durableRecoveryFixture) *int {
	destroys := 0
	fixture.operations.beforeDestroy = func() {
		destroys++
		fixture.store.events = append(fixture.store.events, "destroy")
	}
	return &destroys
}

func requireNeverAttached(t *testing.T, fixture *durableRecoveryFixture) {
	t.Helper()
	require.Empty(t, fixture.store.published, "a ready environment was published")
	require.Equal(t, -1, fixture.log.index("create_instance"), "an agentctl instance was created")
}

// The environment is failed before the resource is destroyed.
func TestAbandonFailsTheEnvironmentBeforeItDestroysTheResource(t *testing.T) {
	fixture, _ := repoBackedCoreFixture(t)
	trackDestroys(fixture)
	_, err := fixture.recover()
	require.NoError(t, err)
	require.Equal(t, []string{"fail", "destroy"}, fixture.store.events)
	require.Equal(t, []string{"environment-plugin-recovery@7"}, fixture.store.failScopes)
}

// If failing the environment errors, nothing has been destroyed: swapping the
// order would remove the row first and strand a creating environment.
func TestAbandonDoesNotDestroyWhenFailingTheEnvironmentErrors(t *testing.T) {
	fixture, _ := repoBackedCoreFixture(t)
	destroys := trackDestroys(fixture)
	fixture.store.failErr = errors.New("database is locked")

	_, err := fixture.recoverOne()
	require.ErrorContains(t, err, "database is locked")
	require.Zero(t, *destroys, "the resource was destroyed before the environment was failed")
	require.Equal(t, "core_workspace_not_materialized", storedInventory(t, fixture).AbandonReason)

	fixture.store.failErr = nil
	_, err = fixture.recoverOne()
	require.NoError(t, err)
	require.Equal(t, 1, *destroys)
	requireNeverAttached(t, fixture)
}

// A failer that reports the session no longer owns a creating environment is
// not an error: the resource is still destroyed.
func TestAbandonDestroysWhenTheEnvironmentIsNotOwnedAnyMore(t *testing.T) {
	fixture, destroys := repoBackedCoreFixture(t)
	fixture.store.failNotOwned = true
	_, err := fixture.recover()
	require.NoError(t, err)
	require.Equal(t, 1, *destroys)
}

// The environment is failed, then the destroy leaves no cleanup mark: a
// non-busy claim error, or a busy claim whose request could not be recorded.
// The abandon marker alone brings the next tick back to destroy it once,
// without attaching.
func TestAbandonIsFinishedByTheNextTickWhenTheDestroyLeavesNoCleanupMark(t *testing.T) {
	for name, claimErr := range map[string]error{
		"claim error": errors.New("database is locked"),
		"busy claim":  recoveryclaim.ErrBusy,
	} {
		t.Run(name, func(t *testing.T) {
			fixture, _ := repoBackedCoreFixture(t)
			destroys := trackDestroys(fixture)
			fixture.store.claimErr = claimErr

			_, err := fixture.recoverOne()
			require.Error(t, err)
			require.Zero(t, *destroys)
			require.Len(t, fixture.store.failedMaterializations, 1, "a crash between fail and destroy")
			require.Equal(t, "core_workspace_not_materialized", storedInventory(t, fixture).AbandonReason)

			fixture.store.claimErr = nil
			instances, err := fixture.recoverOne()
			require.NoError(t, err)
			require.Empty(t, instances)
			require.Equal(t, 1, *destroys)
			requireNeverAttached(t, fixture)
			requireSettledAbsent(t, fixture)
		})
	}
}

// A refused publication marks the record before failing the environment. When
// the destroy then leaves no cleanup mark, the record is still publish-pending
// at phase ready; the marker keeps the next tick from attaching or publishing.
func TestRefusedPublicationIsFinishedByTheNextTickWhenTheDestroyFails(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	destroys := trackDestroys(fixture)
	fixture.store.publishErr = &models.PluginPublishMismatchError{Reason: models.PluginPublishReasonRepos}
	fixture.store.onPublish = func() { fixture.store.claimErr = errors.New("database is locked") }

	_, err := fixture.recoverOne()
	require.Error(t, err)
	require.Zero(t, *destroys)
	inventory := storedInventory(t, fixture)
	require.Equal(t, "publish_refused", inventory.AbandonReason)
	require.True(t, inventory.PublishPending)
	require.Len(t, fixture.store.published, 1)

	fixture.store.claimErr = nil
	_, err = fixture.recoverOne()
	require.NoError(t, err)
	require.Equal(t, 1, *destroys)
	require.Len(t, fixture.store.published, 1, "the next tick published again")
	requireSettledAbsent(t, fixture)
}

func legacyPublishPendingFixture(t *testing.T) (*durableRecoveryFixture, *int) {
	t.Helper()
	fixture := newDurableRecoveryFixture(t, "ready")
	inventory := storedInventory(t, fixture)
	inventory.InstancePort = testPluginExecutorInstancePort
	inventory.PublishPending = true
	fixture.store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	fixture.store.record.TransientAuthToken = "agentctl-secret"
	fixture.store.record.TransientBootstrapNonce = ""
	fixture.operations.attachResponse = &pluginsdk.AttachExecutorEnvironmentResponse{Resource: inventory.Resource}
	fixture.operations.inspectResponses = []*pluginsdk.InspectExecutorEnvironmentResponse{{State: "running"}, {State: "running"}}
	fixture.runtime.operations = confirmingOperations{fixture.operations}
	return fixture, trackDestroys(fixture)
}

// A record written before the marker existed: still publish-pending, core, and
// either the task has repositories or its environment was already failed.
func TestLegacyPublishPendingRecordsThatCannotPublishAreAbandonedNotAttached(t *testing.T) {
	for name, setup := range map[string]func(*durableRecoveryFixture){
		"task has repositories": func(f *durableRecoveryFixture) {
			f.store.taskRepositories = []*models.TaskRepository{{ID: "tr-1", TaskID: "task-plugin-recovery", RepositoryID: "repo-1"}}
			f.store.environment = &models.TaskEnvironment{ID: "environment-plugin-recovery", Status: models.TaskEnvironmentStatusCreating}
		},
		"environment already failed": func(f *durableRecoveryFixture) {
			f.store.environment = &models.TaskEnvironment{ID: "environment-plugin-recovery", Status: models.TaskEnvironmentStatusFailed}
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, destroys := legacyPublishPendingFixture(t)
			setup(fixture)
			instances, err := fixture.recover()
			require.NoError(t, err)
			require.Empty(t, instances)
			require.Equal(t, 1, *destroys)
			require.Empty(t, fixture.store.published)
		})
	}
}

// The publication of a record that attached with PublishPending set is refused
// for good: the attach path abandons it instead of returning the mismatch for
// ever.
func TestAttachWithPendingPublicationAbandonsACoreLaunchRefusedForGood(t *testing.T) {
	fixture, destroys := legacyPublishPendingFixture(t)
	fixture.store.environment = &models.TaskEnvironment{ID: "environment-plugin-recovery", Status: models.TaskEnvironmentStatusCreating}
	fixture.store.publishErr = &models.PluginPublishMismatchError{Reason: models.PluginPublishReasonWorkspace}

	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Empty(t, instances)
	require.Equal(t, 1, *destroys)
	require.Equal(t, "publish_refused", fixture.operations.destroyRequest.GetCleanupReason())
	require.Equal(t, []string{"session-plugin-recovery"}, fixture.store.failedMaterializations)
}

// clearPublishPending only logs a failed write, so a published record can stay
// ready and publish-pending. Its environment is ready: a live session's
// resource must be neither failed nor destroyed.
func TestPublishPendingRecordOfAReadyEnvironmentIsNeverAbandoned(t *testing.T) {
	fixture, destroys := legacyPublishPendingFixture(t)
	fixture.store.taskRepositories = []*models.TaskRepository{{ID: "tr-1", TaskID: "task-plugin-recovery", RepositoryID: "repo-1"}}
	fixture.store.environment = &models.TaskEnvironment{ID: "environment-plugin-recovery", Status: models.TaskEnvironmentStatusReady}

	instances, err := fixture.recoverOne()
	require.NoError(t, err)
	if instances != nil {
		defer instances.Client.Close()
	}
	require.Zero(t, *destroys, "a live environment's resource was destroyed")
	require.Empty(t, fixture.store.failedMaterializations)
}

// Without a readable environment status the legacy rule defers instead of
// guessing.
func TestPublishPendingRecordWithAnUnreadableEnvironmentDefers(t *testing.T) {
	fixture, destroys := legacyPublishPendingFixture(t)
	fixture.store.taskRepositories = []*models.TaskRepository{{ID: "tr-1", TaskID: "task-plugin-recovery", RepositoryID: "repo-1"}}

	_, err := fixture.recoverOne()
	require.ErrorContains(t, err, "deferred")
	require.Zero(t, *destroys)
	require.Empty(t, fixture.store.failedMaterializations)
}

// The marker alone drives the next tick: once the task no longer has
// repositories the up-front rule would resume and publish, so only the stored
// marker keeps the launch from becoming ready.
func TestAbandonMarkerAloneEndsTheLaunchOnTheNextTick(t *testing.T) {
	fixture, _ := repoBackedCoreFixture(t)
	destroys := trackDestroys(fixture)
	fixture.store.claimErr = errors.New("database is locked")
	_, err := fixture.recoverOne()
	require.Error(t, err)
	require.Equal(t, "core_workspace_not_materialized", storedInventory(t, fixture).AbandonReason)

	fixture.store.claimErr = nil
	fixture.store.taskRepositories = nil
	_, err = fixture.recoverOne()
	require.NoError(t, err)
	require.Equal(t, 1, *destroys)
	require.Equal(t, "core_workspace_not_materialized", fixture.operations.destroyRequest.GetCleanupReason())
	requireNeverAttached(t, fixture)
}
