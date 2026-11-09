package process

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/types"
	tools "github.com/kandev/kandev/internal/tools/installer"
)

const managedRuntimeCacheResolutionTimeout = 2 * time.Second

func managedRuntimeCacheEvidenceForFailure(
	cacheRoot string,
	agentID string,
	command []string,
	failure *types.ManagedStartupEvidence,
	rawStderr string,
) *types.ManagedRuntimeCacheEvidence {
	if failure == nil || failure.ProcessGeneration == 0 ||
		failure.ExitDisposition != types.ManagedStartupExitOrdinary ||
		!failure.CollectionComplete || !failure.NPMDiagnosticPresent ||
		!failure.NPMDiagnosticComplete || failure.NPMCode != "" {
		return nil
	}
	packageSpec, ok := exactManagedRuntimePackage(agentID, command)
	if !ok {
		return nil
	}
	evidence, ok := managedruntime.VerifyNpxRootManifestEvidence(cacheRoot, packageSpec, rawStderr)
	if !ok || evidence.Condition != managedruntime.CacheConditionRootManifestMissing {
		return nil
	}
	return &types.ManagedRuntimeCacheEvidence{
		Condition:         string(evidence.Condition),
		PackageSpec:       evidence.PackageSpec,
		ProcessGeneration: failure.ProcessGeneration,
	}
}

func exactManagedRuntimePackage(agentID string, command []string) (string, bool) {
	for _, candidate := range command {
		spec, ok := agents.BuiltinManagedNPMRuntimeForPackage(agentID, candidate)
		if !ok || managedruntime.ValidateExactPackageSpec(candidate) != nil {
			continue
		}
		prefix := spec.Package + "@"
		version := strings.TrimPrefix(candidate, prefix)
		if sameManagedCommand(command, spec.RuntimeCommand(version).Args()) {
			return candidate, true
		}
	}
	return "", false
}

func sameManagedCommand(left, right []string) bool {
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

func (m *Manager) managedRuntimeCacheEvidence(
	generation uint64,
	failure *types.ManagedStartupEvidence,
) *types.ManagedRuntimeCacheEvidence {
	snapshot := m.takeManagedRuntimeStartupSnapshot(generation)
	rawStderr, rawComplete := m.managedNPMRawDiagnosticSnapshot()
	defer m.clearManagedNPMRawDiagnostic()
	if !snapshot.Valid || snapshot.Wrapped || !rawComplete || failure == nil ||
		failure.ExitDisposition != types.ManagedStartupExitOrdinary ||
		!looksLikeMissingExecutionRootDiagnostic(rawStderr) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), managedRuntimeCacheResolutionTimeout)
	defer cancel()
	cacheRoot, err := m.resolveManagedRuntimeCacheRoot(ctx, snapshot)
	if err != nil {
		return nil
	}
	return managedRuntimeCacheEvidenceForFailure(cacheRoot, snapshot.AgentID, snapshot.Command, failure, rawStderr)
}

func looksLikeMissingExecutionRootDiagnostic(stderr string) bool {
	upper := strings.ToUpper(stderr)
	return len(stderr) <= maxManagedNPMRawDiagnosticBytes &&
		(strings.Contains(upper, "NPM ERROR CODE ENOENT") || strings.Contains(upper, "NPM ERR! CODE ENOENT")) &&
		(strings.Contains(upper, "NPM ERROR PATH ") || strings.Contains(upper, "NPM ERR! PATH "))
}

func (m *Manager) resolveManagedRuntimeCacheRoot(ctx context.Context, snapshot managedRuntimeStartupSnapshot) (string, error) {
	args := append(managedruntime.NPMProjectPrefixArgs(), "config", "get", "cache")
	if err := managedruntime.PrepareNPMProjectPrefix(args); err != nil {
		return "", err
	}
	env := environmentMapFromSlice(snapshot.Env)
	stripEnv := managedRuntimeSnapshotStripEnv(snapshot, os.Environ())
	output, err := m.runManagedRuntimeCommand(ctx, tools.CommandSpec{
		Path: "npm", Args: args, Dir: snapshot.WorkDir, Env: env, StripEnv: stripEnv,
	})
	if err != nil {
		return "", errors.New("resolve npm cache for managed runtime evidence")
	}
	return npmCacheRootFromOutput(output)
}

func (m *Manager) validateManagedRuntimePreparationDirective(
	directive types.ManagedRuntimePreparationDirective,
) (managedRuntimeStartupSnapshot, error) {
	if directive.Condition != types.ManagedRuntimeCacheRootManifestMissing ||
		directive.PackageSpec == "" || directive.FailedProcessGeneration == 0 || directive.ProbeAttemptID != "" {
		return managedRuntimeStartupSnapshot{}, errors.New("managed runtime preparation directive is invalid")
	}
	m.startupEvidenceMu.Lock()
	evidence := cloneManagedStartupEvidence(m.startupEvidence)
	currentGeneration := m.startupGeneration
	m.startupEvidenceMu.Unlock()
	if !matchesManagedPreparationEvidence(currentGeneration, evidence, directive) {
		return managedRuntimeStartupSnapshot{}, errors.New("managed runtime preparation evidence is stale")
	}
	if m.cfg.LaunchWrapperRequired || !m.cfg.LaunchWrapper.IsZero() {
		return managedRuntimeStartupSnapshot{}, errors.New("wrapped managed runtime commands cannot use private preparation")
	}
	command := m.managedRuntimeCommand
	if len(command) == 0 {
		command = m.cfg.AgentArgs
	}
	packageSpec, ok := exactManagedRuntimePackage(m.cfg.AgentType, command)
	if !ok || packageSpec != directive.PackageSpec {
		return managedRuntimeStartupSnapshot{}, errors.New("managed runtime command no longer matches verified evidence")
	}
	return managedRuntimeStartupSnapshot{
		AgentID:  m.cfg.AgentType,
		Command:  append([]string(nil), command...),
		Env:      append([]string(nil), m.cfg.AgentEnv...),
		StripEnv: append([]string(nil), m.cfg.StripEnv...),
		WorkDir:  m.cfg.WorkDir,
		Wrapped:  false,
		Valid:    true,
	}, nil
}

