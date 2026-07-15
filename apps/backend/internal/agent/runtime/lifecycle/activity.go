package lifecycle

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	executionActivityPrefix   = "execution:"
	processActivityPrefix     = "process:"
	managedGoCacheMetadataKey = "managed_go_cache_path"
)

var errExecutionActivityInvalidated = errors.New("execution activity invalidated")

func (m *Manager) acquireActivity(ctx context.Context, kind activity.Kind) (*activity.TaskLease, error) {
	m.activityMu.Lock()
	coordinator := m.activityCoordinator
	m.activityMu.Unlock()
	if coordinator == nil {
		return nil, nil
	}
	return coordinator.AcquireTask(ctx, kind)
}

func (m *Manager) trackActivity(key string, lease *activity.TaskLease) {
	if lease == nil {
		return
	}
	m.activityMu.Lock()
	if m.activityLeases == nil {
		m.activityLeases = make(map[string]*activity.TaskLease)
	}
	previous := m.activityLeases[key]
	previousClaimed := m.invalidateExecutionActivityClaimsLocked(key)
	m.activityLeases[key] = lease
	m.activityLeaseOwners[key] = 0
	m.activityMu.Unlock()
	if !previousClaimed {
		previous.Release()
	}
}

func (m *Manager) releaseActivity(key string) {
	m.activityMu.Lock()
	lease := m.activityLeases[key]
	leaseClaimed := m.invalidateExecutionActivityClaimsLocked(key)
	delete(m.activityLeases, key)
	delete(m.activityLeaseOwners, key)
	m.activityMu.Unlock()
	if !leaseClaimed {
		lease.Release()
	}
}

type executionActivityClaim struct {
	manager     *Manager
	key         string
	generation  uint64
	lease       *activity.TaskLease
	existing    bool
	ctx         context.Context
	cancel      context.CancelCauseFunc
	invalidated bool
	once        sync.Once
}

func (c *executionActivityClaim) Context(fallback context.Context) context.Context {
	if c == nil {
		return fallback
	}
	return c.ctx
}

func (c *executionActivityClaim) Commit() {
	if c == nil {
		return
	}
	c.once.Do(func() { c.manager.commitExecutionActivity(c) })
}

func (c *executionActivityClaim) Release() {
	if c == nil {
		return
	}
	c.once.Do(func() { c.manager.releaseExecutionActivityClaim(c) })
}

func (m *Manager) ensureExecutionActivity(
	ctx context.Context,
	executionID string,
	kind activity.Kind,
) (*executionActivityClaim, error) {
	key := executionActivityKey(executionID)
	claimCtx, cancel := context.WithCancelCause(ctx)
	m.activityMu.Lock()
	if m.activityPending == nil {
		m.activityPending = make(map[string]map[uint64]*executionActivityClaim)
	}
	if m.activityLeaseOwners == nil {
		m.activityLeaseOwners = make(map[string]uint64)
	}
	m.activityGeneration++
	generation := m.activityGeneration
	claim := &executionActivityClaim{
		manager: m, key: key, generation: generation, ctx: claimCtx, cancel: cancel,
	}
	pending := m.activityPending[key]
	if pending == nil {
		pending = make(map[uint64]*executionActivityClaim)
		m.activityPending[key] = pending
	}
	pending[generation] = claim
	existing := m.activityLeases[key]
	if existing != nil && m.activityLeaseOwners[key] == 0 {
		m.activityLeaseOwners[key] = generation
		claim.lease = existing
		claim.existing = true
		m.activityMu.Unlock()
		existing.SetKind(kind)
		return claim, nil
	}
	m.activityMu.Unlock()
	lease, err := m.acquireActivity(claimCtx, kind)
	if err != nil {
		m.clearPendingActivity(claim)
		if cause := context.Cause(claimCtx); cause != nil {
			return nil, cause
		}
		return nil, err
	}
	if lease == nil {
		m.clearPendingActivity(claim)
		return nil, nil
	}
	return m.finishPendingActivity(claim, lease)
}

func (m *Manager) clearPendingActivity(claim *executionActivityClaim) {
	m.activityMu.Lock()
	deletePendingGeneration(m.activityPending, claim.key, claim.generation)
	m.activityMu.Unlock()
	claim.cancel(nil)
}

