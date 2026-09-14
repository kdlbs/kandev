package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/fsdiagnostics"
	"github.com/kandev/kandev/internal/common/gitref"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/task/models"
)

type RepositoryDiscoveryConfig struct {
	Roots             []string
	MaxDepth          int
	TaskWorktreeRoots []string
	DesktopRuntime    bool
}

const (
	githubProviderName = "github"
	githubProviderHost = "https://github.com"
)

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
	Roots                    []string
	Repositories             []LocalRepository
	DesktopRuntime           bool
	RootStates               []models.DesktopDiscoveryRoot
	ScanTime                 *time.Time
	Refreshing               bool
	Cached                   bool
	HomeConfirmationRequired bool
	FailedRoots              []string
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

var ErrInvalidRepositoryPath = errors.New("invalid repository path")

// gitHEAD is the HEAD git ref.
const gitHEAD = "HEAD"

// sourceTypeLocal is the Repository.SourceType value for on-machine repos
// (a path the user discovered or added manually).
const sourceTypeLocal = "local"

// sourceTypeProvider is the Repository.SourceType value for provider-backed
// repos that can be cloned/synced from their upstream identity.
const sourceTypeProvider = "provider"

func (s *Service) DiscoverLocalRepositories(ctx context.Context, root string) (RepositoryDiscoveryResult, error) {
	return s.RefreshLocalRepositoryDiscovery(ctx, root)
}

// DiscoverLocalRepositoriesForWorkspace applies the workspace visibility
// check before using the legacy discovery endpoint.
func (s *Service) DiscoverLocalRepositoriesForWorkspace(
	ctx context.Context,
	workspaceID, root string,
) (RepositoryDiscoveryResult, error) {
	return s.refreshLocalRepositoryDiscovery(ctx, workspaceID, root, discoveryTriggerManualRefresh)
}

func (s *Service) ValidateLocalRepositoryPath(ctx context.Context, path string) (RepositoryPathValidation, error) {
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		s.logFilesystemFailure("repository.discovery.validation", path, "user_select", err)
		return RepositoryPathValidation{}, fmt.Errorf("invalid path: %w", err)
	}
	result := RepositoryPathValidation{Path: absPath, Allowed: true}
	canonicalPath, defaultBranch, resolveErr := resolveExplicitLocalRepositoryPath(absPath)
	if resolveErr == nil {
		result.Path = canonicalPath
		result.Exists = true
		result.IsGitRepo = true
		result.DefaultBranch = defaultBranch
		return result, nil
	}

	// codeql[go/path-injection] Intentional read-only diagnostics for the local path selected by the user.
	info, statErr := os.Stat(absPath)
	result.Exists = statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		s.logFilesystemFailure("repository.discovery.validation", absPath, "user_select", statErr)
	}
	switch {
	case errors.Is(statErr, os.ErrNotExist):
		result.Message = "Path does not exist"
	case statErr != nil:
		result.Message = "Path cannot be accessed"
	case !info.IsDir():
		result.Message = "Path is not a directory"
	default:
		result.Message = "Not a git repository"
	}
	return result, nil
}

// resolveExplicitLocalRepositoryPath validates a path the user selected
// directly. Discovery roots intentionally do not apply to explicit choices.
func resolveExplicitLocalRepositoryPath(repoPath string) (string, string, error) {
	if repoPath == "" {
		return "", "", fmt.Errorf("%w: path is required", ErrInvalidRepositoryPath)
	}
	absPath, err := filepath.Abs(filepath.Clean(repoPath))
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrInvalidRepositoryPath, err)
	}
	canonicalPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrInvalidRepositoryPath, err)
	}
	// codeql[go/path-injection] Intentional validation of the canonical local repository selected by the user.
	info, err := os.Stat(canonicalPath)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrInvalidRepositoryPath, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("%w: path is not a directory", ErrInvalidRepositoryPath)
	}
	if err := validateExplicitGitMetadata(canonicalPath); err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrInvalidRepositoryPath, err)
	}
	defaultBranch, err := readGitDefaultBranch(canonicalPath)
	if err != nil {
		return "", "", fmt.Errorf("%w: not a git repository", ErrInvalidRepositoryPath)
	}
	return filepath.Clean(canonicalPath), defaultBranch, nil
}

