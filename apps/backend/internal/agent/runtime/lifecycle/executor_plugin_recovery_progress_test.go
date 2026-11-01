package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// A provider may checkpoint recovery_progress while the host resumes a durable
// bootstrap under the recovery claim. That advances the stored revision, and
// the host's next write must follow it instead of wedging on a stale revision.
func TestResumeFollowsAProviderRecoveryProgressCheckpoint(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	fixture.store.record.TransientAuthToken = durableToken
	fixture.store.record.Metadata[MetadataKeyAuthTokenSecret] = "secret-token"
	inner := fixture.runtime.newTokenControlClient
	fixture.runtime.newTokenControlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, token string) (*agentctl.ControlClient, error) {
		stored, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
		require.NoError(t, err)
		stored.Phase = pluginExecutorPhaseRecoveryProgress
		stored.Revision++
		fixture.store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: stored}
		return inner(ctx, resolver, log, token)
	}

	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	defer instances[0].Client.Close()
	require.Len(t, fixture.store.published, 1)
	persisted, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	require.Equal(t, "ready", persisted.Phase)
}
