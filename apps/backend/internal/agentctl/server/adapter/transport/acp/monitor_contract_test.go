package acp

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// The Monitor view is a cross-package contract: this file (the producer) writes
// `Generic.Output = {"monitor": {...}}`, and streams.IsActiveMonitor (the
// consumer, via the orchestrator's background-work classifier) reads it back to
// decide whether a RUNNING session's foreground turn has yielded to background
// work (ADR-0049).
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

// Provenance guard, from the producer's side. Generic.Output is the agent's own
// raw tool result (NormalizeToolResult assigns it verbatim), so an unrelated tool
// can emit a byte-for-byte perfect Monitor view there. Only the adapter's
// out-of-band attestation — which it stamps solely on the path gated by ACP
// `_meta.claudeCode.toolName`, metadata the model cannot reach — counts.
//
// This is what keeps ADR-0049's contract honest: an agent we don't recognize can't
// relax its own busy gate by shaping its tool output.
func TestMonitorViewContract_ArbitraryToolOutputIsNotMistakenForAMonitor(t *testing.T) {
	// Exactly the map monitorOutputWrapper produces — but written by "the agent"
	// rather than by the adapter, so no seedMonitorView, and no attestation.
	forgedButPerfect := monitorOutputWrapper(monitorPayloadView{
		Kind:   monitorToolName,
		TaskID: "task-abc",
		Ended:  false,
	})

	cases := map[string]any{
		"forged view identical to the adapter's own output": forgedButPerfect,
		"unrelated tool result":                             map[string]any{"status": "ok", "rows": 3},
		"plain string output":                               "Monitor started (task 42, timeout 1000ms)",
	}

	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			p := streams.NewGeneric("other", map[string]any{})
			p.Generic().Output = output

			if p.IsActiveMonitor() {
				t.Fatal("tool output the adapter never attested to must not be classified as background work")
			}
		})
	}
}

// The attestation must actually be stamped by the real producer path — if
// seedMonitorView ever stopped calling SetMonitorIdentity, IsActiveMonitor would
// silently return false for every genuine Monitor and quietly un-ship the feature.
func TestMonitorViewContract_SeedStampsAdapterAttestation(t *testing.T) {
	p := newMonitorGeneric()
	seedMonitorView(p, "task-abc", "gh pr checks --watch")

	m := p.Monitor()
	if m == nil {
		t.Fatal("seedMonitorView must stamp the adapter's Monitor attestation")
	}
	if m.TaskID != "task-abc" {
		t.Fatalf("attested task ID = %q, want task-abc", m.TaskID)
	}
	if m.Ended {
		t.Fatal("a freshly seeded Monitor is not ended")
	}

	markMonitorEnded(p, "exited")
	if !p.Monitor().Ended {
		t.Fatal("markMonitorEnded must keep the attestation in step with the view")
	}
}
