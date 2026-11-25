package opencodeplugin

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

var errTestBehaviorUnavailable = errors.New("test behavior observation is unavailable")

func TestBehaviorRegistryContract(t *testing.T) {
	registry := ProductionRegistry()
	refs := registry.Refs()
	if len(refs) != 1 || refs[0].ID != BehaviorShellOutcomes || refs[0].Version != ShellOutcomesVersion {
		t.Fatalf("production registry = %#v, want one shell-outcomes behavior", refs)
	}
	if _, ok := registry.Get(BehaviorShellOutcomes, ShellOutcomesVersion); !ok {
		t.Fatal("production registry does not resolve its declared version")
	}
	if _, ok := registry.Get(BehaviorShellOutcomes, ShellOutcomesVersion+1); ok {
		t.Fatal("registry resolved an unknown behavior version")
	}
	if _, ok := registry.Get("unknown-behavior", 1); ok {
		t.Fatal("registry resolved an unknown behavior")
	}
}

func TestBehaviorRegistryAcceptsATestModuleWithoutCoreBranches(t *testing.T) {
	testBehavior := testBehaviorModule{}
	registry := NewRegistry(ShellOutcomesBehavior{}, testBehavior)
	refs := registry.Refs()
	if len(refs) != 2 || refs[1].ID != "test-behavior" || refs[1].Version != 7 {
		t.Fatalf("registry refs = %#v", refs)
	}
	if _, ok := registry.Get("test-behavior", 7); !ok {
		t.Fatal("registry did not resolve the test module")
	}
}

func TestShellOutcomesMapsEveryNativeTerminalFact(t *testing.T) {
	behavior := ShellOutcomesBehavior{}
	claim := testClaim()
	state := shellState(t, []Claim{claim})
	cases := []struct {
		name   string
		status string
		exit   *int
		want   streams.RunState
	}{
		{name: "success", status: "exited", exit: intPtr(0), want: streams.RunStateCompleted},
		{name: "nonzero", status: "exited", exit: intPtr(3), want: streams.RunStateFailed},
		{name: "missing exit", status: "exited", exit: nil, want: streams.RunStateEnded},
		{name: "timeout", status: "timeout", exit: nil, want: streams.RunStateFailed},
		{name: "killed", status: "killed", exit: intPtr(137), want: streams.RunStateInterrupted},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload, _ := json.Marshal(ShellTerminalPayload{
				Kind: "shell.terminal", ShellID: claim.ShellID, Status: testCase.status, ExitCode: testCase.exit,
			})
			admission, err := behavior.Admit(state, shellEnvelope(claim, payload))
			if err != nil {
				t.Fatalf("Admit() error = %v", err)
			}
			observation := admission.Observation
			if observation.State != testCase.want {
				t.Fatalf("state = %q, want %q", observation.State, testCase.want)
			}
			if observation.WorkID != claim.WorkID || observation.RunID != claim.RunID ||
				observation.SourceCallID != claim.SourceCallID || observation.Title != claim.Title {
				t.Fatalf("observation lost the launch association: %#v", observation)
			}
			if observation.Capabilities.Output != "none" || observation.Capabilities.Discovery != "events_only" {
				t.Fatalf("observation invented capabilities: %#v", observation.Capabilities)
			}
		})
	}
}

func TestShellOutcomesRejectsForeignOrMalformedEvidence(t *testing.T) {
	behavior := ShellOutcomesBehavior{}
	claim := testClaim()
	state := shellState(t, []Claim{claim})
	for _, payload := range []ShellTerminalPayload{
		{Kind: "other", ShellID: claim.ShellID, Status: "exited"},
		{Kind: "shell.terminal", ShellID: "other-shell", Status: "exited"},
		{Kind: "shell.terminal", ShellID: claim.ShellID, Status: "running"},
		{Kind: "shell.terminal", ShellID: claim.ShellID, Status: "mystery"},
	} {
		raw, _ := json.Marshal(payload)
		if _, err := behavior.Admit(state, shellEnvelope(claim, raw)); err == nil {
			t.Fatalf("Admit(%#v) accepted invalid evidence", payload)
		}
	}
}

func TestClaimValidationRejectsIncompleteAuthority(t *testing.T) {
	cases := []Claim{
		{},
		{ShellID: "shell", NativeGeneration: 1, WorkID: "work"},
		{ShellID: "shell", NativeSessionID: "session", WorkID: "work"},
		{ShellID: "shell", NativeSessionID: "session", NativeGeneration: 1},
	}
	for _, claim := range cases {
		if err := claim.Validate(); err == nil {
			t.Fatalf("Claim.Validate() accepted %#v", claim)
		}
	}
	if err := testClaim().Validate(); err != nil {
		t.Fatalf("valid claim rejected: %v", err)
	}
}

func shellState(t *testing.T, claims []Claim) any {
	t.Helper()
	raw, err := MarshalShellConfig(claims)
	if err != nil {
		t.Fatal(err)
	}
	state, err := (ShellOutcomesBehavior{}).DecodeConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func shellEnvelope(claim Claim, payload []byte) ObservationEnvelope {
	return ObservationEnvelope{
		EventID:          "event-1",
		BehaviorID:       BehaviorShellOutcomes,
		BehaviorVersion:  ShellOutcomesVersion,
		NativeSessionID:  claim.NativeSessionID,
		NativeGeneration: claim.NativeGeneration,
		Payload:          payload,
	}
}

type testBehaviorModule struct{}

func (testBehaviorModule) ID() string   { return "test-behavior" }
func (testBehaviorModule) Version() int { return 7 }

type testBehaviorConfig struct {
	Resource string `json:"resource"`
}

func (testBehaviorModule) DecodeConfig(raw json.RawMessage) (any, error) {
	var config testBehaviorConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func (testBehaviorModule) EncodeConfig(state any) (json.RawMessage, error) {
	return json.Marshal(state)
}

func (testBehaviorModule) Admit(state any, envelope ObservationEnvelope) (Admission, error) {
	config, ok := state.(testBehaviorConfig)
	if !ok {
		return Admission{}, errTestBehaviorUnavailable
	}
	var payload struct {
		Kind     string `json:"kind"`
		Resource string `json:"resource"`
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return Admission{}, errTestBehaviorUnavailable
	}
	if payload.Kind != "test.completed" || payload.Resource != config.Resource {
		return Admission{}, errTestBehaviorUnavailable
	}
	return Admission{
		EventID:          envelope.EventID,
		BehaviorID:       "test-behavior",
		NativeSessionID:  envelope.NativeSessionID,
		NativeGeneration: envelope.NativeGeneration,
		Observation: streams.WorkloadRunObservation{
			WorkID: payload.Resource,
			Kind:   streams.WorkloadKindCustom,
			Title:  payload.Resource,
			State:  streams.RunStateCompleted,
		},
		ClaimKey: payload.Resource,
	}, nil
}

func (testBehaviorModule) Retire(state any, _ Admission) (any, bool) {
	return state, false
}
