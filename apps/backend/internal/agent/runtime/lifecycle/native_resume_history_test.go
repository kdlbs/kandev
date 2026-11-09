package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type continuedHistoryRepository struct {
	cursor    *models.AgentDeliveryCursor
	continued []string
}

func (r *continuedHistoryRepository) ReceiveAgentDeliveryEvent(context.Context, *models.AgentDeliveryEvent, int64) (bool, error) {
	return false, nil
}

func (r *continuedHistoryRepository) GetAgentDeliveryCursor(context.Context, string) (*models.AgentDeliveryCursor, error) {
	return r.cursor, nil
}

func (r *continuedHistoryRepository) ProjectAgentDeliveryEvent(context.Context, *models.AgentDeliveryEvent, *models.AgentDeliveryEffect) (bool, error) {
	return false, nil
}

func (r *continuedHistoryRepository) ListContextContinuedSubmissionIDs(context.Context, string, string, string, int64) ([]string, error) {
	return r.continued, nil
}

func TestContextContinuedHistoryClassifiesBeforeReplayAdvancesCursor(t *testing.T) {
	status := &agentctl.DeliveryStatus{
		SessionID: "session-1", IncarnationID: "inc-1", HarnessGeneration: 2, StreamID: "stream-2",
		Stream: &journal.Stream{SessionID: "session-1", IncarnationID: "inc-1", HarnessGeneration: 2, StreamID: "stream-2", HighWater: 5, FirstRetained: 1},
	}
	// Adoption runs before replay: the peer has produced events SQL has not
	// yet received or projected.
	repository := &continuedHistoryRepository{
		cursor: &models.AgentDeliveryCursor{SessionID: "session-1", IncarnationID: "inc-1", HarnessGeneration: 2,
			StreamID: "stream-2", ReceivedSequence: 3, ProjectedSequence: 2},
		continued: []string{"predecessor", "dispatching-predecessor"},
	}
	execution := &AgentExecution{SessionID: "session-1", ACPSessionID: "native-2"}
	predecessor := &models.AgentDeliverySubmission{ID: "predecessor", IncarnationID: "inc-1", HarnessGeneration: 1,
		State: models.DeliverySubmissionInterruptedUnknown}
	dispatchingPredecessor := &models.AgentDeliverySubmission{ID: "dispatching-predecessor", IncarnationID: "inc-1", HarnessGeneration: 1,
		State: models.DeliverySubmissionDispatching}
	current := &models.AgentDeliverySubmission{ID: "current", IncarnationID: "inc-1", HarnessGeneration: 2,
		State: models.DeliverySubmissionDispatching}

	rows, err := contextContinuedNativeResumeHistory(context.Background(), execution, status, repository, nil,
		[]*models.AgentDeliverySubmission{predecessor, dispatchingPredecessor, current})

	require.NoError(t, err)
	require.Equal(t, []*models.AgentDeliverySubmission{current}, rows)

	_, err = nativeResumeProofCursor(context.Background(), repository, status)
	require.ErrorIs(t, err, errNativeResumeProjectionLag, "the retirement proof still requires a complete projection")
}
