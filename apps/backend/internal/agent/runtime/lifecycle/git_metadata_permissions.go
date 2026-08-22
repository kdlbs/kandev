package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pelletier/go-toml/v2"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/worktree"
)

const (
	gitMetadataProjectionInvalid     = "git_metadata_projection_invalid"
	gitMetadataProjectionUnsupported = "git_metadata_projection_unsupported"
	codexGitMetadataPolicyName       = "kandev_task_git_metadata"
)

// GitMetadataProjectionEnforcer is implemented only by executors that can
// install and attest the task-owned Git metadata policy before an agent
// process starts. It intentionally accepts the typed projections rather than
// paths reconstructed from request metadata, so an executor cannot silently
// widen a task's Git access.
type GitMetadataProjectionEnforcer interface {
	PrepareGitMetadataProjection(context.Context, *ExecutorCreateRequest) error
}

// GitMetadataProjectionRebindEnforcer is deliberately separate from launch
// attestation. A replacement policy must become effective atomically for an
// existing child; mount-based runtimes cannot satisfy that merely by compiling
// a new plan because their bind mounts are immutable.
type GitMetadataProjectionRebindEnforcer interface {
	PrepareGitMetadataProjectionRebind(context.Context, *ExecutorCreateRequest) error
}

// gitMetadataProjectionCapabilityError carries a bounded recovery action from
// an executor capability check. It must never include a resolved checkout or
// configuration path: those are authorization inputs, not user-facing data.
type gitMetadataProjectionCapabilityError struct {
	recovery string
}

func (e *gitMetadataProjectionCapabilityError) Error() string { return e.recovery }

func unsupportedGitMetadataProjection(recovery string) error {
	return &gitMetadataProjectionCapabilityError{recovery: recovery}
}

// preflightGitMetadataProjection verifies a fresh projection and requires an
// executor-specific enforcement attestation. A runtime without this capability
// must fail before CreateInstance: starting an agent that can edit the checkout
// but not its linked metadata reproduces the original index.lock failure.
func preflightGitMetadataProjection(ctx context.Context, runtime ExecutorBackend, req *ExecutorCreateRequest) error {
	if len(req.GitMetadataProjections) == 0 && !requiresCloneGitMetadataPolicy(req) {
		return nil
	}
	if len(req.GitMetadataProjections) > 0 {
		if err := validateGitMetadataProjections(req.GitMetadataProjections); err != nil {
			return err
		}
	}
	enforcer, ok := runtime.(GitMetadataProjectionEnforcer)
	if !ok {
		return fmt.Errorf("%s: executor %q cannot enforce task Git metadata permissions; update the executor or start a new session", gitMetadataProjectionUnsupported, runtime.Name())
	}
	if err := enforcer.PrepareGitMetadataProjection(ctx, req); err != nil {
		var capabilityErr *gitMetadataProjectionCapabilityError
		if errors.As(err, &capabilityErr) {
			return fmt.Errorf("%s: %s", gitMetadataProjectionUnsupported, capabilityErr.recovery)
		}
		return fmt.Errorf("%s: executor %q could not attest task Git metadata permissions; update the executor or start a new session", gitMetadataProjectionUnsupported, runtime.Name())
	}
	return nil
}

func preflightGitMetadataProjectionRebind(ctx context.Context, runtime ExecutorBackend, req *ExecutorCreateRequest) error {
	if err := validateGitMetadataProjections(req.GitMetadataProjections); err != nil {
		return err
	}
	enforcer, ok := runtime.(GitMetadataProjectionRebindEnforcer)
	if !ok {
		return fmt.Errorf("%s: %s cannot atomically replace task Git metadata permissions during workspace attachment; start a new session", gitMetadataProjectionUnsupported, runtime.Name())
	}
	if err := enforcer.PrepareGitMetadataProjectionRebind(ctx, req); err != nil {
		var capabilityErr *gitMetadataProjectionCapabilityError
		if errors.As(err, &capabilityErr) {
			return fmt.Errorf("%s: %s", gitMetadataProjectionUnsupported, capabilityErr.recovery)
		}
		return fmt.Errorf("%s: %s could not atomically replace task Git metadata permissions during workspace attachment; start a new session", gitMetadataProjectionUnsupported, runtime.Name())
	}
	return nil
}

