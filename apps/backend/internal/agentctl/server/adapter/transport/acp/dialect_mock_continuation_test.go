package acp

import (
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestMockInterruptionContinuationWireError(t *testing.T) {
	for _, tc := range []struct {
		name, agentID                                              string
		completedTools                                             uint16
		attested, terminalEvent, orderedFailureAfterIO, emitOutput bool
	}{
		{"supported mock after completed tool", mockAgentID, 1, true, true, true, true},
		{"supported mock before output", mockAgentID, 0, true, true, false, false},
		{"untrusted provider marker", "other-acp", 0, false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, fake, conn := setupHandoffFakeAgent(t)
			a.agentID, a.dialect, a.normalizer = tc.agentID, newACPDialect(tc.agentID), NewNormalizer(tc.agentID)
			fake.promptFailure = &sdk.RequestError{Code: -32603, Message: "peer disconnected before response", Data: map[string]any{"kandevMock": map[string]any{"continuationInterruption": true}}}
			require.NoError(t, a.Initialize(t.Context()))
			a.capabilities.LoadSession = true
			_, err := a.NewSession(t.Context(), nil)
			require.NoError(t, err)
			_ = drainEvents(a)
			done := make(chan error, 1)
			go func() { done <- a.Prompt(t.Context(), "fixture", nil, 7) }()
			select {
			case <-fake.entered:
			case <-time.After(2 * time.Second):
				t.Fatal("prompt not accepted")
			}
			if tc.emitOutput {
				sendCapturedUpdate(t, conn, `{"sessionId":"session-handoff","update":{"sessionUpdate":"tool_call","toolCallId":"read-1","title":"Read","kind":"read","status":"completed","rawInput":{"path":"fixture.txt"}}}`)
			}
			fake.releasePrompts()
			select {
			case err := <-done:
				if !tc.terminalEvent {
					require.Error(t, err)
					return
				}
				require.NoError(t, err, "recognized interruption is delivered once through the terminal event")
			case <-time.After(2 * time.Second):
				t.Fatal("prompt did not settle")
			}
			events := drainEvents(a)
			require.NotEmpty(t, events)
			require.Equal(t, streams.EventTypeError, events[len(events)-1].Type)
			failures := 0
			toolCalls := 0
			errorIndex := -1
			toolIndex := -1
			for i, event := range events {
				if event.Type == streams.EventTypeToolCall {
					toolCalls++
					toolIndex = i
				}
				if event.Type == streams.EventTypeError {
					failures++
					errorIndex = i
					if tc.attested {
						require.True(t, event.ContinuationSafety.SafeFor(7))
						require.Equal(t, tc.completedTools, event.ContinuationSafety.CompletedTools)
						require.Equal(t, streams.PromptFailureDispositionRetainRuntime, event.PromptFailureDisposition)
					} else {
						require.Nil(t, event.ContinuationSafety)
						require.Empty(t, event.PromptFailureDisposition)
					}
				}
				require.NotEqual(t, streams.EventTypeComplete, event.Type)
			}
			expectedToolCalls := 0
			if tc.emitOutput {
				expectedToolCalls = 1
			}
			require.Equal(t, expectedToolCalls, toolCalls)
			require.Equal(t, 1, failures)
			if tc.emitOutput {
				require.NotEqual(t, -1, toolIndex, "completed tool activity must be delivered")
			} else {
				require.Equal(t, -1, toolIndex, "no tool activity should be delivered before output")
			}
			if tc.orderedFailureAfterIO {
				require.Greater(t, errorIndex, toolIndex, "activity events must precede the terminal failure")
			}
		})
	}
}
