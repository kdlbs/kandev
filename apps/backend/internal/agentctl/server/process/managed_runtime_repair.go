package process

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/types"
	tools "github.com/kandev/kandev/internal/tools/installer"
	"go.uber.org/zap"
)

const (
	managedRuntimeDefaultPublicationBudget = 90 * time.Second
	managedRuntimeMaxPublicationBudget     = 90 * time.Second
)

const (
	managedRuntimeRepairPrepared    = "prepared"
	managedRuntimeRepairLegacy      = "legacy"
	managedRuntimeRepairUnavailable = "unavailable"
)

// repairContextForStart completes the installation identity from a snapshot
// of the trusted command and authenticated installer provenance. The mutable
// state lock is released before executor-local version lookup.
func (m *Manager) repairContextForStart(ctx context.Context) (managedruntime.RepairContext, bool) {
	repairContext, ok, err := m.repairContextForStartWithError(ctx)
	if err != nil {
		if ctx.Err() == nil {
			m.logger.Warn("managed runtime repair context is unavailable", zap.Error(err))
		}
		return managedruntime.RepairContext{}, false
	}
	return repairContext, ok
}

func (m *Manager) repairContextForStartWithError(ctx context.Context) (managedruntime.RepairContext, bool, error) {
	m.startMu.Lock()
	input, ok := m.repairContextInputLocked()
	m.startMu.Unlock()
	if !ok {
		return managedruntime.RepairContext{}, false, nil
	}
	repairContext, err := m.repairStore.Context(ctx, input)
	if err != nil {
		return managedruntime.RepairContext{}, false, err
	}
	return repairContext, true, nil
}

// repairContextInputLocked captures the mutable installation inputs while the
// start mutex fences the current configuration snapshot. It performs no
// filesystem or subprocess work.
func (m *Manager) repairContextInputLocked() (managedruntime.RepairContextInput, bool) {
	if m.repairStore == nil {
		return managedruntime.RepairContextInput{}, false
	}
	installContext := m.cfg.ManagedRuntimeInstallContext
	if !installContext.Valid() {
		return managedruntime.RepairContextInput{}, false
	}
	if m.cfg.LaunchWrapperRequired || !m.cfg.LaunchWrapper.IsZero() {
		return managedruntime.RepairContextInput{}, false
	}
	command := m.managedRuntimeCommand
	if len(command) == 0 {
		command = m.cfg.AgentArgs
	}
	packageSpec, ok := exactManagedRuntimePackage(m.cfg.AgentType, command)
	if !ok {
		return managedruntime.RepairContextInput{}, false
	}
	return managedruntime.RepairContextInput{
		OwnerScope:         installContext.OwnerScope,
		InstallerEnvDigest: installContext.InstallerEnvDigest,
		PackageSpec:        packageSpec,
		Protocol:           string(m.cfg.Protocol),
		Env:                append([]string(nil), m.cfg.AgentEnv...),
		InheritedEnv:       os.Environ(),
	}, true
}

// selectManagedRuntimeRepair reads an already published compatible generation
// before a normal first attempt. It never installs and never creates a lock.
func (m *Manager) selectManagedRuntimeRepair(ctx context.Context) (*managedruntime.PreparedRuntime, managedruntime.RepairOutcome) {
	prepared, outcome, _, _ := m.selectManagedRuntimeRepairWithContext(ctx)
	return prepared, outcome
}

func (m *Manager) selectManagedRuntimeRepairWithContext(
	ctx context.Context,
) (*managedruntime.PreparedRuntime, managedruntime.RepairOutcome, managedruntime.RepairContext, bool) {
	repairContext, ok := m.repairContextForStart(ctx)
	if !ok {
		return nil, managedruntime.RepairOutcomeMiss, managedruntime.RepairContext{}, false
	}
	prepared, outcome, err := m.repairStore.Lookup(ctx, repairContext)
	if err != nil {
		if ctx.Err() == nil {
			m.logger.Warn("managed runtime repair lookup failed", zap.Error(err))
		}
		return nil, managedruntime.RepairOutcomeMiss, managedruntime.RepairContext{}, false
	}
	m.logger.Info("managed runtime repair selected", zap.String("outcome", string(outcome)))
	return prepared, outcome, repairContext, true
}

