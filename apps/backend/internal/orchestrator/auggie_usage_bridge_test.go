package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func writeAuggieSessionFixture(t *testing.T, home, acpID, fixtureName string) {
	t.Helper()
	src := filepath.Join("..", "agent", "auggieusage", "testdata", fixtureName)
	raw, err := os.ReadFile(src)
	require.NoError(t, err)
	dir := filepath.Join(home, ".augment", "sessions")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, acpID+".json"), raw, 0o644))
}

func auggieSession(meta map[string]interface{}) *models.TaskSession {
	return &models.TaskSession{
		ID: "s-auggie",
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
			"model":      "snapshot-model",
		},
		Metadata: meta,
	}
}

func usageEvents(eb *recordingEventBus, sessionID string) []lifecycle.SessionPromptUsageEventPayload {
	want := events.BuildSessionPromptUsageSubject(sessionID)
	var out []lifecycle.SessionPromptUsageEventPayload
	for _, rec := range eb.events {
		if rec.subject != want || rec.event == nil {
			continue
		}
		// Event data may be the struct or a map after bus wrap; accept both.
		switch d := rec.event.Data.(type) {
		case lifecycle.SessionPromptUsageEventPayload:
			out = append(out, d)
		case *lifecycle.SessionPromptUsageEventPayload:
			out = append(out, *d)
		default:
			b, err := json.Marshal(d)
			if err != nil {
				continue
			}
			var p lifecycle.SessionPromptUsageEventPayload
			if json.Unmarshal(b, &p) == nil && p.SessionID != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func TestPublishPromptUsage_AuggieDiskBridge(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Some platforms use UserHomeDir from other envs; force USERPROFILE too.
	t.Setenv("USERPROFILE", home)

	acpID := "acp-auggie-1"
	writeAuggieSessionFixture(t, home, acpID, "multi_exchange.json")

	repo := setupTestRepo(t)
	seedSession(t, repo, "t-auggie", "s-auggie", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s-auggie", "acp", map[string]any{
		"session_id": acpID,
	}))
	// Attach agent snapshot on the row so reload paths would see it if needed.
	sess, err := repo.GetTaskSession(ctx, "s-auggie")
	require.NoError(t, err)
	sess.AgentProfileSnapshot = map[string]interface{}{
		"agent_name": "auggie",
		"model":      "snapshot-model",
	}
	require.NoError(t, repo.UpdateTaskSession(ctx, sess))

	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	session := auggieSession(map[string]interface{}{
		"acp": map[string]interface{}{"session_id": acpID},
	})

	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:    "t-auggie",
		SessionID: "s-auggie",
		AgentID:   "exec-auggie",
		Data:      &lifecycle.AgentStreamEventData{ACPSessionID: acpID},
	}

	svc.publishPromptUsage(ctx, payload, session)

	got := usageEvents(eb, "s-auggie")
	require.Len(t, got, 1)
	require.Equal(t, "auggie", got[0].AgentType)
	require.Equal(t, "t-auggie", got[0].TaskID)
	require.Equal(t, "claude-opus-4-8", got[0].Model)
	require.NotNil(t, got[0].Usage)
	require.Equal(t, int64(16), got[0].Usage.InputTokens)
	require.Equal(t, int64(30), got[0].Usage.OutputTokens)
	require.Equal(t, int64(130), got[0].Usage.CachedReadTokens)
	require.Equal(t, int64(52), got[0].Usage.CachedWriteTokens)
	require.Equal(t, int64(46), got[0].Usage.TotalTokens)
	require.False(t, got[0].Usage.Estimated)
	require.NotEmpty(t, got[0].Timestamp)

	// Watermark advanced on session row.
	reloaded, err := repo.GetTaskSession(ctx, "s-auggie")
	require.NoError(t, err)
	require.Equal(t, float64(2), reloaded.Metadata[models.SessionMetaKeyAuggieUsageSeq])

	// Second complete must not double-count.
	session.Metadata = reloaded.Metadata
	eb.events = nil
	svc.publishPromptUsage(ctx, payload, session)
	require.Empty(t, usageEvents(eb, "s-auggie"))
}

func TestPublishPromptUsage_WireWinsOverAuggieDisk(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	acpID := "acp-auggie-wire"
	writeAuggieSessionFixture(t, home, acpID, "multi_exchange.json")

	repo := setupTestRepo(t)
	seedSession(t, repo, "t-wire", "s-wire", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s-wire", "acp", map[string]any{
		"session_id": acpID,
	}))

	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	session := auggieSession(map[string]interface{}{
		"acp": map[string]interface{}{"session_id": acpID},
	})
	wire := &streams.PromptUsage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:    "t-wire",
		SessionID: "s-wire",
		AgentID:   "exec-wire",
		Data: &lifecycle.AgentStreamEventData{
			ACPSessionID: acpID,
			Usage:        wire,
		},
	}
	svc.publishPromptUsage(ctx, payload, session)

	got := usageEvents(eb, "s-wire")
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Usage.InputTokens)
	require.Equal(t, int64(2), got[0].Usage.OutputTokens)
	// Disk totals must not replace wire.
	require.NotEqual(t, int64(16), got[0].Usage.InputTokens)

	reloaded, err := repo.GetTaskSession(ctx, "s-wire")
	require.NoError(t, err)
	// Watermark still advanced so a later nil-wire path does not double count.
	require.Equal(t, float64(2), reloaded.Metadata[models.SessionMetaKeyAuggieUsageSeq])
}

func TestPublishPromptUsage_NonAuggieNilUsageNoop(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeAuggieSessionFixture(t, home, "should-not-read", "multi_exchange.json")

	eb := &recordingEventBus{}
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:    "t1",
		SessionID: "s1",
		Data:      &lifecycle.AgentStreamEventData{ACPSessionID: "should-not-read"},
	}
	session := &models.TaskSession{
		AgentProfileSnapshot: map[string]interface{}{"agent_name": "claude-acp"},
	}
	svc.publishPromptUsage(ctx, payload, session)
	require.Empty(t, eb.events)
}
