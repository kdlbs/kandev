package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type RepositoryDiscoveryConfig struct {
	Roots    []string
	MaxDepth int
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

func (s *Service) ListRepositoryBranches(ctx context.Context, repoID string) ([]Branch, error) {
	repo, err := s.repoEntities.GetRepository(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if repo.LocalPath == "" {
		return nil, fmt.Errorf("repository local path is empty")
	}
	absPath, err := filepath.Abs(repo.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("invalid repository path: %w", err)
	}
	if !isPathAllowed(absPath, s.discoveryRoots()) {
		return nil, ErrPathNotAllowed
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repository path is not a directory")
	}
	return listGitBranches(absPath)
}

func (s *Service) ListLocalRepositoryBranches(ctx context.Context, path string) ([]Branch, error) {
	if path == "" {
		return nil, fmt.Errorf("repository path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid repository path: %w", err)
	}
	if !isPathAllowed(absPath, s.discoveryRoots()) {
		return nil, ErrPathNotAllowed
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repository path is not a directory")
	}
	return listGitBranches(absPath)
}

func (s *Service) discoveryRoots() []string {
	if len(s.discoveryConfig.Roots) > 0 {
		return normalizeRoots(s.discoveryConfig.Roots)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return []string{}
	}
	return []string{filepath.Clean(homeDir)}
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

func readGitDefaultBranch(repoPath string) (string, error) {
	gitDir, err := resolveGitDir(repoPath)
	if err != nil {
		return "", err
	}
	headPath := filepath.Join(gitDir, gitHEAD)
	content, err := os.ReadFile(headPath)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(string(content))
	if strings.HasPrefix(trimmed, "ref: ") {
		ref := strings.TrimPrefix(trimmed, "ref: ")
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}
	if trimmed != "" {
		return gitHEAD, nil
	}
	return "", fmt.Errorf("unable to determine branch")
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
