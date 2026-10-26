package nomad

import (
	"errors"
	"fmt"
	"path/filepath"
)

// BindPoolAllocation binds the single running allocation of a registered pool
// environment and records the node Nomad chose. The node must carry a valid
// kandev.* contract, be in the profile's node pool, and satisfy a pin. Nothing
// about the node is trusted until this succeeds; the returned state carries the
// node, transport and workspace identity that recovery later verifies.
func BindPoolAllocation(state EnvironmentState, records []AllocationRecord, node *NodeRecord) (EnvironmentState, error) {
	if state.PlacementMode != ModePool || state.NodeID != "" {
		return state, errors.New("bind pool allocation: an unplaced pool environment is required")
	}
	if err := state.Validate(); err != nil {
		return state, fmt.Errorf("bind pool allocation: invalid state: %w", err)
	}
	if state.Phase != phaseRegistered || state.AllocationID != "" || len(records) != 1 {
		return state, errors.New("bind pool allocation: exactly one new allocation is required")
	}
	allocation := records[0]
	if node == nil || node.ID == "" || node.ID != allocation.NodeID {
		return state, errors.New("bind pool allocation: the placed node record does not match the allocation")
	}
	contract, err := ValidatePlacedNode(state, node)
	if err != nil {
		return state, err
	}
	placed := state
	placed.NodeID = node.ID
	if err := checkAllocationIdentity(placed, allocation); err != nil {
		return state, err
	}
	if err := placeOnNode(&placed, allocation, contract); err != nil {
		return state, err
	}
	placed.Phase = phaseAllocated
	if err := placed.Validate(); err != nil {
		return state, fmt.Errorf("bind pool allocation: placed state is invalid: %w", err)
	}
	return placed, nil
}

// ValidatePlacedNode proves a placed node is a prepared member of the
// environment's pool and returns its validated contract. It is shared by first
// attach and recovery, which additionally compares the contract digest.
func ValidatePlacedNode(state EnvironmentState, node *NodeRecord) (NodeContract, error) {
	if node == nil {
		return NodeContract{}, errors.New("validate placed node: node record is missing")
	}
	if node.NodePool != state.NodePool {
		return NodeContract{}, errors.New("validate placed node: node is not in the profile node pool")
	}
	if state.PinNodeID != "" && node.ID != state.PinNodeID {
		return NodeContract{}, errors.New("validate placed node: node differs from the pinned node")
	}
	if node.Attributes["kernel.name"] != "darwin" || normalizeArch(node.Attributes["cpu.arch"]) != archARM64 {
		return NodeContract{}, errors.New("validate placed node: node is not a Darwin arm64 worker")
	}
	if state.ExcludeLocal && node.Meta[MetaAttach] == AttachLocal {
		return NodeContract{}, errors.New("validate placed node: local node is excluded by the profile")
	}
	return ParseNodeContract(node)
}

// VerifyPlacedNodeContract is the recovery check: the node's current contract
// must digest to the value recorded at attach, otherwise metadata drifted.
func VerifyPlacedNodeContract(state EnvironmentState, node *NodeRecord) (NodeContract, error) {
	if node == nil || node.ID != state.NodeID {
		return NodeContract{}, errors.New("verify placed node: the node record does not match the recorded node")
	}
	contract, err := ValidatePlacedNode(state, node)
	if err != nil {
		return contract, err
	}
	if NodeContractDigest(contract) != state.NodeContractSHA256 {
		return contract, errors.New("verify placed node: node metadata changed since the environment attached")
	}
	return contract, nil
}

// placeOnNode records the allocation, ports, node contract and derived worker
// paths on a copy of the state.
func placeOnNode(placed *EnvironmentState, allocation AllocationRecord, contract NodeContract) error {
	controlPort, instancePort, err := reservedAgentctlPorts(allocation)
	if err != nil {
		return err
	}
	placed.Revision++
	placed.AllocationID, placed.AllocationCreateIndex = allocation.ID, allocation.CreateIndex
	placed.ControlPort, placed.InstancePort = controlPort, instancePort
	placed.NodeContractSHA256, placed.NodeAttach, placed.StateRoot = NodeContractDigest(contract), contract.Attach, contract.StateRoot
	placed.SSHHost, placed.SSHPort, placed.SSHHostFingerprint = contract.SSHHost, contract.SSHPort, contract.SSHHostKeySHA256
	placed.WorkspaceRoot = filepath.Join(contract.StateRoot, "workspaces")
	placed.AllocationRoot = filepath.Join(contract.StateRoot, "pool", "allocations")
	placed.WorkspacePath, err = EnvironmentWorkspacePath(placed.WorkspaceRoot, placed.EnvironmentID, placed.Generation)
	if err != nil {
		return err
	}
	paths, err := PoolPathsFor(contract.StateRoot, placed.JobID, placed.EnvironmentID, placed.Generation, placed.ArtifactSHA256)
	if err != nil {
		return err
	}
	placed.AgentctlPath, placed.BootstrapPath = paths.ArtifactPath, paths.BootstrapPath
	return nil
}
