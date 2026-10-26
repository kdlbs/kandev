package nomad

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// validateUnplacedPoolEvidence covers a pool environment whose node is not yet
// recorded: the job digest and policy version may be pinned, but no worker path
// exists until the node is known.
func (s EnvironmentState) validateUnplacedPoolEvidence() error {
	if s.AgentctlPath != "" || s.BootstrapPath != "" {
		return fmt.Errorf("nomad pool environment state has worker paths without a placed node")
	}
	if (s.NativeJobPolicyVersion != 0) != (s.NativeJobSpecSHA256 != "") {
		return fmt.Errorf("nomad environment state has incomplete immutable job evidence")
	}
	if s.NativeJobSpecSHA256 != "" && (s.NativeJobPolicyVersion != PoolJobPolicyVersion || !digestPattern.MatchString(s.NativeJobSpecSHA256)) {
		return fmt.Errorf("nomad environment state has unsupported native job policy evidence")
	}
	return nil
}

// poolNodeRequired reports whether the placed node must be recorded: from the
// allocated phase onward, or once an allocation exists.
func (s EnvironmentState) poolNodeRequired() bool {
	return s.Phase == phaseAllocated || s.Phase == phaseReady || s.AllocationID != ""
}

func (s EnvironmentState) carriesPoolFields() bool {
	return s.NodeContractSHA256 != "" || s.NodeAttach != "" || s.StateRoot != "" || s.NodePool != "" || s.Placement != "" ||
		s.PinNodeID != "" || s.ExcludeLocal || s.PreferLocal || s.MemoryMaxMB != 0 || s.ArtifactSHA256 != ""
}

func (s EnvironmentState) validatePlacementMode() error {
	if s.PlacementMode != ModePool {
		return fmt.Errorf("nomad environment state has invalid placement mode")
	}
	return s.validatePool()
}

func (s EnvironmentState) validatePool() error {
	if !s.hasOperationIdentity() {
		return fmt.Errorf("nomad environment state has invalid immutable operation identity")
	}
	if s.Namespace == "" || s.Region == "" || s.Datacenter == "" || s.Platform != WorkerPlatform || s.CPUMHz <= 0 || s.MemoryMB <= 0 {
		return fmt.Errorf("nomad pool environment state has incomplete resource identity")
	}
	if err := s.validatePoolWindowAndInput(); err != nil {
		return err
	}
	if err := s.validatePoolSnapshot(); err != nil {
		return err
	}
	if err := s.validatePoolNode(); err != nil {
		return err
	}
	if err := s.validateJobEvidence(); err != nil {
		return err
	}
	return s.validatePhaseAndIndexes()
}

func (s EnvironmentState) validatePoolWindowAndInput() error {
	if s.DisconnectLostAfterSeconds != 0 && (s.DisconnectLostAfterSeconds < int64(MinDisconnectLostAfter/time.Second) || s.DisconnectLostAfterSeconds > int64(MaxDisconnectLostAfter/time.Second)) {
		return fmt.Errorf("nomad environment state has an out-of-range disconnect window")
	}
	if !s.WorkspaceInputCaptured && s.WorkspaceInputPath != "" {
		return fmt.Errorf("nomad environment state has workspace input without capture evidence")
	}
	return nil
}

func (s EnvironmentState) validatePoolNode() error {
	recorded := s.NodeID != ""
	if !recorded {
		if s.poolNodeRequired() {
			return fmt.Errorf("nomad pool environment state requires the placed node once allocated")
		}
		if s.NodeContractSHA256 != "" || s.NodeAttach != "" || s.StateRoot != "" || s.SSHHost != "" || s.SSHPort != 0 ||
			s.SSHHostFingerprint != "" || s.WorkspaceRoot != "" || s.WorkspacePath != "" {
			return fmt.Errorf("nomad pool environment state has node details without a placed node")
		}
		return nil
	}
	return s.validateRecordedPoolNode()
}

