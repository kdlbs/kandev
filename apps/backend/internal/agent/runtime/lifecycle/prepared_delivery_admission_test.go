package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.1
func TestInitialPromptAdmitsMaterializedPayload(t *testing.T) {
	for _, collision := range []bool{false, true} {
		name := "metadata hydration"
		if collision {
			name = "filename collision"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			sm, execution, repo, mock := initialDeliveryPersistenceFixture(t)
			execution.ACPSessionID = "native-session"
			execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, "first-message")
			sm.SetAttachmentReader(testAttachmentReader{})
			expectedName := "bundle.zip"
			if collision {
				expectedName = "bundle-2.zip"
				mock.mu.Lock()
				mock.materializedName = expectedName
				mock.mu.Unlock()
			}
			input := []v1.MessageAttachment{{AttachmentID: "attachment", Type: "resource", Name: "bundle.zip"}}
			wire := make(chan ws.Message, 1)
			mock.handler = func(msg ws.Message) *ws.Message {
				if msg.Action == "agent.prompt" {
					wire <- msg
				}
				return mock.defaultHandler(msg)
			}
			var admitted []byte
			admit := func(admissionCtx context.Context, prompt string, attachments []v1.MessageAttachment) error {
				require.NoError(t, admissionCtx.Err())
				require.Equal(t, "initial instruction", prompt)
				require.Equal(t, expectedName, attachments[0].Name)
				require.Equal(t, "application/zip", attachments[0].MimeType)
				require.EqualValues(t, 16, attachments[0].SizeBytes)
				require.Equal(t, "prompt", attachments[0].DeliveryMode)
				payload, err := json.Marshal(struct {
					Text        string                 `json:"text"`
					Attachments []v1.MessageAttachment `json:"attachments,omitempty"`
				}{prompt, attachments})
				if err != nil {
					return err
				}
				admitted = payload
				_, err = repo.PrepareAgentDeliverySubmission(admissionCtx, &models.AgentDeliverySubmission{
					ID: "prompt:first-message", SessionID: "session", IncarnationID: "incarnation",
					HarnessGeneration: 2, OwnerGeneration: 2, DispatchAttemptID: "first-message",
					Payload: payload, PayloadHash: journal.SubmissionHash(payload), State: models.DeliverySubmissionDispatching,
				})
				return err
			}
			reached := false
			sm.beforePromptDispatchHook = func() {
				reached = true
				stored, err := repo.GetAgentDeliverySubmission(ctx, "prompt:first-message")
				require.NoError(t, err)
				require.Equal(t, admitted, stored.Payload)
				require.Equal(t, journal.SubmissionHash(admitted), stored.PayloadHash)
			}
			_, err := sm.sendPrompt(ctx, execution, "initial instruction", false, input, true,
				sendPromptCallbacks{beforeAdmission: admit, deliverySubmissionID: "first-message"}, false)
			require.NoError(t, err)
			require.True(t, reached)
			var dispatched struct {
				Text        string                 `json:"text"`
				Attachments []v1.MessageAttachment `json:"attachments,omitempty"`
			}
			message := <-wire
			require.NoError(t, message.ParsePayload(&dispatched))
			wirePayload, err := json.Marshal(dispatched)
			require.NoError(t, err)
			require.Equal(t, admitted, wirePayload, "SQL must hash exactly the payload sent to agentctl")
			require.Equal(t, "bundle.zip", input[0].Name)
			require.Empty(t, input[0].MimeType, "preparation must not mutate the caller's source intent")
		})
	}
}

func TestPreparedDeliveryAdmissionFailureNeverDispatches(t *testing.T) {
	sm, execution, repo, _ := initialDeliveryPersistenceFixture(t)
	execution.ACPSessionID = "native-session"
	sm.SetAttachmentReader(testAttachmentReader{})
	id := initialPromptDeliverySubmissionID(execution)
	reached := false
	sm.beforePromptDispatchHook = func() { reached = true }
	want := errors.New("canonical admission failed")
	_, err := sm.sendPrompt(t.Context(), execution, "initial instruction", false,
		[]v1.MessageAttachment{{AttachmentID: "attachment", Type: "resource", Name: "bundle.zip"}}, true,
		sendPromptCallbacks{deliverySubmissionID: id, beforeAdmission: func(context.Context, string, []v1.MessageAttachment) error { return want }}, false)
	require.ErrorIs(t, err, want)
	require.False(t, reached)
	require.Zero(t, execution.promptGenerationSnapshot())
	submissions, err := repo.ListAgentDeliverySubmissions(t.Context(), execution.SessionID)
	require.NoError(t, err)
	require.Empty(t, submissions)
}

func TestInitialAdmissionPersistenceFailureUsesFailureHandler(t *testing.T) {
	for _, owned := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary launch", true: "owned launch"}[owned], func(t *testing.T) {
			sm, execution, _, _ := initialDeliveryPersistenceFixture(t)
			failures := make(chan InitialPromptFailure, 1)
			sm.SetInitialPromptFailureHandler(func(failure InitialPromptFailure) { failures <- failure })
			want := errors.New("SQL admission unavailable")
			var cleanup func()
			cleaned := make(chan struct{}, 1)
			if owned {
				cleanup = func() { cleaned <- struct{}{} }
			}
			execution.setInitialPromptDispatchCallbacks(func(context.Context, string, []v1.MessageAttachment) error {
				return &PromptAdmissionPersistenceError{Err: want}
			}, nil, cleanup)
			agentConfig, ok := newTestRegistry().Get("claude-acp")
			require.True(t, ok)
			sm.dispatchInitialPrompt(t.Context(), execution, agentConfig, "initial instruction", nil, func(string) error { return nil })
			select {
			case failure := <-failures:
				require.ErrorIs(t, failure.Err, want)
				require.Equal(t, execution.ID, failure.ExecutionID)
				require.Zero(t, failure.PromptGeneration)
			case <-time.After(5 * time.Second):
				t.Fatal("durable admission failure was not reported")
			}
			if owned {
				require.Len(t, cleaned, 1)
			}
			require.Zero(t, execution.promptGenerationSnapshot())
		})
	}
}
