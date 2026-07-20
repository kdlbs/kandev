package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func TestDiscoverLocalRepositoriesSkipsIgnoredRoots(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, filepath.Join(root, "ProjectA"))
	makeRepo(t, filepath.Join(root, "ProjectB"))
	makeRepo(t, filepath.Join(root, "Library", "Caches", "mise", "python", "pyenv", "ProjectC"))
	makeRepo(t, filepath.Join(root, ".cache", "ProjectD"))
	makeRepo(t, filepath.Join(root, "node_modules", "ProjectE"))
	makeRepo(t, filepath.Join(root, "ProjectA", "node_modules", "ProjectF"))

	svc := newDiscoveryService(t, root)
	result, err := svc.DiscoverLocalRepositories(context.Background(), "")
	if err != nil {
		t.Fatalf("DiscoverLocalRepositories error: %v", err)
	}

	paths := make([]string, 0, len(result.Repositories))
	for _, repo := range result.Repositories {
		paths = append(paths, repo.Path)
	}
	sort.Strings(paths)

	expected := []string{
		filepath.Join(root, "ProjectA"),
		filepath.Join(root, "ProjectB"),
	}
	sort.Strings(expected)

	if len(paths) != len(expected) {
		t.Fatalf("expected %d repos, got %d: %#v", len(expected), len(paths), paths)
	}
	for i, path := range paths {
		if path != expected[i] {
			t.Fatalf("expected repo %q, got %q", expected[i], path)
		}
	}
}

func TestValidateLocalRepositoryPath(t *testing.T) {
	root := t.TempDir()
	svc := newDiscoveryService(t, root)

	otherRoot := t.TempDir()
	makeRepo(t, otherRoot)
	outside, err := svc.ValidateLocalRepositoryPath(context.Background(), otherRoot)
	if err != nil {
		t.Fatalf("ValidateLocalRepositoryPath outside error: %v", err)
	}
	if !outside.Allowed || !outside.Exists || !outside.IsGitRepo {
		t.Fatalf("expected explicit outside repository to validate, got %+v", outside)
	}
	if outside.Message != "" {
		t.Fatalf("expected no outside-root validation message, got %q", outside.Message)
	}

	discovered, err := svc.DiscoverLocalRepositories(context.Background(), "")
	if err != nil {
		t.Fatalf("DiscoverLocalRepositories error: %v", err)
	}
	for _, repo := range discovered.Repositories {
		if repo.Path == outside.Path {
			t.Fatalf("explicit validation widened automatic discovery to %q", repo.Path)
		}
	}

	missingPath := filepath.Join(root, "missing")
	missing, err := svc.ValidateLocalRepositoryPath(context.Background(), missingPath)
	if err != nil {
		t.Fatalf("ValidateLocalRepositoryPath missing error: %v", err)
	}
	if !missing.Allowed || missing.Exists {
		t.Fatalf("expected missing path to be allowed and not exist")
	}
	if missing.Message != "Path does not exist" {
		t.Fatalf("expected missing message, got %q", missing.Message)
	}

	filePath := filepath.Join(root, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	fileResult, err := svc.ValidateLocalRepositoryPath(context.Background(), filePath)
	if err != nil {
		t.Fatalf("ValidateLocalRepositoryPath file error: %v", err)
	}
	if !fileResult.Allowed || !fileResult.Exists {
		t.Fatalf("expected file path to be allowed and exist")
	}
	if fileResult.Message != "Path is not a directory" {
		t.Fatalf("expected file message, got %q", fileResult.Message)
	}

	plainDir := filepath.Join(root, "plain")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatalf("mkdir plain dir: %v", err)
	}
	plainResult, err := svc.ValidateLocalRepositoryPath(context.Background(), plainDir)
	if err != nil {
		t.Fatalf("ValidateLocalRepositoryPath plain error: %v", err)
	}
	if plainResult.Message != "Not a git repository" {
		t.Fatalf("expected plain message, got %q", plainResult.Message)
	}

	repoPath := filepath.Join(root, "repo")
	makeRepo(t, repoPath)
	repoResult, err := svc.ValidateLocalRepositoryPath(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("ValidateLocalRepositoryPath repo error: %v", err)
	}
	if !repoResult.IsGitRepo || repoResult.DefaultBranch != "main" || repoResult.Message != "" {
		t.Fatalf("expected git repo with main branch, got %+v", repoResult)
	}
}

