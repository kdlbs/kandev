package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
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

func TestPublishPromptUsage_AuggiePayloadACPSessionIDOnly(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	acpID := "acp-payload-only"
	writeAuggieSessionFixture(t, home, acpID, "multi_exchange.json")

	repo := setupTestRepo(t)
	seedSession(t, repo, "t-payload", "s-payload", "step1")

	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	// No metadata.acp; only complete-frame ACPSessionID (common first-turn path).
	session := auggieSession(nil)
	session.ID = "s-payload"
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:    "t-payload",
		SessionID: "s-payload",
		AgentID:   "exec",
		Data:      &lifecycle.AgentStreamEventData{ACPSessionID: acpID},
	}
	svc.publishPromptUsage(ctx, payload, session)
	got := usageEvents(eb, "s-payload")
	require.Len(t, got, 1)
	require.Equal(t, int64(16), got[0].Usage.InputTokens)
}

func TestPublishPromptUsage_AuggieMissingACPSessionID(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeAuggieSessionFixture(t, home, "orphan-file", "multi_exchange.json")

	eb := &recordingEventBus{}
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	session := auggieSession(nil)
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:    "t-miss",
		SessionID: "s-miss",
		Data:      &lifecycle.AgentStreamEventData{}, // no ACPSessionID
	}
	svc.publishPromptUsage(ctx, payload, session)
	require.Empty(t, usageEvents(eb, "s-miss"))
}

