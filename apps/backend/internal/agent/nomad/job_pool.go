package nomad

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"
)

// PoolStateRootExpr is the Nomad interpolation of the placed node's state root.
// A pool job is built before a node is chosen, so every worker path in it is
// rooted at this expression and resolves to the same path PoolPathsFor returns
// for the concrete state root.
const PoolStateRootExpr = "${meta." + MetaStateRoot + "}"

const (
	nodeUniqueIDTarget = "${node.unique.id}"
	archARM64          = "arm64"
)

// PoolStagingTimeout bounds how long a placed pool task waits for Kandev to
// stage its bootstrap before it exits and the job fails closed.
const PoolStagingTimeout = 5 * time.Minute

const poolWaitScript = `set -eu
i=0
while [ ! -f "$KANDEV_READY_PATH" ]; do
  i=$((i+1))
  [ "$i" -le %d ] || { echo 'Kandev staging timeout' >&2; exit 74; }
  sleep 0.5
done
exec /bin/sh "$KANDEV_BOOTSTRAP_PATH"
`

// PoolPaths are the worker-side paths of one pool environment under a state root.
type PoolPaths struct {
	AllocationRoot string
	WorkspaceRoot  string
	WorkspacePath  string
	JobDir         string
	ArtifactPath   string
	BootstrapPath  string
	EnvironmentEnv string
	ReadyPath      string
}

// PoolPathsFor derives every pool worker path from a state root (concrete or
// PoolStateRootExpr), the environment identity and the agentctl artifact digest.
func PoolPathsFor(stateRoot, jobID, environmentID string, generation int64, artifactDigest string) (PoolPaths, error) {
	if stateRoot == "" || jobID == "" || !digestPattern.MatchString(artifactDigest) ||
		!environmentPathIDPattern.MatchString(environmentID) || generation < 1 {
		return PoolPaths{}, fmt.Errorf("nomad pool paths require a state root, safe environment identity and SHA-256 artifact digest")
	}
	allocationRoot := filepath.Join(stateRoot, "pool", "allocations")
	jobDir := filepath.Join(allocationRoot, jobID)
	bootstrap := filepath.Join(jobDir, "bootstrap-"+artifactDigest+".sh")
	workspaceRoot := filepath.Join(stateRoot, "workspaces")
	return PoolPaths{
		AllocationRoot: allocationRoot, WorkspaceRoot: workspaceRoot,
		WorkspacePath: filepath.Join(workspaceRoot, "environments", environmentID, fmt.Sprintf("g%d", generation)),
		JobDir:        jobDir, ArtifactPath: filepath.Join(jobDir, "agentctl-"+artifactDigest),
		BootstrapPath: bootstrap, EnvironmentEnv: bootstrap + ".env", ReadyPath: filepath.Join(jobDir, "ready"),
	}, nil
}

// PoolJobInput is the pool-mode counterpart of NativeJobInput. ArtifactDigest
// is the verified agentctl SHA-256 that names the staged worker files.
type PoolJobInput struct {
	Profile        ProfileConfig
	State          EnvironmentState
	ArtifactDigest string
}

// PoolJobPolicyVersion identifies the fixed pool job policy in retained evidence.
const PoolJobPolicyVersion = 1

// BuildPoolJob returns the fixed-policy one-controller job for a pool profile.
// It names no node unless the profile pins one, and is independent of the node
// Nomad later selects, so its digest can be recorded before placement.
func BuildPoolJob(input PoolJobInput) (Job, error) {
	p, s := input.Profile, input.State
	if p.Mode != ModePool || s.PlacementMode != ModePool {
		return Job{}, fmt.Errorf("build pool Nomad job: profile and state must both be pool mode")
	}
	if err := s.Validate(); err != nil {
		return Job{}, fmt.Errorf("build pool Nomad job: invalid environment state: %w", err)
	}
	if err := checkPoolProfileMatchesState(p, s); err != nil {
		return Job{}, err
	}
	stableID, err := StableJobID(JobIdentity{EnvironmentID: s.EnvironmentID, Generation: s.Generation, OperationID: s.OperationID, InstallationID: s.InstallationID})
	if err != nil || stableID != s.JobID {
		return Job{}, fmt.Errorf("build pool Nomad job: stable job ID does not match immutable identity")
	}
	paths, err := PoolPathsFor(PoolStateRootExpr, s.JobID, s.EnvironmentID, s.Generation, input.ArtifactDigest)
	if err != nil {
		return Job{}, err
	}
	job := Job{
		ID: s.JobID, Name: s.JobID, Type: "batch", Namespace: p.Namespace, Region: p.Region,
		Priority: 50, NodePool: p.NodePool, Datacenters: []string{p.Datacenter}, Meta: poolJobMetadata(s),
		Constraints: poolConstraints(p), Affinities: poolAffinities(p), Spreads: poolSpreads(p),
		TaskGroups: []TaskGroup{poolTaskGroup(p, s, paths)},
	}
	if err := applyRetainedDigest(&job, s); err != nil {
		return Job{}, err
	}
	return job, nil
}