func TestNormalizeRootsDedupesAndCleans(t *testing.T) {
	root := t.TempDir()
	roots := []string{root, root + string(os.PathSeparator), "", root}
	normalized := normalizeRoots(roots)
	if len(normalized) != 1 {
		t.Fatalf("expected 1 normalized root, got %d: %#v", len(normalized), normalized)
	}
	if normalized[0] != filepath.Clean(root) {
		t.Fatalf("expected normalized root %q, got %q", filepath.Clean(root), normalized[0])
	}
}

// seedBareGitDir creates a minimal .git directory at repoPath with HEAD set
// to the given branch (or commit SHA for detached). Returns the gitDir path.
func seedBareGitDir(t *testing.T, repoPath, headContent string) string {
	t.Helper()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headContent), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	return gitDir
}

// TestReadGitDefaultBranch_UsesOriginHEADWhenMainRefAbsent — origin/HEAD still
// beats the checked-out branch when no main ref exists. Regression for the bug
// where the repo's stored default_branch latched onto whatever feature branch
// the user happened to be on at task-creation time.
func TestReadGitDefaultBranch_UsesOriginHEADWhenMainRefAbsent(t *testing.T) {
	repoPath := t.TempDir()
	gitDir := seedBareGitDir(t, repoPath, "ref: refs/heads/feature/x\n")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", "origin"), 0o755); err != nil {
		t.Fatalf("mkdir origin: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"),
		[]byte("ref: refs/remotes/origin/main\n"),
		0o644,
	); err != nil {
		t.Fatalf("write origin/HEAD: %v", err)
	}
	branch, err := readGitDefaultBranch(repoPath)
	if err != nil {
		t.Fatalf("readGitDefaultBranch error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected main, got %q", branch)
	}
}

// TestReadGitDefaultBranch_FallsBackToRemoteCandidates — when origin/HEAD
// isn't set (older clones, never run `git remote set-head`), the conventional
// origin/main / origin/master refs win over local branches.
func TestReadGitDefaultBranch_FallsBackToRemoteCandidates(t *testing.T) {
	repoPath := t.TempDir()
	gitDir := seedBareGitDir(t, repoPath, "ref: refs/heads/feature/y\n")
	writeRef(t, filepath.Join(gitDir, "refs", "remotes", "origin", "main"))
	branch, err := readGitDefaultBranch(repoPath)
	if err != nil {
		t.Fatalf("readGitDefaultBranch error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected main, got %q", branch)
	}
}

// TestReadGitDefaultBranch_FallsBackToLocalCandidates — local-only repos
// (no remotes) still produce the conventional integration branch when one
// exists locally, even if the user is checked out on a feature branch.
func TestReadGitDefaultBranch_FallsBackToLocalCandidates(t *testing.T) {
	repoPath := t.TempDir()
	gitDir := seedBareGitDir(t, repoPath, "ref: refs/heads/feature/z\n")
	writeRef(t, filepath.Join(gitDir, "refs", "heads", "main"))
	branch, err := readGitDefaultBranch(repoPath)
	if err != nil {
		t.Fatalf("readGitDefaultBranch error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected main, got %q", branch)
	}
}

// TestReadGitDefaultBranch_FallsBackToHEAD — a brand-new repo with only
// a feature branch checked out and no main/master falls back to HEAD so
// callers still get *some* answer instead of an error.
func TestReadGitDefaultBranch_FallsBackToHEAD(t *testing.T) {
	repoPath := t.TempDir()
	seedBareGitDir(t, repoPath, "ref: refs/heads/develop\n")
	branch, err := readGitDefaultBranch(repoPath)
	if err != nil {
		t.Fatalf("readGitDefaultBranch error: %v", err)
	}
	if branch != "develop" {
		t.Fatalf("expected develop, got %q", branch)
	}
}