func (m *Manager) finishPendingActivity(
	claim *executionActivityClaim,
	lease *activity.TaskLease,
) (*executionActivityClaim, error) {
	m.activityMu.Lock()
	pending := m.activityPending[claim.key]
	if pending[claim.generation] != claim || claim.invalidated {
		m.activityMu.Unlock()
		lease.Release()
		claim.cancel(errExecutionActivityInvalidated)
		return nil, errExecutionActivityInvalidated
	}
	claim.lease = lease
	m.activityMu.Unlock()
	return claim, nil
}

func (m *Manager) commitExecutionActivity(claim *executionActivityClaim) {
	m.activityMu.Lock()
	if claim.existing {
		if m.activityLeases[claim.key] == claim.lease &&
			m.activityLeaseOwners[claim.key] == claim.generation && !claim.invalidated {
			m.activityLeaseOwners[claim.key] = 0
			deletePendingGeneration(m.activityPending, claim.key, claim.generation)
			m.activityMu.Unlock()
			claim.cancel(nil)
			return
		}
		deletePendingGeneration(m.activityPending, claim.key, claim.generation)
		m.activityMu.Unlock()
		claim.lease.Release()
		claim.cancel(nil)
		return
	}
	pending := m.activityPending[claim.key]
	if pending[claim.generation] != claim || claim.invalidated {
		deletePendingGeneration(m.activityPending, claim.key, claim.generation)
		m.activityMu.Unlock()
		claim.lease.Release()
		claim.cancel(nil)
		return
	}
	deletePendingGeneration(m.activityPending, claim.key, claim.generation)
	previous := m.activityLeases[claim.key]
	if previous == nil || m.activityLeaseOwners[claim.key] != 0 {
		previousClaimed := m.activityLeaseOwners[claim.key] != 0
		if previousClaimed {
			m.invalidateExecutionActivityOwnerLocked(claim.key)
		}
		m.activityLeases[claim.key] = claim.lease
		m.activityLeaseOwners[claim.key] = 0
		m.activityMu.Unlock()
		if !previousClaimed {
			previous.Release()
		}
		claim.cancel(nil)
		return
	}
	m.activityMu.Unlock()
	claim.lease.Release()
	claim.cancel(nil)
}

func (m *Manager) releaseExecutionActivityClaim(claim *executionActivityClaim) {
	m.activityMu.Lock()
	if claim.existing {
		if m.activityLeases[claim.key] == claim.lease &&
			m.activityLeaseOwners[claim.key] == claim.generation {
			delete(m.activityLeases, claim.key)
			delete(m.activityLeaseOwners, claim.key)
		}
	}
	deletePendingGeneration(m.activityPending, claim.key, claim.generation)
	m.activityMu.Unlock()
	claim.lease.Release()
	claim.cancel(nil)
}

func deletePendingGeneration(
	pendingByKey map[string]map[uint64]*executionActivityClaim,
	key string,
	generation uint64,
) {
	pending := pendingByKey[key]
	delete(pending, generation)
	if len(pending) == 0 {
		delete(pendingByKey, key)
	}
}

func (m *Manager) invalidateExecutionActivityClaimsLocked(key string) bool {
	owner := m.activityLeaseOwners[key]
	for _, claim := range m.activityPending[key] {
		claim.invalidated = true
		claim.cancel(errExecutionActivityInvalidated)
	}
	return owner != 0
}

func (m *Manager) invalidateExecutionActivityOwnerLocked(key string) {
	owner := m.activityLeaseOwners[key]
	if claim := m.activityPending[key][owner]; claim != nil {
		claim.invalidated = true
		claim.cancel(errExecutionActivityInvalidated)
	}
}

func executionActivityKey(executionID string) string {
	return executionActivityPrefix + executionID
}

func processActivityKey(processID string) string {
	return processActivityPrefix + processID
}

func processActivityKind(kind string) activity.Kind {
	switch strings.ToLower(kind) {
	case string(streams.ProcessKindSetup):
		return activity.KindSetupScript
	case string(agentctltypes.ProcessKindCleanup):
		return activity.KindCleanupScript
	case "test":
		return activity.KindTestCommand
	default:
		return activity.KindShellCommand
	}
}

func (m *Manager) releaseTerminalProcessActivity(status *agentctltypes.ProcessStatusUpdate) {
	if status == nil {
		return
	}
	switch status.Status {
	case agentctltypes.ProcessStatusExited,
		agentctltypes.ProcessStatusFailed,
		agentctltypes.ProcessStatusStopped:
		m.releaseActivity(processActivityKey(status.ProcessID))
	}
}
