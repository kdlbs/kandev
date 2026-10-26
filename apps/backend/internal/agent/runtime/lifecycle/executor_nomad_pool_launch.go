package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/nomad"
)

// launchPoolOperation places a new environment on whichever pool node Nomad
// chooses. The order differs from a pinned launch because the node is unknown
// until the allocation exists: register, bind the single allocation and its
// node, checkpoint the node identity, then stage files and attach.
func (r *NomadExecutor) launchPoolOperation(ctx context.Context, req *ExecutorCreateRequest, id nomadLaunchIdentity) (*ExecutorInstance, error) {
	profile := id.profile
	state := newNomadIntentState(req, id)
	client, err := r.preflightPoolLaunch(ctx, profile)
	if err != nil {
		return nil, err
	}
	nonce, err := recordNomadLaunchIntent(ctx, req, state)
	if err != nil {
		return nil, err
	}
	artifact, err := r.checkpointPoolJobEvidence(ctx, req, &state)
	if err != nil {
		return nil, err
	}
	if _, err := r.registerNomadJob(ctx, req, profile, &state); err != nil {
		return nil, err
	}
	allocated, err := waitForNomadAllocation(ctx, client, profile.Namespace, state)
	if err != nil {
		return nil, r.abortRegisteredNomadLaunch(ctx, req, &state, err)
	}
	state = allocated
	if err := checkpointNomadState(ctx, req, &state); err != nil {
		return nil, r.abortRegisteredNomadLaunch(ctx, req, &state, fmt.Errorf("checkpoint placed Nomad node: %w", err))
	}
	effective := profile.EffectiveProfile(state)
	if err := r.admitAndVerifyPlacedNode(ctx, req, client, effective, state); err != nil {
		state.RefusedBeforeStage = true
		return nil, r.abortRegisteredNomadLaunch(ctx, req, &state, err)
	}
	instance, err := r.stageAndAttachPoolWorker(ctx, req, effective, &state, artifact, nonce)
	if err != nil {
		return nil, r.abortRegisteredNomadLaunch(ctx, req, &state, err)
	}
	return instance, nil
}

// preflightPoolLaunch proves the cluster can honour the pool profile.
func (r *NomadExecutor) preflightPoolLaunch(ctx context.Context, profile nomad.ProfileConfig) (*nomad.Client, error) {
	client, err := r.newNomadAdmissionClient(ctx, profile)
	if err != nil {
		return nil, err
	}
	if err := client.PreflightPool(ctx, profile); err != nil {
		return nil, err
	}
	return client, nil
}

func (r *NomadExecutor) admitAndVerifyPlacedNode(ctx context.Context, req *ExecutorCreateRequest, client *nomad.Client, effective nomad.ProfileConfig, state nomad.EnvironmentState) error {
	_, node, err := r.verifyPlacedPoolNode(ctx, client, state)
	if err != nil {
		return err
	}
	return r.admitPlacedPoolNode(ctx, req, client, effective, node)
}

// checkpointPoolJobEvidence pins the verified agentctl digest and the job spec
// digest before registration. The job names no worker path; those derive from
// the node's state root once it is known.
func (r *NomadExecutor) checkpointPoolJobEvidence(ctx context.Context, req *ExecutorCreateRequest, state *nomad.EnvironmentState) ([]byte, error) {
	artifactDigest, artifact, err := r.resolveNomadWorkerArtifact(ctx, req)
	if err != nil {
		return nil, err
	}
	state.ArtifactSHA256 = artifactDigest
	job, err := nomad.NativeJobFromState(*state)
	if err != nil {
		return nil, err
	}
	if state.NativeJobSpecSHA256, err = nomad.NativeJobSpecDigest(job); err != nil {
		return nil, err
	}
	state.NativeJobPolicyVersion = nomad.PoolJobPolicyVersion
	state.Revision++
	if err := checkpointNomadState(ctx, req, state); err != nil {
		return nil, fmt.Errorf("checkpoint immutable Nomad pool job evidence: %w", err)
	}
	return artifact, nil
}

// stageAndAttachPoolWorker stages the worker files over the placed node's
// transport, releases the job's wait script, and attaches to agentctl.
func (r *NomadExecutor) stageAndAttachPoolWorker(
	ctx context.Context,
	req *ExecutorCreateRequest,
	effective nomad.ProfileConfig,
	state *nomad.EnvironmentState,
	artifact []byte,
	nonce string,
) (*ExecutorInstance, error) {
	transport, err := r.openPoolTransport(ctx, effective, *state)
	if err != nil {
		return nil, err
	}
	if err := stagePoolLaunchFiles(ctx, transport, req, effective, *state, artifact, nonce); err != nil {
		_ = transport.Close()
		return nil, err
	}
	bootstrapCtx, cancel := context.WithTimeout(ctx, nomadBootstrapTimeout)
	defer cancel()
	tunnel, err := openPoolAPITunnel(transport, req)
	if err != nil {
		_ = transport.Close()
		return nil, err
	}
	connection, err := connectNomadAgentctlOverTransport(bootstrapCtx, transport, effective, state, req, nonce, r.logger, tunnel)
	if err != nil {
		if tunnel != nil {
			_ = tunnel.Close()
		}
		_ = transport.Close()
		return nil, err
	}
	if err := r.storeNomadConnection(req.InstanceID, connection, "nomad executor closed while connecting to the retained operation"); err != nil {
		return nil, err
	}
	return connection.instance, nil
}

// openPoolAPITunnel reverse-forwards the Kandev API for an SSH node. A local
// node reaches the API on its own loopback.
func openPoolAPITunnel(transport nomadWorkerTransport, req *ExecutorCreateRequest) (*sshRuntimeAPITunnel, error) {
	sshT, ok := transport.(nomadSSHTransport)
	if !ok {
		return nil, nil
	}
	tunnel, _, err := openSSHRuntimeAPITunnelForRequest(sshT.client, req)
	if err != nil {
		return nil, fmt.Errorf("open Nomad worker Kandev API tunnel: %w", err)
	}
	return tunnel, nil
}

// stagePoolLaunchFiles stages the artifact, bootstrap and private environment,
// then writes the ready marker last so the wait script never starts early.
func stagePoolLaunchFiles(
	ctx context.Context,
	transport nomadWorkerTransport,
	req *ExecutorCreateRequest,
	effective nomad.ProfileConfig,
	state nomad.EnvironmentState,
	artifact []byte,
	nonce string,
) error {
	paths, err := stageNomadLaunchFilesOver(ctx, transport, req, effective, state, artifact, nonce)
	if err != nil {
		return err
	}
	poolPaths, err := nomad.PoolPathsFor(state.StateRoot, state.JobID, state.EnvironmentID, state.Generation, state.ArtifactSHA256)
	if err != nil {
		return err
	}
	if poolPaths.ArtifactPath != paths.ArtifactPath || poolPaths.BootstrapPath != paths.BootstrapPath {
		return errors.New("staged Nomad pool worker paths differ from the job's pre-placement evidence")
	}
	if err := transport.Upload(poolPaths.ReadyPath, []byte(poolReadyContent), 0o600); err != nil {
		return fmt.Errorf("release Nomad pool wait marker: %w", err)
	}
	return nil
}
