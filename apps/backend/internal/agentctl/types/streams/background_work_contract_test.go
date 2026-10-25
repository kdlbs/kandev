package streams

import (
	"encoding/json"
	"testing"
)

func TestBackgroundWorkContractCompatibility(t *testing.T) {
	tests := []struct {
		input string
		want  WorkloadKind
	}{
		{"shell", WorkloadKindShell},
		{"subagent", WorkloadKindSubagent},
		{"monitor", WorkloadKindMonitor},
		{"custom", WorkloadKindCustom},
		{"unknown", WorkloadKindUnknown},
		{"random-provider-string", WorkloadKindUnknown},
		{"", WorkloadKindUnknown},
	}

	for _, tt := range tests {
		if got := NormalizeWorkloadKind(tt.input); got != tt.want {
			t.Errorf("NormalizeWorkloadKind(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}

	terminalStates := []RunState{RunStateCompleted, RunStateFailed, RunStateInterrupted, RunStateEnded}
	for _, state := range terminalStates {
		if !state.IsTerminal() {
			t.Errorf("RunState(%q).IsTerminal() = false, want true", state)
		}
	}

	nonTerminalStates := []RunState{RunStateRunning, RunStateWaiting, RunStateUnknown}
	for _, state := range nonTerminalStates {
		if state.IsTerminal() {
			t.Errorf("RunState(%q).IsTerminal() = true, want false", state)
		}
	}

	obs := WorkloadRunObservation{
		WorkID: "w-1",
		Kind:   WorkloadKindShell,
		State:  RunStateRunning,
		Capabilities: WorkloadCapabilities{
			Discovery: "snapshot",
			Output:    "stream",
			Actions: map[WorkloadActionKind]ActionCapability{
				WorkloadActionStop: {Supported: true, Available: true},
				WorkloadActionWriteInput: {
					Supported: true,
					Available: false,
					Reason:    string(ActionReasonDisconnected),
				},
			},
		},
	}

	encoded, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("Marshal WorkloadRunObservation: %v", err)
	}

	var decoded WorkloadRunObservation
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal WorkloadRunObservation: %v", err)
	}

	if decoded.WorkID != "w-1" || decoded.Kind != WorkloadKindShell || decoded.State != RunStateRunning {
		t.Fatalf("decoded observation mismatch: %#v", decoded)
	}
	if !decoded.Capabilities.Actions[WorkloadActionStop].Available {
		t.Errorf("expected stop action to be available")
	}
	if decoded.Capabilities.Actions[WorkloadActionWriteInput].Reason != string(ActionReasonDisconnected) {
		t.Errorf("expected write_input action reason disconnected")
	}
}