func TestPublishPromptUsage_AuggieCacheOnlyTokens(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	acpID := "acp-cache-only"
	dir := filepath.Join(home, ".augment", "sessions")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	// Real Augment shape: completed exchange with only cache counters (input/output 0).
	raw := []byte(`{
  "agentState": {"modelId": "cache-model"},
  "chatHistory": [{
    "sequenceId": 1,
    "completed": true,
    "finishedAt": "2026-08-11T12:00:00.000Z",
    "exchange": {
      "model_id": "cache-model",
      "response_nodes": [{
        "token_usage": {
          "input_tokens": 0,
          "output_tokens": 0,
          "cache_read_input_tokens": 999,
          "cache_creation_input_tokens": 12
        }
      }]
    }
  }]
}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, acpID+".json"), raw, 0o644))

	repo := setupTestRepo(t)
	seedSession(t, repo, "t-cache", "s-cache", "step1")
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	session := auggieSession(map[string]interface{}{
		"acp": map[string]interface{}{"session_id": acpID},
	})
	session.ID = "s-cache"
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID: "t-cache", SessionID: "s-cache", AgentID: "exec",
		Data: &lifecycle.AgentStreamEventData{ACPSessionID: acpID},
	}
	svc.publishPromptUsage(ctx, payload, session)

	got := usageEvents(eb, "s-cache")
	require.Len(t, got, 1, "cache-only exchange must still publish for Office costs / pile metadata")
	require.Equal(t, int64(0), got[0].Usage.InputTokens)
	require.Equal(t, int64(0), got[0].Usage.OutputTokens)
	require.Equal(t, int64(0), got[0].Usage.TotalTokens)
	require.Equal(t, int64(999), got[0].Usage.CachedReadTokens)
	require.Equal(t, int64(12), got[0].Usage.CachedWriteTokens)

	// Watermark advanced even though Kandy may ignore total=0 observations.
	reloaded, err := repo.GetTaskSession(ctx, "s-cache")
	require.NoError(t, err)
	require.Equal(t, float64(1), reloaded.Metadata[models.SessionMetaKeyAuggieUsageSeq])
}

func TestPublishPromptUsage_FailedPublishDoesNotAdvanceWatermark(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	acpID := "acp-fail-bus"
	writeAuggieSessionFixture(t, home, acpID, "multi_exchange.json")

	repo := setupTestRepo(t)
	seedSession(t, repo, "t-fail", "s-fail", "step1")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = &failingEventBus{err: errBusDown}

	session := auggieSession(map[string]interface{}{
		"acp": map[string]interface{}{"session_id": acpID},
	})
	session.ID = "s-fail"
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID: "t-fail", SessionID: "s-fail",
		Data: &lifecycle.AgentStreamEventData{ACPSessionID: acpID},
	}
	svc.publishPromptUsage(ctx, payload, session)

	reloaded, err := repo.GetTaskSession(ctx, "s-fail")
	require.NoError(t, err)
	_, has := reloaded.Metadata[models.SessionMetaKeyAuggieUsageSeq]
	require.False(t, has, "failed publish must leave watermark unset for retry")
}

func TestPublishPromptUsage_AuggieDeferredReread(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	acpID := "acp-deferred"
	// File intentionally absent at first complete.

	repo := setupTestRepo(t)
	seedSession(t, repo, "t-def", "s-def", "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "s-def", "acp", map[string]any{
		"session_id": acpID,
	}))
	sess, err := repo.GetTaskSession(ctx, "s-def")
	require.NoError(t, err)
	sess.AgentProfileSnapshot = map[string]interface{}{"agent_name": "auggie", "model": "m"}
	require.NoError(t, repo.UpdateTaskSession(ctx, sess))

	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	session := auggieSession(map[string]interface{}{
		"acp": map[string]interface{}{"session_id": acpID},
	})
	session.ID = "s-def"
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID: "t-def", SessionID: "s-def", AgentID: "exec",
		Data: &lifecycle.AgentStreamEventData{ACPSessionID: acpID},
	}
	svc.publishPromptUsage(ctx, payload, session)
	require.Empty(t, usageEvents(eb, "s-def"))

	// Flush appears after complete (Auggie write lag).
	writeAuggieSessionFixture(t, home, acpID, "multi_exchange.json")
	// Drive deferred path immediately (same logic as AfterFunc callback).
	svc.runAuggieUsageReread("s-def", "t-def", "exec", acpID, 0)

	got := usageEvents(eb, "s-def")
	require.Len(t, got, 1)
	require.Equal(t, int64(16), got[0].Usage.InputTokens)
	require.Equal(t, "auggie", got[0].AgentType)

	reloaded, err := repo.GetTaskSession(ctx, "s-def")
	require.NoError(t, err)
	require.Equal(t, float64(2), reloaded.Metadata[models.SessionMetaKeyAuggieUsageSeq])
}

func TestPublishPromptUsage_StaleInMemoryWatermarkNoDoubleCount(t *testing.T) {
	// Two completes with the same stale session.Metadata watermark (as when a
	// deferred re-read and a second complete both started from after=0). The
	// second path must reload auggie_usage_seq from the DB and skip.
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	acpID := "acp-stale"
	writeAuggieSessionFixture(t, home, acpID, "multi_exchange.json")

	repo := setupTestRepo(t)
	seedSession(t, repo, "t-stale", "s-stale", "step1")

	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb

	meta := map[string]interface{}{"acp": map[string]interface{}{"session_id": acpID}}
	sessionA := auggieSession(meta)
	sessionA.ID = "s-stale"
	sessionB := cloneSession(sessionA)
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID: "t-stale", SessionID: "s-stale", AgentID: "exec",
		Data: &lifecycle.AgentStreamEventData{ACPSessionID: acpID},
	}

	svc.publishPromptUsage(ctx, payload, sessionA)
	svc.publishPromptUsage(ctx, payload, sessionB) // still after=0 in memory

	got := usageEvents(eb, "s-stale")
	require.Len(t, got, 1, "persisted watermark must prevent stale in-memory double publish")
	require.Equal(t, int64(16), got[0].Usage.InputTokens)
}

func cloneSession(s *models.TaskSession) *models.TaskSession {
	if s == nil {
		return nil
	}
	cp := *s
	if s.Metadata != nil {
		cp.Metadata = make(map[string]interface{}, len(s.Metadata))
		for k, v := range s.Metadata {
			cp.Metadata[k] = v
		}
	}
	if s.AgentProfileSnapshot != nil {
		cp.AgentProfileSnapshot = make(map[string]interface{}, len(s.AgentProfileSnapshot))
		for k, v := range s.AgentProfileSnapshot {
			cp.AgentProfileSnapshot[k] = v
		}
	}
	return &cp
}

var errBusDown = errors.New("bus down")

type failingEventBus struct {
	err error
}

func (b *failingEventBus) Publish(context.Context, string, *bus.Event) error { return b.err }
func (b *failingEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (b *failingEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (b *failingEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (b *failingEventBus) Close()            {}
func (b *failingEventBus) IsConnected() bool { return true }
