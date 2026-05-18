package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/common/gitref"
)

type RepositoryDiscoveryConfig struct {
	Roots    []string
	MaxDepth int
}

// LocalRepoStatus reports the current branch and dirty file paths for a
// local repository on disk. Used by the task-create dialog to preflight the
// fresh-branch flow.
type LocalRepoStatus struct {
	CurrentBranch string
	DirtyFiles    []string
}

type LocalRepository struct {
	Path          string
	Name          string
	DefaultBranch string
}

type Branch struct {
	Name   string
	Type   string // "local" or "remote"
	Remote string // remote name for remote branches
}

type RepositoryDiscoveryResult struct {
	Roots        []string
	Repositories []LocalRepository
}

type RepositoryPathValidation struct {
	Path          string
	Exists        bool
	IsGitRepo     bool
	Allowed       bool
	DefaultBranch string
	Message       string
}

var ErrPathNotAllowed = errors.New("path is not within an allowed root")

// gitHEAD is the HEAD git ref.
const gitHEAD = "HEAD"

// sourceTypeLocal is the Repository.SourceType value for on-machine repos
// (a path the user discovered or added manually). Provider-backed repos use
// other values like "provider".
const sourceTypeLocal = "local"

func (s *Service) DiscoverLocalRepositories(ctx context.Context, root string) (RepositoryDiscoveryResult, error) {
	roots := s.discoveryRoots()
	if root != "" {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return RepositoryDiscoveryResult{}, fmt.Errorf("invalid root path: %w", err)
		}
		if !isPathAllowed(absRoot, roots) {
			return RepositoryDiscoveryResult{}, ErrPathNotAllowed
		}
		roots = []string{absRoot}
	}

	repos := make([]LocalRepository, 0)
	seen := make(map[string]struct{})
	for _, scanRoot := range roots {
		select {
		case <-ctx.Done():
			return RepositoryDiscoveryResult{}, ctx.Err()
		default:
		}
		found, err := scanRootForRepos(ctx, scanRoot, s.discoveryMaxDepth())
		if err != nil {
			return RepositoryDiscoveryResult{}, err
		}
		for _, repo := range found {
			if _, ok := seen[repo.Path]; ok {
				continue
			}
			seen[repo.Path] = struct{}{}
			repos = append(repos, repo)
		}
	}

	return RepositoryDiscoveryResult{
		Roots:        roots,
		Repositories: repos,
	}, nil
}

func (s *Service) ValidateLocalRepositoryPath(ctx context.Context, path string) (RepositoryPathValidation, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return RepositoryPathValidation{}, fmt.Errorf("invalid path: %w", err)
	}
	roots := s.discoveryRoots()
	allowed := isPathAllowed(absPath, roots)
	info, statErr := os.Stat(absPath)
	exists := statErr == nil
	isDir := exists && info.IsDir()
	isGit := false
	defaultBranch := ""
	message := ""

	switch {
	case !allowed:
		message = "Path is outside the allowed roots"
	case !exists:
		message = "Path does not exist"
	case !isDir:
		message = "Path is not a directory"
	default:
		var gitErr error
		defaultBranch, gitErr = readGitDefaultBranch(absPath)
		isGit = gitErr == nil
		if !isGit {
			message = "Not a git repository"
		}
	}

	return RepositoryPathValidation{
		Path:          absPath,
		Exists:        exists,
		IsGitRepo:     isGit,
		Allowed:       allowed,
		DefaultBranch: defaultBranch,
		Message:       message,
	}, nil
}

// BranchListResult bundles a branch list with the currently-checked-out
// branch so the unified branches endpoint can answer both in one call.
type BranchListResult struct {
	Branches      []Branch
	CurrentBranch string
}

// ListBranches lists git branches for either an imported workspace repo
// (by id, path resolved from the DB row) or an on-machine folder (by path
// directly). Exactly one of `repoID` or `path` should be set; the caller
// (the HTTP handler) validates that.
//
// For provider-backed workspace repos (the "Remote" badge in the UI), the
// branches come from the remote API rather than a local clone. This makes
// the picker work the moment a URL is added - before the orchestrator's
// async clone finishes, and even when no clone ever happens because the
// chosen executor runs the agent in a container that clones on its own.
func (s *Service) ListBranches(ctx context.Context, repoID, path string) ([]Branch, error) {
	if remote, ok, err := s.listRemoteBranchesIfApplicable(ctx, repoID); ok {
		return remote, err
	}
	resolved, err := s.resolveBranchListingPath(ctx, repoID, path)
	if err != nil {
		return nil, err
	}
	return listGitBranches(resolved)
}

