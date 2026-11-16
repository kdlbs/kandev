package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestUndeliveredPromptWithoutClientRetainsProof(t *testing.T) {
	sm := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
	err := sm.dispatchPrompt(t.Context(), &AgentExecution{ID: "execution"}, "unsent", nil, 1, false, "submission")
	var notSent *agentctl.RequestNotSentError
	require.ErrorAs(t, err, &notSent)
}

// @covers AC-PLATFORM-UNDELIVERED-PROMPT-001.1
func TestUndeliveredInitialPromptSettlesBeforeFailureCallback(t *testing.T) {
	for _, callerAdmitted := range []bool{false, true} {
		t.Run(map[bool]string{false: "runtime admission", true: "caller admission"}[callerAdmitted], func(t *testing.T) {
			sm, execution, repo, _ := initialDeliveryPersistenceFixture(t)
			id := initialPromptDeliverySubmissionID(execution)
			callbacks := sendPromptCallbacks{deliverySubmissionID: id}
			if callerAdmitted {
				execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, "first-message")
				id = "prompt:first-message"
				callbacks.deliverySubmissionID = "first-message"
				callbacks.beforeAdmission = func(ctx context.Context, prompt string, _ []v1.MessageAttachment) error {
					payload := []byte(`{"text":"initial instruction"}`)
					_, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
						ID: id, SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
						HarnessGeneration: 2, OwnerGeneration: 2, DispatchAttemptID: "first-message",
						Payload: payload, PayloadHash: journal.SubmissionHash(payload), State: models.DeliverySubmissionDispatching,
					})
					return err
				}
			}
			var observed models.DeliverySubmissionState
			callbacks.onFailure = func(InitialPromptFailure) {
				row, err := repo.GetAgentDeliverySubmission(t.Context(), id)
				require.NoError(t, err)
				observed = row.State
			}
			sm.beforePromptDispatchHook = func() {
				execution.agentctlLifecycleMu.Lock()
				execution.detachAgentctlClient()
				execution.agentctlLifecycleMu.Unlock()
			}
			_, err := sm.sendPrompt(t.Context(), execution, "initial instruction", false, nil, true, callbacks, false)
			require.Error(t, err)
			require.Equal(t, models.DeliverySubmissionFailed, observed)
			row, err := repo.GetAgentDeliverySubmission(t.Context(), id)
			require.NoError(t, err)
			require.NotNil(t, row.FullySettledAt)
			require.Zero(t, row.TerminalSequence)
		})
	}
}

func TestUndeliveredInitialPromptPreservesUncertainOrChangedReservation(t *testing.T) {
	for _, changedOwner := range []bool{false, true} {
		t.Run(map[bool]string{false: "peer error", true: "changed owner"}[changedOwner], func(t *testing.T) {
			sm, execution, repo, mock := initialDeliveryPersistenceFixture(t)
			id := initialPromptDeliverySubmissionID(execution)
			if changedOwner {
				sm.beforePromptDispatchHook = func() {
					_, err := repo.DB().ExecContext(t.Context(), `UPDATE agent_delivery_submissions SET owner_generation = 3, harness_generation = 3 WHERE id = ?`, id)
					require.NoError(t, err)
					execution.agentctlLifecycleMu.Lock()
					execution.detachAgentctlClient()
					execution.agentctlLifecycleMu.Unlock()
				}
			} else {
				mock.handler = func(message ws.Message) *ws.Message {
					if message.Action == "agent.prompt" {
						response, _ := ws.NewError(message.ID, message.Action, ws.ErrorCodeInternalError, "agent stream not connected", nil)
						return response
					}
					return mock.defaultHandler(message)
				}
			}
			_, err := sm.sendPrompt(t.Context(), execution, "initial instruction", false, nil, true, sendPromptCallbacks{deliverySubmissionID: id}, false)
			require.Error(t, err)
			row, err := repo.GetAgentDeliverySubmission(t.Context(), id)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionDispatching, row.State)
			require.Nil(t, row.FullySettledAt)
			if changedOwner {
				require.EqualValues(t, 3, row.OwnerGeneration)
			}
		})
	}
}