// prepareCodexGitMetadataPolicy is the standalone half of the projection
// contract. The host filesystem sandbox is owned by Codex itself, so the
// executor must install a server-authored profile before creating agentctl.
// Docker retains its own layered mount enforcement and does not reach this
// path.
func prepareCodexGitMetadataPolicy(req *ExecutorCreateRequest) error {
	policyAgent, ok := req.AgentConfig.(agents.FilesystemPolicyAgent)
	if !ok {
		return errors.New("agent does not support filesystem policy")
	}
	descriptor, ok := policyAgent.FilesystemPolicyDescriptor()
	if !ok || descriptor == nil || descriptor.ConfigEnvKey == "" || descriptor.Renderer == nil {
		return errors.New("agent filesystem policy is unavailable")
	}
	if err := rejectLegacyCodexSandbox(req); err != nil {
		return err
	}
	config, err := codexConfigFromEnvironment(req.Env, descriptor.ConfigEnvKey)
	if err != nil {
		return err
	}
	if hasLegacyCodexSandbox(config) {
		return errors.New("legacy Codex sandbox configuration conflicts with task filesystem policy")
	}
	policy, err := descriptor.Renderer.Render(gitMetadataFilesystemPolicy(req.GitMetadataProjections))
	if err != nil {
		return err
	}
	mergeCodexConfig(config, policy)
	encoded, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode agent filesystem policy: %w", err)
	}
	if req.Env == nil {
		req.Env = make(map[string]string)
	}
	req.Env[descriptor.ConfigEnvKey] = string(encoded)
	return nil
}

func gitMetadataFilesystemPolicy(projections []*worktree.GitMetadataProjection) agents.FilesystemPolicy {
	rules := make([]agents.FilesystemPolicyRule, 0, len(projections)*8)
	seen := make(map[string]struct{}, len(projections)*8)
	add := func(path string, access agents.FilesystemAccess) {
		if path == "" {
			return
		}
		key := string(access) + "\x00" + path
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		rules = append(rules, agents.FilesystemPolicyRule{Path: path, Access: access})
	}
	add(":minimal", agents.FilesystemAccessRead)
	for _, projection := range projections {
		add(projection.CommonDir, agents.FilesystemAccessRead)
		add(projection.WorktreesDir, agents.FilesystemAccessDeny)
		for _, path := range projection.WritablePaths {
			add(path, agents.FilesystemAccessWrite)
		}
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Path == rules[j].Path {
			return rules[i].Access < rules[j].Access
		}
		return rules[i].Path < rules[j].Path
	})
	return agents.FilesystemPolicy{Name: codexGitMetadataPolicyName, Rules: rules}
}

func rejectLegacyCodexSandbox(req *ExecutorCreateRequest) error {
	for _, path := range codexConfigPaths(req) {
		contents, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return errors.New("unable to validate Codex sandbox configuration")
		}
		var config map[string]any
		if err := toml.Unmarshal(contents, &config); err != nil {
			return errors.New("unable to validate Codex sandbox configuration")
		}
		if hasLegacyCodexSandbox(config) {
			return errors.New("legacy Codex sandbox configuration conflicts with task filesystem policy")
		}
	}
	return nil
}

func codexConfigPaths(req *ExecutorCreateRequest) []string {
	home := ""
	if req != nil && req.Env != nil {
		home = req.Env["CODEX_HOME"]
	}
	if home == "" {
		home = os.Getenv("CODEX_HOME")
	}
	if home == "" {
		baseHome := ""
		if req != nil && req.Env != nil {
			baseHome = req.Env["HOME"]
		}
		if baseHome == "" {
			baseHome, _ = os.UserHomeDir()
		}
		if baseHome != "" {
			home = filepath.Join(baseHome, ".codex")
		}
	}
	paths := make([]string, 0, 2)
	if home != "" {
		paths = append(paths, filepath.Join(home, "config.toml"))
	}
	if req != nil && req.WorkspacePath != "" {
		paths = append(paths, filepath.Join(req.WorkspacePath, ".codex", "config.toml"))
	}
	return paths
}

func codexConfigFromEnvironment(env map[string]string, key string) (map[string]any, error) {
	config := make(map[string]any)
	if env == nil || env[key] == "" {
		return config, nil
	}
	if err := json.Unmarshal([]byte(env[key]), &config); err != nil {
		return nil, errors.New("unable to validate Codex session configuration")
	}
	return config, nil
}

