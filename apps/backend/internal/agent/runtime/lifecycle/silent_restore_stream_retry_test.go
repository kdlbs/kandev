package lifecycle

import (
	"context"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type silentRestoreStreamRecoverer interface {
	RecoverSilentRestoreCandidateStream(context.Context, string, string, string) error
}

func silentRestoreSetupDescriptor(e *AgentExecution) *agentctl.DeliveryStatus {
	return &agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true, Unresolved: true},
		SessionID:         e.SessionID, IncarnationID: e.DeliveryIncarnationID,
		HarnessGeneration: e.DeliveryHarnessGeneration, StreamID: e.DeliveryStreamID,
		Stream: &journal.Stream{
			SessionID: e.SessionID, IncarnationID: e.DeliveryIncarnationID,
			HarnessGeneration: e.DeliveryHarnessGeneration, StreamID: e.DeliveryStreamID,
			FirstRetained: 1, HighWater: 1,
		},
	}
}

func TestSilentRestoreStreamRetryReplaysOnlyIntactSetup(t *testing.T) {
	for _, scenario := range []string{"intact", "pruned", "acknowledged", "foreign native", "foreign generation", "uninitialized", "submission"} {
		t.Run(scenario, func(t *testing.T) {
			e, g := silentRestoreDeliveryOwner()
			e.ACPSessionID = e.RequiredNativeConversationID
			e.setSessionInitialized(true)
			status := silentRestoreSetupDescriptor(e)
			switch scenario {
			case "pruned":
				status.Stream.FirstRetained = 2
			case "acknowledged":
				status.Stream.Acknowledged = 1
			case "foreign native":
				g.NativeSessionID = "foreign"
			case "foreign generation":
				g.Generation = 3
			case "uninitialized":
				e.setSessionInitialized(false)
			case "submission":
				status.SubmissionCount = 1
			}
			repo := &adoptionDeliveryRepository{generation: g}
			cursor, err := authenticatedDeliveryReplayCursor(t.Context(), e, repo, status)
			if scenario == "intact" {
				require.NoError(t, err)
				require.Zero(t, cursor)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestSilentRestoreStreamRetryRecoversExactInitializedCandidate(t *testing.T) {
	for _, scenario := range []string{"matching", "different execution", "different native", "uninitialized"} {
		t.Run(scenario, func(t *testing.T) {
			mock := newMockAgentServer(t)
			defer mock.Close()
			client := createTestClient(t, mock.server.URL)
			defer client.Close()
			mgr := newTestManager(t)
			e, g := silentRestoreDeliveryOwner()
			e.agentctl, e.ACPSessionID, e.Status = client, e.RequiredNativeConversationID, v1.AgentStatusReady
			e.setSessionInitialized(true)
			mock.deliveryStatus = silentRestoreSetupDescriptor(e)
			mgr.SetAgentDeliveryRepository(&adoptionDeliveryRepository{generation: g})
			require.NoError(t, mgr.executionStore.Add(e))
			owner, cancelOwner := context.WithCancel(t.Context())
			e.bindSuccessorGenerationOwner(owner, &LaunchRequest{RequiredNativeConversationID: e.RequiredNativeConversationID})
			cancelOwner()
			executionID, nativeID := e.ID, e.ACPSessionID
			switch scenario {
			case "different execution":
				executionID = "replacement"
			case "different native":
				nativeID = "foreign"
			case "uninitialized":
				e.setSessionInitialized(false)
			}
			recoverer, supported := any(mgr).(silentRestoreStreamRecoverer)
			require.True(t, supported, "commit retry must reconnect the retained candidate")
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			err := recoverer.RecoverSilentRestoreCandidateStream(ctx, e.SessionID, executionID, nativeID)
			if scenario == "matching" {
				require.NoError(t, err)
				require.True(t, mgr.IsAgentReadyForPrompt(ctx, e.SessionID))
			} else {
				require.Error(t, err)
				require.False(t, client.HasAgentStream())
			}
		})
	}
}
