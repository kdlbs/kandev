package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/worktree"
)

const (
	gitMetadataProjectionInvalid     = "git_metadata_projection_invalid"
	gitMetadataProjectionUnsupported = "git_metadata_projection_unsupported"
)

// GitMetadataProjectionEnforcer is implemented only by executors that can
// install and attest the task-owned Git metadata policy before an agent
// process starts. It intentionally accepts the typed projections rather than
// paths reconstructed from request metadata, so an executor cannot silently
// widen a task's Git access.
type GitMetadataProjectionEnforcer interface {
	PrepareGitMetadataProjection(context.Context, []*worktree.GitMetadataProjection) error
}

// preflightGitMetadataProjection verifies a fresh projection and requires an
// executor-specific enforcement attestation. A runtime without this capability
// must fail before CreateInstance: starting an agent that can edit the checkout
// but not its linked metadata reproduces the original index.lock failure.
func preflightGitMetadataProjection(ctx context.Context, runtime ExecutorBackend, projections []*worktree.GitMetadataProjection) error {
	if len(projections) == 0 {
		return nil
	}
	if err := validateGitMetadataProjections(projections); err != nil {
		return err
	}
	enforcer, ok := runtime.(GitMetadataProjectionEnforcer)
	if !ok {
		return fmt.Errorf("%s: executor %q cannot enforce task Git metadata permissions; update the executor or start a new session", gitMetadataProjectionUnsupported, runtime.Name())
	}
	if err := enforcer.PrepareGitMetadataProjection(ctx, projections); err != nil {
		return fmt.Errorf("%s: executor %q could not attest task Git metadata permissions; update the executor or start a new session", gitMetadataProjectionUnsupported, runtime.Name())
	}
	return nil
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
