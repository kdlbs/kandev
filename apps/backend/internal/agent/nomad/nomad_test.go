package nomad

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNomadProfileRequiresPinnedNativeWorkerAndSecretReference(t *testing.T) {
	profile := validTestProfile()
	parsed, err := ParseProfileConfig(profile)
	if err != nil {
		t.Fatalf("ParseProfileConfig(valid) error: %v", err)
	}
	if parsed.NodeID != "node-b" || parsed.Platform != "darwin/arm64" || parsed.NomadTokenSecretID != "secret-nomad-token" {
		t.Fatalf("parsed profile lost pinned identity or secret reference: %+v", parsed)
	}

	for name, mutate := range map[string]func(map[string]string){
		"missing node id":      func(c map[string]string) { delete(c, "node_id") },
		"unsupported platform": func(c map[string]string) { c["platform"] = "linux/arm64" },
		"token literal":        func(c map[string]string) { c["nomad_token"] = "do-not-persist-this" },
		"remote cleartext api": func(c map[string]string) { c["address"] = "http://nomad.example:4646" },
	} {
		t.Run(name, func(t *testing.T) {
			config := cloneTestConfig(profile)
			mutate(config)
			if _, err := ParseProfileConfig(config); err == nil {
				t.Fatal("ParseProfileConfig() accepted an unsafe or incomplete profile")
			}
		})
	}
}

func TestNomadEnvironmentStateValidatesIdentityAndExcludesCredentialValues(t *testing.T) {
	state := validTestState()
	encoded, err := MarshalEnvironmentState(state)
	if err != nil {
		t.Fatalf("MarshalEnvironmentState(valid) error: %v", err)
	}
	if strings.Contains(string(encoded), "do-not-persist-this") {
		t.Fatal("serialized Nomad state contains a credential value")
	}
	var decoded EnvironmentState
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode serialized state: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("decoded state validation: %v", err)
	}

	invalid := state
	invalid.InputDigest = ""
	if _, err := MarshalEnvironmentState(invalid); err == nil {
		t.Fatal("MarshalEnvironmentState() accepted missing immutable operation identity")
	}
	invalid = state
	invalid.WorkspacePath = "/srv/kandev/workspaces/other-environment/g3"
	if _, err := MarshalEnvironmentState(invalid); err == nil {
		t.Fatal("MarshalEnvironmentState() accepted a workspace path owned by another environment")
	}
}

func TestNomadJobIDIsDeterministicAndGenerationScoped(t *testing.T) {
	identity := JobIdentity{
		EnvironmentID: "env-1", Generation: 3, OperationID: "op-1", InstallationID: "install-1",
	}
	first, err := StableJobID(identity)
	if err != nil {
		t.Fatalf("StableJobID() error: %v", err)
	}
	second, err := StableJobID(identity)
	if err != nil || first != second {
		t.Fatalf("same identity produced different job IDs: %q %q (%v)", first, second, err)
	}
	identity.Generation++
	third, err := StableJobID(identity)
	if err != nil || third == first {
		t.Fatalf("successor generation reused job ID %q: %v", first, err)
	}
}

func TestNomadJobIDRequiresEveryOwnerDimension(t *testing.T) {
	identity := JobIdentity{EnvironmentID: "env-1", Generation: 1, OperationID: "op-1"}
	if _, err := StableJobID(identity); err == nil {
		t.Fatal("StableJobID() accepted an identity without installation ownership")
	}
}