// validateExplicitGitMetadata rejects metadata indirection that does not prove
// it belongs to the selected repository. A real linked worktree has a .git
// pointer whose target contains both a reciprocal gitdir pointer and a
// commondir placing that target under <common-dir>/worktrees. Without those
// checks, a crafted folder could borrow another repository's .git directory
// and turn an exact-path grant into permission to mutate unrelated Git refs.
func validateExplicitGitMetadata(repoPath string) error {
	gitPath := filepath.Join(repoPath, ".git")
	// codeql[go/path-injection] The canonical repository path is validated before inspecting its exact .git child.
	info, err := os.Lstat(gitPath)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New(".git metadata must not be a symbolic link")
	}
	if info.IsDir() {
		return validateStandaloneGitMetadata(gitPath)
	}
	if !info.Mode().IsRegular() {
		return errors.New(".git metadata must be a directory or linked-worktree pointer")
	}
	return validateGitFileMetadata(repoPath, gitPath)
}

func validateGitFileMetadata(repoPath, gitPath string) error {
	linkedWorktreeErr := validateLinkedWorktreeMetadata(repoPath, gitPath)
	if linkedWorktreeErr == nil {
		return nil
	}
	submoduleErr := validateSubmoduleMetadata(repoPath)
	if submoduleErr == nil {
		return nil
	}
	return errors.Join(linkedWorktreeErr, submoduleErr)
}

func validateSubmoduleMetadata(repoPath string) error {
	gitDir, err := resolveGitDir(repoPath)
	if err != nil {
		return err
	}
	canonicalGitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		return err
	}
	canonicalRepoPath, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(canonicalGitDir, "commondir")); err == nil {
		return errors.New("submodule metadata must not redirect its common directory")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	config, err := os.ReadFile(filepath.Join(canonicalGitDir, "config"))
	if err != nil {
		return err
	}
	worktree, err := parseGitConfigCoreWorktree(string(config))
	if err != nil {
		return err
	}
	resolvedWorktree, err := resolveMetadataPathValue(worktree, canonicalGitDir)
	if err != nil {
		return err
	}
	if !sameCanonicalPath(resolvedWorktree, canonicalRepoPath) {
		return errors.New("submodule metadata does not point back to the selected repository")
	}
	return nil
}

