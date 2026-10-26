package lifecycle

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agent/nomad"
)

const nomadHostIdentityTokenFile = "nomad-host-identity"

// ErrNomadLocalNodeRefused reports a node that declares itself local but is not
// provably the Kandev host. There is deliberately no SSH fallback.
var ErrNomadLocalNodeRefused = errors.New("nomad local node refused")

func newNomadPoolIntentState(req *ExecutorCreateRequest, id nomadLaunchIdentity) nomad.EnvironmentState {
	profile := id.profile
	return nomad.EnvironmentState{
		Version: 1, Revision: 1, EnvironmentID: req.TaskEnvironmentID, Generation: id.generation,
		OperationID: req.InstanceID, InstallationID: id.profileID, ExecutionID: req.InstanceID,
		InputDigest: id.inputDigest, JobID: id.jobID, Namespace: profile.Namespace, Region: profile.Region,
		Datacenter: profile.Datacenter, Platform: profile.Platform,
		WorkspaceInputPath: req.WorkspacePath, WorkspaceInputCaptured: true,
		CPUMHz: profile.CPUMHz, MemoryMB: profile.MemoryMB,
		DisconnectLostAfterSeconds: int64(profile.DisconnectLostAfter.Seconds()),
		PlacementMode:              nomad.ModePool, NodePool: profile.NodePool, Placement: profile.Placement,
		PinNodeID: profile.PinNodeID, ExcludeLocal: profile.ExcludeLocal, PreferLocal: profile.PreferLocal,
		MemoryMaxMB: profile.MemoryMaxMB, Phase: nomadPhaseIntent,
	}
}

// nomadPoolSnapshotMatches reports whether the retained placement snapshot is
// the one the profile resolves to. The snapshot is frozen at launch.
func nomadPoolSnapshotMatches(state nomad.EnvironmentState, profile nomad.ProfileConfig) bool {
	return state.NodePool == profile.NodePool && state.Placement == profile.Placement &&
		state.PinNodeID == profile.PinNodeID && state.ExcludeLocal == profile.ExcludeLocal &&
		state.PreferLocal == profile.PreferLocal && state.MemoryMaxMB == profile.MemoryMaxMB &&
		state.MemoryMB == profile.MemoryMB && state.CPUMHz == profile.CPUMHz &&
		state.Namespace == profile.Namespace && state.Region == profile.Region && state.Datacenter == profile.Datacenter
}

// nomadJobNodeMetaMatches compares the node identity a job carries: a legacy
// job names its node, a pool job names none.
func nomadJobNodeMetaMatches(job *nomad.JobRecord, state nomad.EnvironmentState) bool {
	if state.PlacementMode == nomad.ModePool {
		return job.Meta["kandev.placement_mode"] == nomad.ModePool && job.Meta["kandev.node_id"] == ""
	}
	return job.Meta["kandev.node_id"] == state.NodeID
}

// nomadEffectiveProfile parses the saved profile and fills a placed pool
// environment's node identity from its retained state.
func nomadEffectiveProfile(metadata map[string]interface{}, state nomad.EnvironmentState) (nomad.ProfileConfig, error) {
	profile, err := nomadProfileFromMetadata(metadata)
	if err != nil {
		return profile, err
	}
	return profile.EffectiveProfile(state), nil
}

func buildNomadJobForState(profile nomad.ProfileConfig, state nomad.EnvironmentState) (nomad.Job, error) {
	if state.PlacementMode == nomad.ModePool {
		return nomad.NativeJobFromState(state)
	}
	return nomad.BuildNativeJob(nomad.NativeJobInput{Profile: profile, State: state, AgentctlPath: state.AgentctlPath, BootstrapPath: state.BootstrapPath})
}

