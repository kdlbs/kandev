package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantContextScopesBudgetAndMandatoryOverflow(t *testing.T) {
	b := &models.AssistantBinding{ID: "binding", OwnerUserID: "owner", WorkspaceID: "ws"}
	p := &models.ContextPacket{ContextScope: models.ContextScope{ProfileID: "profile", TaskID: "task"}}
	expired := time.Now().Add(-time.Hour)
	rows := []*models.AgentMemory{
		{ID: "global-legacy", Scope: "user", Content: "UNOWNED_GLOBAL"},
		{ID: "foreign", Scope: "workspace", ScopeID: "other", Content: "FOREIGN"},
		{ID: "expired", Scope: "workspace", ExpiresAt: &expired, Content: "EXPIRED"},
		{ID: "project", Scope: "project", ScopeID: "other", Content: "PROJECT"},
		{ID: "match", Scope: "task", ScopeID: "task", Content: strings.Repeat("界", 1000)},
	}
	require.NoError(t, fillContextMemory(p, b, rows))
	require.Len(t, p.Memory, 1)
	require.True(t, p.Memory[0].Truncated)
	require.True(t, utf8.ValidString(p.Memory[0].Content))
	require.LessOrEqual(t, len(p.Memory[0].Content), 1024)
	required := &models.AgentMemory{ID: "must", Scope: "workspace", Confirmed: true, Priority: 100, Content: strings.Repeat("a", 1025)}
	require.ErrorContains(t, fillContextMemory(&models.ContextPacket{}, b, []*models.AgentMemory{required}), "required memory")
	p = &models.ContextPacket{UserInstruction: strings.Repeat("x", models.ContextBudgetBytes)}
	require.ErrorContains(t, fillContextMemory(p, b, nil), "indispensable")
}

func TestAssistantMemoryRedactsKnownTokensAndProtectsConfirmation(t *testing.T) {
	s, _, _, _ := assistantContextFixture(t)
	canary := "ghp_" + strings.Repeat("A", 36)
	put := runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant/memory/redacted", "", "", map[string]any{
		"key": "token", "content": "Do not store " + canary, "scope": "workspace", "source_comment_id": "source", "confirmed": true,
	})
	require.Equal(t, 200, put.Code, put.Body.String())
	require.NotContains(t, put.Body.String(), canary)
	err := s.Repo.UpsertAgentMemory(context.Background(), &models.AgentMemory{
		AgentProfileID: "chief", Layer: "user", Key: "token", Content: "runtime overwrite", Metadata: "{}",
	})
	require.ErrorIs(t, err, models.ErrConflict)
}

func TestAssistantContextForgetAndExpiryChangeDigest(t *testing.T) {
	s, token, run, goal := assistantContextFixture(t)
	human, agent := assistantRouter(s), assistantRuntimeRouter(s)
	memoryPath := "/api/v1/orchestration/assistant/memory/preference"
	req := map[string]any{"key": "keep", "content": "SCOPED_PREFERENCE", "scope": "workspace", "source_comment_id": "source", "confirmed": true}
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", memoryPath, "", "", req).Code)
	packetPath := "/api/v1/orchestration/runtime/context/" + goal + "?profile_id=personal"
	fetch := func() models.ContextPacket {
		r := runtimeRequest(t, agent, "GET", packetPath, token, run, nil)
		require.Equal(t, 200, r.Code, r.Body.String())
		var p models.ContextPacket
		require.NoError(t, json.Unmarshal(r.Body.Bytes(), &p))
		return p
	}
	first := fetch()
	require.Len(t, first.Memory, 1)
	req["expected_revision"], req["expires_at"] = 1, time.Now().Add(-time.Hour)
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", memoryPath, "", "", req).Code)
	expired := fetch()
	require.Empty(t, expired.Memory)
	require.NotEqual(t, first.ID, expired.ID)
	req["expected_revision"], req["expires_at"] = 2, nil
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", memoryPath, "", "", req).Code)
	require.Len(t, fetch().Memory, 1)
	require.Equal(t, 200, runtimeRequest(t, human, "DELETE", memoryPath, "", "", map[string]int{"expected_revision": 3}).Code)
	require.Empty(t, fetch().Memory)
}
