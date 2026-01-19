package service

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
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
	if outside.Allowed {
		t.Fatalf("expected outside path to be disallowed")
	}
	if outside.Message != "Path is outside the allowed roots" {
		t.Fatalf("expected outside message, got %q", outside.Message)
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

func TestReadGitDefaultBranch(t *testing.T) {
	repoPath := t.TempDir()
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git: %v", err)
	}

	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/develop\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	branch, err := readGitDefaultBranch(repoPath)
	if err != nil {
		t.Fatalf("readGitDefaultBranch error: %v", err)
	}
	if branch != "develop" {
		t.Fatalf("expected develop, got %q", branch)
	}

	if err := os.WriteFile(headPath, []byte("3a3f2d3b\n"), 0o644); err != nil {
		t.Fatalf("write HEAD detached: %v", err)
	}
	branch, err = readGitDefaultBranch(repoPath)
	if err != nil {
		t.Fatalf("readGitDefaultBranch detached error: %v", err)
	}
	if branch != "HEAD" {
		t.Fatalf("expected HEAD, got %q", branch)
	}

	if err := os.WriteFile(headPath, []byte("\n"), 0o644); err != nil {
		t.Fatalf("write HEAD empty: %v", err)
	}
	if _, err := readGitDefaultBranch(repoPath); err == nil {
		t.Fatalf("expected error for empty HEAD")
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
	relPath := filepath.Join("gitdir")
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

func newDiscoveryService(t *testing.T, root string) *Service {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	repoImpl, cleanup, err := repository.Provide(dbConn)
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	repo := repository.Repository(repoImpl)
	t.Cleanup(func() {
		if err := dbConn.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if err := cleanup(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	})
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	eventBus := bus.NewMemoryEventBus(log)
	return NewService(repo, eventBus, log, RepositoryDiscoveryConfig{
		Roots:    []string{root},
		MaxDepth: 6,
	})
}
