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
	return promptSuggestionPayloadForTurn(text, "turn-2")
}

func promptSuggestionPayloadForTurn(text, originTurnID string) *lifecycle.AgentStreamEventPayload {
	return &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data:      &lifecycle.AgentStreamEventData{Text: text, TurnID: originTurnID},
	}
}

// TestHandlePromptSuggestionEvent_PersistsAndPublishes verifies a native
// suggestion from the latest turn is stored on the session under that turn and
// published.
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
// effective native-suggestion value is persisted on the session and broadcast,
// while the support-only value is forwarded without changing source semantics.
// A later session without effective support clears the marker.
func TestHandleAgentCapabilitiesEvent_RecordsNativePromptSuggestions(t *testing.T) {
	ctx := context.Background()
	svc, eb := seedPromptSuggestionSession(t, models.TaskSessionStateStarting)
	trueValue, falseValue := true, false
	capabilities := func(effective bool, raw *bool) *lifecycle.AgentStreamEventPayload {
		return &lifecycle.AgentStreamEventPayload{
			TaskID:    "t1",
			SessionID: "s1",
			Data: &lifecycle.AgentStreamEventData{
				SupportsPromptSuggestions:        effective,
				NativePromptSuggestionsSupported: raw,
			},
		}
	}
	requireSource := func(want string) {
		t.Helper()
		session, err := svc.repo.GetTaskSession(ctx, "s1")
		require.NoError(t, err)
		require.Equal(t, want, session.Metadata[promptSuggestionSourceMetadataKey])
	}
	requirePublished := func(wantEffective bool, wantRaw *bool) {
		t.Helper()
		require.NotEmpty(t, eb.events)
		published, ok := eb.events[len(eb.events)-1].event.Data.(lifecycle.AgentCapabilitiesEventPayload)
		require.True(t, ok)
		require.Equal(t, wantEffective, published.SupportsPromptSuggestions)
		require.Equal(t, wantRaw, published.NativePromptSuggestionsSupported)
	}

	svc.handleAgentCapabilitiesEvent(ctx, capabilities(true, &trueValue))
	requireSource(promptSuggestionSourceNative)
	requirePublished(true, &trueValue)

	// Supported but not requested: support stays true, effective is false, and
	// the session must stop being native.
	svc.handleAgentCapabilitiesEvent(ctx, capabilities(false, &trueValue))
	requireSource(promptSuggestionSourceNone)
	requirePublished(false, &trueValue)

	// An older producer can still send effective true with unknown raw support.
	svc.handleAgentCapabilitiesEvent(ctx, capabilities(true, nil))
	requireSource(promptSuggestionSourceNative)
	requirePublished(true, nil)

	// Effective false with the raw field absent still clears the source.
	svc.handleAgentCapabilitiesEvent(ctx, capabilities(false, nil))
	requireSource(promptSuggestionSourceNone)
	requirePublished(false, nil)

	// A new explicit denial clears the source and is forwarded as false.
	svc.handleAgentCapabilitiesEvent(ctx, capabilities(true, nil))
	requireSource(promptSuggestionSourceNative)
	svc.handleAgentCapabilitiesEvent(ctx, capabilities(false, &falseValue))
	requireSource(promptSuggestionSourceNone)
	requirePublished(false, &falseValue)
}

// TestHandlePromptSuggestionEvent_RejectsSuggestionWithoutLatestOrigin verifies
// a suggestion whose originating turn is older than the latest turn, or
// unknown, is neither stored nor published, and never takes the latest turn.
func TestHandlePromptSuggestionEvent_RejectsSuggestionWithoutLatestOrigin(t *testing.T) {
	cases := []struct {
		name   string
		origin string
	}{
		{name: "older turn", origin: "turn-1"},
		{name: "unknown origin", origin: ""},
		{name: "foreign turn", origin: "turn-other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, eb := seedPromptSuggestionSession(t, models.TaskSessionStateWaitingForInput)
			svc.handlePromptSuggestionEvent(ctx, promptSuggestionPayloadForTurn("delayed", tc.origin))

			session, err := svc.repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			require.NotContains(t, session.Metadata, promptSuggestionMetadataKey)
			require.Empty(t, eb.events)
		})
	}
}
