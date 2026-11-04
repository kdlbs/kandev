package main

import (
	acp "github.com/coder/acp-go-sdk"
)

func scenarioContextCompaction(e *emitter) {
	if !e.supportsCompaction {
		e.text("This agent did not receive context compaction support.")
		return
	}

	e.text("I am preserving the key decisions while compacting the earlier context.")
	e.compactionUpdate("compaction-completed", "in_progress", nil, nil)
	waitForDelay(e.ctx, 120)
	e.compactionSummaryChunk("compaction-completed", "## Retained context\n")
	e.compactionSummaryChunk("compaction-completed", "\n- Keep the delivery cursor stable.\n- Preserve the user's request.")
	e.compactionUpdate("compaction-completed", "completed", []acp.ContentBlock{acp.TextBlock("## Retained context\n\n- Keep the delivery cursor stable.\n- Preserve the user's request.")}, map[string]any{
		"contextCompaction": map[string]any{"trigger": "manual", "pre_tokens": 31_300, "post_tokens": 2_400, "duration_ms": 26_800},
	})
	e.compactionUpdate("compaction-failed", "failed", nil, map[string]any{"error": "provider could not summarize one optional section"})
}

func (e *emitter) compactionUpdate(id, status string, summary []acp.ContentBlock, meta map[string]any) {
	update := &acp.SessionCompactionUpdate{CompactionId: acp.CompactionId(id), Status: acp.CompactionStatus(status)}
	if summary != nil {
		update.Summary = acp.OptionalValue(summary)
	}
	if meta != nil {
		if message, ok := meta["error"].(string); ok {
			update.Error = acp.OptionalValue(message)
		}
		if _, hasContextMetadata := meta["contextCompaction"]; hasContextMetadata {
			update.Meta = acp.OptionalValue(meta)
		}
	}
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update:    acp.SessionUpdate{CompactionUpdate: update},
	})
}

func (e *emitter) compactionSummaryChunk(id, text string) {
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update: acp.SessionUpdate{CompactionSummaryChunk: &acp.SessionUpdateCompactionSummaryChunk{
			CompactionId: acp.CompactionId(id),
			Content:      acp.TextBlock(text),
		}},
	})
}
