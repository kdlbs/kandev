package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"syscall"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// credential_recorded and handshake_rejected let drain_receipt@2 tell a
// handshake that left the host without a credential from one that may not have.

var drainV2Durable = []string{"drain_receipt@2", "durable_bootstrap@1"}

func boolRef(value bool) *bool { return &value }

// drainOutcomeOf is the outcome a destroy of a stored record with a started
// handshake and no credential carries.
func drainOutcomeOf(t *testing.T, negotiated []string, mutate func(*pluginExecutorInventory, *models.ExecutorRunning)) string {
	t.Helper()
	operations, _, drainer := destroyStoredRecord(t, negotiated, "task_cleanup", mutate)
	require.Empty(t, drainer.calls)
	return operations.destroyRequest.GetDrainOutcome()
}

func handshakeStarted(recorded *bool, rejected bool) func(*pluginExecutorInventory, *models.ExecutorRunning) {
	return func(inventory *pluginExecutorInventory, _ *models.ExecutorRunning) {
		inventory.HandshakeStarted, inventory.CredentialRecorded, inventory.HandshakeRejected = true, recorded, rejected
	}
}

func TestVersionTwoStartedHandshakeWithoutACredentialEverRecorded(t *testing.T) {
	require.Equal(t, pluginDrainOutcomeNotAttempted, drainOutcomeOf(t, drainV2Durable, handshakeStarted(boolRef(false), false)))
}

func TestVersionTwoStaysStrictWhereTheHostMayHaveHeldACredential(t *testing.T) {
	cases := []struct {
		name       string
		negotiated []string
		mutate     func(*pluginExecutorInventory, *models.ExecutorRunning)
	}{
		{"handshake rejected", drainV2Durable, handshakeStarted(boolRef(false), true)},
		{"credential recorded but the secret is unavailable", drainV2Durable, handshakeStarted(boolRef(true), false)},
		{"credential recorded, rejected too", drainV2Durable, handshakeStarted(boolRef(true), true)},
		{"non-durable provider with a started handshake", drainV2, handshakeStarted(boolRef(false), false)},
		{"record without the new fields", drainV2Durable, handshakeStarted(nil, false)},
		{"version 1 with a started handshake", []string{"drain_receipt@1", "durable_bootstrap@1"}, handshakeStarted(boolRef(false), false)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, pluginDrainOutcomeUnreachable, drainOutcomeOf(t, tc.negotiated, tc.mutate))
		})
	}
}

// A recorded token reference whose secret was lost leaves the host without a
// credential it knows it once held: strict, with the reference present.
func TestVersionTwoCredentialRecordedWithAnUnreadableSecretIsUnreachable(t *testing.T) {
	runtime, operations, store, _ := drainRuntimeFixture(t, drainV2Durable, func(inventory *pluginExecutorInventory, record *models.ExecutorRunning) {
		record.TransientAuthToken = ""
		handshakeStarted(boolRef(true), false)(inventory, record)
	})
	secrets := newRuntimeSecretsFake(&eventLog{})
	runtime.SetRuntimeSecretStore(secrets)
	store.record.Metadata[MetadataKeyAuthTokenSecret] = "kandev-runtime:execution-plugin-recovery:agentctl-auth"
	inventory, err := decodePluginExecutorInventory(store.record.Metadata)
	require.NoError(t, err)
	require.NoError(t, runtime.destroyPluginExecutorRecord(context.Background(), store.record, inventory, inventory.Resource, "task_cleanup", destroyProfile()))
	require.Equal(t, pluginDrainOutcomeUnreachable, operations.destroyRequest.GetDrainOutcome())
}

// --- launch: handshake failures before any token exists ---

func drainV2LaunchProvider(negotiated ...models.ExecutorProviderCapabilityRequirement) models.ExecutorProvider {
	provider := testPluginExecutorLaunchProvider()
	provider.RequiredCapabilities = negotiated
	return provider
}