func validateStandaloneGitMetadata(gitPath string) error {
	// codeql[go/path-injection] gitPath is the validated repository's real, non-symlink .git directory.
	if _, err := os.Lstat(filepath.Join(gitPath, "commondir")); err == nil {
		return errors.New("standalone .git metadata must not redirect its common directory")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func validateLinkedWorktreeMetadata(repoPath, gitPath string) error {
	gitDir, err := resolveGitDir(repoPath)
	if err != nil {
		return err
	}
	canonicalGitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		return err
	}
	canonicalGitFile, err := filepath.EvalSymlinks(gitPath)
	if err != nil {
		return err
	}

	backPointer, err := readMetadataPath(filepath.Join(canonicalGitDir, "gitdir"), canonicalGitDir)
	if err != nil {
		return errors.New(".git pointer is not a linked worktree")
	}
	if !sameCanonicalPath(backPointer, canonicalGitFile) {
		return errors.New("linked-worktree metadata does not point back to the selected repository")
	}

	commonDir, err := readMetadataPath(filepath.Join(canonicalGitDir, "commondir"), canonicalGitDir)
	if err != nil {
		return errors.New("linked-worktree metadata has no valid common directory")
	}
	worktreesDir := filepath.Join(commonDir, "worktrees")
	rel, err := filepath.Rel(worktreesDir, canonicalGitDir)
	if err != nil || rel == "." || rel == ".." || filepath.Dir(rel) != "." {
		return errors.New("linked-worktree metadata is outside its common directory")
	}
	return nil
}

func readMetadataPath(path, relativeTo string) (string, error) {
	// codeql[go/path-injection] Linked-worktree metadata is canonicalized and verified by reciprocal pointers.
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	metadataPath := strings.TrimSpace(string(content))
	if metadataPath == "" {
		return "", errors.New("metadata path is empty")
	}
	return resolveMetadataPathValue(metadataPath, relativeTo)
}

func resolveMetadataPathValue(value, relativeTo string) (string, error) {
	if !filepath.IsAbs(value) {
		value = filepath.Join(relativeTo, value)
	}
	return filepath.EvalSymlinks(filepath.Clean(value))
}

func parseGitConfigCoreWorktree(config string) (string, error) {
	section := ""
	var worktree string
	var foundWorktree bool
	for _, rawLine := range strings.Split(config, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if parsedSection, ok := parseGitConfigSection(line); ok {
			if err := rejectGitConfigAlternateSection(parsedSection); err != nil {
				return "", err
			}
			section = parsedSection
			continue
		}
		switch {
		case strings.EqualFold(section, "extensions"):
			if err := rejectGitConfigWorktreeConfig(line); err != nil {
				return "", err
			}
		case strings.EqualFold(section, "core"):
			parsedWorktree, found, err := parseGitConfigWorktreeLine(line)
			if err != nil {
				return "", err
			}
			if found {
				worktree = parsedWorktree
				foundWorktree = true
			}
		}
	}
	if !foundWorktree {
		return "", errors.New("core.worktree is missing")
	}
	if worktree == "" {
		return "", errors.New("core.worktree is empty")
	}
	return worktree, nil
}

func rejectGitConfigAlternateSection(section string) error {
	sectionName := strings.ToLower(section)
	if separator := strings.IndexAny(sectionName, " \t"); separator >= 0 {
		sectionName = sectionName[:separator]
	}
	if sectionName == "include" || sectionName == "includeif" {
		return errors.New("git config includes are not allowed")
	}
	return nil
}

func rejectGitConfigWorktreeConfig(line string) error {
	key, value, found := strings.Cut(line, "=")
	if !found {
		if strings.EqualFold(strings.TrimSpace(line), "worktreeConfig") {
			return errors.New("git worktree configuration is not allowed")
		}
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(key), "worktreeConfig") {
		return nil
	}
	enabled, err := parseGitConfigBoolean(value)
	if err != nil {
		return fmt.Errorf("parse extensions.worktreeConfig: %w", err)
	}
	if enabled {
		return errors.New("git worktree configuration is not allowed")
	}
	return nil
}

func parseGitConfigWorktreeLine(line string) (string, bool, error) {
	key, value, found := strings.Cut(line, "=")
	if !found || !strings.EqualFold(strings.TrimSpace(key), "worktree") {
		return "", false, nil
	}
	parsedWorktree, err := parseGitConfigValue(value)
	if err != nil {
		return "", false, fmt.Errorf("parse core.worktree: %w", err)
	}
	return parsedWorktree, true, nil
}

func parseGitConfigSection(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") {
		return "", false
	}
	inQuote := false
	escaped := false
	for index := 1; index < len(line); index++ {
		character := line[index]
		if escaped {
			escaped = false
			continue
		}
		if character == '\\' && inQuote {
			escaped = true
			continue
		}
		if character == '"' {
			inQuote = !inQuote
			continue
		}
		if character != ']' || inQuote {
			continue
		}
		remainder := strings.TrimSpace(line[index+1:])
		if remainder != "" && remainder[0] != '#' && remainder[0] != ';' {
			return "", false
		}
		return strings.TrimSpace(line[1:index]), true
	}
	return "", false
}

func parseGitConfigBoolean(value string) (bool, error) {
	parsed, err := parseGitConfigValue(value)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(parsed) {
	case "true", "yes", "on", "1":
		return true, nil
	case "false", "no", "off", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value %q", parsed)
	}
}

func parseGitConfigValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	var parsed strings.Builder
	for len(value) > 0 {
		if value[0] == '"' {
			end, err := gitConfigQuotedValueEnd(value)
			if err != nil {
				return "", err
			}
			unquoted, err := strconv.Unquote(value[:end+1])
			if err != nil {
				return "", err
			}
			parsed.WriteString(unquoted)
			value = value[end+1:]
			continue
		}
		quoteIndex := strings.IndexByte(value, '"')
		commentIndex := gitConfigCommentStart(value)
		if commentIndex >= 0 && (quoteIndex < 0 || commentIndex < quoteIndex) {
			parsed.WriteString(value[:commentIndex])
			return strings.TrimSpace(parsed.String()), nil
		}
		if quoteIndex >= 0 {
			parsed.WriteString(value[:quoteIndex])
			value = value[quoteIndex:]
			continue
		}
		parsed.WriteString(value)
		break
	}
	return strings.TrimSpace(parsed.String()), nil
}

func gitConfigCommentStart(value string) int {
	for index, character := range value {
		if character == '#' || character == ';' {
			return index
		}
	}
	return -1
}

func gitConfigQuotedValueEnd(value string) (int, error) {
	escaped := false
	for index := 1; index < len(value); index++ {
		if escaped {
			escaped = false
			continue
		}
		if value[index] == '\\' {
			escaped = true
			continue
		}
		if value[index] == '"' {
			return index, nil
		}
	}
	return 0, errors.New("unterminated quoted Git config value")
}

func sameCanonicalPath(left, right string) bool {
	rel, err := filepath.Rel(left, right)
	return err == nil && rel == "."
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
		CurrentBranch: readExplicitGitCurrentBranch(resolved),
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
	branches, err := s.remoteBranchLister.ListRepoBranches(ctx, RemoteBranchSource{
		WorkspaceID:          repo.WorkspaceID,
		Provider:             repo.Provider,
		ProviderHost:         repo.ProviderHost,
		ProviderScope:        repo.ProviderScope,
		ProviderRepositoryID: repo.ProviderRepoID,
		Owner:                repo.ProviderOwner,
		Name:                 repo.ProviderName,
		RemoteURL:            repo.RemoteURL,
		DefaultBranch:        repo.DefaultBranch,
	})
	return branches, true, err
}

// resolveBranchListingPath turns the request inputs into a validated,
// canonical path to list branches in. A repository ID resolves the durable
// path grant stored on the repository; a raw path is an explicit read-only
// pre-registration probe. Discovery roots do not authorize either case.
func (s *Service) resolveBranchListingPath(ctx context.Context, repoID, path string) (string, error) {
	switch {
	case repoID != "":
		return s.resolveRepositoryLocalPath(ctx, repoID)
	case path == "":
		return "", fmt.Errorf("repository_id or path is required")
	}
	resolved, _, err := resolveExplicitLocalRepositoryPath(path)
	return resolved, err
}

func (s *Service) resolveRepositoryLocalPath(ctx context.Context, repoID string) (string, error) {
	if repoID == "" {
		return "", errors.New("repository ID is required")
	}
	repo, err := s.repoEntities.GetRepository(ctx, repoID)
	if err != nil {
		return "", err
	}
	if repo.LocalPath == "" {
		return "", errors.New("repository local path is empty")
	}
	resolved, _, err := resolveExplicitLocalRepositoryPath(repo.LocalPath)
	if err != nil {
		return "", err
	}
	if !sameCanonicalPath(filepath.Clean(repo.LocalPath), resolved) {
		return "", fmt.Errorf("%w: saved repository path resolves to a different location", ErrInvalidRepositoryPath)
	}
	return resolved, nil
}

// ResolveRepositoryLocalPath is the exported form of
// resolveRepositoryLocalPath. It is the narrow, read-only,
// single-method checkout-resolution port docs/specs/task-delivery-ledger/
// spec.md, "Which checkout" describes: repository ID in, a validated
// absolute checkout path or error out, with no sentinel empty path on
// rejection. internal/delivery's ancestry check depends on this method
// through a local interface rather than importing this package, so it
// inherits the existing canonicalization and containment checks instead
// of duplicating them.
func (s *Service) ResolveRepositoryLocalPath(ctx context.Context, repoID string) (string, error) {
	return s.resolveRepositoryLocalPath(ctx, repoID)
}

