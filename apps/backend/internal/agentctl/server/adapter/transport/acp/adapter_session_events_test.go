package acp

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestConvertCompactionUpdatesAndSummaryChunksUpsertOneEntry(t *testing.T) {
	a := newTestAdapter()
	a.sessionID = "session-1"
	a.usageBySession["session-1"] = &usageTracker{latestUsed: 50_000, maxSize: 200_000}

	started := a.convertNotification(makeNotification("session-1", acp.SessionUpdate{
		CompactionUpdate: &acp.SessionCompactionUpdate{
			CompactionId: "compact-a",
			Status:       "in_progress",
			Summary:      acp.OptionalValue([]acp.ContentBlock{acp.TextBlock("first "), acp.TextBlock("summary")}),
			Meta: acp.OptionalValue(map[string]any{"contextCompaction": map[string]any{
				"trigger": "manual", "pre_tokens": float64(31_300), "post_tokens": float64(2_400), "duration_ms": float64(26_800),
			}}),
		},
	}))
	if started == nil {
		t.Fatal("initial compaction update returned no event")
	}
	if started.Type != streams.EventTypeToolCall || started.ToolCallID != "compaction:session-1:compact-a" {
		t.Fatalf("initial event identity = (%q, %q)", started.Type, started.ToolCallID)
	}
	if got := started.NormalizedPayload.ContextCompaction(); got == nil || got.Status != "in_progress" || got.Summary != "first \nsummary" || got.Trigger != "manual" || got.PreTokens == nil || *got.PreTokens != 31_300 || got.PostTokens == nil || *got.PostTokens != 2_400 || got.DurationMs == nil || *got.DurationMs != 26_800 {
		t.Fatalf("initial compaction payload = %+v", got)
	}

	chunk := a.convertNotification(makeNotification("session-1", acp.SessionUpdate{
		CompactionSummaryChunk: &acp.SessionUpdateCompactionSummaryChunk{CompactionId: "compact-a", Content: acp.TextBlock(" streamed")},
	}))
	if chunk == nil || chunk.Type != streams.EventTypeToolUpdate || chunk.ToolCallID != started.ToolCallID {
		t.Fatalf("summary chunk event = %+v", chunk)
	}
	if got := chunk.NormalizedPayload.ContextCompaction().Summary; got != "first \nsummary streamed" {
		t.Fatalf("accumulated summary = %q", got)
	}

	completed := a.convertNotification(makeNotification("session-1", acp.SessionUpdate{
		CompactionUpdate: &acp.SessionCompactionUpdate{CompactionId: "compact-a", Status: "completed"},
	}))
	if completed == nil || completed.Type != streams.EventTypeToolUpdate || completed.ToolStatus != "complete" {
		t.Fatalf("completion event = %+v", completed)
	}
	if got := completed.NormalizedPayload.ContextCompaction(); got.Status != "completed" || got.Summary != "first \nsummary streamed" {
		t.Fatalf("completion payload did not retain summary: %+v", got)
	}

	a.mu.RLock()
	usage := *a.usageBySession["session-1"]
	a.mu.RUnlock()
	if usage.latestUsed != 50_000 || usage.maxSize != 200_000 {
		t.Fatalf("compaction changed usage tracker: %+v", usage)
	}
}

func TestConvertCompactionOptionalFieldsAndUnknownStatus(t *testing.T) {
	a := newTestAdapter()
	first := a.convertNotification(makeNotification("session-1", acp.SessionUpdate{
		CompactionUpdate: &acp.SessionCompactionUpdate{
			CompactionId: "compact-b",
			Status:       "provider-added-status",
			Error:        acp.OptionalValue("temporary failure"),
			Summary:      acp.OptionalValue([]acp.ContentBlock{acp.TextBlock("old summary")}),
			Meta:         acp.OptionalValue(map[string]any{"contextCompaction": map[string]any{"trigger": "auto", "pre_tokens": float64(9)}}),
		},
	}))
	if got := first.NormalizedPayload.ContextCompaction(); got.Status != "in_progress" || got.Error != "temporary failure" || got.Summary != "old summary" || got.Trigger != "auto" || got.PreTokens == nil || *got.PreTokens != 9 {
		t.Fatalf("initial normalized compaction = %+v", got)
	}

	cleared := a.convertNotification(makeNotification("session-1", acp.SessionUpdate{
		CompactionUpdate: &acp.SessionCompactionUpdate{
			CompactionId: "compact-b",
			Status:       "failed",
			Error:        acp.OptionalNull[string](),
			Summary:      acp.OptionalNull[[]acp.ContentBlock](),
			Meta:         acp.OptionalNull[map[string]any](),
		},
	}))
	if cleared == nil || cleared.Type != streams.EventTypeToolUpdate {
		t.Fatalf("clear event = %+v", cleared)
	}
	if got := cleared.NormalizedPayload.ContextCompaction(); got.Status != "failed" || got.Error != "" || got.Summary != "" || got.Trigger != "" || got.PreTokens != nil {
		t.Fatalf("explicit null did not clear optional fields: %+v", got)
	}
}

