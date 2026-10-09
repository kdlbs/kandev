package reachability

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// connectionConfigKeys names the exact SSH connection-configuration fields —
// host, port, user, identity source, optional ProxyJump, and pinned host
// fingerprint — that a save-detection comparison must diff. Any other Config
// key (or a non-config field) changing is not connection-relevant and must
// not trigger a reset.
var connectionConfigKeys = []string{
	"ssh_host",
	"ssh_host_alias",
	"ssh_port",
	"ssh_user",
	"ssh_identity_source",
	"ssh_identity_file",
	"ssh_proxy_jump",
	"ssh_host_fingerprint",
}

// SaveObserver implements service.ExecutorSaveObserver: it resets the stored
// reachability record and dispatches an off-cycle probe whenever an active
// SSH executor's connection configuration changes. Declared in
// internal/task/service (this package structurally satisfies it there
// without an import back to it).
type SaveObserver struct {
	poller *Poller
}

// NewSaveObserver builds a SaveObserver over poller. poller supplies the
// repository (for the reset and the pre-reset read), the publisher (for the
// change event), and ProbeNow (for the post-reset off-cycle probe) — the same
// primitives task 03/04 already exercise elsewhere in this package.
func NewSaveObserver(poller *Poller) *SaveObserver {
	return &SaveObserver{poller: poller}
}

// OnExecutorSaved resets and re-probes after when it is an active SSH
// executor whose connection configuration changed. before is nil on create,
// which is always treated as a change — there is no prior state to compare.
func (o *SaveObserver) OnExecutorSaved(ctx context.Context, before, after *models.Executor) {
	if !eligibleForReachability(after) {
		return
	}
	if !connectionConfigChanged(before, after) {
		return
	}
	o.resetAndProbe(ctx, after)
}

func eligibleForReachability(executor *models.Executor) bool {
	return executor != nil &&
		executor.Type == models.ExecutorTypeSSH &&
		executor.Status == models.ExecutorStatusActive
}

func connectionConfigChanged(before, after *models.Executor) bool {
	if before == nil {
		return true
	}
	for _, key := range connectionConfigKeys {
		if before.Config[key] != after.Config[key] {
			return true
		}
	}
	return false
}

// resetAndProbe resets the stored record to the new host, publishes a change
// event when doing so actually changes what a client would see, and dispatches
// an off-cycle probe. The pre-reset record is read first because after the
// reset the "was it worth publishing" comparison is no longer possible — the
// reset itself always leaves the unknown/empty-reason placeholder behind.
func (o *SaveObserver) resetAndProbe(ctx context.Context, executor *models.Executor) {
	repo := o.poller.store.repo
	previous, readErr := repo.GetExecutorReachability(ctx, executor.ID)
	if readErr != nil && !errors.Is(readErr, models.ErrExecutorReachabilityNotFound) {
		o.poller.log.Warn("executor ssh reachability: read before reset failed",
			zap.String("executor_id", executor.ID), zap.Error(readErr))
	}

	newHost := executor.Config["ssh_host"]
	if err := repo.ResetExecutorReachability(ctx, executor.ID, newHost, executor.UpdatedAt); err != nil {
		o.poller.log.Warn("executor ssh reachability: reset failed",
			zap.String("executor_id", executor.ID), zap.Error(err))
		return
	}

	if recordWorthPublishing(previous) || (readErr != nil && !errors.Is(readErr, models.ErrExecutorReachabilityNotFound)) {
		reset, err := repo.GetExecutorReachability(ctx, executor.ID)
		if err != nil {
			o.poller.log.Warn("executor ssh reachability: read after reset failed",
				zap.String("executor_id", executor.ID), zap.Error(err))
			reset = &models.ExecutorReachability{
				ExecutorID: executor.ID,
				State:      models.ExecutorReachabilityStateUnknown,
				Host:       newHost,
				UpdatedAt:  time.Now().UTC(),
			}
		}
		if o.poller.publisher != nil {
			o.poller.publisher.PublishChanged(ctx, reset, o.poller.EffectiveIntervalSeconds())
		}
	}

	o.poller.ProbeNow(executor)
}

// recordWorthPublishing reports whether record differs from the synthesized
// unknown/empty-reason placeholder a client already sees for "no record yet"
// — publishing a reset that lands on that same placeholder would be a no-op
// event.
func recordWorthPublishing(record *models.ExecutorReachability) bool {
	if record == nil {
		return false
	}
	return record.State != models.ExecutorReachabilityStateUnknown || record.Reason != ""
}
