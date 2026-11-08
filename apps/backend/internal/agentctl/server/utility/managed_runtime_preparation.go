package utility

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

const (
	managedRuntimeProbeAttemptTTL = 15 * time.Minute
	managedRuntimeProbeAttemptMax = 64
)

type managedRuntimeProbeAttempt struct {
	packageSpec string
	fingerprint [32]byte
	createdAt   time.Time
}

type managedRuntimePreparationState struct {
	mu       sync.Mutex
	attempts map[string]managedRuntimeProbeAttempt
	owners   []*managedruntime.PreparedRuntime
	store    *managedruntime.RepairStore
	closed   bool
}

func newManagedRuntimePreparationState() *managedRuntimePreparationState {
	return &managedRuntimePreparationState{attempts: make(map[string]managedRuntimeProbeAttempt)}
}

func (s *managedRuntimePreparationState) rememberProbeFailure(
	packageSpec string,
	agentID string,
	protocol string,
	cfg *InferenceConfigDTO,
) *agentctltypes.ManagedRuntimeCacheEvidence {
	if s == nil || managedruntime.ValidateExactPackageSpec(packageSpec) != nil {
		return nil
	}
	attemptID := uuid.NewString()
	fingerprint := managedRuntimeProbeFingerprint(agentID, protocol, cfg)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.pruneExpiredLocked(now)
	if len(s.attempts) >= managedRuntimeProbeAttemptMax {
		s.evictOldestLocked()
	}
	s.attempts[attemptID] = managedRuntimeProbeAttempt{
		packageSpec: packageSpec,
		fingerprint: fingerprint,
		createdAt:   now,
	}
	return &agentctltypes.ManagedRuntimeCacheEvidence{
		Condition:      agentctltypes.ManagedRuntimeCacheRootManifestMissing,
		PackageSpec:    packageSpec,
		ProbeAttemptID: attemptID,
	}
}

func managedRuntimeProbePackageForAgent(agentID string, command []string) (string, bool) {
	for _, candidate := range command {
		runtimeSpec, ok := agents.BuiltinManagedNPMRuntimeForPackage(agentID, candidate)
		if !ok || managedruntime.ValidateExactPackageSpec(candidate) != nil {
			continue
		}
		prefix := runtimeSpec.Package + "@"
		version := strings.TrimPrefix(candidate, prefix)
		if sameUtilityCommand(command, runtimeSpec.RuntimeCommand(version).Args()) {
			return candidate, true
		}
	}
	return "", false
}

func sameUtilityCommand(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *managedRuntimePreparationState) managedRuntimeCacheEvidenceForProbe(
	ctx context.Context,
	agentID string,
	protocol string,
	cfg *InferenceConfigDTO,
	launchArgs []string,
	env []string,
	launchedExecutable string,
	workDir string,
	stderr *stderrBuffer,
	logger *zap.Logger,
) *agentctltypes.ManagedRuntimeCacheEvidence {
	if ctx == nil || ctx.Err() != nil || !eligibleManagedRuntimeProbeConfig(cfg) {
		return nil
	}
	packageSpec, ok := managedRuntimeProbePackageForAgent(agentID, cfg.Command)
	if !ok || stderr == nil {
		return nil
	}
	rawStderr, complete := stderr.managedNPMDiagnosticSnapshot()
	if !complete || !looksLikeManagedRuntimeRootManifestFailure(rawStderr) {
		return nil
	}
	cacheRoot, err := resolveNPMCacheRoot(ctx, launchedExecutable, launchArgs, env, workDir, logger)
	if err != nil {
		return nil
	}
	cacheEvidence, ok := managedruntime.VerifyNpxRootManifestEvidence(cacheRoot, packageSpec, rawStderr)
	if !ok || cacheEvidence.Condition != managedruntime.CacheConditionRootManifestMissing {
		return nil
	}
	return s.rememberProbeFailure(packageSpec, agentID, protocol, cfg)
}

