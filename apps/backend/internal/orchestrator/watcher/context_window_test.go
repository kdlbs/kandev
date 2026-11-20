package watcher

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// TestContextWindowCompactionPolicyRoundTrip proves the eligibility marker
// survives the JSON conversion the watcher applies before persistence. A
// dropped field would silently restore false compaction increments.
func TestContextWindowCompactionPolicyRoundTrip(t *testing.T) {
	w := &Watcher{logger: logger.Default()}
	var got ContextWindowData
	w.handlers = EventHandlers{
		OnContextWindowUpdated: func(_ context.Context, data ContextWindowData) {
			got = data
		},
	}
	handler := w.createContextWindowHandler()

	excluded := lifecycle.ContextWindowEventPayload{
		TaskID:                      "task-policy",
		SessionID:                   "sess-policy",
		AgentID:                     "exec-copilot",
		ContextWindowSize:           200000,
		ContextWindowUsed:           2877,
		ContextWindowRemaining:      197123,
		ContextEfficiency:           1,
		CompactionInferenceDisabled: true,
	}
	if err := handler(context.Background(), bus.NewEvent(events.ContextWindowUpdated, "test", excluded)); err != nil {
		t.Fatalf("excluded event: %v", err)
	}
	if !got.CompactionInferenceDisabled {
		t.Fatal("excluded marker was dropped during watcher conversion")
	}
	if got.ContextWindowUsed != 2877 || got.TaskSessionID != "sess-policy" {
		t.Fatalf("usage fields were not preserved: %#v", got)
	}

	legacy := map[string]interface{}{
		"task_id":                  "task-policy",
		"session_id":               "sess-policy",
		"agent_id":                 "exec-legacy",
		"context_window_size":      int64(200000),
		"context_window_used":      int64(120000),
		"context_window_remaining": int64(80000),
		"context_efficiency":       60.0,
	}
	if err := handler(context.Background(), bus.NewEvent(events.ContextWindowUpdated, "test", legacy)); err != nil {
		t.Fatalf("legacy event: %v", err)
	}
	if got.CompactionInferenceDisabled {
		t.Fatal("legacy event without a marker must stay eligible")
	}
	if got.ContextWindowUsed != 120000 {
		t.Fatalf("legacy usage fields were not preserved: %#v", got)
	}
}
