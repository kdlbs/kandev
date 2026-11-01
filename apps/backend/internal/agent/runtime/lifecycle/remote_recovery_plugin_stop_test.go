package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

// pluginStopWriter is the executors_running writer of a backend that restarted
// over a stored plugin_remote record: reads go to the stored inventory, and it
// counts the deletions the lifecycle manager asks for.
type pluginStopWriter struct {
	*pluginExecutorInventoryStoreFake
	deletes int
}

func (w *pluginStopWriter) UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error {
	return nil
}

func (w *pluginStopWriter) DeleteExecutorRunningBySessionID(context.Context, string) error {
	w.deletes++
	return nil
}

func (w *pluginStopWriter) RepairExecutorRunningDead(context.Context, string) error { return nil }

const pluginStopTokenSecret = "kandev-runtime:execution-plugin-recovery:agentctl-auth"

type pendingPluginStopFixture struct {
	manager    *Manager
	writer     *pluginStopWriter
	runtime    *PluginRemoteExecutor
	operations *pluginExecutorOperationsFake
	store      *pluginExecutorInventoryStoreFake
	drainer    *pluginDrainerFake
	secrets    *inMemorySecretStore
	row        *models.ExecutorRunning
}

// newPendingPluginStopFixture is a backend that restarted while agentctl was
// down: the stored record has no in-memory credential, the provider cannot
// inspect the environment, and the session waits in the pending recovery queue.
func newPendingPluginStopFixture(t *testing.T, negotiated []string, tokenSecret string, mutate func(*pluginExecutorInventory)) pendingPluginStopFixture {
	t.Helper()
	runtime, operations, store, drainer := drainRuntimeFixture(t, negotiated, func(inventory *pluginExecutorInventory, record *models.ExecutorRunning) {
		record.TransientAuthToken = ""
		record.Status = models.ExecutorRunningStatusRunning
		if mutate != nil {
			mutate(inventory)
		}
	})
	operations.inspectErr = errors.New("executor tunnel unavailable")
	secretStore := newInMemorySecretStore()
	if tokenSecret != "" {
		store.record.Metadata[MetadataKeyAuthTokenSecret] = pluginStopTokenSecret
		require.NoError(t, secretStore.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: pluginStopTokenSecret, Name: "agentctl-auth"}, Value: tokenSecret,
		}))
	}
	runtime.SetRuntimeSecretStore(secretStore)
	manager, _ := task08ManagerWithBackend(t, runtime)
	writer := &pluginStopWriter{pluginExecutorInventoryStoreFake: store}
	manager.SetExecutorRunningWriter(writer)
	manager.SetSecretStore(secretStore)
	row := cloneRemoteRecoveryRecord(store.record)
	manager.workspaceInfoProvider = &task08RemoteSnapshotProvider{snapshots: map[string]*RemoteRecoverySnapshot{row.SessionID: task08RemoteSnapshot(row)}}
	require.NotNil(t, manager.queueRemoteRecovery(row, task08RemoteSnapshot(row)))
	return pendingPluginStopFixture{manager: manager, writer: writer, runtime: runtime, operations: operations, store: store, drainer: drainer, secrets: secretStore, row: row}
}

func (f pendingPluginStopFixture) stop(reason string) error {
	return f.manager.StopAgentWithReason(context.Background(), f.row.AgentExecutionID, reason, false)
}

func (f pendingPluginStopFixture) destroyEnvironment() error {
	return f.manager.DestroyPluginExecutorEnvironment(context.Background(), &models.TaskEnvironment{
		ID: "environment-plugin-recovery", TaskID: "task-plugin-recovery", OwnershipGeneration: 7,
	})
}

func (f pendingPluginStopFixture) pendingCount() int {
	f.manager.remoteRecoveryMu.Lock()
	defer f.manager.remoteRecoveryMu.Unlock()
	return len(f.manager.remoteRecoveryPending)
}