// ListBranchesWithCurrent is ListBranches plus the current-branch readout.
// One method so the handler resolves the path once instead of twice.
func (s *Service) ListBranchesWithCurrent(ctx context.Context, repoID, path string) (BranchListResult, error) {
	if remote, ok, err := s.listRemoteBranchesIfApplicable(ctx, repoID); ok {
		if err != nil {
			return BranchListResult{}, err
		}
		// No CurrentBranch for remote: there's no working tree to check.
		// The dialog falls back to its preferred-default-branch heuristic.
		return BranchListResult{Branches: remote}, nil
	}
	resolved, err := s.resolveBranchListingPath(ctx, repoID, path)
	if err != nil {
		return BranchListResult{}, err
	}
	branches, err := listGitBranches(resolved)
	if err != nil {
		return BranchListResult{}, err
	}
	// Current branch is best-effort metadata; an empty string is fine if
	// HEAD is detached or unreadable.
	return BranchListResult{
		Branches:      branches,
		CurrentBranch: readGitCurrentBranch(resolved, s.discoveryRoots()),
	}, nil
}

// listRemoteBranchesIfApplicable returns (branches, true, err) when the repo
// should be answered from a remote provider, and (_, false, _) when the
// caller should fall through to the local-path code path.
//
// "Applicable" means: a repository id is supplied, the repo has a non-local
// source_type (i.e. is provider-backed), and the provider has a registered
// remote lister. The local-path arm of ListBranches stays untouched; this
// only widens the answer for the existing repository-id arm.
//
// Errors from GetRepository propagate with handled=true so callers
// short-circuit instead of falling through to resolveBranchListingPath,
// which would re-issue the same DB lookup. Repo-not-found falls through.
func (s *Service) listRemoteBranchesIfApplicable(ctx context.Context, repoID string) ([]Branch, bool, error) {
	if repoID == "" || s.remoteBranchLister == nil {
		return nil, false, nil
	}
	repo, err := s.repoEntities.GetRepository(ctx, repoID)
	if err != nil {
		return nil, true, err
	}
	if repo == nil {
		return nil, false, nil
	}
	if repo.SourceType == sourceTypeLocal || repo.ProviderOwner == "" || repo.ProviderName == "" {
		return nil, false, nil
	}
	branches, err := s.remoteBranchLister.ListRepoBranches(ctx, repo.ProviderOwner, repo.ProviderName)
	return branches, true, err
}

// resolveBranchListingPath turns the request inputs into a validated
// absolute path to list branches in. Both inputs go through the same
// allowed-roots / dir / symlink validation via resolveAllowedLocalPath.
func (s *Service) resolveBranchListingPath(ctx context.Context, repoID, path string) (string, error) {
	switch {
	case repoID != "":
		repo, err := s.repoEntities.GetRepository(ctx, repoID)
		if err != nil {
			return "", err
		}
		if repo.LocalPath == "" {
			return "", fmt.Errorf("repository local path is empty")
		}
		path = repo.LocalPath
	case path == "":
		return "", fmt.Errorf("repository_id or path is required")
	}
	return s.resolveAllowedLocalPath(path)
}

// LocalRepositoryCurrentBranch returns the currently checked-out branch for a
// local repository on disk. Returns the branch name (e.g. "main") or an empty
// string if HEAD is detached or unreadable.
func (s *Service) LocalRepositoryCurrentBranch(ctx context.Context, path string) (string, error) {
	absPath, err := s.resolveAllowedLocalPath(path)
	if err != nil {
		return "", err
	}
	return readGitCurrentBranch(absPath, s.discoveryRoots()), nil
}

// LocalRepositoryStatus returns the current branch and dirty file list for a
// local repository on disk. Used by the task-create dialog to preflight the
// fresh-branch flow before committing to a destructive checkout.
func (s *Service) LocalRepositoryStatus(ctx context.Context, path string) (LocalRepoStatus, error) {
	absPath, err := s.resolveAllowedLocalPath(path)
	if err != nil {
		return LocalRepoStatus{}, err
	}
	dirty, err := readGitDirtyFiles(ctx, absPath)
	if err != nil {
		return LocalRepoStatus{}, err
	}
	return LocalRepoStatus{
		CurrentBranch: readGitCurrentBranch(absPath, s.discoveryRoots()),
		DirtyFiles:    dirty,
	}, nil
}