// nomadKandevHome is the directory tree a worker state root must stay outside.
func (r *NomadExecutor) nomadKandevHome() string {
	if r.kandevHomeDir != "" {
		return r.kandevHomeDir
	}
	if home := os.Getenv("KANDEV_HOME_DIR"); home != "" {
		return filepath.Clean(home)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kandev")
}

func (r *NomadExecutor) nomadHostIdentityPath() string {
	if r.hostIdentityPath != "" {
		return r.hostIdentityPath
	}
	return filepath.Join(r.nomadKandevHome(), "data", nomadHostIdentityTokenFile)
}

// verifyLocalNode proves a node declaring itself local is this host: its
// identity digest matches the token on Kandev's own disk, it reserves headroom
// for Kandev, and its state root is outside Kandev's home.
func (r *NomadExecutor) verifyLocalNode(contract nomad.NodeContract, node *nomad.NodeRecord) error {
	token, err := os.ReadFile(r.nomadHostIdentityPath())
	if err != nil || len(strings.TrimSpace(string(token))) == 0 {
		return fmt.Errorf("%w: the host identity token is unavailable", ErrNomadLocalNodeRefused)
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(string(token))))
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(contract.HostIdentitySHA256)) != 1 {
		return fmt.Errorf("%w: the node host identity does not match this host", ErrNomadLocalNodeRefused)
	}
	if node.ReservedResources.CPU.CPUShares <= 0 || node.ReservedResources.Memory.MemoryMB <= 0 {
		return fmt.Errorf("%w: the local node reserves no CPU and memory headroom for Kandev", ErrNomadLocalNodeRefused)
	}
	if home := r.nomadKandevHome(); home != "" && pathWithin(home, contract.StateRoot) {
		return fmt.Errorf("%w: the node state root lies inside the Kandev home directory", ErrNomadLocalNodeRefused)
	}
	return nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// verifyPlacedPoolNode reads the placed node and proves it is still the node,
// pool and contract recorded at attach. Launch and recovery share it.
func (r *NomadExecutor) verifyPlacedPoolNode(ctx context.Context, client *nomad.Client, state nomad.EnvironmentState) (nomad.NodeContract, *nomad.NodeRecord, error) {
	node, err := client.GetNode(ctx, state.NodeID)
	if err != nil {
		return nomad.NodeContract{}, nil, fmt.Errorf("read placed Nomad node: %w", err)
	}
	contract, err := nomad.VerifyPlacedNodeContract(state, node)
	if err != nil {
		return contract, node, err
	}
	if contract.Attach == nomad.AttachLocal {
		if err := r.verifyLocalNode(contract, node); err != nil {
			return contract, node, err
		}
	}
	return contract, node, nil
}

// admitPlacedPoolNode refuses a node that is quarantined, cleaning up for
// another environment, or not eligible and ready.
func (r *NomadExecutor) admitPlacedPoolNode(ctx context.Context, req *ExecutorCreateRequest, client *nomad.Client, profile nomad.ProfileConfig, node *nomad.NodeRecord) error {
	if err := admitAgainstRetainedInventory(ctx, req, profile); err != nil {
		return err
	}
	marker, _, err := client.ReadQuarantineMarker(ctx, profile.Namespace, profile.NodeID)
	if err != nil {
		return fmt.Errorf("%w: quarantine marker could not be read: %w", ErrNomadWorkerQuarantined, err)
	}
	if marker != nil {
		return fmt.Errorf("%w: node carries a quarantine marker", ErrNomadWorkerQuarantined)
	}
	if node.SchedulingEligibility != nomadNodeEligible || node.Status != nomadNodeStatusReadyStr {
		return fmt.Errorf("%w: node is not eligible and ready", ErrNomadWorkerQuarantined)
	}
	return nil
}

// openPoolTransport opens the one transport a placed node allows: loopback for
// a verified local node, a host-key-pinned SSH connection otherwise.
func (r *NomadExecutor) openPoolTransport(ctx context.Context, profile nomad.ProfileConfig, state nomad.EnvironmentState) (nomadWorkerTransport, error) {
	if state.NodeAttach == nomad.AttachLocal {
		return nomadLocalTransport{}, nil
	}
	if profile.SSHUser == "" {
		return nil, errors.New("pool profile has no ssh_user for a remote node")
	}
	sshClient, err := r.dialPinnedNomadWorker(ctx, profile)
	if err != nil {
		return nil, err
	}
	return nomadSSHTransport{client: sshClient}, nil
}

