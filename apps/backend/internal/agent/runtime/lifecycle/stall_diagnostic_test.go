package lifecycle

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"
)

// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.15
func TestStallDiagnosticBoundsAndExcludesPrivatePayloads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log, observed := observedLogger(t)
		sm := NewSessionManager(log, make(chan struct{}))
		const private = "private-prompt-and-token-sentinel"
		execution := &AgentExecution{
			ID:     strings.Repeat("x", 127) + "é",
			TaskID: string([]byte{0xff}), SessionID: "session",
			AgentID: "codex", AgentCommand: private, AgentArgs: []string{private},
			DeliveryMode:  DurableDeliveryMode(strings.Repeat(private, 200)),
			WorkspacePath: private,
			metadata:      map[string]interface{}{"prompt": private, "cli_version": private},
			promptDoneCh:  make(chan PromptCompletionSignal, 1),
		}
		generation := beginExecutionPrompt(execution)
		sm.markPromptDispatched(execution, generation)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			_, _ = sm.waitForPromptDone(ctx, execution, generation)
			close(done)
		}()
		defer func() { cancel(); <-done }()
		time.Sleep(7 * time.Minute)
		synctest.Wait()

		entries := observed.FilterMessage("agent stall detected: no events received").All()
		if len(entries) != 1 {
			t.Fatalf("stall records = %d, want 1", len(entries))
		}
		raw, err := json.Marshal(entries[0].ContextMap())
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 4096 || strings.Contains(string(raw), private) {
			t.Fatal("stall diagnostic exceeds its bound or contains private payloads")
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		var snapshot map[string]interface{}
		if err := json.Unmarshal(record["stall"], &snapshot); err != nil {
			t.Fatalf("missing bounded stall snapshot: %v", err)
		}
		if snapshot["prompt_accepted"] != true || snapshot["output_observed"] != false ||
			snapshot["effect_observed"] != false {
			t.Fatalf("incorrect acceptance/output/effect snapshot: %#v", snapshot)
		}
		id, _ := snapshot["execution_id"].(string)
		if id != strings.Repeat("x", 127) || !utf8.ValidString(id) || len(id) > 128 {
			t.Fatalf("execution ID was not truncated at a rune boundary: %q", id)
		}
		if _, exists := snapshot["task_id"]; exists {
			t.Fatal("malformed UTF-8 must be omitted")
		}
	})
}
