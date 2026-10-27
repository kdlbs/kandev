package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/nomad"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// ErrNomadCleanupRetained reports that cleanup could not prove the environment
// absent. The environment stays cleanup_pending and the worker stays fenced.
var ErrNomadCleanupRetained = errors.New("nomad cleanup evidence is incomplete; the environment is retained")

const (
	// nomadDrainDeadline stays below the agentctl server's maximum drain deadline.
	nomadDrainDeadline = 25 * time.Second
	// nomadDrainReplyMargin bounds the time allowed to read the drain reply.
	nomadDrainReplyMargin   = 5 * time.Second
	nomadStopConfirmTimeout = 60 * time.Second
	// nomadAllocationClientStatusPending is a client status that has not yet started.
	nomadAllocationClientStatusPending = "pending"
)

// Closed set of retained-cleanup reasons. They are the only cleanup detail
// that reaches logs, errors, and quarantine markers.
const (
	nomadCleanupReasonCheckpoint           = "checkpoint_failed"
	nomadCleanupReasonIdentityUnrecorded   = "runtime_identity_unrecorded"
	nomadCleanupReasonWorkerUnreachable    = "worker_unreachable"
	nomadCleanupReasonFenceMismatch        = "worker_fence_mismatch"
	nomadCleanupReasonFenceAbsent          = "worker_fence_absent"
	nomadCleanupReasonRuntimeUnreachable   = "runtime_unreachable"
	nomadCleanupReasonDrainUnsupported     = "drain_unsupported"
	nomadCleanupReasonIdentityChanged      = "server_identity_changed"
	nomadCleanupReasonDrainTimeout         = "drain_timeout"
	nomadCleanupReasonDrainFailed          = "drain_failed"
	nomadCleanupReasonReceiptCheckpoint    = "receipt_checkpoint_failed"
	nomadCleanupReasonStopFailed           = "job_stop_failed"
	nomadCleanupReasonHistoryMissing       = "allocation_history_missing"
	nomadCleanupReasonUnexpectedAllocation = "unexpected_allocation"
	nomadCleanupReasonNodeLost             = "node_lost"
	nomadCleanupReasonStopUnconfirmed      = "stop_unconfirmed"
	nomadCleanupReasonReceiptMissing       = "receipt_missing"
	nomadCleanupReasonFenceReleaseFailed   = "fence_release_failed"
	nomadCleanupReasonWorkspaceRemove      = "workspace_remove_failed"
	nomadCleanupReasonAllocationDirRemove  = "allocation_dir_remove_failed"
	nomadCleanupReasonInventoryRelease     = "inventory_release_failed"
	nomadCleanupReasonAbsentCheckpoint     = "absence_checkpoint_failed"
	nomadCleanupReasonInterrupted          = "cleanup_interrupted"
)

var nomadBootstrapFileName = regexp.MustCompile(`^bootstrap-[0-9a-f]{64}\.sh$`)

// nomadCleanup carries the exact identities of one cleanup attempt.
type nomadCleanup struct {
	exec    *NomadExecutor
	req     *ExecutorCreateRequest
	profile nomad.ProfileConfig
	state   nomad.EnvironmentState
	claimID string
	client  *nomad.Client
	worker  nomadWorkerTransport
}

// destroyNomadOperation runs positive cleanup for one retained environment. It
// resumes from the persisted receipt and never drains without the claim that
// owns the receipt digest.
func (r *NomadExecutor) destroyNomadOperation(ctx context.Context, req *ExecutorCreateRequest, claimID string) error {
	return r.runNomadCleanup(ctx, req, claimID, (*nomadCleanup).run)
}

// resumeNomadCleanup repeats the idempotent tail of a retained cleanup: the
// quarantine interlock, then stop, confirm, absence and release when a receipt
// is already persisted. It never drains.
func (r *NomadExecutor) resumeNomadCleanup(ctx context.Context, req *ExecutorCreateRequest, claimID string) error {
	return r.runNomadCleanup(ctx, req, claimID, (*nomadCleanup).resume)
}