// poolReadyContent is the marker that releases the job's wait script.
const poolReadyContent = "ready\n"

// bindUnplacedPoolAllocation binds the allocation of a pool environment whose
// node is not yet recorded. The allocation, its embedded job and the node it
// landed on are all proven before the node becomes part of the state.
func bindUnplacedPoolAllocation(
	ctx context.Context,
	client *nomad.Client,
	namespace string,
	state nomad.EnvironmentState,
	stub nomad.AllocationRecord,
) (nomad.EnvironmentState, error) {
	if stub.ID == "" || stub.CreateIndex == 0 || stub.JobID != state.JobID || stub.NodeID == "" || stub.Namespace != namespace {
		return state, errors.New("nomad allocation summary identity does not match the immutable job")
	}
	allocation, err := client.GetAllocation(ctx, namespace, stub.ID)
	if err != nil {
		return state, fmt.Errorf("read full Nomad allocation: %w", err)
	}
	if allocation.ID != stub.ID || allocation.CreateIndex != stub.CreateIndex || allocation.JobID != stub.JobID ||
		allocation.NodeID != stub.NodeID || allocation.Namespace != stub.Namespace {
		return state, errors.New("full Nomad allocation identity differs from its immutable job summary")
	}
	if err := validateAllocationPolicy(ctx, client, state, allocation); err != nil {
		return state, err
	}
	node, err := client.GetNode(ctx, allocation.NodeID)
	if err != nil {
		return state, fmt.Errorf("read placed Nomad node: %w", err)
	}
	return nomad.BindPoolAllocation(state, []nomad.AllocationRecord{*allocation}, node)
}

// connectRetainedPoolWorker reattaches to a placed pool environment. The node
// is re-read and its contract compared with the digest recorded at attach
// before any transport opens, so drifted metadata fails closed.
func (r *NomadExecutor) connectRetainedPoolWorker(
	ctx context.Context,
	req *ExecutorCreateRequest,
	client *nomad.Client,
	effective nomad.ProfileConfig,
	allocated *nomad.EnvironmentState,
) (*nomadAgentctlConnection, error) {
	if _, _, err := r.verifyPlacedPoolNode(ctx, client, *allocated); err != nil {
		return nil, err
	}
	transport, err := r.openPoolTransport(ctx, effective, *allocated)
	if err != nil {
		return nil, err
	}
	if err := verifyNomadWorkerFenceOver(ctx, transport, effective, *allocated); err != nil {
		_ = transport.Close()
		return nil, err
	}
	tunnel, err := openPoolAPITunnel(transport, req)
	if err != nil {
		_ = transport.Close()
		return nil, err
	}
	connection, err := connectNomadAgentctlOverTransport(ctx, transport, effective, allocated, req, req.BootstrapNonce, r.logger, tunnel)
	if err != nil {
		if tunnel != nil {
			_ = tunnel.Close()
		}
		_ = transport.Close()
		return nil, err
	}
	return connection, nil
}

// dialNomadCleanupWorker opens the transport cleanup uses to reach the worker:
// the placed node's transport in pool mode, the pinned SSH worker otherwise.
func (r *NomadExecutor) dialNomadCleanupWorker(ctx context.Context, client *nomad.Client, profile nomad.ProfileConfig, state nomad.EnvironmentState) (nomadWorkerTransport, error) {
	if state.PlacementMode == nomad.ModePool {
		if _, _, err := r.verifyPlacedPoolNode(ctx, client, state); err != nil {
			return nil, err
		}
		return r.openPoolTransport(ctx, profile, state)
	}
	sshClient, err := r.dialPinnedNomadWorker(ctx, profile)
	if err != nil {
		return nil, err
	}
	return nomadSSHTransport{client: sshClient}, nil
}