// RepositoryCurrentBranch returns the current branch for a saved repository,
// resolving its exact path grant by repository ID.
func (s *Service) RepositoryCurrentBranch(ctx context.Context, repoID string) (string, error) {
	resolved, err := s.resolveRepositoryLocalPath(ctx, repoID)
	if err != nil {
		return "", err
	}
	return readExplicitGitCurrentBranch(resolved), nil
}

// LocalRepositoryCurrentBranch returns the currently checked-out branch for a
// local repository on disk. Returns the branch name (e.g. "main") or an empty
// string if HEAD is detached or unreadable.
func (s *Service) LocalRepositoryCurrentBranch(ctx context.Context, path string) (string, error) {
	absPath, _, err := resolveExplicitLocalRepositoryPath(path)
	if err != nil {
		return "", err
	}
	return readExplicitGitCurrentBranch(absPath), nil
}

// LocalRepositoryStatus returns the current branch and dirty file list for a
// local repository on disk. Used by the task-create dialog to preflight the
// fresh-branch flow before committing to a destructive checkout.
func (s *Service) LocalRepositoryStatus(ctx context.Context, path string) (LocalRepoStatus, error) {
	absPath, _, err := resolveExplicitLocalRepositoryPath(path)
	if err != nil {
		return LocalRepoStatus{}, err
	}
	dirty, err := readGitDirtyFiles(ctx, absPath)
	if err != nil {
		return LocalRepoStatus{}, err
	}
	return LocalRepoStatus{
		CurrentBranch: readExplicitGitCurrentBranch(absPath),
		DirtyFiles:    dirty,
	}, nil
}

// readExplicitGitCurrentBranch preserves linked-worktree support: an explicit
// repository's .git file may legitimately point at metadata outside the
// worktree directory. The repository has already passed Git validation before
// this helper is called, so its resolved metadata directory is trusted for the
// narrow HEAD read.
func readExplicitGitCurrentBranch(repoPath string) string {
	gitDir, err := resolveGitDir(repoPath)
	if err != nil {
		return ""
	}
	canonicalGitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		return ""
	}
	return readGitCurrentBranch(repoPath, []string{canonicalGitDir})
}