// A cleanup stop that cannot reach agentctl has nothing left to stop: the
// environment cleanup is the authority that drains, quarantines and destroys,
// so the stop completes and leaves the stored record for it.
func TestPendingPluginStopForCleanupDefersToEnvironmentDestroyWhenAgentctlIsUnreachable(t *testing.T) {
	for _, reason := range []string{StopReasonTaskArchived, StopReasonTaskDeleted, StopReasonCascadeArchive, StopReasonTaskTreeDeleted} {
		t.Run(reason, func(t *testing.T) {
			fixture := newPendingPluginStopFixture(t, drainV2Durable, "agentctl-token", func(inventory *pluginExecutorInventory) {
				inventory.HandshakeStarted, inventory.CredentialRecorded = true, boolRef(true)
			})

			require.NoError(t, fixture.stop(reason))

			require.Zero(t, fixture.pendingCount(), "a completed stop must leave the pending recovery queue, not retry forever")
			require.Zero(t, fixture.writer.deletes, "the record is the environment cleanup's input and survives the stop")
			require.Nil(t, fixture.operations.destroyRequest, "the stop itself never destroys the environment")
			require.NotNil(t, fixture.store.record)
		})
	}
}

// Stops that do not end the environment keep waiting for agentctl.
func TestPendingPluginStopWithoutCleanupStaysRetryableWhenAgentctlIsUnreachable(t *testing.T) {
	fixture := newPendingPluginStopFixture(t, drainV2Durable, "agentctl-token", nil)

	require.Error(t, fixture.stop(StopReasonIdleSuspension))
	require.Equal(t, 1, fixture.pendingCount())
	require.Zero(t, fixture.writer.deletes)
}

// The restart scenario end to end: stop, then the cleanup's destroy over the
// same stored record, for each way the credential can look after the restart.
func TestPendingPluginStopThenDestroyCarriesTheDrainOutcomeTheCredentialStateProves(t *testing.T) {
	cases := []struct {
		name        string
		token       string
		mutate      func(*pluginExecutorInventory)
		want        string
		drainCalled bool
	}{
		{
			name: "credential recorded and the secret is lost", token: "",
			mutate: func(inventory *pluginExecutorInventory) {
				inventory.HandshakeStarted, inventory.CredentialRecorded = true, boolRef(true)
			},
			want: pluginDrainOutcomeUnreachable,
		},
		{
			name: "credential never recorded", token: "",
			mutate: func(inventory *pluginExecutorInventory) {
				inventory.HandshakeStarted, inventory.CredentialRecorded = true, boolRef(false)
			},
			want: pluginDrainOutcomeNotAttempted,
		},
		{
			name: "credential readable", token: "agentctl-token",
			mutate: func(inventory *pluginExecutorInventory) {
				inventory.HandshakeStarted, inventory.CredentialRecorded = true, boolRef(true)
			},
			want: pluginDrainOutcomeDrained, drainCalled: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newPendingPluginStopFixture(t, drainV2Durable, tc.token, tc.mutate)
			if tc.token == "" && tc.want == pluginDrainOutcomeUnreachable {
				fixture.store.record.Metadata[MetadataKeyAuthTokenSecret] = pluginStopTokenSecret
			}
			var persistedAtDestroy pluginExecutorInventory
			fixture.operations.beforeDestroy = func() {
				persistedAtDestroy, _ = decodePluginExecutorInventory(fixture.store.record.Metadata)
			}

			require.NoError(t, fixture.stop(StopReasonTaskArchived))
			require.NoError(t, fixture.destroyEnvironment())

			require.NotNil(t, fixture.operations.destroyRequest, "the cleanup must reach the provider's destroy")
			require.Equal(t, tc.want, fixture.operations.destroyRequest.GetDrainOutcome())
			require.Equal(t, tc.want, persistedAtDestroy.DrainOutcome, "the outcome is persisted before the destroy call")
			require.Equal(t, tc.drainCalled, len(fixture.drainer.calls) > 0)
			require.Zero(t, fixture.pendingCount())
		})
	}
}