func (r *NomadExecutor) runNomadCleanup(
	ctx context.Context, req *ExecutorCreateRequest, claimID string, step func(*nomadCleanup, context.Context) error,
) error {
	if req == nil || claimID == "" {
		return errors.New("nomad cleanup requires the request and its cleanup claim")
	}
	c, err := r.newNomadCleanup(ctx, req, claimID)
	if err != nil {
		return err
	}
	release, err := r.beginNomadCleanup(c.state.ExecutionID)
	if err != nil {
		return err
	}
	defer release()
	defer c.closeWorker()
	return step(c, ctx)
}

func (r *NomadExecutor) newNomadCleanup(ctx context.Context, req *ExecutorCreateRequest, claimID string) (*nomadCleanup, error) {
	state, err := nomad.EnvironmentStateFromMetadata(req.Metadata)
	if err != nil || state.ExecutionID != req.InstanceID || state.EnvironmentID == "" || state.EnvironmentID != req.TaskEnvironmentID {
		return nil, errors.New("nomad cleanup requires the exact retained environment identity")
	}
	profile, err := nomadEffectiveProfile(req.Metadata, state)
	if err != nil || profile.NodeID != state.NodeID || profile.WorkspaceRoot != state.WorkspaceRoot {
		return nil, errors.New("nomad cleanup profile differs from the retained worker identity")
	}
	client, err := r.newNomadAdmissionClient(ctx, profile)
	if err != nil {
		return nil, errors.New("nomad cleanup cannot configure the Nomad API client")
	}
	return &nomadCleanup{exec: r, req: req, profile: profile, state: state, claimID: claimID, client: client}, nil
}

func (r *NomadExecutor) beginNomadCleanup(executionID string) (func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cleanupActive == nil {
		r.cleanupActive = make(map[string]struct{})
	}
	if _, active := r.cleanupActive[executionID]; active {
		return nil, errors.New("nomad cleanup is already running for this execution")
	}
	r.cleanupActive[executionID] = struct{}{}
	return func() {
		r.mu.Lock()
		delete(r.cleanupActive, executionID)
		r.mu.Unlock()
	}, nil
}

func (c *nomadCleanup) closeWorker() {
	if c.worker != nil {
		_ = c.worker.Close()
		c.worker = nil
	}
}

func (c *nomadCleanup) hasReceipt() bool {
	return len(c.state.DrainReceiptDigest) == 64 && c.state.DrainReceiptGeneration == c.state.Generation
}

func (c *nomadCleanup) run(ctx context.Context) error {
	switch {
	case c.hasReceipt() && (c.state.Phase == nomadPhaseAbsent || c.state.Phase == nomadPhaseCleanupPending):
		return c.continueFromReceipt(ctx)
	case c.state.Phase == nomadPhaseAbsent:
		return c.retained(nomadCleanupReasonReceiptMissing)
	}
	return c.drainAndSettle(ctx)
}

func (c *nomadCleanup) resume(ctx context.Context) error {
	if c.state.QuarantineState == nomad.QuarantinePending {
		if err := c.exec.reconcilePendingNomadQuarantine(ctx, c.req, c.profile, c.client, &c.state); err != nil {
			return c.retained(nomadCleanupReasonCheckpoint)
		}
	}
	switch {
	case c.hasReceipt() && (c.state.Phase == nomadPhaseAbsent || c.state.Phase == nomadPhaseCleanupPending):
		return c.continueFromReceipt(ctx)
	case unplacedPoolLaunch(c.state):
		return c.drainAndSettle(ctx)
	case c.state.Phase == nomadPhaseCleanupPending && c.state.QuarantineState == "" && c.state.RefusedBeforeStage:
		return c.drainAndSettle(ctx)
	case c.state.Phase == nomadPhaseCleanupPending && c.state.QuarantineState == "":
		// The cleanup checkpoint committed but neither a drain receipt nor a
		// quarantine did. Fence the node; resume never drains.
		return c.fail(ctx, nomadCleanupReasonInterrupted)
	}
	return nil
}

