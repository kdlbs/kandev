package nomad

import (
	"strings"
	"testing"
)

func poolState(phase string) EnvironmentState {
	return EnvironmentState{
		Version: 1, EnvironmentID: "env-1", Generation: 1, OperationID: "op-1", InstallationID: "i-1", ExecutionID: "x-1",
		InputDigest: strings.Repeat("a", 64), JobID: "job-1", Namespace: "default", Region: "global", Datacenter: "dc1",
		Platform: "darwin/arm64", CPUMHz: 100, MemoryMB: 256, Phase: phase, PlacementMode: ModePool,
		NodePool: "kandev", Placement: PlacementSpread, MemoryMaxMB: 2048,
	}
}

func placePool(s EnvironmentState) EnvironmentState {
	s.NodeID, s.NodeAttach, s.StateRoot = "node-1", AttachSSH, "/var/kandev"
	s.NodeContractSHA256 = strings.Repeat("b", 64)
	s.SSHHost, s.SSHPort, s.SSHHostFingerprint = "worker-b.example.test", 22, testHostKey
	s.WorkspaceRoot, s.AllocationRoot = "/var/kandev/workspaces", "/var/kandev/pool/allocations"
	s.WorkspacePath = "/var/kandev/workspaces/environments/env-1/g1"
	return s
}

func TestStateValidate_PoolAllowsEmptyNodeBeforeAllocated(t *testing.T) {
	for _, phase := range []string{"intent", "registered"} {
		if err := poolState(phase).Validate(); err != nil {
			t.Fatalf("%s: %v", phase, err)
		}
	}
	if err := placePool(poolState("allocated")).Validate(); err != nil {
		t.Fatal(err)
	}
	local := placePool(poolState("ready"))
	local.NodeAttach, local.SSHHost, local.SSHPort, local.SSHHostFingerprint = AttachLocal, "", 0, ""
	if err := local.Validate(); err != nil {
		t.Fatal(err)
	}
	enc, err := MarshalEnvironmentState(placePool(poolState("ready")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalEnvironmentState(enc); err != nil {
		t.Fatal(err)
	}
}

func TestStateValidate_PoolRequiresNodeAfterAllocated(t *testing.T) {
	if err := poolState("allocated").Validate(); err == nil {
		t.Fatal("allocated pool state without node accepted")
	}
	s := poolState("registered")
	s.AllocationID, s.AllocationCreateIndex = "alloc", 1
	if err := s.Validate(); err == nil {
		t.Fatal("allocation without node accepted")
	}
	s = placePool(poolState("allocated"))
	s.NodeContractSHA256 = ""
	if err := s.Validate(); err == nil {
		t.Fatal("missing contract digest accepted")
	}
	s = placePool(poolState("allocated"))
	s.StateRoot = "relative"
	if err := s.Validate(); err == nil {
		t.Fatal("relative state root accepted")
	}
	s = poolState("intent")
	s.StateRoot = "/x"
	if err := s.Validate(); err == nil {
		t.Fatal("node details without node accepted")
	}
	prev := placePool(poolState("allocated"))
	next := prev
	next.Phase = "ready"
	if err := ValidatePoolStateTransition(prev, next); err != nil {
		t.Fatal(err)
	}
	next.NodeID = "node-2"
	if err := ValidatePoolStateTransition(prev, next); err == nil {
		t.Fatal("node change accepted")
	}
}

func TestStateValidate_LegacyRejectsPoolFields(t *testing.T) {
	s := validTestState()
	s.StateRoot = "/x"
	if err := s.Validate(); err == nil {
		t.Fatal("legacy state with pool field accepted")
	}
}
