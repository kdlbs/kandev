package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
)

// ErrNomadWorkerQuarantined reports that the dedicated Nomad worker must not
// receive a new operation until an operator clears the quarantine.
var ErrNomadWorkerQuarantined = errors.New("nomad worker is quarantined")

const (
	nomadQuarantineTimeout  = 30 * time.Second
	nomadNodeEligible       = "eligible"
	nomadNodeIneligible     = "ineligible"
	nomadNodeStatusReadyStr = "ready"
)

// admitNewNomadOperation is the durable worker start fence for new operations.
// Every read that cannot positively prove the node is clean fails closed.
func (r *NomadExecutor) admitNewNomadOperation(ctx context.Context, req *ExecutorCreateRequest, profile nomad.ProfileConfig, client *nomad.Client) error {
	if req == nil || client == nil {
		return fmt.Errorf("%w: admission inputs are unavailable", ErrNomadWorkerQuarantined)
	}
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
	node, err := client.GetNode(ctx, profile.NodeID)
	if err != nil {
		return fmt.Errorf("%w: node state could not be read: %w", ErrNomadWorkerQuarantined, err)
	}
	if err := nomad.CheckDedicatedNode(profile, node); err != nil {
		return fmt.Errorf("%w: %w", ErrNomadWorkerQuarantined, err)
	}
	if node.SchedulingEligibility != nomadNodeEligible || node.Status != nomadNodeStatusReadyStr {
		return fmt.Errorf("%w: node is not eligible and ready", ErrNomadWorkerQuarantined)
	}
	return nil
}

func admitAgainstRetainedInventory(ctx context.Context, req *ExecutorCreateRequest, profile nomad.ProfileConfig) error {
	if req.ListNomadInventory == nil {
		return fmt.Errorf("%w: retained inventory is unavailable", ErrNomadWorkerQuarantined)
	}
	states, err := req.ListNomadInventory(ctx)
	if err != nil {
		return fmt.Errorf("%w: retained inventory could not be read: %w", ErrNomadWorkerQuarantined, err)
	}
	for _, other := range states {
		if other.NodeID != profile.NodeID || other.Phase == nomadPhaseOperatorReleased ||
			(other.EnvironmentID == req.TaskEnvironmentID && other.ExecutionID == req.InstanceID) {
			continue
		}
		if other.QuarantineState == nomad.QuarantinePending || other.QuarantineState == nomad.QuarantineMarked ||
			other.Phase == nomadPhaseCleanupPending || other.Phase == "quarantined" {
			return fmt.Errorf("%w: another retained operation holds the worker", ErrNomadWorkerQuarantined)
		}
	}
	return nil
}

// quarantineWorker and reconcilePendingNomadQuarantine are triggered only by
// cleanup and recovery, never by launch admission.
// quarantineWorker persists the pending interlock before any control call, then
// completes the node mutation. A control failure leaves the pending interlock.
func (r *NomadExecutor) quarantineWorker(ctx context.Context, req *ExecutorCreateRequest, profile nomad.ProfileConfig, client *nomad.Client, state *nomad.EnvironmentState, reason string) error {
	if state == nil || client == nil {
		return errors.New("nomad quarantine requires state and a Nomad client")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nomadQuarantineTimeout)
	defer cancel()
	state.QuarantineState = nomad.QuarantinePending
	state.Revision++
	if err := checkpointNomadState(ctx, req, state); err != nil {
		return fmt.Errorf("persist Nomad quarantine interlock: %w", err)
	}
	return completeNomadQuarantine(ctx, req, profile, client, state, reason)
}

// reconcilePendingNomadQuarantine repeats the node mutation idempotently. It
// never re-enables placement and leaves an already-ineligible node untouched.
func (r *NomadExecutor) reconcilePendingNomadQuarantine(ctx context.Context, req *ExecutorCreateRequest, profile nomad.ProfileConfig, client *nomad.Client, state *nomad.EnvironmentState) error {
	if state == nil || client == nil {
		return errors.New("nomad quarantine reconcile requires state and a Nomad client")
	}
	if state.QuarantineState == nomad.QuarantineMarked {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nomadQuarantineTimeout)
	defer cancel()
	return completeNomadQuarantine(ctx, req, profile, client, state, "reconcile")
}

func completeNomadQuarantine(ctx context.Context, req *ExecutorCreateRequest, profile nomad.ProfileConfig, client *nomad.Client, state *nomad.EnvironmentState, reason string) error {
	node, err := client.GetNode(ctx, profile.NodeID)
	if err != nil {
		return fmt.Errorf("nomad quarantine pending: read node: %w", err)
	}
	if err := checkQuarantineTarget(profile, node); err != nil {
		return fmt.Errorf("nomad quarantine pending: refusing to mutate a non-dedicated node: %w", err)
	}
	if node.SchedulingEligibility != nomadNodeIneligible {
		if err := client.SetNodeIneligible(ctx, profile.NodeID); err != nil {
			return fmt.Errorf("nomad quarantine pending: mark node ineligible: %w", err)
		}
	}
	marker := nomad.QuarantineMarker{
		Version: 1, NodeID: profile.NodeID, EnvironmentID: state.EnvironmentID, OperationID: state.OperationID,
		JobID: state.JobID, Reason: reason, Generation: state.Generation,
	}
	if _, _, err := client.PutQuarantineMarkerCAS(ctx, profile.Namespace, marker); err != nil {
		return fmt.Errorf("nomad quarantine pending: write marker: %w", err)
	}
	state.QuarantineState = nomad.QuarantineMarked
	state.Revision++
	if err := checkpointNomadState(ctx, req, state); err != nil {
		return fmt.Errorf("persist Nomad quarantine marked state: %w", err)
	}
	return nil
}

// checkQuarantineTarget proves the node about to be made ineligible is the one
// the profile names: the dedicated worker, or a member of the profile's pool.
func checkQuarantineTarget(profile nomad.ProfileConfig, node *nomad.NodeRecord) error {
	if profile.Mode != nomad.ModePool {
		return nomad.CheckDedicatedNode(profile, node)
	}
	if node == nil || profile.NodeID == "" || node.ID != profile.NodeID || node.NodePool != profile.NodePool {
		return errors.New("nomad node is not the pool node named by the profile")
	}
	return nil
}