// continueFromReceipt finishes a cleanup whose drain receipt is already
// persisted. The staged bootstrap environment file is removed first because the
// run that staged it may not have reached its own removal.
func (c *nomadCleanup) continueFromReceipt(ctx context.Context) error {
	if !c.nothingStaged() {
		c.removeBootstrapEnvironment(ctx)
	}
	if c.state.Phase == nomadPhaseAbsent {
		return c.finish(ctx)
	}
	return c.settle(ctx)
}

// retained logs only the closed reason and returns the retained-cleanup error.
func (c *nomadCleanup) retained(reason string) error {
	c.exec.logger.Warn("nomad cleanup retained",
		zap.String("reason", reason), zap.String("execution_id", c.state.ExecutionID),
		zap.String("environment_id", c.state.EnvironmentID), zap.String("phase", c.state.Phase))
	return fmt.Errorf("%w: %s", ErrNomadCleanupRetained, reason)
}

// fail quarantines the worker before reporting a retained cleanup. The
// pending interlock is persisted first; a control failure leaves it pending.
func (c *nomadCleanup) fail(ctx context.Context, reason string) error {
	c.bindUnplacedQuarantineNode(ctx)
	if err := c.exec.quarantineWorker(ctx, c.req, c.profile, c.client, &c.state, reason); err != nil {
		return c.retained(reason + "; quarantine_incomplete")
	}
	return c.retained(reason)
}

// bindUnplacedQuarantineNode names the node a pool job runs on when the launch
// failed before the state recorded one, so the quarantine still lands on the
// node whose slot the job holds. Allocations on more than one node, or none,
// leave the node unnamed and the quarantine reports itself incomplete.
func (c *nomadCleanup) bindUnplacedQuarantineNode(ctx context.Context) {
	if c.profile.Mode != nomad.ModePool || c.profile.NodeID != "" || c.state.JobID == "" {
		return
	}
	allocs, err := c.client.ListAllocations(ctx, c.state.Namespace, c.state.JobID)
	if err != nil {
		return
	}
	c.profile.NodeID = soleAllocationNode(allocs)
}

func soleAllocationNode(allocs []nomad.AllocationRecord) string {
	node := ""
	for _, alloc := range allocs {
		if alloc.NodeID == "" || (node != "" && alloc.NodeID != node) {
			return ""
		}
		node = alloc.NodeID
	}
	return node
}

func (c *nomadCleanup) markCleanupPending(ctx context.Context) error {
	if c.state.Phase == nomadPhaseCleanupPending {
		return nil
	}
	c.state.Phase = nomadPhaseCleanupPending
	c.state.Revision++
	return checkpointNomadState(ctx, c.req, &c.state)
}

func (c *nomadCleanup) runtimeIdentityRecorded() bool {
	return c.state.ServerIdentity != "" && c.req.AuthToken != "" && c.state.AllocationID != "" && c.state.ControlPort > 0
}

func (c *nomadCleanup) drainAndSettle(ctx context.Context) error {
	if err := c.markCleanupPending(ctx); err != nil {
		return c.retained(nomadCleanupReasonCheckpoint)
	}
	if !c.runtimeIdentityRecorded() {
		if c.nothingStaged() {
			return c.settleRefusedLaunch(ctx)
		}
		return c.fail(ctx, nomadCleanupReasonIdentityUnrecorded)
	}
	receipt, reason := c.drain(ctx)
	if reason != "" {
		return c.fail(ctx, reason)
	}
	c.state.DrainReceiptDigest = nomad.ReceiptDigest(c.state, *receipt, c.claimID)
	c.state.DrainReceiptGeneration = c.state.Generation
	c.state.Revision++
	if err := checkpointNomadState(ctx, c.req, &c.state); err != nil {
		return c.fail(ctx, nomadCleanupReasonReceiptCheckpoint)
	}
	c.removeBootstrapEnvironment(ctx)
	return c.settle(ctx)
}