func (s EnvironmentState) validateRecordedPoolNode() error {
	if !safeNodeID.MatchString(s.NodeID) || !digestPattern.MatchString(s.NodeContractSHA256) {
		return fmt.Errorf("nomad pool environment state has an invalid node identity")
	}
	if _, err := normalizedAbsolutePath(s.StateRoot); err != nil {
		return fmt.Errorf("nomad pool environment state has an invalid state root")
	}
	switch s.NodeAttach {
	case AttachLocal:
		if s.SSHHost != "" || s.SSHPort != 0 || s.SSHHostFingerprint != "" {
			return fmt.Errorf("nomad pool environment state has SSH identity on a local node")
		}
	case AttachSSH:
		if !s.hasSSHIdentityWithoutUser() {
			return fmt.Errorf("nomad environment state has incomplete SSH identity")
		}
	default:
		return fmt.Errorf("nomad pool environment state has invalid node attach")
	}
	expected, err := EnvironmentWorkspacePath(s.WorkspaceRoot, s.EnvironmentID, s.Generation)
	if err != nil || expected != s.WorkspacePath {
		return fmt.Errorf("nomad environment state workspace does not match its owner and generation")
	}
	if s.WorkspaceRoot != filepath.Join(s.StateRoot, "workspaces") || s.AllocationRoot != filepath.Join(s.StateRoot, "pool", "allocations") {
		return fmt.Errorf("nomad pool environment state roots do not derive from the node state root")
	}
	return nil
}

func (s EnvironmentState) validatePoolSnapshot() error {
	if !safeNodeID.MatchString(s.NodePool) || (s.Placement != PlacementSpread && s.Placement != PlacementBinpack) ||
		s.MemoryMaxMB < s.MemoryMB || (s.PinNodeID != "" && !safeNodeID.MatchString(s.PinNodeID)) || (s.ExcludeLocal && s.PreferLocal) {
		return fmt.Errorf("nomad pool environment state has an invalid placement snapshot")
	}
	if s.ArtifactSHA256 != "" && !digestPattern.MatchString(s.ArtifactSHA256) {
		return fmt.Errorf("nomad pool environment state has an invalid artifact digest")
	}
	if s.PinNodeID != "" && s.NodeID != "" && s.NodeID != s.PinNodeID {
		return fmt.Errorf("nomad pool environment state node differs from the pinned node")
	}
	return nil
}

func (s EnvironmentState) hasSSHIdentityWithoutUser() bool {
	return s.SSHHost != "" && s.SSHPort >= 1 && s.SSHPort <= 65535 && strings.HasPrefix(s.SSHHostFingerprint, "SHA256:")
}

// ValidatePoolStateTransition rejects a change to the node, transport or
// workspace identity of a pool environment once the node was recorded.
func ValidatePoolStateTransition(prev, next EnvironmentState) error {
	if prev.PlacementMode != next.PlacementMode {
		return fmt.Errorf("nomad environment placement mode is immutable")
	}
	if prev.PlacementMode != ModePool || prev.NodeID == "" {
		return nil
	}
	if prev.snapshotChanged(next) {
		return fmt.Errorf("nomad pool environment placement snapshot is immutable")
	}
	if prev.nodeIdentityChanged(next) {
		return fmt.Errorf("nomad pool environment node identity is immutable once recorded")
	}
	return nil
}

func (s EnvironmentState) snapshotChanged(next EnvironmentState) bool {
	return s.NodePool != next.NodePool || s.Placement != next.Placement || s.PinNodeID != next.PinNodeID ||
		s.ExcludeLocal != next.ExcludeLocal || s.PreferLocal != next.PreferLocal || s.MemoryMaxMB != next.MemoryMaxMB ||
		(s.ArtifactSHA256 != "" && s.ArtifactSHA256 != next.ArtifactSHA256)
}

func (s EnvironmentState) nodeIdentityChanged(next EnvironmentState) bool {
	return s.NodeID != next.NodeID || s.NodeContractSHA256 != next.NodeContractSHA256 || s.NodeAttach != next.NodeAttach ||
		s.StateRoot != next.StateRoot || s.SSHHost != next.SSHHost || s.SSHPort != next.SSHPort ||
		s.SSHHostFingerprint != next.SSHHostFingerprint || s.WorkspaceRoot != next.WorkspaceRoot ||
		s.WorkspacePath != next.WorkspacePath
}

func (s EnvironmentState) expectedJobPolicyVersion() int {
	if s.PlacementMode == ModePool {
		return PoolJobPolicyVersion
	}
	return NativeJobPolicyVersion
}
