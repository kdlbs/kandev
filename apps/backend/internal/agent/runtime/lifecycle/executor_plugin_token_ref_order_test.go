package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// tokenRefRecorder models the durable inventory record's agentctl token
// reference. A checkpoint that carries the reference makes it durable and one
// that omits it leaves it in place, as the repository's merge does
// (task/repository/sqlite/executor_plugin_inventory.go, covered by
// plugin_secret_ref_checkpoint_test.go). Each checkpoint appends one event that
// states the reference then stored and whether the secret it names holds the
// token, so the order against create_instance is readable from the event log.
type tokenRefRecorder struct {
	log     *eventLog
	secrets *runtimeSecretsFake
	durable string
}

func (r *tokenRefRecorder) record(metadata map[string]interface{}) {
	if ref, ok := metadata[MetadataKeyAuthTokenSecret].(string); ok && ref != "" {
		r.durable = ref
	}
	holdsToken := r.durable != "" && r.secrets.values[r.durable] == durableToken
	r.log.add("checkpoint:token_ref=" + r.durable + ":holds_token=" + boolWord(holdsToken))
}

func boolWord(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// requireTokenRefDurableBeforeCreateInstance requires that, at the moment
// agentctl's CreateInstance ran, the last checkpoint before it had already made
// the reference to wantRef durable and the secret behind it held the token.
func requireTokenRefDurableBeforeCreateInstance(t *testing.T, log *eventLog, wantRef string) {
	t.Helper()
	log.mu.Lock()
	events := append([]string(nil), log.events...)
	log.mu.Unlock()
	create := -1
	for i, event := range events {
		if event == "create_instance" {
			create = i
			break
		}
	}
	require.GreaterOrEqual(t, create, 0, "agentctl CreateInstance was never called: %v", events)
	last := ""
	for _, event := range events[:create] {
		if strings.HasPrefix(event, "checkpoint:token_ref=") {
			last = event
		}
	}
	require.Equal(t, "checkpoint:token_ref="+wantRef+":holds_token=yes", last,
		"CreateInstance ran before the token reference was durably checkpointed; events: %v", events)
}

// TestLaunchCheckpointsTheTokenReferenceBeforeCreateInstance pins P2 for a
// provider that declares durable_bootstrap: the reference to the agentctl token
// secret is checkpointed before the fake agentctl is asked to create the instance.
func TestLaunchCheckpointsTheTokenReferenceBeforeCreateInstance(t *testing.T) {
	fixture := newDurableLaunchFixture(t, durableLaunchProvider())
	recorder := &tokenRefRecorder{log: fixture.log, secrets: fixture.secrets}
	fixture.request.CheckpointRuntimeInventory = func(_ context.Context, checkpoint map[string]interface{}) error {
		running, err := pluginExecutorLaunchRunning(fixture.request, checkpoint, nil)
		require.NoError(t, err)
		recorder.record(running.Metadata)
		return nil
	}

	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()

	wantRef := kubernetesRuntimeSecretID(fixture.request.InstanceID, pluginRuntimeSecretKindAuth)
	requireTokenRefDurableBeforeCreateInstance(t, fixture.log, wantRef)
	require.Equal(t, wantRef, recorder.durable)
}

// TestRecoveryCheckpointsTheTokenReferenceBeforeCreateInstance pins P2 for the
// durable-bootstrap recovery path, whether the token comes from a repeated
// handshake or is already held by the record.
func TestRecoveryCheckpointsTheTokenReferenceBeforeCreateInstance(t *testing.T) {
	for name, heldToken := range map[string]string{"repeated handshake": "", "token already held": durableToken} {
		t.Run(name, func(t *testing.T) {
			fixture := newDurableRecoveryFixture(t, "provisioned")
			fixture.store.record.TransientAuthToken = heldToken
			recorder := &tokenRefRecorder{log: fixture.log, secrets: fixture.secrets}
			fixture.store.onCheckpoint = func(incoming *models.ExecutorRunning) { recorder.record(incoming.Metadata) }

			instances, err := fixture.recover()
			require.NoError(t, err)
			require.Len(t, instances, 1)
			defer instances[0].Client.Close()

			wantRef := kubernetesRuntimeSecretID(fixture.store.record.AgentExecutionID, pluginRuntimeSecretKindAuth)
			requireTokenRefDurableBeforeCreateInstance(t, fixture.log, wantRef)
			require.Equal(t, wantRef, recorder.durable)
		})
	}
}