func looksLikeManagedRuntimeRootManifestFailure(stderr string) bool {
	upper := strings.ToUpper(stderr)
	return len(stderr) <= maxManagedRuntimeRawNPMDiagnosticBytes &&
		(strings.Contains(upper, "NPM ERROR CODE ENOENT") || strings.Contains(upper, "NPM ERR! CODE ENOENT")) &&
		(strings.Contains(upper, "NPM ERROR PATH ") || strings.Contains(upper, "NPM ERR! PATH "))
}

func preparePrivateProbeRuntime(
	ctx context.Context,
	packageSpec string,
	env []string,
	logger *zap.Logger,
) (*managedruntime.PreparedRuntime, error) {
	return managedruntime.PreparePrivateRuntime(ctx, packageSpec, env, runPrivateProbeCommand(logger))
}

func (s *managedRuntimePreparationState) validatesDirective(
	directive *agentctltypes.ManagedRuntimePreparationDirective,
	agentID string,
	protocol string,
	cfg *InferenceConfigDTO,
) bool {
	if s == nil || directive == nil || directive.Condition != agentctltypes.ManagedRuntimeCacheRootManifestMissing ||
		directive.ProbeAttemptID == "" || directive.FailedProcessGeneration != 0 ||
		managedruntime.ValidateExactPackageSpec(directive.PackageSpec) != nil {
		return false
	}
	if !eligibleManagedRuntimeProbeConfig(cfg) {
		return false
	}
	packageSpec, ok := managedRuntimeProbePackageForAgent(agentID, cfg.Command)
	if !ok || packageSpec != directive.PackageSpec {
		return false
	}
	fingerprint := managedRuntimeProbeFingerprint(agentID, protocol, cfg)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.pruneExpiredLocked(now)
	attempt, ok := s.attempts[directive.ProbeAttemptID]
	return ok && attempt.packageSpec == directive.PackageSpec && attempt.fingerprint == fingerprint
}

func (s *managedRuntimePreparationState) retireProbeAttempt(attemptID string) {
	if s == nil || attemptID == "" {
		return
	}
	s.mu.Lock()
	delete(s.attempts, attemptID)
	s.mu.Unlock()
}

func (s *managedRuntimePreparationState) retainOwner(owner *managedruntime.PreparedRuntime) {
	if s == nil || owner == nil {
		return
	}
	s.mu.Lock()
	s.owners = append(s.owners, owner)
	s.mu.Unlock()
}

func (s *managedRuntimePreparationState) close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.closed = true
	s.attempts = make(map[string]managedRuntimeProbeAttempt)
	owners := s.owners
	s.owners = nil
	s.mu.Unlock()
	var closeErrors []error
	var retained []*managedruntime.PreparedRuntime
	for _, owner := range owners {
		if err := owner.Close(); err != nil {
			closeErrors = append(closeErrors, err)
			retained = append(retained, owner)
		}
	}
	if len(retained) != 0 {
		s.mu.Lock()
		s.owners = append(s.owners, retained...)
		s.mu.Unlock()
	}
	return errors.Join(closeErrors...)
}

func (s *managedRuntimePreparationState) pruneExpiredLocked(now time.Time) {
	for id, attempt := range s.attempts {
		if now.Sub(attempt.createdAt) > managedRuntimeProbeAttemptTTL {
			delete(s.attempts, id)
		}
	}
}

func (s *managedRuntimePreparationState) evictOldestLocked() {
	oldestID := ""
	var oldest time.Time
	for id, attempt := range s.attempts {
		if oldestID == "" || attempt.createdAt.Before(oldest) {
			oldestID, oldest = id, attempt.createdAt
		}
	}
	delete(s.attempts, oldestID)
}