func (m *Manager) prepareManagedRuntimePrivately(
	ctx context.Context,
	packageSpec string,
	snapshot managedRuntimeStartupSnapshot,
) (*managedruntime.PreparedRuntime, error) {
	env := environmentMapFromSlice(snapshot.Env)
	stripEnv := managedRuntimeSnapshotStripEnv(snapshot, os.Environ())
	return managedruntime.PreparePrivateRuntime(ctx, packageSpec, snapshot.Env, func(
		commandCtx context.Context,
		command managedruntime.ManagedRuntimeCommand,
	) error {
		_, err := m.runManagedRuntimeCommand(commandCtx, tools.CommandSpec{
			Path: command.Path, Args: command.Args, Dir: command.Dir, Env: env, StripEnv: stripEnv,
		}, command.RetainWhile)
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	})
}

func (m *Manager) runManagedRuntimeCommand(parent context.Context, spec tools.CommandSpec, retainWhile ...func(func() bool)) ([]byte, error) {
	ctx, release, err := m.BeginOwnedOperation(parent)
	if err != nil {
		return nil, err
	}
	defer release()
	if m.processRunner == nil {
		return nil, errors.New("managed runtime process runner is unavailable")
	}
	proc, err := m.processRunner.StartPiped(PipedStartRequest{
		SessionID: "managed-runtime", Kind: types.ProcessKindCustom, ScriptName: "managed-runtime",
		Command: spec.Path, Args: spec.Args, WorkingDir: spec.Dir, Env: spec.Env,
		StripEnv: spec.StripEnv, PipeStderr: true,
	})
	if err != nil {
		return nil, err
	}
	for _, retain := range retainWhile {
		retain(func() bool {
			select {
			case <-proc.Done:
				return m.processGroupAlive(proc.process.cmd.Process.Pid)
			default:
				return true
			}
		})
	}
	_ = proc.Stdin.Close()
	defer func() { _ = proc.Stdout.Close() }()
	defer func() { _ = proc.Stderr.Close() }()
	stdout := collectManagedCommandOutput(proc.Stdout)
	stderr := collectManagedCommandOutput(proc.Stderr)
	return m.waitManagedRuntimeCommand(ctx, proc, stdout, stderr)
}

func (m *Manager) waitManagedRuntimeCommand(ctx context.Context, proc *PipedProcess, stdout, stderr <-chan []byte) ([]byte, error) {
	select {
	case <-proc.Done:
	case <-ctx.Done():
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = m.processRunner.Stop(stopCtx, StopProcessRequest{ProcessID: proc.ID})
		select {
		case <-proc.Done:
		case <-stopCtx.Done():
			return nil, errors.Join(ctx.Err(), stopCtx.Err())
		}
	}
	var output []byte
	select {
	case output = <-stdout:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-stderr:
	case <-ctx.Done():
		return output, ctx.Err()
	}
	waitErr := proc.Wait()
	if ctx.Err() != nil {
		return output, ctx.Err()
	}
	return output, waitErr
}

func (m *Manager) retainPreparedRuntime(prepared *managedruntime.PreparedRuntime) {
	if prepared == nil {
		return
	}
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if m.preparedRuntime == prepared {
		return
	}
	for _, retained := range m.retainedPreparedRuntimes {
		if retained == prepared {
			return
		}
	}
	m.retainedPreparedRuntimes = append(m.retainedPreparedRuntimes, prepared)
}

func (m *Manager) ownsPreparedRuntime(prepared *managedruntime.PreparedRuntime) bool {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if m.preparedRuntime == prepared {
		return true
	}
	for _, retained := range m.retainedPreparedRuntimes {
		if retained == prepared {
			return true
		}
	}
	return false
}

func managedRuntimeSnapshotStripEnv(snapshot managedRuntimeStartupSnapshot, current ...[]string) []string {
	keys := make(map[string]struct{}, len(snapshot.Env))
	for _, entry := range snapshot.Env {
		if key, _, ok := strings.Cut(entry, "="); ok {
			keys[managedRuntimeEnvKey(key)] = struct{}{}
		}
	}
	strip := make(map[string]struct{}, len(snapshot.StripEnv))
	for _, key := range snapshot.StripEnv {
		strip[key] = struct{}{}
	}
	for _, env := range current {
		for _, entry := range env {
			key, _, ok := strings.Cut(entry, "=")
			if !ok {
				continue
			}
			if _, retained := keys[managedRuntimeEnvKey(key)]; !retained {
				strip[key] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(strip))
	for key := range strip {
		result = append(result, key)
	}
	return result
}

func managedRuntimeEnvKey(key string) string {
	if runtime.GOOS == checkpointWindowsOS {
		return strings.ToUpper(key)
	}
	return key
}

func matchesManagedPreparationEvidence(currentGeneration uint64, evidence *types.ManagedStartupEvidence, directive types.ManagedRuntimePreparationDirective) bool {
	return currentGeneration == directive.FailedProcessGeneration && evidence != nil &&
		evidence.ProcessGeneration == directive.FailedProcessGeneration && evidence.CacheEvidence != nil &&
		evidence.CacheEvidence.Condition == directive.Condition && evidence.CacheEvidence.PackageSpec == directive.PackageSpec
}
