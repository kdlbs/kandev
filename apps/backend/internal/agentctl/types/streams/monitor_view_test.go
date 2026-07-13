package streams

import "testing"

// monitorView builds a Generic payload shaped like the one acp/monitor.go's
// monitorOutputWrapper produces. The producer→consumer contract itself is pinned
// against the *real* producer functions in acp's monitor_contract_test.go; these
// cases exercise the predicate's own branches.
func monitorView(ended bool) *NormalizedPayload {
	p := NewGeneric("other", map[string]any{})
	p.Generic().Output = map[string]any{
		MonitorViewKey: map[string]any{
			MonitorViewKindKey:    MonitorSubkind,
			MonitorViewEndedKey:   ended,
			MonitorViewTaskIDKey:  "task-1",
			MonitorViewCommandKey: "gh pr checks --watch",
		},
	}
	return p
}

// genericWithOutput builds the payload an *arbitrary* agent tool produces:
// NormalizeToolResult assigns the agent's raw result straight to Generic.Output,
// so these are the shapes the predicate has to refuse.
func genericWithOutput(output any) *NormalizedPayload {
	p := NewGeneric("other", map[string]any{})
	p.Generic().Output = output
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
			genericWithOutput(map[string]any{
				MonitorViewKey: map[string]any{MonitorViewKindKey: "Something", MonitorViewEndedKey: false},
			}),
			false,
		},
		{
			// Provenance: the adapter only ever publishes an active view once the
			// registration banner handed it a real task ID. A monitor-shaped blob
			// without one did not come off the Monitor path — an unrelated agent's
			// tool result must not relax the busy gate.
			"monitor-shaped output with no task_id",
			genericWithOutput(map[string]any{
				MonitorViewKey: map[string]any{
					MonitorViewKindKey:  MonitorSubkind,
					MonitorViewEndedKey: false,
				},
			}),
			false,
		},
		{
			"monitor-shaped output with empty task_id",
			genericWithOutput(map[string]any{
				MonitorViewKey: map[string]any{
					MonitorViewKindKey:   MonitorSubkind,
					MonitorViewEndedKey:  false,
					MonitorViewTaskIDKey: "",
				},
			}),
			false,
		},
		{"generic with string output", genericWithOutput("monitor"), false},
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