// TestReadGitDefaultBranch_FallsBackToHEAD_PreservesSlashes — branch names
// can contain slashes (e.g. "feature/my-feature"). The HEAD fallback must
// strip the refs/heads/ prefix verbatim rather than split on "/" and take
// the tail, otherwise nested branches get silently corrupted into their
// last path component, which then becomes the stored default_branch and
// poisons every downstream merge-base lookup.
func TestReadGitDefaultBranch_FallsBackToHEAD_PreservesSlashes(t *testing.T) {
	repoPath := t.TempDir()
	seedBareGitDir(t, repoPath, "ref: refs/heads/feature/my-feature\n")
	branch, err := readGitDefaultBranch(repoPath)
	if err != nil {
		t.Fatalf("readGitDefaultBranch error: %v", err)
	}
	if branch != "feature/my-feature" {
		t.Fatalf("expected feature/my-feature, got %q", branch)
	}
}

// TestReadGitDefaultBranch_DetachedHEADReturnsHEADLiteral — detached HEAD on
// a repo with no integration branches yields the literal "HEAD" sentinel
// (preserved from the previous behavior so callers depending on it don't
// silently start failing).
func TestReadGitDefaultBranch_DetachedHEADReturnsHEADLiteral(t *testing.T) {
	repoPath := t.TempDir()
	seedBareGitDir(t, repoPath, "3a3f2d3b\n")
	branch, err := readGitDefaultBranch(repoPath)
	if err != nil {
		t.Fatalf("readGitDefaultBranch error: %v", err)
	}
	if branch != "HEAD" {
		t.Fatalf("expected HEAD, got %q", branch)
	}
}

// TestReadGitDefaultBranch_ErrorsOnEmptyHEAD — preserves the existing
// contract: HEAD with no content is an unparseable repo.
func TestReadGitDefaultBranch_ErrorsOnEmptyHEAD(t *testing.T) {
	repoPath := t.TempDir()
	seedBareGitDir(t, repoPath, "\n")
	if _, err := readGitDefaultBranch(repoPath); err == nil {
		t.Fatalf("expected error for empty HEAD")
	}
}

func TestReadGitCurrentBranchDetachedHead(t *testing.T) {
	repoPath := canonicalTempDir(t)
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	headPath := filepath.Join(gitDir, "HEAD")
	roots := []string{filepath.Dir(repoPath)}

	// Branch HEAD: returns the branch name.
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if got := readGitCurrentBranch(repoPath, roots); got != "main" {
		t.Fatalf("ref HEAD: got %q, want %q", got, "main")
	}

	// Detached HEAD with a 40-char SHA returns the short (7-char) form. This
	// is the regression: pre-fix it returned "" and the task-create dialog's
	// locked branch chip fell back to the "branch" placeholder.
	const sha = "a7f5558af69cd3cc2813536b775687b9bfaf65db"
	if err := os.WriteFile(headPath, []byte(sha+"\n"), 0o644); err != nil {
		t.Fatalf("write detached HEAD: %v", err)
	}
	if got := readGitCurrentBranch(repoPath, roots); got != "a7f5558" {
		t.Fatalf("detached HEAD: got %q, want %q", got, "a7f5558")
	}

	// Garbled HEAD content (not a valid SHA): return empty rather than
	// echo back arbitrary bytes.
	if err := os.WriteFile(headPath, []byte("not-a-sha\n"), 0o644); err != nil {
		t.Fatalf("write garbled HEAD: %v", err)
	}
	if got := readGitCurrentBranch(repoPath, roots); got != "" {
		t.Fatalf("garbled HEAD: got %q, want empty", got)
	}
}

func TestResolveGitDir(t *testing.T) {
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	resolved, err := resolveGitDir(repoPath)
	if err != nil {
		t.Fatalf("resolveGitDir dir error: %v", err)
	}
	if resolved != gitDir {
		t.Fatalf("expected git dir %q, got %q", gitDir, resolved)
	}

	altRepo := t.TempDir()
	altGit := filepath.Join(altRepo, "gitdir")
	if err := os.MkdirAll(altGit, 0o755); err != nil {
		t.Fatalf("mkdir alt git: %v", err)
	}
	gitRef := filepath.Join(altRepo, ".git")
	relPath := "gitdir"
	if err := os.WriteFile(gitRef, []byte("gitdir: "+relPath+"\n"), 0o644); err != nil {
		t.Fatalf("write gitdir ref: %v", err)
	}
	resolved, err = resolveGitDir(altRepo)
	if err != nil {
		t.Fatalf("resolveGitDir file error: %v", err)
	}
	expected := filepath.Clean(filepath.Join(altRepo, relPath))
	if resolved != expected {
		t.Fatalf("expected git dir %q, got %q", expected, resolved)
	}
}

