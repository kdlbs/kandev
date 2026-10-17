package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func seedPromptSuggestionSession(t *testing.T, state models.TaskSessionState) (*Service, *recordingEventBus) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-1", TaskSessionID: "s1", TaskID: "t1", StartedAt: now.Add(-2 * time.Minute)}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn-2", TaskSessionID: "s1", TaskID: "t1", StartedAt: now.Add(-time.Minute)}))
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "s1", state, ""))
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb
	return svc, eb
}

func promptSuggestionPayload(text string) *lifecycle.AgentStreamEventPayload {
	return &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data:      &lifecycle.AgentStreamEventData{Text: text},
	}
}

// TestHandlePromptSuggestionEvent_PersistsAndPublishes verifies a native
// suggestion is bound to the latest turn, stored on the session, and published.
func TestHandlePromptSuggestionEvent_PersistsAndPublishes(t *testing.T) {
	ctx := context.Background()
	svc, eb := seedPromptSuggestionSession(t, models.TaskSessionStateWaitingForInput)

	svc.handlePromptSuggestionEvent(ctx, promptSuggestionPayload("  sim, corre os testes "))

	session, err := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	stored, ok := session.Metadata[promptSuggestionMetadataKey].(map[string]interface{})
	require.True(t, ok, "metadata = %#v", session.Metadata)
	require.Equal(t, "turn-2", stored["turn_id"])
	require.Equal(t, "sim, corre os testes", stored["text"])

	require.Len(t, eb.events, 1)
	require.Equal(t, events.BuildSessionPromptSuggestionSubject("s1"), eb.events[0].subject)
	published, ok := eb.events[0].event.Data.(PromptSuggestionEventPayload)
	require.True(t, ok)
	require.Equal(t, PromptSuggestionEventPayload{
		TaskID: "t1", SessionID: "s1", TurnID: "turn-2", Text: "sim, corre os testes",
	}, published)
}

// TestHandlePromptSuggestionEvent_IgnoresUnusableSuggestions verifies running
// sessions, empty text, and oversized text are neither stored nor published.
func TestHandlePromptSuggestionEvent_IgnoresUnusableSuggestions(t *testing.T) {
	cases := []struct {
		name  string
		state models.TaskSessionState
		text  string
	}{
		{name: "session running", state: models.TaskSessionStateRunning, text: "sim"},
		{name: "empty", state: models.TaskSessionStateWaitingForInput, text: "   "},
		{name: "oversized", state: models.TaskSessionStateWaitingForInput, text: strings.Repeat("a", maxPromptSuggestionRunes)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, eb := seedPromptSuggestionSession(t, tc.state)
			svc.handlePromptSuggestionEvent(ctx, promptSuggestionPayload(tc.text))

			session, err := svc.repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			require.NotContains(t, session.Metadata, promptSuggestionMetadataKey)
			require.Empty(t, eb.events)
		})
	}
}

// TestPromptSuggestionEventPayloadRoutesToSession verifies the WebSocket
// session broadcaster can resolve the payload's session.
func TestPromptSuggestionEventPayloadRoutesToSession(t *testing.T) {
	var payload any = PromptSuggestionEventPayload{SessionID: "s1"}
	routed, ok := payload.(interface{ GetSessionID() string })
	require.True(t, ok, "payload must expose GetSessionID for session broadcasts")
	require.Equal(t, "s1", routed.GetSessionID())
}

// TestHandleAgentCapabilitiesEvent_RecordsNativePromptSuggestions verifies the
// negotiated native-suggestion capability is persisted on the session and
// broadcast, and that a later session without it clears the marker.
func TestHandleAgentCapabilitiesEvent_RecordsNativePromptSuggestions(t *testing.T) {
	ctx := context.Background()
	svc, eb := seedPromptSuggestionSession(t, models.TaskSessionStateStarting)
	capabilities := func(native bool) *lifecycle.AgentStreamEventPayload {
		return &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			Data:      &lifecycle.AgentStreamEventData{SupportsPromptSuggestions: native},
		}
	}

	svc.handleAgentCapabilitiesEvent(ctx, capabilities(true))
	session, err := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, promptSuggestionSourceNative, session.Metadata[promptSuggestionSourceMetadataKey])
	require.Len(t, eb.events, 1)
	published, ok := eb.events[0].event.Data.(lifecycle.AgentCapabilitiesEventPayload)
	require.True(t, ok)
	require.True(t, published.SupportsPromptSuggestions)

	svc.handleAgentCapabilitiesEvent(ctx, capabilities(false))
	session, err = svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, promptSuggestionSourceNone, session.Metadata[promptSuggestionSourceMetadataKey])
}