func (s *Service) resolveAllowedLocalPath(repoPath string) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("repository path is required")
	}
	roots := s.discoveryRoots()
	abs, err := filepath.Abs(filepath.Clean(repoPath))
	if err != nil {
		return "", fmt.Errorf("invalid repository path: %w", err)
	}
	// Resolve symlinks before the allowlist check so that OS-level symlinks
	// (e.g. /var -> /private/var on macOS) don't produce false negatives.
	// normalizeRoots already resolves symlinks on the roots themselves, so both
	// sides of the comparison are canonical after this step.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	safe, err := pathWithinRoots(filepath.Clean(resolved), roots)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(safe)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository path is not a directory")
	}
	return safe, nil
}

// pathWithinRoots returns abs only when it sits inside one of the allowed
// roots after resolving relative segments. The returned string is the
// trusted, validated path; callers should never use the original input for
// subsequent file operations.
//
// Each root is also passed through filepath.EvalSymlinks before comparison so
// that OS-level symlinks (e.g. /var -> /private/var on macOS) do not produce
// false negatives when abs was obtained via EvalSymlinks but the stored root
// was not.
func pathWithinRoots(abs string, roots []string) (string, error) {
	for _, root := range roots {
		if root == "" {
			continue
		}
		// Resolve symlinks on the root so both sides of the comparison are
		// canonical. Ignore errors — if the root itself doesn't exist we skip it.
		canonRoot := root
		if r, err := filepath.EvalSymlinks(root); err == nil {
			canonRoot = r
		}
		rel, err := filepath.Rel(canonRoot, abs)
		if err != nil {
			continue
		}
		if rel == "." {
			return abs, nil
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		return abs, nil
	}
	return "", ErrPathNotAllowed
}

func (s *Service) discoveryRoots() []string {
	var roots []string
	if len(s.discoveryConfig.Roots) > 0 {
		roots = append(roots, s.discoveryConfig.Roots...)
	} else if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, home)
	}
	// The orchestrator clones provider-backed repos into a configurable base
	// path. When that base path sits outside HOME (e.g. /data/repos in a
	// container deployment) it would otherwise be rejected by the allow-list
	// and local branch listing would silently return nothing. Adding it here
	// keeps the allow-list narrow while still covering kandev's own clone
	// destination.
	if s.repoCloneLocation != nil {
		if base, err := s.repoCloneLocation.ExpandedBasePath(); err == nil && base != "" {
			roots = append(roots, base)
		}
	}
	return normalizeRoots(roots)
}

func (s *Service) discoveryMaxDepth() int {
	if s.discoveryConfig.MaxDepth > 0 {
		return s.discoveryConfig.MaxDepth
	}
	return 5
}

func normalizeRoots(roots []string) []string {
	normalized := make([]string, 0, len(roots))
	seen := make(map[string]struct{})
	for _, root := range roots {
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		clean := filepath.Clean(abs)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		normalized = append(normalized, clean)
	}
	return normalized
}

func scanRootForRepos(ctx context.Context, root string, maxDepth int) ([]LocalRepository, error) {
	repos := make([]LocalRepository, 0)
	walker := &repoWalker{
		root:        root,
		maxDepth:    maxDepth,
		libraryRoot: filepath.Join(root, "Library"),
		cacheRoot:   filepath.Join(root, ".cache"),
		ctx:         ctx,
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		repo, walkErr := walker.visit(path, d, err)
		if walkErr != nil {
			return walkErr
		}
		if repo != nil {
			repos = append(repos, *repo)
		}
		return nil
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return repos, nil
}

// repoWalker holds state for the WalkDir callback used in scanRootForRepos.
type repoWalker struct {
	root        string
	maxDepth    int
	libraryRoot string
	cacheRoot   string
	ctx         context.Context
}

// visit is the WalkDir callback. Returns a non-nil *LocalRepository when a git repo is found.
func (w *repoWalker) visit(path string, d fs.DirEntry, err error) (*LocalRepository, error) {
	if err != nil {
		return nil, nil //nolint:nilerr // skip entries that cannot be accessed
	}
	if w.ctx.Err() != nil {
		return nil, w.ctx.Err()
	}
	if path == w.root {
		return nil, nil
	}

	if skip := w.skipDir(path, d); skip != nil {
		return nil, skip
	}

	if d.Name() == ".git" {
		return w.makeRepo(path, d), nil
	}
	return nil, nil
}

// skipDir returns fs.SkipDir when a directory should not be traversed, or nil to continue.
func (w *repoWalker) skipDir(path string, d fs.DirEntry) error {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return nil
	}
	depth := strings.Count(rel, string(os.PathSeparator))
	if d.IsDir() && depth > w.maxDepth {
		return fs.SkipDir
	}
	if isWithinRoot(path, w.libraryRoot) || isWithinRoot(path, w.cacheRoot) {
		if d.IsDir() {
			return fs.SkipDir
		}
		return nil
	}
	return w.skipByName(path, d)
}