// prepareManagedRuntimeRepair prepares the single replacement through the
// owned store when the installation context is complete. Otherwise it keeps
// the existing private preparation path.
func (m *Manager) prepareManagedRuntimeRepair(
	ctx context.Context,
	directive types.ManagedRuntimePreparationDirective,
	snapshot managedRuntimeStartupSnapshot,
	repairContext managedruntime.RepairContext,
	contextOK bool,
) (*managedruntime.PreparedRuntime, string, error) {
	if !contextOK || m.repairStore == nil {
		prepared, err := m.prepareManagedRuntimePrivately(ctx, directive.PackageSpec, snapshot)
		return prepared, managedRuntimeRepairLegacy, err
	}
	lease, err := m.repairStore.BeginRecovery(ctx, repairContext)
	if err != nil {
		if ctx.Err() != nil {
			return nil, managedRuntimeRepairUnavailable, ctx.Err()
		}
		m.logger.Warn("managed runtime repair store is unavailable; using private preparation", zap.Error(err))
		prepared, prepareErr := m.prepareManagedRuntimePrivately(ctx, directive.PackageSpec, snapshot)
		return prepared, managedRuntimeRepairLegacy, prepareErr
	}
	if prepared, outcome, reuseErr := lease.Reuse(ctx); reuseErr != nil || outcome == managedruntime.RepairOutcomeHit {
		if outcome != managedruntime.RepairOutcomeHit {
			_ = lease.Close()
		}
		if reuseErr != nil && ctx.Err() == nil {
			m.logger.Warn("managed runtime repair recheck failed; using private preparation", zap.Error(reuseErr))
			prepared, prepareErr := m.prepareManagedRuntimePrivately(ctx, directive.PackageSpec, snapshot)
			return prepared, managedRuntimeRepairLegacy, prepareErr
		}
		return prepared, string(outcome), reuseErr
	}
	prepared, err := lease.Prepare(ctx, snapshot.Env, m.managedRuntimeRepairRunner(snapshot))
	if err != nil {
		if prepared == nil {
			_ = lease.Close()
		}
		if ctx.Err() != nil {
			return prepared, managedRuntimeRepairPrepared, ctx.Err()
		}
		m.logger.Warn("managed runtime repair preparation failed", zap.Error(err))
		return prepared, managedRuntimeRepairPrepared, errors.New("reusable managed runtime preparation failed")
	}
	return prepared, managedRuntimeRepairPrepared, nil
}

// managedRuntimeRepairRunner installs one generation through the same owned
// process runner and environment used by private preparation.
func (m *Manager) managedRuntimeRepairRunner(snapshot managedRuntimeStartupSnapshot) managedruntime.ManagedRuntimeCommandRunner {
	env := environmentMapFromSlice(snapshot.Env)
	stripEnv := managedRuntimeSnapshotStripEnv(snapshot, os.Environ())
	return func(commandCtx context.Context, command managedruntime.ManagedRuntimeCommand) error {
		_, err := m.runManagedRuntimeCommand(commandCtx, tools.CommandSpec{
			Path: command.Path, Args: command.Args, Dir: command.Dir, Env: env, StripEnv: stripEnv,
		}, command.RetainWhile)
		if err != nil && commandCtx.Err() != nil {
			return commandCtx.Err()
		}
		return err
	}
}

// retireManagedRuntimeRepair retires an unpublished ticket when the process
// stops. Published generations keep their files and selection.
func (m *Manager) retireManagedRuntimeRepair() {
	m.startMu.Lock()
	m.stopGeneration++
	m.repairInitialized = false
	prepared := m.preparedRuntime
	timer := m.repairPublicationTimer
	m.repairPublicationTimer = nil
	m.startMu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	if prepared != nil {
		prepared.RetireRepairPublication()
	}
}

func managedRuntimePublicationDeadline(now time.Time, budgetSeconds int) time.Time {
	budget := time.Duration(budgetSeconds) * time.Second
	if budget <= 0 {
		budget = managedRuntimeDefaultPublicationBudget
	}
	if budget > managedRuntimeMaxPublicationBudget {
		budget = managedRuntimeMaxPublicationBudget
	}
	return now.Add(budget)
}

func (m *Manager) scheduleManagedRuntimePublicationExpiry(
	prepared *managedruntime.PreparedRuntime,
	deadline time.Time,
	revision uint64,
) {
	if prepared == nil || prepared.RepairPublished() {
		return
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		m.expireManagedRuntimeRepair(prepared, revision)
		return
	}
	timer := time.AfterFunc(remaining, func() {
		m.expireManagedRuntimeRepair(prepared, revision)
	})
	m.startMu.Lock()
	if m.repairPublicationTimer != nil {
		m.repairPublicationTimer.Stop()
	}
	m.repairPublicationTimer = timer
	m.startMu.Unlock()
}

// expireManagedRuntimeRepair retires an uninitialized publication ticket at
// its original recovery deadline and reaps only the exact owned process.
func (m *Manager) expireManagedRuntimeRepair(prepared *managedruntime.PreparedRuntime, revision uint64) {
	m.startMu.Lock()
	owned := m.preparedRuntime == prepared && m.preparedRuntimeRevision == revision &&
		!m.repairInitialized
	generation := m.ProcessGeneration()
	m.startMu.Unlock()
	if !owned || prepared.RepairPublished() {
		return
	}
	prepared.RetireRepairPublication()
	m.logger.Warn("managed runtime repair publication window expired",
		zap.Uint64("process_generation", generation))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The timer callback is already asynchronous. Serialize with starts so the
	// generation cannot change between the check and the stop.
	m.startOperationMu.Lock()
	defer m.startOperationMu.Unlock()
	if current := m.ProcessGeneration(); current != generation {
		return
	}
	if err := m.Stop(ctx); err != nil {
		m.logger.Warn("failed to reap uninitialized managed runtime", zap.Error(err))
		return
	}
	if err := prepared.Close(); err != nil {
		m.logger.Warn("failed to remove uninitialized managed runtime candidate", zap.Error(err))
	}
}