// settleRefusedLaunch finishes a launch that placement admission refused before
// any worker file was staged. No runtime exists, so there is nothing to drain
// and the node is not fenced; the absence receipt records an empty drain.
func (c *nomadCleanup) settleRefusedLaunch(ctx context.Context) error {
	if err := c.recordEmptyDrainReceipt(ctx); err != nil {
		return err
	}
	return c.settle(ctx)
}

// recordEmptyDrainReceipt persists the absence receipt of an environment that
// never had a runtime to drain.
func (c *nomadCleanup) recordEmptyDrainReceipt(ctx context.Context) error {
	c.state.DrainReceiptDigest = nomad.ReceiptDigest(c.state, agentctl.RuntimeDrainReceipt{}, c.claimID)
	c.state.DrainReceiptGeneration = c.state.Generation
	c.state.Revision++
	if err := checkpointNomadState(ctx, c.req, &c.state); err != nil {
		return c.retained(nomadCleanupReasonReceiptCheckpoint)
	}
	return nil
}

// drain seals the exact recorded runtime and waits for its managed teardown.
func (c *nomadCleanup) drain(ctx context.Context) (*agentctl.RuntimeDrainReceipt, string) {
	worker, err := c.exec.dialNomadCleanupWorker(ctx, c.client, c.profile, c.state)
	if err != nil {
		return nil, nomadCleanupReasonWorkerUnreachable
	}
	c.worker = worker
	if err := verifyNomadWorkerFenceOver(ctx, worker, c.profile, c.state); err != nil {
		if errors.Is(err, errNomadWorkerFenceAbsent) {
			return nil, nomadCleanupReasonFenceAbsent
		}
		return nil, nomadCleanupReasonFenceMismatch
	}
	forwarder, err := worker.Forward(c.state.ControlPort, c.exec.logger)
	if err != nil {
		return nil, nomadCleanupReasonRuntimeUnreachable
	}
	defer func() { _ = forwarder.Close() }()
	control := agentctl.NewControlClient(sshAgentctlLoopbackHost, forwarder.LocalPort(), c.exec.logger,
		agentctl.WithControlAuthToken(c.req.AuthToken))
	defer control.Close()
	if reason := c.verifyRuntimeIdentity(ctx, control); reason != "" {
		return nil, reason
	}
	return c.sealAndDrain(ctx, control)
}

func (c *nomadCleanup) verifyRuntimeIdentity(ctx context.Context, control *agentctl.ControlClient) string {
	identity, err := control.GetIdentity(ctx)
	switch {
	case err != nil || identity == nil:
		return nomadCleanupReasonRuntimeUnreachable
	case identity.ServerIdentity != c.state.ServerIdentity:
		return nomadCleanupReasonIdentityChanged
	case !containsNomadCapability(identity.Capabilities, nomadDrainCapability):
		return nomadCleanupReasonDrainUnsupported
	}
	return ""
}

func (c *nomadCleanup) sealAndDrain(ctx context.Context, control *agentctl.ControlClient) (*agentctl.RuntimeDrainReceipt, string) {
	deadline := c.exec.drainDeadlineOrDefault()
	drainCtx, cancel := context.WithTimeout(ctx, deadline+nomadDrainReplyMargin)
	defer cancel()
	receipt, err := control.SealAndDrainRuntime(drainCtx, agentctl.RuntimeDrainRequest{
		RequestID: nomadDrainRequestID(c.state, c.claimID), ExpectedServerIdentity: c.state.ServerIdentity,
		DeadlineMS: deadline.Milliseconds(),
	})
	switch {
	case errors.Is(err, context.DeadlineExceeded) || drainCtx.Err() != nil:
		return nil, nomadCleanupReasonDrainTimeout
	case err != nil || receipt == nil:
		return nil, nomadCleanupReasonDrainFailed
	}
	return receipt, ""
}