func managedRuntimeProbeFingerprint(agentID, protocol string, cfg *InferenceConfigDTO) [32]byte {
	if cfg == nil {
		return sha256.Sum256(nil)
	}
	context := struct {
		AgentID         string            `json:"agent_id"`
		Protocol        string            `json:"protocol"`
		Command         []string          `json:"command"`
		WorkDir         string            `json:"work_dir"`
		Env             map[string]string `json:"env"`
		StripEnv        []string          `json:"strip_env"`
		CommandPrefix   []string          `json:"command_prefix"`
		CLIFlags        []string          `json:"cli_flags"`
		LaunchWrapper   any               `json:"launch_wrapper"`
		OperatorDefined bool              `json:"operator_defined"`
	}{
		AgentID: agentID, Protocol: protocol, Command: cfg.Command, WorkDir: cfg.WorkDir,
		Env: cfg.Env, StripEnv: cfg.StripEnv, CommandPrefix: cfg.CommandPrefix,
		CLIFlags: cfg.CLIFlags, LaunchWrapper: cfg.LaunchWrapper, OperatorDefined: cfg.OperatorDefined,
	}
	data, err := json.Marshal(context)
	if err != nil {
		return sha256.Sum256([]byte("invalid-fingerprint"))
	}
	return sha256.Sum256(data)
}

func eligibleManagedRuntimeProbeConfig(cfg *InferenceConfigDTO) bool {
	return cfg != nil && !cfg.OperatorDefined && len(cfg.CommandPrefix) == 0 && len(cfg.CLIFlags) == 0 && cfg.LaunchWrapper.IsZero()
}

// newManagedRuntimePreparationStateWithHome opens the executor-local reusable
// repair store. An unavailable store leaves the legacy private path intact.
func newManagedRuntimePreparationStateWithHome(
	homeDir string,
	excludedRoots []string,
	logger *zap.Logger,
) *managedRuntimePreparationState {
	state := newManagedRuntimePreparationState()
	if homeDir == "" {
		return state
	}
	store, err := managedruntime.OpenRepairStore(homeDir, excludedRoots...)
	if err != nil {
		if logger != nil {
			logger.Warn("managed runtime repair reuse is unavailable", zap.Error(err))
		}
		return state
	}
	state.store = store
	return state
}

// probeRepairContext builds the installation identity for one system utility
// probe. The probe DTO's Env is already the complete authorized child
// environment, so it carries installer provenance without name guessing.
func (s *managedRuntimePreparationState) probeRepairContext(
	req *ProbeRequest,
	cfg *InferenceConfigDTO,
) (managedruntime.RepairContext, bool) {
	if s == nil || s.store == nil || req == nil || cfg == nil || !eligibleManagedRuntimeProbeConfig(cfg) {
		return managedruntime.RepairContext{}, false
	}
	if !req.InstallContext.Valid() {
		return managedruntime.RepairContext{}, false
	}
	packageSpec, ok := managedRuntimeProbePackageForAgent(req.AgentID, cfg.Command)
	if !ok {
		return managedruntime.RepairContext{}, false
	}
	protocol := string(cfg.Protocol)
	if protocol == "" {
		protocol = "acp"
	}
	repairContext, err := s.store.Context(managedruntime.RepairContextInput{
		OwnerScope:         req.InstallContext.OwnerScope,
		InstallerEnvDigest: req.InstallContext.InstallerEnvDigest,
		PackageSpec:        packageSpec,
		Protocol:           protocol,
		Env:                sanitizeEnvForAgent(cfg),
	})
	if err != nil {
		return managedruntime.RepairContext{}, false
	}
	return repairContext, true
}

// selectProbeRuntime reads a published compatible repair before the first
// probe attempt. It never installs packages and never changes result ownership.
func (s *managedRuntimePreparationState) selectProbeRuntime(
	ctx context.Context,
	req *ProbeRequest,
	cfg *InferenceConfigDTO,
) (*managedruntime.PreparedRuntime, managedruntime.RepairOutcome) {
	repairContext, ok := s.probeRepairContext(req, cfg)
	if !ok {
		return nil, managedruntime.RepairOutcomeMiss
	}
	prepared, outcome, err := s.store.Lookup(ctx, repairContext)
	if err != nil {
		return nil, managedruntime.RepairOutcomeMiss
	}
	return prepared, outcome
}