func (s *Service) discoveryRoots() []string {
	var roots []string
	if len(s.discoveryConfig.Roots) > 0 {
		roots = append(roots, s.discoveryConfig.Roots...)
	} else if !s.discoveryConfig.DesktopRuntime {
		if home, err := os.UserHomeDir(); err == nil {
			roots = append(roots, home)
		}
	}
	if s.discoveryConfig.DesktopRuntime && s.desktopRootStore != nil {
		if saved, err := s.desktopRootStore.ListDesktopDiscoveryRoots(context.Background()); err == nil {
			for _, root := range saved {
				if root != nil {
					roots = append(roots, root.Path)
				}
			}
		}
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
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	repos := make([]LocalRepository, 0)
	home, _ := os.UserHomeDir()
	walker := &repoWalker{
		root:        root,
		maxDepth:    maxDepth,
		libraryRoot: filepath.Join(root, "Library"),
		cacheRoot:   filepath.Join(root, ".cache"),
		homeRoot:    home,
		goos:        runtime.GOOS,
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
	homeRoot    string
	goos        string
	ctx         context.Context
}

// visit is the WalkDir callback. Returns a non-nil *LocalRepository when a git repo is found.
func (w *repoWalker) visit(path string, d fs.DirEntry, err error) (*LocalRepository, error) {
	if err != nil {
		if path == w.root || fsdiagnostics.IsAccessDenied(err) {
			return nil, err
		}
		return nil, nil //nolint:nilerr // skip non-permission entries that cannot be accessed
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
	if shouldSkipMacOSHomeChild(w.root, path, w.homeRoot, w.goos) {
		return fs.SkipDir
	}
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

func shouldSkipMacOSHomeChild(root, path, home, goos string) bool {
	if goos != "darwin" || home == "" || !sameCanonicalPath(root, home) {
		return false
	}
	if !sameCanonicalPath(filepath.Dir(path), root) {
		return false
	}
	name := filepath.Base(path)
	return strings.EqualFold(name, "Desktop") ||
		strings.EqualFold(name, "Documents") ||
		strings.EqualFold(name, "Downloads")
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
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
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
	cmd := subproc.NewGitCommand(ctx, "status", "--porcelain=v1", "-z")
	cmd.Dir = repoPath
	out, err := subproc.RunGitOutputClass(ctx, subproc.GitInteractive, cmd)
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

// ParseGitRemoteIdentity returns a normalized provider origin and full project
// path. HTTP(S) origins preserve their scheme; SSH remotes use ssh://host.
func ParseGitRemoteIdentity(remoteURL string) (origin, projectPath string) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", ""
	}
	if strings.Contains(remoteURL, "@") && !strings.Contains(remoteURL, "://") {
		_, remoteURL, _ = strings.Cut(remoteURL, "@")
		host, path, ok := cutSCPHostPath(remoteURL)
		if !ok {
			return "", ""
		}
		return normalizeRemoteIdentity("ssh", host, path)
	}
	parsed, err := url.Parse(remoteURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ""
	}
	return normalizeRemoteIdentity(parsed.Scheme, parsed.Host, parsed.Path)
}

// cutSCPHostPath splits an scp-style remote's "host:path" segment (the part
// after "git@"), honoring bracketed IPv6 literals such as
// "[::1]:group/project.git" whose embedded colons would otherwise be
// mistaken for the host/path separator by a plain strings.Cut on ":".
func cutSCPHostPath(rest string) (host, path string, ok bool) {
	if strings.HasPrefix(rest, "[") {
		closeIdx := strings.Index(rest, "]")
		if closeIdx < 0 || closeIdx+1 >= len(rest) || rest[closeIdx+1] != ':' {
			return "", "", false
		}
		return rest[:closeIdx+1], rest[closeIdx+2:], true
	}
	return strings.Cut(rest, ":")
}

func normalizeRemoteIdentity(scheme, host, path string) (string, string) {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	host = strings.ToLower(strings.TrimSpace(host))
	if scheme != "http" && scheme != "https" && scheme != "ssh" {
		return "", ""
	}
	path = strings.TrimSuffix(strings.Trim(strings.TrimSpace(path), "/"), ".git")
	if host == "" || path == "" || !strings.Contains(path, "/") {
		return "", ""
	}
	return scheme + "://" + host, path
}

// ParseGitRemoteURL retains the legacy GitHub-only provider API.
func ParseGitRemoteURL(remoteURL string) (provider, owner, name string) {
	origin, projectPath := ParseGitRemoteIdentity(remoteURL)
	parsed, err := url.Parse(origin)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return "", "", ""
	}
	owner, name = splitProjectPath(projectPath)
	if owner == "" {
		return "", "", ""
	}
	return githubProviderName, owner, name
}

func splitProjectPath(projectPath string) (owner, name string) {
	lastSlash := strings.LastIndex(projectPath, "/")
	if lastSlash <= 0 || lastSlash == len(projectPath)-1 {
		return "", ""
	}
	return projectPath[:lastSlash], projectPath[lastSlash+1:]
}

// ResolveGitRemoteIdentity reads and parses a local checkout's origin.
func ResolveGitRemoteIdentity(repoPath string) (origin, owner, name string) {
	remoteURL, err := readGitRemoteOriginURL(repoPath)
	if err != nil {
		return "", "", ""
	}
	origin, projectPath := ParseGitRemoteIdentity(remoteURL)
	owner, name = splitProjectPath(projectPath)
	return origin, owner, name
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

// ResolveGitRemoteProviderIdentity includes the durable normalized origin.
func ResolveGitRemoteProviderIdentity(repoPath string) (provider, origin, owner, name string) {
	origin, owner, name = ResolveGitRemoteIdentity(repoPath)
	parsed, err := url.Parse(origin)
	if err != nil {
		return "", "", "", ""
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "github.com":
		provider = githubProviderName
		origin = githubProviderHost
	case "gitlab.com":
		provider = "gitlab"
		origin = "https://gitlab.com"
	}
	return provider, origin, owner, name
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