// skipByName skips well-known directories that should never be scanned.
func (w *repoWalker) skipByName(path string, d fs.DirEntry) error {
	if !d.IsDir() {
		return nil
	}
	name := d.Name()
	if (name == "Library" || name == ".cache") && filepath.Dir(path) == w.root {
		return fs.SkipDir
	}
	if strings.HasPrefix(name, ".") && name != ".git" {
		return fs.SkipDir
	}
	if name == "node_modules" {
		return fs.SkipDir
	}
	return nil
}

// makeRepo builds a LocalRepository from a .git entry path.
func (w *repoWalker) makeRepo(path string, d fs.DirEntry) *LocalRepository {
	repoPath := filepath.Dir(path)
	repo := &LocalRepository{
		Path: repoPath,
		Name: filepath.Base(repoPath),
	}
	if branch, err := readGitDefaultBranch(repoPath); err == nil {
		repo.DefaultBranch = branch
	}
	return repo
}

func isPathAllowed(path string, roots []string) bool {
	for _, root := range roots {
		if root == "" {
			continue
		}
		if isWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func isWithinRoot(path string, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absPath = filepath.Clean(absPath)
	absRoot = filepath.Clean(absRoot)
	if absPath == absRoot {
		return true
	}
	separator := string(os.PathSeparator)
	if !strings.HasSuffix(absRoot, separator) {
		absRoot += separator
	}
	return strings.HasPrefix(absPath, absRoot)
}

// readGitCurrentBranch returns the currently checked-out branch by reading
// .git/HEAD directly. Returns an empty string if HEAD is detached, the path
// is not a clean absolute path, the resolved git dir escapes the allowed
// roots, or HEAD is unreadable. We avoid `git rev-parse --abbrev-ref HEAD`
// to skip the subprocess cost on the hot path of branch discovery.
func readGitCurrentBranch(repoPath string, allowedRoots []string) string {
	if !filepath.IsAbs(repoPath) {
		return ""
	}
	cleanRepo := filepath.Clean(repoPath)
	gitDir, err := resolveGitDirWithin(cleanRepo, allowedRoots)
	if err != nil {
		return ""
	}
	headPath := filepath.Clean(filepath.Join(gitDir, gitHEAD))
	if !filepath.IsAbs(headPath) {
		return ""
	}
	content, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(content))
	ref, ok := strings.CutPrefix(trimmed, "ref: ")
	if ok {
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	// Detached HEAD: HEAD is a raw commit SHA (no "ref: " prefix). Return the
	// short form so the task-create dialog's locked branch chip surfaces a
	// meaningful identifier ("a7f5558") instead of the empty placeholder.
	// Validate it's a hex string of 40 chars to avoid leaking garbled HEAD content.
	if isHexCommitSHA(trimmed) {
		return trimmed[:7]
	}
	return ""
}

// isHexCommitSHA returns true if s is a 40-character lowercase hex string —
// the format git writes to HEAD when detached. Anything else (truncated,
// uppercase, with prefix) is treated as unparseable rather than echoed back.
func isHexCommitSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// readGitDirtyFiles returns the list of dirty file paths in a repository, as
// reported by `git status --porcelain=v1 -z`. The `-z` form is NUL-terminated
// and disables path quoting, so paths with spaces, unicode, or control chars
// round-trip cleanly through the consent flow. Renames (status `R`/`C`)
// emit two NUL-separated records: the rename target then the original; we
// keep only the target since that's what's currently in the working tree.
// Returns an empty slice for a clean working tree.
func readGitDirtyFiles(ctx context.Context, repoPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1", "-z")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	entries := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	var paths []string
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		status := entry[:2]
		path := entry[3:]
		paths = append(paths, path)
		// Rename / copy entries push an extra "old name" record after the
		// "new name" record in -z mode; consume and skip it.
		if status[0] == 'R' || status[0] == 'C' {
			i++
		}
	}
	return paths, nil
}

// readGitDefaultBranch is a thin wrapper around gitref.DefaultBranch so this
// file's existing callers (ValidateLocalRepositoryPath, repoWalker.makeRepo,
// the test suite) keep their existing API.
func readGitDefaultBranch(repoPath string) (string, error) {
	return gitref.DefaultBranch(repoPath)
}

func resolveGitDir(repoPath string) (string, error) {
	gitPath := filepath.Join(repoPath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return gitPath, nil
	}
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir:") {
		return "", fmt.Errorf("invalid gitdir reference")
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if filepath.IsAbs(gitDir) {
		return gitDir, nil
	}
	return filepath.Clean(filepath.Join(repoPath, gitDir)), nil
}

// resolveGitDirWithin is the trust-boundary-aware variant of resolveGitDir:
// when `.git` is a file pointer (worktree case), the embedded `gitdir:` line
// can point anywhere on disk. This wrapper rejects any resolved gitdir that
// is not inside the repo path or one of the allowed discovery roots, so a
// crafted `.git` file inside an otherwise-allowed directory cannot make the
// caller read from outside the sandbox.
func resolveGitDirWithin(repoPath string, allowedRoots []string) (string, error) {
	gitDir, err := resolveGitDir(repoPath)
	if err != nil {
		return "", err
	}
	// Resolve symlinks before the root check — `repoPath/.git` itself can be
	// a symlink to a directory outside the sandbox, and a lexical Clean on
	// the gitdir string would not catch that.
	resolved, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean(resolved)
	if !filepath.IsAbs(cleaned) {
		abs, absErr := filepath.Abs(cleaned)
		if absErr != nil {
			return "", absErr
		}
		cleaned = abs
	}
	if isWithinRoot(cleaned, repoPath) {
		return cleaned, nil
	}
	for _, root := range allowedRoots {
		if root != "" && isWithinRoot(cleaned, root) {
			return cleaned, nil
		}
	}
	return "", ErrPathNotAllowed
}

func resolveCommonGitDir(gitDir string) string {
	commonFile := filepath.Join(gitDir, "commondir")
	content, err := os.ReadFile(commonFile)
	if err != nil {
		return gitDir
	}
	commonDir := strings.TrimSpace(string(content))
	if commonDir == "" {
		return gitDir
	}
	if filepath.IsAbs(commonDir) {
		return filepath.Clean(commonDir)
	}
	return filepath.Clean(filepath.Join(gitDir, commonDir))
}

func listGitBranches(repoPath string) ([]Branch, error) {
	gitDir, err := resolveGitDir(repoPath)
	if err != nil {
		return nil, err
	}
	refsRoot := resolveCommonGitDir(gitDir)
	branchMap := make(map[string]Branch)

	collectLocalBranches(filepath.Join(refsRoot, "refs", "heads"), branchMap)
	collectRemoteBranches(filepath.Join(refsRoot, "refs", "remotes"), branchMap)
	parsePackedRefs(refsRoot, branchMap)

	if len(branchMap) == 0 {
		return nil, fmt.Errorf("no branches found")
	}

	result := make([]Branch, 0, len(branchMap))
	for _, branch := range branchMap {
		result = append(result, branch)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type == "local"
		}
		if result[i].Type == "remote" && result[i].Remote != result[j].Remote {
			return result[i].Remote < result[j].Remote
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func collectLocalBranches(localRefsRoot string, branchMap map[string]Branch) {
	_ = filepath.WalkDir(localRefsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(localRefsRoot, path)
		if err != nil || rel == "" || rel == "." {
			return nil
		}
		name := filepath.ToSlash(rel)
		branchMap[name] = Branch{Name: name, Type: "local"}
		return nil
	})
}

func collectRemoteBranches(remoteRefsRoot string, branchMap map[string]Branch) {
	_ = filepath.WalkDir(remoteRefsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(remoteRefsRoot, path)
		if err != nil || rel == "" || rel == "." {
			return nil
		}
		fullPath := filepath.ToSlash(rel)
		parts := strings.SplitN(fullPath, "/", 2)
		if len(parts) < 2 || parts[1] == gitHEAD {
			return nil
		}
		branchMap["remotes/"+fullPath] = Branch{Name: parts[1], Type: "remote", Remote: parts[0]}
		return nil
	})
}

// readGitRemoteOriginURL reads the origin remote URL from a git repository's config.
// Handles both normal repos and worktrees by resolving the common git dir.
func readGitRemoteOriginURL(repoPath string) (string, error) {
	gitDir, err := resolveGitDir(repoPath)
	if err != nil {
		return "", err
	}
	configDir := resolveCommonGitDir(gitDir)
	configPath := filepath.Join(configDir, "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	return parseGitConfigOriginURL(string(content)), nil
}

// parseGitConfigOriginURL extracts the origin remote URL from git config content.
func parseGitConfigOriginURL(config string) string {
	inOrigin := false
	for line := range strings.SplitSeq(config, "\n") {
		line = strings.TrimSpace(line)
		if line == `[remote "origin"]` {
			inOrigin = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inOrigin = false
			continue
		}
		if inOrigin {
			if url, ok := strings.CutPrefix(line, "url = "); ok {
				return url
			}
		}
	}
	return ""
}

// ParseGitRemoteURL extracts provider, owner, and repo name from a git remote URL.
// Supports HTTPS (https://github.com/owner/repo.git), SSH (git@github.com:owner/repo.git),
// and ssh:// (ssh://git@github.com/owner/repo.git) formats.
// Returns empty strings for unrecognized URLs or non-GitHub providers.
func ParseGitRemoteURL(remoteURL string) (provider, owner, name string) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", "", ""
	}

	host, path := splitRemoteURL(remoteURL)
	if host == "" || path == "" {
		return "", "", ""
	}

	if !strings.Contains(strings.ToLower(host), "github.com") {
		return "", "", ""
	}

	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", ""
	}
	return "github", parts[0], parts[1]
}