func checkPoolProfileMatchesState(p ProfileConfig, s EnvironmentState) error {
	if p.Namespace != s.Namespace || p.Region != s.Region || p.Datacenter != s.Datacenter || p.Platform != s.Platform ||
		p.CPUMHz != s.CPUMHz || p.MemoryMB != s.MemoryMB ||
		(s.DisconnectLostAfterSeconds != 0 && p.DisconnectLostAfter != time.Duration(s.DisconnectLostAfterSeconds)*time.Second) {
		return fmt.Errorf("build pool Nomad job: profile does not match the immutable environment snapshot")
	}
	return nil
}

// poolConstraints admits only prepared Darwin arm64 nodes, so a node without
// valid kandev.* metadata can never receive an environment.
func poolConstraints(p ProfileConfig) []Constraint {
	constraints := []Constraint{
		{LTarget: "${attr.kernel.name}", Operand: "=", RTarget: "darwin"},
		{LTarget: "${attr.cpu.arch}", Operand: "=", RTarget: archARM64},
		{LTarget: "${meta." + MetaAttach + "}", Operand: "is_set"},
		{LTarget: "${meta." + MetaStateRoot + "}", Operand: "is_set"},
	}
	if p.PinNodeID != "" {
		constraints = append(constraints, Constraint{LTarget: nodeUniqueIDTarget, Operand: "=", RTarget: p.PinNodeID})
	}
	if p.ExcludeLocal {
		constraints = append(constraints, Constraint{LTarget: "${meta." + MetaAttach + "}", Operand: "!=", RTarget: AttachLocal})
	}
	return constraints
}

func poolAffinities(p ProfileConfig) []Affinity {
	if !p.PreferLocal {
		return nil
	}
	return []Affinity{{LTarget: "${meta." + MetaAttach + "}", Operand: "=", RTarget: AttachLocal, Weight: 50}}
}

// poolSpreads distributes environments across nodes for spread placement.
// Binpack relies on the node pool's binpack scheduler algorithm.
func poolSpreads(p ProfileConfig) []Spread {
	if p.Placement == PlacementBinpack {
		return nil
	}
	return []Spread{{Attribute: nodeUniqueIDTarget, Weight: 100}}
}

func poolJobMetadata(s EnvironmentState) map[string]string {
	return map[string]string{
		"kandev.environment_id":            s.EnvironmentID,
		"kandev.generation":                strconv.FormatInt(s.Generation, 10),
		"kandev.operation_id":              s.OperationID,
		"kandev.installation_id":           s.InstallationID,
		"kandev.input_digest":              s.InputDigest,
		"kandev.placement_mode":            ModePool,
		"kandev.native_job_policy_version": strconv.Itoa(PoolJobPolicyVersion),
	}
}

func poolTaskGroup(p ProfileConfig, s EnvironmentState, paths PoolPaths) TaskGroup {
	group := nativeTaskGroup(p, s, paths.ArtifactPath, paths.BootstrapPath)
	task := &group.Tasks[0]
	task.Config = map[string]any{"command": "/bin/sh", "args": []string{"-c", fmt.Sprintf(poolWaitScript, int(PoolStagingTimeout/(500*time.Millisecond)))}}
	task.Env["KANDEV_WORKSPACE_ROOT"] = paths.WorkspacePath
	task.Env["KANDEV_ALLOCATION_ROOT"] = paths.AllocationRoot
	task.Env["KANDEV_WORKER_STATE_ROOT"] = filepath.Join(PoolStateRootExpr, "worker-state", nodeUniqueIDTarget)
	task.Env["KANDEV_STATE_ROOT"] = PoolStateRootExpr
	task.Env["KANDEV_BOOTSTRAP_PATH"] = paths.BootstrapPath
	task.Env["KANDEV_READY_PATH"] = paths.ReadyPath
	task.Resources = TaskResources{CPU: p.CPUMHz, MemoryMB: p.MemoryMB, MemoryMaxMB: p.MemoryMaxMB}
	return group
}

// PoolProfileFromState rebuilds the placement-relevant profile snapshot a pool
// job was built from. Connection fields (address, token) are not part of a job.
func PoolProfileFromState(s EnvironmentState) ProfileConfig {
	return ProfileConfig{
		Mode: ModePool, Namespace: s.Namespace, Region: s.Region, Datacenter: s.Datacenter, Platform: s.Platform,
		NodePool: s.NodePool, Placement: s.Placement, PinNodeID: s.PinNodeID, ExcludeLocal: s.ExcludeLocal,
		PreferLocal: s.PreferLocal, CPUMHz: s.CPUMHz, MemoryMB: s.MemoryMB, MemoryMaxMB: s.MemoryMaxMB,
		DisconnectLostAfter: time.Duration(s.DisconnectLostAfterSeconds) * time.Second,
	}
}

// EffectiveProfile returns the profile with the placed node's transport and
// storage identity filled in from the immutable environment state, so code that
// stages files or attaches over SSH sees one worker regardless of profile mode.
// Legacy profiles and unplaced pool environments are returned unchanged.
func (p ProfileConfig) EffectiveProfile(s EnvironmentState) ProfileConfig {
	if p.Mode != ModePool || s.PlacementMode != ModePool || s.NodeID == "" {
		return p
	}
	p.NodeID, p.SSHHost, p.SSHPort, p.SSHHostFingerprint = s.NodeID, s.SSHHost, s.SSHPort, s.SSHHostFingerprint
	p.WorkspaceRoot, p.AllocationRoot = s.WorkspaceRoot, s.AllocationRoot
	return p
}