func nomadDrainRequestID(state nomad.EnvironmentState, claimID string) string {
	return "drain-" + nomad.ReceiptDigest(state, agentctl.RuntimeDrainReceipt{}, claimID)[:32]
}

func (r *NomadExecutor) drainDeadlineOrDefault() time.Duration {
	if r.drainDeadline > 0 {
		return r.drainDeadline
	}
	return nomadDrainDeadline
}

func (r *NomadExecutor) stopConfirmTimeoutOrDefault() time.Duration {
	if r.stopConfirmTimeout > 0 {
		return r.stopConfirmTimeout
	}
	return nomadStopConfirmTimeout
}

// removeBootstrapEnvironment deletes only the private bootstrap environment
// file this operation staged. A removal failure never blocks cleanup.
func (c *nomadCleanup) removeBootstrapEnvironment(ctx context.Context) {
	path, ok := c.ownedBootstrapEnvironmentPath()
	if !ok {
		return
	}
	if c.worker == nil {
		worker, err := c.exec.dialNomadCleanupWorker(ctx, c.client, c.profile, c.state)
		if err != nil {
			c.exec.logger.Warn("nomad cleanup left the staged bootstrap environment file",
				zap.String("reason", "bootstrap_secret_removal_failed"), zap.String("execution_id", c.state.ExecutionID))
			return
		}
		c.worker = worker
	}
	if _, _, err := c.worker.Run(ctx, "rm -f -- "+shellQuote(path)); err != nil {
		c.exec.logger.Warn("nomad cleanup left the staged bootstrap environment file",
			zap.String("reason", "bootstrap_secret_removal_failed"), zap.String("execution_id", c.state.ExecutionID))
	}
}

func (c *nomadCleanup) ownedBootstrapEnvironmentPath() (string, bool) {
	script := c.state.BootstrapPath
	if script == "" || filepath.Dir(script) != filepath.Join(c.profile.AllocationRoot, c.state.JobID) ||
		!nomadBootstrapFileName.MatchString(filepath.Base(script)) {
		return "", false
	}
	return script + ".env", true
}

// settle stops the whole job, requires exact terminal evidence, and only then
// records absence.
func (c *nomadCleanup) settle(ctx context.Context) error {
	if c.neverPlaced() || c.state.RefusedBeforeStage {
		return c.settleNeverPlaced(ctx)
	}
	if err := c.client.StopJob(ctx, c.state.Namespace, c.state.JobID); err != nil {
		return c.fail(ctx, nomadCleanupReasonStopFailed)
	}
	if reason := c.confirmStopped(ctx); reason != "" {
		return c.fail(ctx, reason)
	}
	c.state.Phase = nomadPhaseAbsent
	c.state.Revision++
	if err := checkpointNomadState(ctx, c.req, &c.state); err != nil {
		return c.retained(nomadCleanupReasonAbsentCheckpoint)
	}
	return c.finish(ctx)
}

func (c *nomadCleanup) confirmStopped(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, c.exec.stopConfirmTimeoutOrDefault())
	defer cancel()
	ticker := time.NewTicker(nomadAllocationPollInterval)
	defer ticker.Stop()
	for {
		reason, pending := c.observeStop(ctx)
		if reason == "" {
			return ""
		}
		if !pending {
			return reason
		}
		select {
		case <-ctx.Done():
			return nomadCleanupReasonStopUnconfirmed
		case <-ticker.C:
		}
	}
}