func launchWithHandshakeError(t *testing.T, provider models.ExecutorProvider, handshakeErr error) (*durableLaunchFixture, pluginExecutorInventory) {
	t.Helper()
	fixture := newDurableLaunchFixture(t, provider)
	fixture.operations.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}
	fixture.runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	fixture.runtime.newAgentctlControlClient = func(context.Context, agentctl.ConnectionLeaseResolver, *logger.Logger, string) (*agentctl.ControlClient, string, error) {
		fixture.log.add("handshake")
		return nil, "", handshakeErr
	}
	_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.Error(t, err)
	require.NotNil(t, fixture.operations.destroyRequest, "the failed launch was not rolled back")
	require.NotEmpty(t, fixture.checkpoints)
	persisted, decodeErr := decodePluginExecutorInventory(fixture.checkpoints[len(fixture.checkpoints)-1])
	require.NoError(t, decodeErr)
	return fixture, persisted
}

var (
	requireDrainV2    = models.ExecutorProviderCapabilityRequirement{Name: "drain_receipt", Version: 2}
	requireDrainV1    = models.ExecutorProviderCapabilityRequirement{Name: "drain_receipt", Version: 1}
	requireDurableCap = models.ExecutorProviderCapabilityRequirement{Name: "durable_bootstrap", Version: 1}
)

func TestVersionTwoHandshakeFailureBeforeATokenExistsIsNotAttempted(t *testing.T) {
	for name, handshakeErr := range map[string]error{
		"end of stream":      io.EOF,
		"deadline":           context.DeadlineExceeded,
		"connection refused": fmt.Errorf("dial: %w", syscall.ECONNREFUSED),
	} {
		t.Run(name, func(t *testing.T) {
			fixture, persisted := launchWithHandshakeError(t, drainV2LaunchProvider(requireDrainV2, requireDurableCap), handshakeErr)
			request := fixture.operations.destroyRequest
			require.Equal(t, pluginDrainOutcomeNotAttempted, request.GetDrainOutcome())
			require.Equal(t, launchCleanupReason("control_handshake"), request.GetCleanupReason())
			require.Equal(t, pluginDrainOutcomeNotAttempted, persisted.DrainOutcome, "the outcome is persisted with the destroy")
			require.True(t, persisted.HandshakeStarted)
			require.NotNil(t, persisted.CredentialRecorded)
			require.False(t, *persisted.CredentialRecorded)
			require.False(t, persisted.HandshakeRejected)
		})
	}
}

func TestVersionOneHandshakeFailureStaysUnreachable(t *testing.T) {
	fixture, persisted := launchWithHandshakeError(t, drainV2LaunchProvider(requireDrainV1, requireDurableCap), io.EOF)
	require.Equal(t, pluginDrainOutcomeUnreachable, fixture.operations.destroyRequest.GetDrainOutcome())
	require.Nil(t, persisted.CredentialRecorded, "a version 1 record is written exactly as before")
}

func TestVersionTwoWithoutDurableBootstrapStaysUnreachableAfterAHandshake(t *testing.T) {
	fixture, _ := launchWithHandshakeError(t, drainV2LaunchProvider(requireDrainV2), io.EOF)
	require.Equal(t, pluginDrainOutcomeUnreachable, fixture.operations.destroyRequest.GetDrainOutcome())
}

func TestRejectedHandshakeIsUnreachableNowAndOnEveryLaterDestroy(t *testing.T) {
	rejected := fmt.Errorf("%w: nonce spent", agentctl.ErrHandshakeRejected)
	fixture, persisted := launchWithHandshakeError(t, drainV2LaunchProvider(requireDrainV2, requireDurableCap), rejected)
	require.Equal(t, pluginDrainOutcomeUnreachable, fixture.operations.destroyRequest.GetDrainOutcome())
	require.True(t, persisted.HandshakeRejected, "the rejection must be durable")

	require.Equal(t, pluginDrainOutcomeUnreachable, drainOutcomeOf(t, drainV2Durable, func(inventory *pluginExecutorInventory, _ *models.ExecutorRunning) {
		inventory.HandshakeStarted, inventory.CredentialRecorded, inventory.HandshakeRejected = persisted.HandshakeStarted, persisted.CredentialRecorded, persisted.HandshakeRejected
	}))
}