// canonicalTempDir returns a t.TempDir() path with symlinks resolved. macOS
// returns `/var/folders/...` from t.TempDir() while EvalSymlinks (used inside
// resolveGitDirWithin) yields `/private/var/folders/...`; without
// canonicalization the test's expected paths and the function's output live in
// different namespaces and equality checks fail. On Linux this is a no-op.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("eval temp dir: %v", err)
	}
	return resolved
}

// TestResolveGitDirWithin_RejectsEscapedGitdir exercises the security-critical
// branch of resolveGitDirWithin: a repo whose `.git` is a file pointer
// (worktree style) whose embedded `gitdir:` points outside any allowed root.
// The function must return ErrPathNotAllowed; if the bounds check is ever
// removed, callers like readGitCurrentBranch would read arbitrary files on
// disk.
func TestResolveGitDirWithin_RejectsEscapedGitdir(t *testing.T) {
	root := canonicalTempDir(t)
	outside := canonicalTempDir(t)

	repoPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	gitFile := filepath.Join(repoPath, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+outside+"\n"), 0o644); err != nil {
		t.Fatalf("write .git pointer: %v", err)
	}

	if _, err := resolveGitDirWithin(repoPath, []string{root}); !errors.Is(err, ErrPathNotAllowed) {
		t.Fatalf("expected ErrPathNotAllowed for gitdir outside roots, got %v", err)
	}
}

// TestResolveGitDirWithin_RejectsSymlinkedGitDir covers the case where
// `.git` itself is a symlink whose target lives outside the allowed roots.
// The lexical path looks fine — it's `<allowed>/repo/.git` — but following
// the symlink would read from elsewhere. EvalSymlinks must catch that.
func TestResolveGitDirWithin_RejectsSymlinkedGitDir(t *testing.T) {
	root := canonicalTempDir(t)
	outside := canonicalTempDir(t)
	repoPath := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	// `.git` symlinks to an out-of-root directory containing a HEAD file.
	if err := os.Symlink(outside, filepath.Join(repoPath, ".git")); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	if _, err := resolveGitDirWithin(repoPath, []string{root}); !errors.Is(err, ErrPathNotAllowed) {
		t.Fatalf("expected ErrPathNotAllowed for symlinked .git escaping roots, got %v", err)
	}
}

// TestResolveGitDirWithin_AllowsRepoLocalGitFile covers the legitimate
// worktree case: `.git` file points to a sibling path that's still inside the
// repo (e.g. main repo's `.git/worktrees/<name>`).
func TestResolveGitDirWithin_AllowsRepoLocalGitFile(t *testing.T) {
	repoPath := canonicalTempDir(t)
	worktreesDir := filepath.Join(repoPath, ".git", "worktrees", "wt")
	if err := os.MkdirAll(worktreesDir, 0o755); err != nil {
		t.Fatalf("mkdir worktrees: %v", err)
	}
	gitFile := filepath.Join(repoPath, "wt-clone", ".git")
	if err := os.MkdirAll(filepath.Dir(gitFile), 0o755); err != nil {
		t.Fatalf("mkdir wt-clone: %v", err)
	}
	if err := os.WriteFile(gitFile, []byte("gitdir: "+worktreesDir+"\n"), 0o644); err != nil {
		t.Fatalf("write .git pointer: %v", err)
	}

	got, err := resolveGitDirWithin(filepath.Dir(gitFile), []string{repoPath})
	if err != nil {
		t.Fatalf("expected legitimate worktree to resolve, got %v", err)
	}
	if got != worktreesDir {
		t.Fatalf("expected gitdir %q, got %q", worktreesDir, got)
	}
}