// prepareProbeRepair prepares one replacement generation for a verified
// probe failure. It reuses a generation another caller already published and
// keeps the existing private path when storage or provenance is unavailable.
func (s *managedRuntimePreparationState) prepareProbeRepair(
	ctx context.Context,
	req *ProbeRequest,
	cfg *InferenceConfigDTO,
	env []string,
	logger *zap.Logger,
) (*managedruntime.PreparedRuntime, error) {
	repairContext, ok := s.probeRepairContext(req, cfg)
	if !ok {
		return preparePrivateProbeRuntime(ctx, req.Preparation.PackageSpec, env, logger)
	}
	lease, err := s.store.BeginRecovery(ctx, repairContext)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if logger != nil {
			logger.Warn("probe repair store is unavailable; using private preparation", zap.Error(err))
		}
		return preparePrivateProbeRuntime(ctx, req.Preparation.PackageSpec, env, logger)
	}
	if prepared, outcome, reuseErr := lease.Reuse(ctx); outcome == managedruntime.RepairOutcomeHit {
		return prepared, reuseErr
	}
	prepared, err := lease.Prepare(ctx, env, runPrivateProbeCommand(logger))
	if err != nil && prepared == nil {
		_ = lease.Close()
	}
	return prepared, err
}

// publishProbeRepair revalidates the probe's installation context and
// publishes at protocol-initialization success. A changed context retires the
// candidate instead of publishing under an obsolete identity.
func (s *managedRuntimePreparationState) publishProbeRepair(
	prepared *managedruntime.PreparedRuntime,
	req *ProbeRequest,
	cfg *InferenceConfigDTO,
) error {
	if prepared == nil || prepared.RepairPublished() {
		return nil
	}
	repairContext, ok := s.probeRepairContext(req, cfg)
	if !ok || !s.store.MatchesRepairContext(prepared, repairContext) {
		prepared.RetireRepairPublication()
		return errors.New("probe installer context changed before publication")
	}
	return prepared.PublishRepair()
}

// runPrivateProbeCommand installs one exact package through the existing
// private probe process lifecycle.
func runPrivateProbeCommand(logger *zap.Logger) managedruntime.ManagedRuntimeCommandRunner {
	return func(commandCtx context.Context, command managedruntime.ManagedRuntimeCommand) error {
		cmd := exec.CommandContext(commandCtx, command.Path, command.Args...)
		cmd.Dir = command.Dir
		cmd.Env = append([]string(nil), command.Env...)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		configureACPCommand(cmd, logger)
		cmd.WaitDelay = 2 * acpCommandTerminateGrace
		if err := cmd.Start(); err != nil {
			return err
		}
		waited := make(chan struct{})
		command.RetainWhile(func() bool {
			select {
			case <-waited:
				return acpProcessGroupAlive(cmd.Process.Pid)
			default:
				return true
			}
		})
		lifecycle, lifecycleErr := installACPCommandLifecycle(cmd)
		if lifecycleErr != nil {
			acpCommandLogger(logger).Warn("failed to install private npm lifecycle", zap.Error(lifecycleErr))
		}
		waitErr := cmd.Wait()
		close(waited)
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(commandCtx), 2*acpCommandTerminateGrace+2*acpCommandForceKillGrace)
		reapACPProcessGroup(cleanupCtx, cmd.Process.Pid, logger)
		cancel()
		releaseACPCommandLifecycle(lifecycle)
		if commandCtx.Err() != nil {
			return commandCtx.Err()
		}
		if waitErr != nil {
			return errors.New("private npm package preparation failed")
		}
		return nil
	}
}