func TestEnvironmentWorkspacePathIsScopedByEnvironmentAndGeneration(t *testing.T) {
	first, err := EnvironmentWorkspacePath("/srv/kandev/workspaces", "env-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	secondEnvironment, err := EnvironmentWorkspacePath("/srv/kandev/workspaces", "env-b", 1)
	if err != nil {
		t.Fatal(err)
	}
	secondGeneration, err := EnvironmentWorkspacePath("/srv/kandev/workspaces", "env-a", 2)
	if err != nil {
		t.Fatal(err)
	}
	if first == secondEnvironment || first == secondGeneration {
		t.Fatalf("environment workspace path was reused: %q", first)
	}
	if want := "/srv/kandev/workspaces/environments/env-a/g1"; first != want {
		t.Fatalf("environment workspace = %q, want %q", first, want)
	}
	for _, input := range []struct {
		root string
		id   string
		gen  int64
	}{{"relative", "env-a", 1}, {"/srv/workspaces", "../escape", 1}, {"/srv/workspaces", "env-a", 0}} {
		if _, err := EnvironmentWorkspacePath(input.root, input.id, input.gen); err == nil {
			t.Errorf("accepted unsafe workspace identity root=%q environment=%q generation=%d", input.root, input.id, input.gen)
		}
	}
}

func TestUnmarshalEnvironmentStateRejectsUnknownCredentialFields(t *testing.T) {
	encoded, err := json.Marshal(validTestState())
	if err != nil {
		t.Fatal(err)
	}
	withCredential := strings.TrimSuffix(string(encoded), "}") + `,"nomad_token":"must-not-persist"}`
	if _, err := UnmarshalEnvironmentState([]byte(withCredential)); err == nil {
		t.Fatal("UnmarshalEnvironmentState() accepted an unknown credential field")
	}
}

func TestEnvironmentStateRejectsPartialAllocationAndReceiptIdentity(t *testing.T) {
	state := validTestState()
	state.AllocationID = "alloc-1"
	if err := state.Validate(); err == nil {
		t.Fatal("Validate() accepted allocation identity without create index")
	}
	state = validTestState()
	state.DrainReceiptDigest = strings.Repeat("b", 64)
	if err := state.Validate(); err == nil {
		t.Fatal("Validate() accepted a receipt without matching ownership generation")
	}
}

func TestEnvironmentStateRequiresPositiveReceiptForAbsence(t *testing.T) {
	state := validTestState()
	state.Phase = "absent"
	if err := state.Validate(); err == nil {
		t.Fatal("Validate() accepted absent state without cleanup receipt")
	}
	state.DrainReceiptDigest = strings.Repeat("b", 64)
	state.DrainReceiptGeneration = state.Generation
	if err := state.Validate(); err != nil {
		t.Fatalf("valid absent receipt rejected: %v", err)
	}
}

func TestNomadProfileAllowsOnlyLoopbackHTTPForDevelopment(t *testing.T) {
	profile := validTestProfile()
	profile["address"] = "http://127.0.0.1:4646"
	if _, err := ParseProfileConfig(profile); err != nil {
		t.Fatalf("loopback development profile rejected: %v", err)
	}
	profile["address"] = "http://nomad.example.test:4646"
	if _, err := ParseProfileConfig(profile); err == nil {
		t.Fatal("ParseProfileConfig() accepted non-loopback cleartext Nomad API")
	}
}

func TestEnvironmentStateRoundTripsThroughExecutorRunningMetadata(t *testing.T) {
	metadata := map[string]any{"unrelated": "keep"}
	if err := PutEnvironmentState(metadata, validTestState()); err != nil {
		t.Fatalf("PutEnvironmentState() error: %v", err)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var reopened map[string]any
	if err := json.Unmarshal(encoded, &reopened); err != nil {
		t.Fatal(err)
	}
	state, err := EnvironmentStateFromMetadata(reopened)
	if err != nil {
		t.Fatalf("EnvironmentStateFromMetadata() error: %v", err)
	}
	if state.Generation != 3 || reopened["unrelated"] != "keep" {
		t.Fatalf("metadata round trip lost owner state or unrelated metadata: %+v %#v", state, reopened)
	}
}

func TestEnvironmentStatePreservesCapturedEmptyWorkspaceInput(t *testing.T) {
	state := validTestState()
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	withCapturedInput := strings.TrimSuffix(string(encoded), "}") + `,"workspace_input_path":"","workspace_input_captured":true}`
	decoded, err := UnmarshalEnvironmentState([]byte(withCapturedInput))
	if err != nil {
		t.Fatalf("UnmarshalEnvironmentState() rejected a captured empty original input: %v", err)
	}
	reencoded, err := MarshalEnvironmentState(decoded)
	if err != nil {
		t.Fatalf("MarshalEnvironmentState() rejected the captured state: %v", err)
	}
	if !strings.Contains(string(reencoded), `"workspace_input_captured":true`) || !strings.Contains(string(reencoded), `"workspace_input_path":""`) {
		t.Fatalf("captured empty workspace input was not preserved: %s", reencoded)
	}

	withUncapturedPath := strings.TrimSuffix(string(encoded), "}") + `,"workspace_input_path":"/stale/input","workspace_input_captured":false}`
	if _, err := UnmarshalEnvironmentState([]byte(withUncapturedPath)); err == nil {
		t.Fatal("UnmarshalEnvironmentState() accepted a nonempty workspace input without capture evidence")
	}
}

func validTestProfile() map[string]string {
	return map[string]string{
		"address":              "https://nomad.example.test:4646",
		"namespace":            "default",
		"region":               "global",
		"datacenter":           "dc1",
		"node_id":              "node-b",
		"platform":             "darwin/arm64",
		"ssh_target":           "kandev@worker-b.example",
		"ssh_port":             "22",
		"ssh_host_fingerprint": "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		"workspace_root":       "/var/lib/kandev/workspaces",
		"allocation_root":      "/var/lib/kandev/allocations",
		"cpu_mhz":              "1500",
		"memory_mb":            "4096",
		"nomad_token":          "$kandev-secret:secret-nomad-token",
	}
}

func validTestState() EnvironmentState {
	return EnvironmentState{
		Version: 1, EnvironmentID: "env-1", Generation: 3, OperationID: "op-1", InstallationID: "install-1", ExecutionID: "execution-1",
		InputDigest: strings.Repeat("a", 64), JobID: "kandev-env-1-g3-op-1",
		Namespace: "default", Region: "global", Datacenter: "dc1", NodeID: "node-b",
		Platform: "darwin/arm64", SSHHost: "worker-b.example", SSHUser: "kandev", SSHPort: 22,
		SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
		WorkspaceRoot:      "/var/lib/kandev/workspaces", WorkspacePath: "/var/lib/kandev/workspaces/environments/env-1/g3",
		AllocationRoot: "/var/lib/kandev/allocations",
		CPUMHz:         1500, MemoryMB: 4096, Phase: "intent",
	}
}

func cloneTestConfig(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func TestNomadWorkerQuarantineStateValidation(t *testing.T) {
	for _, value := range []string{"", QuarantinePending, QuarantineMarked} {
		state := validTestState()
		state.QuarantineState = value
		encoded, err := MarshalEnvironmentState(state)
		if err != nil {
			t.Fatalf("quarantine state %q rejected: %v", value, err)
		}
		decoded, err := UnmarshalEnvironmentState(encoded)
		if err != nil || decoded.QuarantineState != value {
			t.Fatalf("round trip %q = %q, %v", value, decoded.QuarantineState, err)
		}
	}
	state := validTestState()
	state.QuarantineState = "released"
	if err := state.Validate(); err == nil {
		t.Fatal("unknown quarantine state accepted")
	}
}
