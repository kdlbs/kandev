package orchestrator

import (
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/stretchr/testify/require"
)

func TestPrepareAgentDeliverySubmissionSupportedNonDurablePeer(t *testing.T) {
	for _, tc := range []struct {
		name       string
		version    uint32
		reason     string
		protocol   string
		unresolved bool
		wantBlock  string
	}{
		{name: "supported_no_retained_volume", version: journal.CurrentVersion, reason: durableDeliveryStorageNotDurable},
		{name: "existing_durable_protocol", version: journal.CurrentVersion, reason: durableDeliveryStorageNotDurable, protocol: messagequeue.DeliveryProtocolV1, wantBlock: "legacy_peer"},
		{name: "unresolved_no_retained_volume", version: journal.CurrentVersion, reason: durableDeliveryStorageNotDurable, unresolved: true, wantBlock: durableDeliveryUnresolvedReason},
		{name: "future_version", version: journal.CurrentVersion + 1, reason: durableDeliveryStorageNotDurable, wantBlock: "incompatible_delivery_version"},
		{name: "journal_lost", version: journal.CurrentVersion, reason: "journal_lost", wantBlock: "incompatible_delivery_version"},
		{name: "storage_unavailable", version: journal.CurrentVersion, reason: "storage_unavailable", wantBlock: "incompatible_delivery_version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "task-nondurable", "session-nondurable", "step-1")
			session, err := repo.GetTaskSession(t.Context(), "session-nondurable")
			require.NoError(t, err)
			manager := &durableDeliveryTestAgentManager{
				mockAgentManager: &mockAgentManager{}, advertised: true,
				capability: agentruntime.DurableDeliveryCapability{
					Version: tc.version, Reason: tc.reason, Unresolved: tc.unresolved,
				},
			}
			service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			runtime, err := service.prepareAgentDeliverySubmission(t.Context(), session, "execution-1", "hello", nil, promptTaskOptions{
				deliveryProtocol: tc.protocol, deliverySubmissionID: "queue:nondurable",
			})
			if tc.wantBlock == "" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrSessionRecoveryRequired)
				block, blockErr := service.GetOpenSessionRecoveryBlock(t.Context(), session.ID)
				require.NoError(t, blockErr)
				require.NotNil(t, block)
				require.Equal(t, tc.wantBlock, block.Reason)
			}
			require.Nil(t, runtime)
			submissions, err := repo.ListAgentDeliverySubmissions(t.Context(), session.ID)
			require.NoError(t, err)
			require.Empty(t, submissions)
		})
	}
}
