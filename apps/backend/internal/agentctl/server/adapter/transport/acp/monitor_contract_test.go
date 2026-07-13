package acp

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// The Monitor view is a cross-package contract: this file (the producer) writes
// `Generic.Output = {"monitor": {...}}`, and streams.IsActiveMonitor (the
// consumer, via the orchestrator's background-work classifier) reads it back to
// decide whether a RUNNING session's foreground turn has yielded to background
// work (ADR-0035).
//
// The two sides used to spell the map keys out independently, so a rename on
// either side compiled cleanly and silently reverted Monitor sessions to the
// coarse busy signal. They now share streams.MonitorView*Key, and these tests
// pin the round trip through the *real* producer functions rather than a
// hand-rolled replica of the shape.

// newMonitorGeneric mirrors what the adapter actually has in hand when a Monitor
// registers: normalizeGeneric built the payload from the ACP tool *kind*, which
// claude-agent-acp sends as "other" for Monitor (the "Monitor" name only appears
// in `_meta.claudeCode.toolName`). Starting from this exact payload is what makes
// the test a contract test — it is why GenericPayload.Name cannot serve as the
// Monitor discriminator.
func newMonitorGeneric() *streams.NormalizedPayload {
	return streams.NewGeneric("other", map[string]any{})
}

func TestMonitorViewContract_SeededMonitorIsRecognizedAsActive(t *testing.T) {
	p := newMonitorGeneric()
	seedMonitorView(p, "task-abc", "gh pr checks --watch")

	if !p.IsActiveMonitor() {
		t.Fatal("a Monitor view written by seedMonitorView must be recognized as active background work")
	}
}

func TestMonitorViewContract_EventsKeepMonitorActive(t *testing.T) {
	p := newMonitorGeneric()
	seedMonitorView(p, "task-abc", "gh pr checks --watch")
	appendMonitorEvent(p, "task-abc", "gh pr checks --watch", "all checks passing")

	if !p.IsActiveMonitor() {
		t.Fatal("a Monitor that fired an event is still watching and must stay active")
	}
}

func TestMonitorViewContract_EndedMonitorIsNotActive(t *testing.T) {
	p := newMonitorGeneric()
	seedMonitorView(p, "task-abc", "gh pr checks --watch")
	markMonitorEnded(p, "exited")

	if p.IsActiveMonitor() {
		t.Fatal("an ended Monitor must not hold the busy signal open")
	}
}

// The payload crosses a process boundary (agentctl → orchestrator) as JSON, so
// the producer's typed view decodes back into map[string]any before the consumer
// ever sees it. The contract has to survive that, not just hold in-process.
func TestMonitorViewContract_SurvivesSerializationToOrchestrator(t *testing.T) {
	p := newMonitorGeneric()
	seedMonitorView(p, "task-abc", "gh pr checks --watch")

	data, err := p.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded streams.NormalizedPayload
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !decoded.IsActiveMonitor() {
		t.Fatal("the Monitor view must still classify as active after the agentctl→orchestrator round trip")
	}
}

// Provenance guard. A Generic payload's Output is otherwise the agent's own raw
// tool result, assigned verbatim by NormalizeToolResult. An unrelated agent whose
// tool happens to emit a `monitor` key must NOT trip the background gate — the
// PR's contract is that any agent we don't recognize keeps the historical
// reject-while-RUNNING behavior. The adapter only ever publishes an active view
// with a real task_id (it comes from the registration banner), so a forged-looking
// view without one is rejected.
func TestMonitorViewContract_ArbitraryToolOutputIsNotMistakenForAMonitor(t *testing.T) {
	cases := map[string]any{
		"monitor-shaped output with no task_id": map[string]any{
			streams.MonitorViewKey: map[string]any{
				streams.MonitorViewKindKey:  streams.MonitorSubkind,
				streams.MonitorViewEndedKey: false,
			},
		},
		"unrelated tool result": map[string]any{"status": "ok", "rows": 3},
		"plain string output":   "Monitor started (task 42, timeout 1000ms)",
	}

	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			p := streams.NewGeneric("other", map[string]any{})
			p.Generic().Output = output

			if p.IsActiveMonitor() {
				t.Fatal("non-Monitor tool output must not be classified as background work")
			}
		})
	}
}