// --- crash at the handshake barrier ---

// A backend killed after agentctl accepted the handshake and before the token
// was persisted leaves a record with a started handshake and no credential. A
// destroy after the restart finds no instance can exist.
func TestCrashAtTheHandshakeBarrierThenDestroyIsNotAttempted(t *testing.T) {
	set := installCrashingBarriers(t)
	fixture := newDurableLaunchFixture(t, drainV2LaunchProvider(requireDrainV2, requireDurableCap))
	require.NoError(t, set.arm(BarrierAfterHandshake, CrashBarrierModeTerminate))
	require.True(t, crashAt(func() { _, _ = fixture.runtime.CreateInstance(context.Background(), fixture.request) }), "the barrier did not fire")
	require.Equal(t, -1, fixture.log.index("create_instance"), "an instance was created past the crash point")
	require.NotContains(t, fixture.secrets.values, "kandev-runtime:execution-plugin-1:agentctl-auth", "the token was persisted past the crash point")

	persisted, err := decodePluginExecutorInventory(fixture.checkpoints[len(fixture.checkpoints)-1])
	require.NoError(t, err)
	require.True(t, persisted.HandshakeStarted)
	require.Equal(t, drainV2Durable, persisted.NegotiatedCapabilities)

	require.Equal(t, pluginDrainOutcomeNotAttempted, drainOutcomeOf(t, drainV2Durable, func(inventory *pluginExecutorInventory, _ *models.ExecutorRunning) {
		inventory.HandshakeStarted, inventory.CredentialRecorded, inventory.HandshakeRejected = persisted.HandshakeStarted, persisted.CredentialRecorded, persisted.HandshakeRejected
	}))
}

// The restarted backend's recovery repeats the handshake with the spent nonce.
// That rejection is the strict case and is recorded before the cleanup.
func TestCrashAtTheHandshakeBarrierThenRecoveryRejectionIsStrict(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	fixture.store.record.Metadata = metadataWith(t, fixture.store.record.Metadata, func(inventory *pluginExecutorInventory) {
		inventory.NegotiatedCapabilities = []string{"drain_receipt@2", "durable_bootstrap@1"}
		inventory.HandshakeStarted, inventory.CredentialRecorded = true, boolRef(false)
	})
	fixture.handshake = func() error { return fmt.Errorf("%w: spent", agentctl.ErrHandshakeRejected) }
	fixture.store.onCheckpoint = nil
	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Empty(t, instances)
	require.Equal(t, "bootstrap_consumed", fixture.operations.destroyRequest.GetCleanupReason())
	require.Equal(t, pluginDrainOutcomeUnreachable, fixture.operations.destroyRequest.GetDrainOutcome())
	persisted, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	require.True(t, persisted.HandshakeRejected)
}

func metadataWith(t *testing.T, metadata map[string]interface{}, change func(*pluginExecutorInventory)) map[string]interface{} {
	t.Helper()
	inventory, err := decodePluginExecutorInventory(metadata)
	require.NoError(t, err)
	change(&inventory)
	out := clonePluginExecutorMetadata(metadata)
	out[MetadataKeyPluginExecutor] = inventory
	return out
}

// --- the credential is recorded in the checkpoint that names the secret ---

// requireCredentialRecordedBeforeCreateInstance requires the last checkpoint
// before agentctl's CreateInstance to already state credential_recorded.
func requireCredentialRecordedBeforeCreateInstance(t *testing.T, log *eventLog) {
	t.Helper()
	log.mu.Lock()
	events := append([]string(nil), log.events...)
	log.mu.Unlock()
	last, create := "", -1
	for i, event := range events {
		if event == "create_instance" {
			create = i
			break
		}
		if event == "checkpoint:credential_recorded=true" || event == "checkpoint:credential_recorded=false" {
			last = event
		}
	}
	require.GreaterOrEqual(t, create, 0, "agentctl CreateInstance was never called: %v", events)
	require.Equal(t, "checkpoint:credential_recorded=true", last,
		"CreateInstance ran before credential_recorded was durable; events: %v", events)
}

