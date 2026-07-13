package streams

import "testing"

// monitorPayload builds the payload the ACP adapter produces for a recognized
// Monitor: a Generic payload (Monitor arrives with ACP kind "other") carrying both
// the presentation view in Output *and* the adapter's out-of-band attestation.
func monitorPayload(ended bool) *NormalizedPayload {
	p := NewGeneric("other", map[string]any{})
	p.Generic().Output = monitorViewOutput(ended)
	p.SetMonitorIdentity("task-1", ended)
	return p
}

// monitorViewOutput is the presentation map the frontend Monitor card renders.
// On its own it proves nothing about provenance — see the forgery test below.
func monitorViewOutput(ended bool) map[string]any {
	return map[string]any{
		MonitorViewKey: map[string]any{
			MonitorViewKindKey:    MonitorSubkind,
			MonitorViewEndedKey:   ended,
			MonitorViewTaskIDKey:  "task-1",
			MonitorViewCommandKey: "gh pr checks --watch",
		},
	}
}

func TestIsActiveMonitor(t *testing.T) {
	cases := []struct {
		name    string
		payload *NormalizedPayload
		want    bool
	}{
		{"active monitor", monitorPayload(false), true},
		{"ended monitor", monitorPayload(true), false},
		{"nil payload", nil, false},
		{"non-generic payload", NewShellExec("ls", "", "", 0, false), false},
		{"generic with no monitor identity", NewGeneric("SomeTool", map[string]any{"a": 1}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.payload.IsActiveMonitor(); got != tc.want {
				t.Fatalf("IsActiveMonitor() = %v, want %v", got, tc.want)
			}
		})
	}
}

// The provenance test that matters. Generic.Output is the agent's own raw tool
// result (NormalizeToolResult assigns it verbatim), so an unrelated tool can emit a
// *byte-for-byte perfect* Monitor view there. Without the adapter's attestation it
// must still not be classified as background work — otherwise an agent could relax
// its own busy gate and slip a second prompt into a live foreground turn, breaking
// ADR-0035's "unrecognized agents keep reject-while-RUNNING" contract.
func TestIsActiveMonitor_ForgedOutputWithoutAdapterAttestationIsRejected(t *testing.T) {
	forged := NewGeneric("other", map[string]any{})
	forged.Generic().Output = monitorViewOutput(false) // identical to the real thing
	// …but no SetMonitorIdentity: the adapter never recognized this as a Monitor.

	if forged.IsActiveMonitor() {
		t.Fatal("a monitor-shaped tool result with no adapter attestation must not be classified as background work")
	}

	// And it stays rejected across the wire, where a naive shape-matcher would be
	// fooled by the decoded map.
	data, err := forged.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded NormalizedPayload
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.IsActiveMonitor() {
		t.Fatal("forged monitor output must stay unrecognized after serialization")
	}
}

// The attestation is what has to survive the agentctl→orchestrator boundary — the
// classifier runs on the far side of it.
func TestIsActiveMonitor_AttestationSurvivesJSONRoundTrip(t *testing.T) {
	data, err := monitorPayload(false).MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got NormalizedPayload
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.IsActiveMonitor() {
		t.Fatal("an attested active Monitor must remain recognized after a JSON round-trip")
	}
	if got.Monitor() == nil || got.Monitor().TaskID != "task-1" {
		t.Fatalf("attested task ID must survive the round-trip, got %+v", got.Monitor())
	}
}