// observeStop reads the job, its allocations and the node once. pending is true
// only while the single recorded allocation has not reached a terminal state.
func (c *nomadCleanup) observeStop(ctx context.Context) (reason string, pending bool) {
	job, err := c.client.GetJob(ctx, c.state.Namespace, c.state.JobID)
	if err != nil {
		return nomadCleanupReasonHistoryMissing, false
	}
	allocs, err := c.client.ListAllocations(ctx, c.state.Namespace, c.state.JobID)
	if err != nil {
		return nomadCleanupReasonHistoryMissing, false
	}
	node, err := c.client.GetNode(ctx, c.state.NodeID)
	switch {
	case err != nil || node == nil || node.Status != nomadNodeStatusReadyStr:
		return nomadCleanupReasonNodeLost, false
	case len(allocs) == 0:
		return nomadCleanupReasonHistoryMissing, false
	case len(allocs) > 1:
		return nomadCleanupReasonUnexpectedAllocation, false
	case allocs[0].ClientStatus == nomadAllocationClientStatusRunning || allocs[0].ClientStatus == nomadAllocationClientStatusPending:
		return nomadCleanupReasonStopUnconfirmed, true
	}
	if err := nomad.ConfirmStopped(c.state, job, allocs, node); err != nil {
		return nomadCleanupReasonStopUnconfirmed, false
	}
	return "", false
}

// finish releases the worker fence for exactly this environment, then the
// inventory. Both are idempotent and retried from the persisted absent phase.
func (c *nomadCleanup) finish(ctx context.Context) error {
	if !c.nothingStaged() {
		if reason := c.removeStagedState(ctx); reason != "" {
			return c.retained(reason)
		}
	}
	if c.req.ReleaseRuntimeInventory == nil {
		return c.retained(nomadCleanupReasonInventoryRelease)
	}
	if err := c.req.ReleaseRuntimeInventory(ctx); err != nil {
		return c.retained(nomadCleanupReasonInventoryRelease)
	}
	c.exec.discardRecoveredNomadConnection(c.state.ExecutionID)
	return nil
}

// removeStagedState removes what a launch staged on its node, in order, and
// reports the retained-cleanup reason of the first step that failed.
func (c *nomadCleanup) removeStagedState(ctx context.Context) string {
	if err := c.removeWorkspace(ctx); err != nil {
		return nomadCleanupReasonWorkspaceRemove
	}
	if err := c.removeAllocationDir(ctx); err != nil {
		return nomadCleanupReasonAllocationDirRemove
	}
	if err := c.releaseFence(ctx); err != nil {
		return nomadCleanupReasonFenceReleaseFailed
	}
	return ""
}

// removeWorkspace deletes a pool environment's workspace once the runtime is
// proven absent. It runs before the fence is released, so a failure keeps the
// environment retained with its workspace.
func (c *nomadCleanup) removeWorkspace(ctx context.Context) error {
	command, err := nomad.WorkspaceRemoveCommand(c.state)
	if err != nil || command == "" {
		return err
	}
	if err := c.ensureWorker(ctx); err != nil {
		return err
	}
	_, _, err = c.worker.Run(ctx, command)
	return err
}

// removeAllocationDir deletes a pool environment's staged job directory (which
// holds the bootstrap environment file) once the runtime is proven absent. A
// failure keeps the environment retained with the directory.
func (c *nomadCleanup) removeAllocationDir(ctx context.Context) error {
	command, err := nomad.AllocationDirRemoveCommand(c.state)
	if err != nil || command == "" {
		return err
	}
	if err := c.ensureWorker(ctx); err != nil {
		return err
	}
	_, _, err = c.worker.Run(ctx, command)
	return err
}

func (c *nomadCleanup) ensureWorker(ctx context.Context) error {
	if c.worker != nil {
		return nil
	}
	worker, err := c.exec.dialNomadCleanupWorker(ctx, c.client, c.profile, c.state)
	if err != nil {
		return err
	}
	c.worker = worker
	return nil
}

func (c *nomadCleanup) releaseFence(ctx context.Context) error {
	command, err := nomad.FenceReleaseCommand(c.profile, c.state)
	if err != nil {
		return err
	}
	if err := c.ensureWorker(ctx); err != nil {
		return err
	}
	_, _, err = c.worker.Run(ctx, command)
	return err
}