func credentialEvent(inventory pluginExecutorInventory) string {
	return fmt.Sprintf("checkpoint:credential_recorded=%t", inventory.CredentialRecorded != nil && *inventory.CredentialRecorded)
}

func TestLaunchRecordsTheCredentialInTheCheckpointBeforeCreateInstance(t *testing.T) {
	fixture := newDurableLaunchFixture(t, drainV2LaunchProvider(requireDrainV2, requireDurableCap))
	inner := fixture.request.CheckpointRuntimeInventory
	fixture.request.CheckpointRuntimeInventory = func(ctx context.Context, checkpoint map[string]interface{}) error {
		running, err := pluginExecutorLaunchRunning(fixture.request, checkpoint, nil)
		require.NoError(t, err)
		inventory, err := decodePluginExecutorInventory(running.Metadata)
		require.NoError(t, err)
		fixture.log.add(credentialEvent(inventory))
		return inner(ctx, checkpoint)
	}
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()
	requireCredentialRecordedBeforeCreateInstance(t, fixture.log)
	last, err := decodePluginExecutorInventory(fixture.checkpoints[len(fixture.checkpoints)-1])
	require.NoError(t, err)
	require.NotNil(t, last.CredentialRecorded)
	require.True(t, *last.CredentialRecorded, "credential_recorded is sticky once set")
}

func TestRecoveryRecordsTheCredentialInTheCheckpointBeforeCreateInstance(t *testing.T) {
	for name, heldToken := range map[string]string{"repeated handshake": "", "token already held": durableToken} {
		t.Run(name, func(t *testing.T) {
			fixture := newDurableRecoveryFixture(t, "provisioned")
			fixture.store.record.Metadata = metadataWith(t, fixture.store.record.Metadata, func(inventory *pluginExecutorInventory) {
				inventory.NegotiatedCapabilities = []string{"drain_receipt@2", "durable_bootstrap@1"}
				inventory.CredentialRecorded = boolRef(false)
			})
			fixture.store.record.TransientAuthToken = heldToken
			fixture.store.onCheckpoint = func(incoming *models.ExecutorRunning) {
				inventory, err := decodePluginExecutorInventory(incoming.Metadata)
				require.NoError(t, err)
				fixture.log.add(credentialEvent(inventory))
			}
			instances, err := fixture.recover()
			require.NoError(t, err)
			require.Len(t, instances, 1)
			defer instances[0].Client.Close()
			requireCredentialRecordedBeforeCreateInstance(t, fixture.log)
		})
	}
}

func TestEnvelopeFieldsRoundTripAndAreOmittedWhenUnset(t *testing.T) {
	data, err := json.Marshal(pluginExecutorInventory{HandshakeStarted: true})
	require.NoError(t, err)
	require.NotContains(t, string(data), "credential_recorded")
	require.NotContains(t, string(data), "handshake_rejected")

	data, err = json.Marshal(pluginExecutorInventory{CredentialRecorded: boolRef(false), HandshakeRejected: true})
	require.NoError(t, err)
	var back pluginExecutorInventory
	require.NoError(t, json.Unmarshal(data, &back))
	require.NotNil(t, back.CredentialRecorded)
	require.False(t, *back.CredentialRecorded, "an explicit false is kept distinct from an absent field")
	require.True(t, back.HandshakeRejected)
	require.Contains(t, string(data), `"credential_recorded":false`)

	absent, err := decodePluginExecutorInventory(map[string]interface{}{MetadataKeyPluginExecutor: map[string]interface{}{"handshake_started": true}})
	require.NoError(t, err)
	require.Nil(t, absent.CredentialRecorded)
}
