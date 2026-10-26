package nomad

import (
	"encoding/json"
	"strings"
	"testing"
)

func registeredPoolState(t *testing.T, mutate ...func(*EnvironmentState)) EnvironmentState {
	t.Helper()
	s := poolTestState(t)
	for _, m := range mutate {
		m(&s)
	}
	s.Phase = "registered"
	s.ArtifactSHA256 = strings.Repeat("c", 64)
	job, err := BuildPoolJob(PoolJobInput{Profile: PoolProfileFromState(s), State: s, ArtifactDigest: s.ArtifactSHA256})
	if err != nil {
		t.Fatal(err)
	}
	s.NativeJobSpecSHA256, err = NativeJobSpecDigest(job)
	if err != nil {
		t.Fatal(err)
	}
	s.NativeJobPolicyVersion = PoolJobPolicyVersion
	return s
}

func poolAlloc(nodeID string) AllocationRecord {
	return AllocationRecord{
		ID: "alloc-1", NodeID: nodeID, JobID: "", Namespace: "default",
		DesiredStatus: "run", ClientStatus: "running", CreateIndex: 12,
		TaskStates: map[string]AllocationTaskState{"agentctl": {State: "running"}},
		AllocatedResources: &AllocationResources{Shared: AllocationSharedResources{Ports: []AllocationPort{
			{Label: "agentctl_control", Value: 22341}, {Label: "agentctl_instance", Value: 22342},
		}}},
	}
}

func placedNode(id string, meta map[string]string) *NodeRecord {
	return &NodeRecord{
		ID: id, NodePool: "kandev", Meta: meta,
		Attributes: map[string]string{"kernel.name": "darwin", "cpu.arch": "arm64"},
	}
}

func bindPool(t *testing.T, s EnvironmentState, node *NodeRecord) (EnvironmentState, error) {
	t.Helper()
	a := poolAlloc(node.ID)
	a.JobID = s.JobID
	return BindPoolAllocation(s, []AllocationRecord{a}, node)
}

func TestBindPoolAllocationRecordsNodeAndDerivesPaths(t *testing.T) {
	s := registeredPoolState(t)
	bound, err := bindPool(t, s, placedNode("node-1", sshMeta()))
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if bound.NodeID != "node-1" || bound.Phase != "allocated" || bound.NodeAttach != AttachSSH || bound.SSHHost != "worker-b.example" ||
		bound.StateRoot != "/var/kandev" || bound.AllocationRoot != "/var/kandev/pool/allocations" ||
		bound.WorkspaceRoot != "/var/kandev/workspaces" || bound.ControlPort != 22341 || bound.NodeContractSHA256 == "" {
		t.Fatalf("bound = %+v", bound)
	}
	paths, _ := PoolPathsFor("/var/kandev", s.JobID, s.EnvironmentID, s.Generation, s.ArtifactSHA256)
	if bound.AgentctlPath != paths.ArtifactPath || bound.BootstrapPath != paths.BootstrapPath {
		t.Fatalf("staged paths %q %q", bound.AgentctlPath, bound.BootstrapPath)
	}
	if err := ValidatePoolStateTransition(s, bound); err != nil {
		t.Fatalf("unplaced to placed transition rejected: %v", err)
	}
}

func TestBindPoolAllocationFailsClosedOnUnpreparedOrWrongNodes(t *testing.T) {
	s := registeredPoolState(t)
	cases := map[string]func(*NodeRecord){
		"no metadata":     func(n *NodeRecord) { n.Meta = nil },
		"wrong pool":      func(n *NodeRecord) { n.NodePool = "other" },
		"not darwin":      func(n *NodeRecord) { n.Attributes["kernel.name"] = "linux" },
		"missing hostkey": func(n *NodeRecord) { delete(n.Meta, MetaSSHHostKeySHA256) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			node := placedNode("node-1", sshMeta())
			mutate(node)
			if _, err := bindPool(t, s, node); err == nil {
				t.Fatal("unprepared node accepted")
			}
		})
	}
}

func TestBindPoolAllocationHonorsPinAndLocalExclusion(t *testing.T) {
	s := registeredPoolState(t, func(s *EnvironmentState) { s.PinNodeID = "node-pinned" })
	if _, err := bindPool(t, s, placedNode("node-1", sshMeta())); err == nil {
		t.Fatal("allocation on a node other than the pin accepted")
	}
	s = registeredPoolState(t, func(s *EnvironmentState) { s.ExcludeLocal = true })
	if _, err := bindPool(t, s, placedNode("node-1", localMeta())); err == nil {
		t.Fatal("excluded local node accepted")
	}
}

func TestBindPoolAllocationRejectsMismatchedNodeRecord(t *testing.T) {
	s := registeredPoolState(t)
	a := poolAlloc("node-1")
	a.JobID = s.JobID
	if _, err := BindPoolAllocation(s, []AllocationRecord{a}, placedNode("node-2", sshMeta())); err == nil {
		t.Fatal("node record for a different node accepted")
	}
}

func TestVerifyPlacedNodeContractDetectsMetadataDrift(t *testing.T) {
	s := registeredPoolState(t)
	bound, err := bindPool(t, s, placedNode("node-1", sshMeta()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyPlacedNodeContract(bound, placedNode("node-1", sshMeta())); err != nil {
		t.Fatalf("unchanged metadata rejected: %v", err)
	}
	drift := sshMeta()
	drift[MetaSSHHostKeySHA256] = "SHA256:" + strings.Repeat("Z", 43)
	if _, err := VerifyPlacedNodeContract(bound, placedNode("node-1", drift)); err == nil {
		t.Fatal("host key drift accepted")
	}
	if _, err := VerifyPlacedNodeContract(bound, placedNode("node-9", sshMeta())); err == nil {
		t.Fatal("different node accepted")
	}
}

func TestNativeJobFromStateRebuildsPoolJobAfterPlacement(t *testing.T) {
	s := registeredPoolState(t)
	before, err := NativeJobFromState(s)
	if err != nil {
		t.Fatalf("unplaced rebuild: %v", err)
	}
	bound, err := bindPool(t, s, placedNode("node-1", sshMeta()))
	if err != nil {
		t.Fatal(err)
	}
	after, err := NativeJobFromState(bound)
	if err != nil {
		t.Fatalf("placed rebuild: %v", err)
	}
	d1, _ := NativeJobSpecDigest(before)
	d2, _ := NativeJobSpecDigest(after)
	if d1 != d2 {
		t.Fatal("placing the environment changed the pool job specification")
	}
}

func TestValidateNativeJobResponseAcceptsPoolJobAsNomadReturnsIt(t *testing.T) {
	s := registeredPoolState(t, func(s *EnvironmentState) { s.PreferLocal = true })
	job, err := NativeJobFromState(s)
	if err != nil {
		t.Fatal(err)
	}
	// Nomad returns an unset SpreadTarget as null and echoes the fixed fields.
	raw, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, sp := range decoded["Spreads"].([]any) {
		sp.(map[string]any)["SpreadTarget"] = nil
	}
	withDefaults, _ := json.Marshal(decoded)
	if err := ValidateNativeJobResponse(withDefaults, job); err != nil {
		t.Fatalf("pool job rejected: %v", err)
	}
	decoded["Constraints"] = decoded["Constraints"].([]any)[:3]
	tampered, _ := json.Marshal(decoded)
	if err := ValidateNativeJobResponse(tampered, job); err == nil {
		t.Fatal("job missing a placement constraint accepted")
	}
}