func TestListGitBranches(t *testing.T) {
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}
	writeRef(t, filepath.Join(gitDir, "refs", "heads", "main"))
	writeRef(t, filepath.Join(gitDir, "refs", "heads", "feature", "test"))
	writeRef(t, filepath.Join(gitDir, "refs", "remotes", "origin", "main"))
	writeRef(t, filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD"))
	writeRef(t, filepath.Join(gitDir, "refs", "remotes", "upstream", "dev"))

	packedRefs := strings.Join([]string{
		"# pack-refs with: peeled fully-peeled",
		"deadbeef refs/heads/packed",
		"cafebabe refs/remotes/origin/packed-remote",
		"^deadbeef",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(gitDir, "packed-refs"), []byte(packedRefs), 0o644); err != nil {
		t.Fatalf("write packed-refs: %v", err)
	}

	branches, err := listGitBranches(repoPath)
	if err != nil {
		t.Fatalf("listGitBranches error: %v", err)
	}

	expected := []Branch{
		{Name: "feature/test", Type: "local"},
		{Name: "main", Type: "local"},
		{Name: "packed", Type: "local"},
		{Name: "main", Type: "remote", Remote: "origin"},
		{Name: "packed-remote", Type: "remote", Remote: "origin"},
		{Name: "dev", Type: "remote", Remote: "upstream"},
	}

	if len(branches) != len(expected) {
		t.Fatalf("expected %d branches, got %d: %#v", len(expected), len(branches), branches)
	}
	for i, branch := range branches {
		if branch != expected[i] {
			t.Fatalf("expected branch %#v, got %#v", expected[i], branch)
		}
	}
}

func makeRepo(t *testing.T, path string) {
	t.Helper()
	gitDir := filepath.Join(path, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
}

func writeRef(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir ref dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("0000000\n"), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}
}

func TestParseGitRemoteURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantProvider string
		wantOwner    string
		wantName     string
	}{
		{"https with .git", "https://github.com/owner/repo.git", "github", "owner", "repo"},
		{"https without .git", "https://github.com/owner/repo", "github", "owner", "repo"},
		{"http", "http://github.com/owner/repo.git", "github", "owner", "repo"},
		{"ssh colon", "git@github.com:owner/repo.git", "github", "owner", "repo"},
		{"ssh colon no .git", "git@github.com:owner/repo", "github", "owner", "repo"},
		{"ssh protocol", "ssh://git@github.com/owner/repo.git", "github", "owner", "repo"},
		{"ssh protocol no .git", "ssh://git@github.com/owner/repo", "github", "owner", "repo"},
		{"trailing slash", "https://github.com/owner/repo/", "github", "owner", "repo"},
		{"empty", "", "", "", ""},
		{"not github", "https://gitlab.com/owner/repo.git", "", "", ""},
		{"no path", "https://github.com", "", "", ""},
		{"no repo", "https://github.com/owner", "", "", ""},
		{"malformed", "not-a-url", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, owner, name := ParseGitRemoteURL(tt.url)
			if provider != tt.wantProvider {
				t.Errorf("provider = %q, want %q", provider, tt.wantProvider)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestParseGitConfigOriginURL(t *testing.T) {
	config := `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = https://github.com/owner/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
`
	got := parseGitConfigOriginURL(config)
	if got != "https://github.com/owner/repo.git" {
		t.Errorf("parseGitConfigOriginURL = %q, want %q", got, "https://github.com/owner/repo.git")
	}
}

func TestParseGitConfigOriginURL_NoOrigin(t *testing.T) {
	config := `[core]
	repositoryformatversion = 0
[remote "upstream"]
	url = https://github.com/other/repo.git
`
	got := parseGitConfigOriginURL(config)
	if got != "" {
		t.Errorf("parseGitConfigOriginURL = %q, want empty", got)
	}
}

func TestResolveGitRemoteProvider(t *testing.T) {
	// Create a temp git repo with an origin remote
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write HEAD
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write config with origin remote
	config := `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = git@github.com:myorg/myrepo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	provider, owner, name := ResolveGitRemoteProvider(dir)
	if provider != "github" {
		t.Errorf("provider = %q, want %q", provider, "github")
	}
	if owner != "myorg" {
		t.Errorf("owner = %q, want %q", owner, "myorg")
	}
	if name != "myrepo" {
		t.Errorf("name = %q, want %q", name, "myrepo")
	}
}

func TestResolveGitRemoteProvider_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	provider, owner, name := ResolveGitRemoteProvider(dir)
	if provider != "" || owner != "" || name != "" {
		t.Errorf("expected empty for non-git dir, got %q %q %q", provider, owner, name)
	}
}

func newDiscoveryService(t *testing.T, root string) *Service {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repoImpl, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	repo := repoImpl
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if err := cleanup(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	})
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	eventBus := bus.NewMemoryEventBus(log)
	return NewService(Repos{
		Workspaces:   repo,
		Tasks:        repo,
		TaskRepos:    repo,
		Workflows:    repo,
		Messages:     repo,
		Turns:        repo,
		Sessions:     repo,
		GitSnapshots: repo,
		RepoEntities: repo,
		Executors:    repo,
		Environments: repo,
		Reviews:      repo,
	}, eventBus, log, RepositoryDiscoveryConfig{
		Roots:    []string{root},
		MaxDepth: 6,
	})
}

// --- listRemoteBranchesIfApplicable + discoveryRoots routing ---

// stubRemoteLister captures the call args and returns canned branches/err.
type stubRemoteLister struct {
	branches []Branch
	err      error
	calls    int
}

func (s *stubRemoteLister) ListRepoBranches(_ context.Context, _, _ string) ([]Branch, error) {
	s.calls++
	return s.branches, s.err
}

// stubRepoErrors embeds a real RepositoryEntityRepository and overrides
// GetRepository to return a forced error - lets us cover the DB-error
// branch without standing up a broken sqlite.
type stubRepoErrors struct {
	repository.RepositoryEntityRepository
	getErr error
}

func (s *stubRepoErrors) GetRepository(_ context.Context, _ string) (*models.Repository, error) {
	return nil, s.getErr
}

func TestListBranches_RoutesProviderRepoToRemoteLister(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "ws"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	mustCreateRepo(t, repo, &models.Repository{
		ID:            "remote-1",
		WorkspaceID:   "ws-1",
		Name:          "owner/repo",
		SourceType:    "provider",
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
	})
	lister := &stubRemoteLister{branches: []Branch{{Name: "main", Type: "remote"}, {Name: "develop", Type: "remote"}}}
	svc.SetRemoteBranchLister(lister)

	got, err := svc.ListBranches(ctx, "remote-1", "")
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if lister.calls != 1 {
		t.Fatalf("remote lister calls = %d, want 1", lister.calls)
	}
	if len(got) != 2 || got[0].Name != "main" || got[1].Name != "develop" {
		t.Fatalf("unexpected branches: %+v", got)
	}
}

func TestListBranches_SavedRepositoryOutsideDiscoveryRoots(t *testing.T) {
	isolateGitEnvForTest(t)
	discoveryRoot := t.TempDir()
	repoPath := filepath.Join(t.TempDir(), "explicit-repo")
	initRealGitRepo(t, repoPath)

	svc := newDiscoveryService(t, discoveryRoot)
	ctx := context.Background()
	if err := svc.workspaces.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := svc.repoEntities.CreateRepository(ctx, &models.Repository{
		ID: "outside-repo", WorkspaceID: "ws-1", Name: "outside", SourceType: sourceTypeLocal, LocalPath: repoPath,
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	result, err := svc.ListBranchesWithCurrent(ctx, "outside-repo", "")
	if err != nil {
		t.Fatalf("ListBranchesWithCurrent by repository ID: %v", err)
	}
	if len(result.Branches) != 1 || result.Branches[0].Name != "main" {
		t.Fatalf("branches = %+v, want main", result.Branches)
	}
	if result.CurrentBranch != "main" {
		t.Fatalf("CurrentBranch = %q, want main", result.CurrentBranch)
	}
}

func TestSavedRepositoryRejectsRetargetedCanonicalPath(t *testing.T) {
	isolateGitEnvForTest(t)
	originalPath := filepath.Join(t.TempDir(), "saved-repo")
	initRealGitRepo(t, originalPath)

	svc := newDiscoveryService(t, t.TempDir())
	ctx := context.Background()
	if err := svc.workspaces.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1", Name: "saved", SourceType: sourceTypeLocal, LocalPath: originalPath,
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	movedPath := originalPath + "-moved"
	if err := os.Rename(originalPath, movedPath); err != nil {
		t.Fatalf("Rename saved repository: %v", err)
	}
	victimPath := filepath.Join(t.TempDir(), "victim-repo")
	initRealGitRepo(t, victimPath)
	if err := os.Symlink(victimPath, originalPath); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	t.Run("identity-bound read", func(t *testing.T) {
		if _, err := svc.ListBranchesWithCurrent(ctx, created.ID, ""); err == nil {
			t.Fatal("expected saved repository read to reject a retargeted canonical path")
		}
	})
	t.Run("identity-bound mutation", func(t *testing.T) {
		err := svc.PerformFreshBranch(ctx, FreshBranchRequest{
			RepositoryID: created.ID,
			BaseBranch:   "main",
			NewBranch:    "feature/retargeted",
		})
		if err == nil {
			t.Fatal("expected fresh branch to reject a retargeted canonical path")
		}
		cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/feature/retargeted")
		cmd.Dir = victimPath
		cmd.Env = isolatedGitEnv()
		if err := cmd.Run(); err == nil {
			t.Fatal("fresh branch mutated the retargeted repository")
		}
	})
}

func TestListBranches_RawExplicitPathOutsideDiscoveryRoots(t *testing.T) {
	isolateGitEnvForTest(t)
	discoveryRoot := t.TempDir()
	repoPath := filepath.Join(t.TempDir(), "explicit-repo")
	initRealGitRepo(t, repoPath)

	svc := newDiscoveryService(t, discoveryRoot)
	branches, err := svc.ListBranches(context.Background(), "", repoPath)
	if err != nil {
		t.Fatalf("ListBranches by explicit path: %v", err)
	}
	if len(branches) != 1 || branches[0].Name != "main" {
		t.Fatalf("branches = %+v, want main", branches)
	}
}

func TestListBranches_SavedLinkedWorktreeOutsideDiscoveryRoots(t *testing.T) {
	isolateGitEnvForTest(t)
	primaryPath := filepath.Join(t.TempDir(), "primary")
	linkedPath := filepath.Join(t.TempDir(), "linked")
	initRealGitRepo(t, primaryPath)
	cmd := exec.Command("git", "worktree", "add", "-b", "linked", linkedPath, "main")
	cmd.Dir = primaryPath
	cmd.Env = isolatedGitEnv()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v: %s", err, output)
	}

	svc := newDiscoveryService(t, t.TempDir())
	ctx := context.Background()
	if err := svc.workspaces.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := svc.repoEntities.CreateRepository(ctx, &models.Repository{
		ID: "linked-repo", WorkspaceID: "ws-1", Name: "linked", SourceType: sourceTypeLocal, LocalPath: linkedPath,
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	result, err := svc.ListBranchesWithCurrent(ctx, "linked-repo", "")
	if err != nil {
		t.Fatalf("ListBranchesWithCurrent: %v", err)
	}
	if result.CurrentBranch != "linked" {
		t.Fatalf("CurrentBranch = %q, want linked", result.CurrentBranch)
	}
}

func TestValidateLinkedWorktreeMetadataRejectsSelfReferentialCommonDir(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "linked")
	gitDir := filepath.Join(t.TempDir(), "metadata")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll metadata: %v", err)
	}
	gitPath := filepath.Join(repoPath, ".git")
	if err := os.WriteFile(gitPath, []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "gitdir"), []byte(gitPath+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile gitdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte(".\n"), 0o600); err != nil {
		t.Fatalf("WriteFile commondir: %v", err)
	}

	if err := validateLinkedWorktreeMetadata(repoPath, gitPath); err == nil {
		t.Fatal("expected self-referential common directory to be rejected")
	}
}

func TestListBranches_FallsThroughWhenNoRemoteListerWired(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "ws"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	mustCreateRepo(t, repo, &models.Repository{
		ID: "remote-2", WorkspaceID: "ws-1", Name: "o/r",
		SourceType: "provider", Provider: "github", ProviderOwner: "o", ProviderName: "r",
	})
	// No SetRemoteBranchLister call. listRemoteBranchesIfApplicable should
	// return ok=false and the call falls through to the local-path arm,
	// which errors out on empty local_path.
	_, err := svc.ListBranches(ctx, "remote-2", "")
	if err == nil {
		t.Fatal("expected local-path error when remote lister unwired")
	}
}

func TestListBranches_FallsThroughForLocalSourceType(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "ws"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	mustCreateRepo(t, repo, &models.Repository{
		ID: "local-1", WorkspaceID: "ws-1", Name: "local",
		SourceType: sourceTypeLocal, Provider: "github", ProviderOwner: "o", ProviderName: "r",
	})
	lister := &stubRemoteLister{branches: []Branch{{Name: "main"}}}
	svc.SetRemoteBranchLister(lister)

	// Local source-type repos must use the local-path arm even with provider
	// info populated. local_path is empty here, so we expect an error - what
	// matters for the test is that the remote lister is never consulted.
	_, _ = svc.ListBranches(ctx, "local-1", "")
	if lister.calls != 0 {
		t.Fatalf("remote lister called for source_type=local: calls = %d", lister.calls)
	}
}

func TestListBranches_FallsThroughOnMissingProviderInfo(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "ws"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	mustCreateRepo(t, repo, &models.Repository{
		ID: "remote-3", WorkspaceID: "ws-1", Name: "no-owner",
		SourceType: "provider", Provider: "github", ProviderOwner: "", ProviderName: "repo",
	})
	lister := &stubRemoteLister{branches: []Branch{{Name: "main"}}}
	svc.SetRemoteBranchLister(lister)

	_, _ = svc.ListBranches(ctx, "remote-3", "")
	if lister.calls != 0 {
		t.Fatalf("remote lister called for repo with no owner: calls = %d", lister.calls)
	}
}

func TestListBranches_PropagatesRemoteListerError(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "ws"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	mustCreateRepo(t, repo, &models.Repository{
		ID: "remote-4", WorkspaceID: "ws-1", Name: "o/r",
		SourceType: "provider", Provider: "github", ProviderOwner: "o", ProviderName: "r",
	})
	wantErr := errors.New("rate limited")
	svc.SetRemoteBranchLister(&stubRemoteLister{err: wantErr})

	if _, err := svc.ListBranches(ctx, "remote-4", ""); !errors.Is(err, wantErr) {
		t.Fatalf("ListBranches err = %v, want %v", err, wantErr)
	}
}

func TestListBranches_PropagatesGetRepositoryError(t *testing.T) {
	svc, _, repo := createTestService(t)
	wrapped := &stubRepoErrors{RepositoryEntityRepository: repo, getErr: errors.New("db down")}
	// Swap repoEntities for the wrapper to force the error path. Direct field
	// access stays inside the package, which is fine for a white-box test.
	svc.repoEntities = wrapped
	svc.SetRemoteBranchLister(&stubRemoteLister{})

	_, err := svc.ListBranches(context.Background(), "repo-id", "")
	if err == nil || err.Error() != "db down" {
		t.Fatalf("ListBranches err = %v, want db down", err)
	}
}

// --- discoveryRoots extension ---

type stubCloneLocation struct{ path string }

func (s stubCloneLocation) ExpandedBasePath() (string, error) { return s.path, nil }

func TestDiscoveryRoots_IncludesCloneBasePath(t *testing.T) {
	svc, _, _ := createTestService(t)
	cloneDir := t.TempDir()
	svc.SetRepoCloneLocation(stubCloneLocation{path: cloneDir})

	roots := svc.discoveryRoots()
	normalizedClone := filepath.Clean(cloneDir)
	for _, r := range roots {
		if r == normalizedClone {
			return
		}
	}
	t.Fatalf("clone base path %q missing from discoveryRoots %v", normalizedClone, roots)
}

func mustCreateRepo(t *testing.T, repo repository.RepositoryEntityRepository, r *models.Repository) {
	t.Helper()
	if err := repo.CreateRepository(context.Background(), r); err != nil {
		t.Fatalf("create repository: %v", err)
	}
}
