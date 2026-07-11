package streams

import "testing"

// monitorView builds a Generic payload shaped exactly like the one
// acp/monitor.go's monitorOutputWrapper produces, so the predicate is tested
// against the real wire shape rather than a hand-tuned proxy.
func monitorView(ended bool) *NormalizedPayload {
	p := NewGeneric("Monitor", map[string]any{})
	p.Generic().Output = map[string]any{
		monitorViewKey: map[string]any{
			monitorViewKindKey:  MonitorSubkind,
			monitorViewEndedKey: ended,
			"task_id":           "task-1",
			"command":           "gh pr checks --watch",
		},
	}
	return p
}

func TestIsActiveMonitor(t *testing.T) {
	cases := []struct {
		name    string
		payload *NormalizedPayload
		want    bool
	}{
		{"active monitor", monitorView(false), true},
		{"ended monitor", monitorView(true), false},
		{"nil payload", nil, false},
		{"non-generic payload", NewShellExec("ls", "", "", 0, false), false},
		{"generic without monitor view", NewGeneric("SomeTool", map[string]any{"a": 1}), false},
		{
			"generic with wrong subkind",
			func() *NormalizedPayload {
				p := NewGeneric("Other", map[string]any{})
				p.Generic().Output = map[string]any{
					monitorViewKey: map[string]any{monitorViewKindKey: "Something", monitorViewEndedKey: false},
				}
				return p
			}(),
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.payload.IsActiveMonitor(); got != tc.want {
				t.Fatalf("IsActiveMonitor() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIsActiveMonitor_SurvivesJSONRoundTrip proves the predicate still works
// after the payload crosses the agentctl→orchestrator serialization boundary,
// where Generic.Output decodes back into a map[string]any.
func TestIsActiveMonitor_SurvivesJSONRoundTrip(t *testing.T) {
	orig := monitorView(false)
	data, err := orig.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got NormalizedPayload
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.IsActiveMonitor() {
		t.Fatal("active Monitor must remain recognized after a JSON round-trip")
	}
}