func hasLegacyCodexSandbox(config map[string]any) bool {
	if config == nil {
		return false
	}
	_, sandboxMode := config["sandbox_mode"]
	_, workspaceWrite := config["sandbox_workspace_write"]
	return sandboxMode || workspaceWrite
}

func mergeCodexConfig(base, overlay map[string]any) {
	for key, value := range overlay {
		if key != "permissions" {
			base[key] = value
			continue
		}
		existing, _ := base[key].(map[string]any)
		if existing == nil {
			existing = make(map[string]any)
			base[key] = existing
		}
		permissions, ok := value.(map[string]any)
		if !ok {
			continue
		}
		for name, profile := range permissions {
			existing[name] = profile
		}
	}
}

// validateGitMetadataProjections is the policy-independent half of launch
// preflight. Workspace rebind uses it before stopping an existing child so a
// forged replacement can never interrupt a healthy session or replace its
// authoritative in-memory projection.
func validateGitMetadataProjections(projections []*worktree.GitMetadataProjection) error {
	for _, projection := range projections {
		if projection == nil || projection.Revalidate() != nil {
			return errors.New(gitMetadataProjectionInvalid)
		}
	}
	return nil
}

// projectionsFromPrepareResult returns a complete, revalidated projection set.
// A partial set is never handed to an executor because it would make a
// multi-repository task appear usable while one checkout remains read-only.
func projectionsFromPrepareResult(result *EnvPrepareResult) ([]*worktree.GitMetadataProjection, error) {
	if result == nil {
		return nil, nil
	}
	projections := make([]*worktree.GitMetadataProjection, 0, len(result.Worktrees)+1)
	if len(result.Worktrees) == 0 && result.GitMetadataProjection != nil {
		projections = append(projections, result.GitMetadataProjection)
	}
	for _, prepared := range result.Worktrees {
		if prepared.GitMetadataProjection == nil {
			return nil, errors.New(gitMetadataProjectionInvalid)
		}
		projections = append(projections, prepared.GitMetadataProjection)
	}
	seen := make(map[string]struct{}, len(projections))
	for _, projection := range projections {
		if projection == nil || projection.Revalidate() != nil {
			return nil, errors.New(gitMetadataProjectionInvalid)
		}
		if _, exists := seen[projection.CheckoutPath]; exists {
			return nil, fmt.Errorf("%s: duplicate checkout", gitMetadataProjectionInvalid)
		}
		seen[projection.CheckoutPath] = struct{}{}
	}
	return projections, nil
}

// gitMetadataMounts compiles projections into deterministic layered Docker
// mounts. The common directory is read-only; its worktrees parent is masked;
// only the owned entry and ordinary commit dependencies are reopened writable.
func gitMetadataMounts(projections []*worktree.GitMetadataProjection) ([]docker.MountConfig, error) {
	if len(projections) == 0 {
		return nil, nil
	}
	common := make(map[string]struct{}, len(projections))
	worktrees := make(map[string]struct{}, len(projections))
	writable := make(map[string]struct{}, len(projections)*4)
	for _, projection := range projections {
		if projection == nil || projection.Revalidate() != nil {
			return nil, errors.New(gitMetadataProjectionInvalid)
		}
		common[projection.CommonDir] = struct{}{}
		if projection.WorktreesDir != "" {
			worktrees[projection.WorktreesDir] = struct{}{}
		}
		for _, path := range projection.WritablePaths {
			writable[path] = struct{}{}
		}
	}
	mounts := make([]docker.MountConfig, 0, len(common)+len(worktrees)+len(writable))
	for _, path := range sortedGitMetadataPaths(common) {
		mounts = append(mounts, docker.MountConfig{Source: path, Target: path, ReadOnly: true})
	}
	for _, path := range sortedGitMetadataPaths(worktrees) {
		mounts = append(mounts, docker.MountConfig{Target: path, Tmpfs: true})
	}
	for _, path := range sortedGitMetadataPaths(writable) {
		mounts = append(mounts, docker.MountConfig{Source: path, Target: path})
	}
	return mounts, nil
}

func sortedGitMetadataPaths(paths map[string]struct{}) []string {
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}
