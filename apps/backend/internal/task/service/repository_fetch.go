package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// branchFetchCooldown is the minimum interval between successive `git fetch`
// invocations for the same repository when triggered via the branches API.
// The branches dropdown auto-refreshes on every dialog open, so the cooldown
// keeps a noisy UI from spamming the network and the local git index.
const branchFetchCooldown = 30 * time.Second

// branchFetchTimeout bounds a single fetch invocation. Long-lived auth prompts
// or unreachable remotes must not stall the dropdown beyond this window.
const branchFetchTimeout = 30 * time.Second

// BranchRefreshResult reports the outcome of a `git fetch` issued by the
// branches API. Skipped is true when the cooldown short-circuited the fetch;
// in that case FetchedAt is the timestamp of the previous attempt and Err is
// nil.
type BranchRefreshResult struct {
	FetchedAt time.Time
	Skipped   bool
	Err       error
}

// branchFetcher serializes `git fetch` calls for a repository path with a
// per-path cooldown and single-flight deduplication. Concurrent callers for
// the same path observe the same result; callers within the cooldown window
// receive the cached result without re-fetching.
type branchFetcher struct {
	group     singleflight.Group
	mu        sync.Mutex
	lastByKey map[string]BranchRefreshResult
	logger    *zap.Logger
}

func newBranchFetcher(log *zap.Logger) *branchFetcher {
	return &branchFetcher{
		lastByKey: make(map[string]BranchRefreshResult),
		logger:    log,
	}
}

// Fetch runs `git fetch --all --prune --no-tags` for repoPath unless the
// cooldown is still active, in which case the previous result is returned
// with Skipped=true. Concurrent calls for the same path coalesce.
func (f *branchFetcher) Fetch(ctx context.Context, repoPath string) BranchRefreshResult {
	if cached, ok := f.checkCooldown(repoPath); ok {
		return cached
	}
	v, _, _ := f.group.Do(repoPath, func() (any, error) {
		// Re-check inside the singleflight to avoid running back-to-back fetches
		// when many callers arrived during the previous flight.
		if cached, ok := f.checkCooldown(repoPath); ok {
			return cached, nil
		}
		res := f.runFetch(ctx, repoPath)
		f.mu.Lock()
		f.lastByKey[repoPath] = res
		f.mu.Unlock()
		return res, nil
	})
	return v.(BranchRefreshResult)
}

func (f *branchFetcher) checkCooldown(repoPath string) (BranchRefreshResult, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prev, ok := f.lastByKey[repoPath]
	if !ok {
		return BranchRefreshResult{}, false
	}
	if time.Since(prev.FetchedAt) >= branchFetchCooldown {
		return BranchRefreshResult{}, false
	}
	return BranchRefreshResult{FetchedAt: prev.FetchedAt, Skipped: true, Err: prev.Err}, true
}

func (f *branchFetcher) runFetch(ctx context.Context, repoPath string) BranchRefreshResult {
	fetchCtx, cancel := context.WithTimeout(ctx, branchFetchTimeout)
	defer cancel()

	cmd := newNonInteractiveGitFetchCmd(fetchCtx, repoPath)
	output, err := cmd.CombinedOutput()
	res := BranchRefreshResult{FetchedAt: time.Now()}
	if err != nil {
		res.Err = fmt.Errorf("git fetch: %w", err)
		if f.logger != nil {
			f.logger.Warn("branch refresh fetch failed",
				zap.String("path", repoPath),
				zap.String("output", string(output)),
				zap.Error(err))
		}
	}
	return res
}

// newNonInteractiveGitFetchCmd builds a `git fetch --all --prune --no-tags`
// command with the same non-interactive environment used by the worktree
// manager. Auth prompts are disabled so missing credentials surface as fast
// failures rather than hanging the dropdown.
func newNonInteractiveGitFetchCmd(ctx context.Context, repoPath string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "--all", "--prune", "--no-tags")
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=Never",
		"GIT_ASKPASS=echo",
		"SSH_ASKPASS=/bin/false",
		"GIT_SSH_COMMAND=ssh -oBatchMode=yes",
	)
	cmd.WaitDelay = 500 * time.Millisecond
	return cmd
}

// RefreshRepositoryBranches resolves the repository's local path and runs
// `git fetch` for it, subject to a per-repository cooldown and single-flight
// deduplication. Returns ErrPathNotAllowed when the repository's path is not
// inside an allowed discovery root.
func (s *Service) RefreshRepositoryBranches(ctx context.Context, repoID string) (BranchRefreshResult, error) {
	repo, err := s.repoEntities.GetRepository(ctx, repoID)
	if err != nil {
		return BranchRefreshResult{}, err
	}
	if repo.LocalPath == "" {
		return BranchRefreshResult{}, errors.New("repository local path is empty")
	}
	absPath, err := filepath.Abs(repo.LocalPath)
	if err != nil {
		return BranchRefreshResult{}, fmt.Errorf("invalid repository path: %w", err)
	}
	if !isPathAllowed(absPath, s.discoveryRoots()) {
		return BranchRefreshResult{}, ErrPathNotAllowed
	}
	if info, statErr := os.Stat(absPath); statErr != nil || !info.IsDir() {
		return BranchRefreshResult{}, errors.New("repository path is not a directory")
	}
	return s.branchFetcher.Fetch(ctx, absPath), nil
}