// ManagedRuntimeInitialized publishes the pending repair for the exact
// process generation that completed protocol initialization. The manager start
// mutex fences the mutable process snapshot against a concurrent replacement;
// the publication itself is a bounded record write, not an installer or
// protocol call.
func (m *Manager) ManagedRuntimeInitialized(ctx context.Context, generation uint64) {
	prepared, repairInput, stopGeneration, ok := m.beginManagedRuntimePublication(generation)
	if !ok {
		return
	}
	if repairInput != nil && !m.revalidateManagedRuntimePublication(ctx, prepared, *repairInput, generation) {
		return
	}
	m.finishManagedRuntimePublication(ctx, prepared, generation, stopGeneration)
}

func (m *Manager) beginManagedRuntimePublication(
	generation uint64,
) (*managedruntime.PreparedRuntime, *managedruntime.RepairContextInput, uint64, bool) {
	m.startMu.Lock()
	prepared := m.preparedRuntime
	if prepared == nil || generation == 0 || generation != m.ProcessGeneration() {
		m.startMu.Unlock()
		return nil, nil, 0, false
	}
	// Initialization success is the publication boundary. Disarm expiry before
	// any publication attempt so a file or context failure keeps the healthy
	// current agent running.
	m.repairInitialized = true
	if m.repairPublicationTimer != nil {
		m.repairPublicationTimer.Stop()
		m.repairPublicationTimer = nil
	}
	if prepared.RepairPublished() {
		m.startMu.Unlock()
		return nil, nil, 0, false
	}
	if m.preparedRuntimeRevision != m.configRevision {
		m.startMu.Unlock()
		prepared.RetireRepairPublication()
		m.logger.Warn("managed runtime repair retired after configuration changed",
			zap.Uint64("process_generation", generation))
		return nil, nil, 0, false
	}
	var repairInput *managedruntime.RepairContextInput
	if m.preparedRepairContext != nil && m.repairStore != nil {
		repairInput = &managedruntime.RepairContextInput{
			OwnerScope:         m.preparedRepairContext.OwnerScope,
			InstallerEnvDigest: m.preparedRepairContext.InstallerEnvDigest,
			PackageSpec:        m.preparedRepairContext.PackageSpec,
			Protocol:           m.preparedRepairContext.Protocol,
			Env:                append([]string(nil), m.preparedRepairEnv...),
			InheritedEnv:       append([]string(nil), m.preparedRepairInherited...),
		}
	}
	stopGeneration := m.stopGeneration
	m.startMu.Unlock()
	return prepared, repairInput, stopGeneration, true
}

func (m *Manager) revalidateManagedRuntimePublication(
	ctx context.Context,
	prepared *managedruntime.PreparedRuntime,
	repairInput managedruntime.RepairContextInput,
	generation uint64,
) bool {
	revalidateCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	revalidated, revalidateErr := m.completeRepairPublicationContext(revalidateCtx, repairInput)
	cancel()
	if revalidateErr != nil || !m.repairStore.MatchesRepairContext(prepared, revalidated) {
		prepared.RetireRepairPublication()
		m.logger.Warn("managed runtime repair retired after its installer context changed",
			zap.Uint64("process_generation", generation))
		return false
	}
	return true
}

func (m *Manager) completeRepairPublicationContext(
	ctx context.Context,
	input managedruntime.RepairContextInput,
) (managedruntime.RepairContext, error) {
	if m.repairPublicationContext != nil {
		return m.repairPublicationContext(ctx, input)
	}
	return m.repairStore.Context(ctx, input)
}

func (m *Manager) finishManagedRuntimePublication(
	ctx context.Context,
	prepared *managedruntime.PreparedRuntime,
	generation uint64,
	expectedStopGeneration uint64,
) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if ctx.Err() != nil || m.lifetimeCtx.Err() != nil ||
		m.preparedRuntime != prepared || generation != m.ProcessGeneration() ||
		!m.repairInitialized || m.preparedRuntimeRevision != m.configRevision ||
		m.stopGeneration != expectedStopGeneration {
		prepared.RetireRepairPublication()
		return
	}
	if err := prepared.PublishRepair(); err != nil {
		m.logger.Warn("managed runtime repair publication failed",
			zap.Uint64("process_generation", generation),
			zap.Error(err))
		return
	}
	m.logger.Info("managed runtime repair published", zap.Uint64("process_generation", generation))
}