// splitRemoteURL splits a git remote URL into host and path components.
func splitRemoteURL(remoteURL string) (host, path string) {
	switch {
	case strings.HasPrefix(remoteURL, "https://") || strings.HasPrefix(remoteURL, "http://"):
		trimmed := strings.TrimPrefix(remoteURL, "https://")
		trimmed = strings.TrimPrefix(trimmed, "http://")
		host, path, _ = strings.Cut(trimmed, "/")

	case strings.HasPrefix(remoteURL, "ssh://"):
		trimmed := strings.TrimPrefix(remoteURL, "ssh://")
		if _, after, ok := strings.Cut(trimmed, "@"); ok {
			trimmed = after
		}
		host, path, _ = strings.Cut(trimmed, "/")

	case strings.Contains(remoteURL, "@") && strings.Contains(remoteURL, ":"):
		// git@github.com:owner/repo.git
		_, afterAt, _ := strings.Cut(remoteURL, "@")
		host, path, _ = strings.Cut(afterAt, ":")
	}
	return host, path
}

// ResolveGitRemoteProvider detects the provider, owner, and repo name from a
// local git repository's origin remote. Returns empty strings on any error.
func ResolveGitRemoteProvider(repoPath string) (provider, owner, name string) {
	url, err := readGitRemoteOriginURL(repoPath)
	if err != nil || url == "" {
		return "", "", ""
	}
	return ParseGitRemoteURL(url)
}

func parsePackedRefs(refsRoot string, branchMap map[string]Branch) {
	content, err := os.ReadFile(filepath.Join(refsRoot, "packed-refs"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) < 2 {
			continue
		}
		ref := parts[1]
		if strings.HasPrefix(ref, "refs/heads/") {
			name := strings.TrimPrefix(ref, "refs/heads/")
			if _, exists := branchMap[name]; !exists {
				branchMap[name] = Branch{Name: name, Type: "local"}
			}
		} else if strings.HasPrefix(ref, "refs/remotes/") {
			fullPath := strings.TrimPrefix(ref, "refs/remotes/")
			rp := strings.SplitN(fullPath, "/", 2)
			if len(rp) < 2 || rp[1] == gitHEAD {
				continue
			}
			key := "remotes/" + fullPath
			if _, exists := branchMap[key]; !exists {
				branchMap[key] = Branch{Name: rp[1], Type: "remote", Remote: rp[0]}
			}
		}
	}
}