func TestConvertCompactionDropsUnknownChunksAndCapsSummary(t *testing.T) {
	a := newTestAdapter()
	unknown := a.convertNotification(makeNotification("session-1", acp.SessionUpdate{
		CompactionSummaryChunk: &acp.SessionUpdateCompactionSummaryChunk{CompactionId: "missing", Content: acp.TextBlock("orphan")},
	}))
	if unknown != nil {
		t.Fatalf("unknown summary chunk created an event: %+v", unknown)
	}

	started := a.convertNotification(makeNotification("session-1", acp.SessionUpdate{
		CompactionUpdate: &acp.SessionCompactionUpdate{CompactionId: "compact-c", Status: "in_progress"},
	}))
	large := strings.Repeat("x", 70<<10)
	updated := a.convertNotification(makeNotification("session-1", acp.SessionUpdate{
		CompactionSummaryChunk: &acp.SessionUpdateCompactionSummaryChunk{CompactionId: "compact-c", Content: acp.TextBlock(large)},
	}))
	if started == nil || updated == nil {
		t.Fatal("expected compaction and summary events")
	}
	if got := updated.NormalizedPayload.ContextCompaction().Summary; len(got) != 64<<10 {
		t.Fatalf("summary length = %d, want %d", len(got), 64<<10)
	}
}

func TestCapturedClaudeAndCodexCompactionsNormalizeOncePerIdentity(t *testing.T) {
	for _, fixture := range []string{"context-compaction-claude.jsonl", "context-compaction-codex.jsonl"} {
		t.Run(fixture, func(t *testing.T) {
			file, err := os.Open(filepath.Join("testdata", fixture))
			if err != nil {
				t.Fatalf("open provider capture: %v", err)
			}
			t.Cleanup(func() {
				if err := file.Close(); err != nil {
					t.Errorf("close provider capture: %v", err)
				}
			})

			adapter := newTestAdapter()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				var frame struct {
					Params acp.SessionNotification `json:"params"`
				}
				if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
					t.Fatalf("decode captured ACP frame: %v", err)
				}
				notification := frame.Params
				adapter.sessionID = string(notification.SessionId)
				adapter.handleACPUpdate(notification, 0)
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("read captured ACP frames: %v", err)
			}

			events := drainEvents(adapter)
			var calls, updates []AgentEvent
			var contextWindowEvents int
			for _, event := range events {
				switch event.Type {
				case streams.EventTypeToolCall:
					calls = append(calls, event)
				case streams.EventTypeToolUpdate:
					updates = append(updates, event)
				case streams.EventTypeContextWindow:
					contextWindowEvents++
				}
			}
			if contextWindowEvents != 0 {
				t.Fatalf("compaction emitted %d context-window events", contextWindowEvents)
			}
			if len(calls) != 1 {
				t.Fatalf("emitted %d compaction entries, want one", len(calls))
			}
			for _, update := range updates {
				if update.ToolCallID != calls[0].ToolCallID {
					t.Fatalf("update identity = %q, want %q", update.ToolCallID, calls[0].ToolCallID)
				}
			}
			if got := calls[0].ToolCallID; !strings.HasPrefix(got, "compaction:") {
				t.Fatalf("compaction tool call id = %q", got)
			}

			last := calls[0]
			if len(updates) > 0 {
				last = updates[len(updates)-1]
			}
			payload := last.NormalizedPayload.ContextCompaction()
			if payload == nil || payload.Status != "completed" {
				t.Fatalf("final captured compaction payload = %+v", payload)
			}
			if fixture == "context-compaction-claude.jsonl" {
				if !strings.Contains(payload.Summary, "Retained implementation context.") || payload.Trigger != "manual" || payload.PreTokens == nil || *payload.PreTokens != 31_319 || payload.PostTokens == nil || *payload.PostTokens != 2_433 || payload.DurationMs == nil || *payload.DurationMs != 26_847 {
					t.Fatalf("Claude captured fields were not normalized: %+v", payload)
				}
			} else if payload.Summary != "" || payload.Trigger != "" || payload.PreTokens != nil || payload.PostTokens != nil || payload.DurationMs != nil {
				t.Fatalf("Codex capture should only provide standard fields: %+v", payload)
			}
		})
	}
}

func TestClientCapabilitiesForAgentAdvertisesCompactionOnly(t *testing.T) {
	caps := clientCapabilitiesForAgent("claude-acp", false)
	if caps.Session == nil || caps.Session.Compaction == nil || caps.Session.Notices != nil {
		t.Fatalf("session capabilities = %+v, want compaction true and notices false", caps.Session)
	}
	if caps.Subagents != nil {
		t.Fatalf("client capabilities unexpectedly advertise subagents: %+v", caps)
	}
}
