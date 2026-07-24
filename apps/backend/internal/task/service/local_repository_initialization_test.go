package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func TestServiceInitializeLocalRepositoryCreatesCommitlessMainRepository(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	realParent := t.TempDir()
	parentLink := filepath.Join(t.TempDir(), "projects")
	if err := os.Symlink(realParent, parentLink); err != nil {
		t.Fatalf("Symlink parent: %v", err)
	}

	created, err := svc.InitializeLocalRepository(ctx, &InitializeLocalRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "  new-project  ",
		ParentPath:  parentLink,
	})
	if err != nil {
		t.Fatalf("InitializeLocalRepository: %v", err)
	}
	wantPath := filepath.Join(realParent, "new-project")
	if created.WorkspaceID != "ws-1" || created.Name != "new-project" || created.SourceType != sourceTypeLocal ||
		created.LocalPath != wantPath || created.DefaultBranch != "main" {
		t.Fatalf("created repository = %+v, want workspace ws-1 local repository at %q on main", created, wantPath)
	}
	entries, err := os.ReadDir(wantPath)
	if err != nil {
		t.Fatalf("ReadDir target: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != ".git" || !entries[0].IsDir() {
		t.Fatalf("target entries = %+v, want only .git directory", entries)
	}
	if branch := runGitOutput(t, wantPath, "symbolic-ref", "--short", "HEAD"); branch != "main" {
		t.Fatalf("unborn branch = %q, want main", branch)
	}
	if command := exec.Command("git", "-C", wantPath, "rev-parse", "--verify", "HEAD"); command.Run() == nil {
		t.Fatal("rev-parse HEAD succeeded, want repository with no commits")
	}
	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.LocalPath != wantPath || stored.DefaultBranch != "main" {
		t.Fatalf("stored repository = %+v, want path %q on main", stored, wantPath)
	}
	eventsPublished := eventBus.GetPublishedEvents()
	if len(eventsPublished) != 1 || eventsPublished[0].Type != events.RepositoryCreated {
		t.Fatalf("published events = %+v, want one %q event", eventsPublished, events.RepositoryCreated)
	}
}

func TestServiceInitializeLocalRepositoryCreatesMissingParentDirectories(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	parent := filepath.Join(t.TempDir(), "projects", "team")

	created, err := svc.InitializeLocalRepository(ctx, &InitializeLocalRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "new-project",
		ParentPath:  parent,
	})
	if err != nil {
		t.Fatalf("InitializeLocalRepository: %v", err)
	}
	wantPath := filepath.Join(parent, "new-project")
	if created.LocalPath != wantPath {
		t.Fatalf("created path = %q, want %q", created.LocalPath, wantPath)
	}
	for _, path := range []string{parent, wantPath, filepath.Join(wantPath, ".git")} {
		if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
			t.Fatalf("directory %q: info=%v error=%v", path, info, statErr)
		}
	}
}

func TestServiceInitializeLocalRepositoryRejectsInvalidInputWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		parent     func(t *testing.T) string
		target     func(parent string) string
	}{
		{name: "empty name", repository: "   ", parent: func(t *testing.T) string { return t.TempDir() }},
		{name: "dot name", repository: ".", parent: func(t *testing.T) string { return t.TempDir() }},
		{name: "parent name", repository: "..", parent: func(t *testing.T) string { return t.TempDir() }},
		{name: "nested name", repository: "nested/name", parent: func(t *testing.T) string { return t.TempDir() }},
		{name: "windows nested name", repository: `nested\name`, parent: func(t *testing.T) string { return t.TempDir() }},
		{name: "relative parent", repository: "new-project", parent: func(t *testing.T) string { return "relative" }},
		{name: "file parent", repository: "new-project", parent: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "file")
			if err := os.WriteFile(path, []byte("file"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			return path
		}},
		{name: "unwritable parent", repository: "new-project", parent: func(t *testing.T) string {
			path := t.TempDir()
			if err := os.Chmod(path, 0o555); err != nil {
				t.Fatalf("Chmod: %v", err)
			}
			return path
		}, target: func(parent string) string { return filepath.Join(parent, "new-project") }},
		{name: "shared writable parent", repository: "new-project", parent: func(t *testing.T) string {
			path := t.TempDir()
			if err := os.Chmod(path, 0o777); err != nil {
				t.Fatalf("Chmod: %v", err)
			}
			return path
		}, target: func(parent string) string { return filepath.Join(parent, "new-project") }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
				t.Fatalf("CreateWorkspace: %v", err)
			}
			parent := test.parent(t)

			_, err := svc.InitializeLocalRepository(ctx, &InitializeLocalRepositoryRequest{
				WorkspaceID: "ws-1",
				Name:        test.repository,
				ParentPath:  parent,
			})
			if !errors.Is(err, ErrInvalidLocalRepositoryInitialization) {
				t.Fatalf("error = %v, want ErrInvalidLocalRepositoryInitialization", err)
			}
			if test.target != nil {
				if _, statErr := os.Stat(test.target(parent)); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("target stat error = %v, want not exist", statErr)
				}
			}
			assertNoInitializedRepositories(t, repo, "ws-1")
		})
	}
}

func TestOpenLocalRepositoryDirectoryDoesNotFollowSymlink(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "staging-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("Symlink is unavailable: %v", err)
	}

	file, err := openLocalRepositoryDirectory(link)
	if file != nil {
		_ = file.Close()
	}
	if err == nil {
		t.Fatal("openLocalRepositoryDirectory followed symlink")
	}
}

func TestServiceInitializeLocalRepositoryRejectsExistingTargetWithoutMutation(t *testing.T) {
	for _, existingContent := range []string{"", "keep me"} {
		name := "empty"
		if existingContent != "" {
			name = "non-empty"
		}
		t.Run(name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
				t.Fatalf("CreateWorkspace: %v", err)
			}
			parent := t.TempDir()
			target := filepath.Join(parent, "existing")
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatalf("Mkdir target: %v", err)
			}
			if existingContent != "" {
				if err := os.WriteFile(filepath.Join(target, "marker"), []byte(existingContent), 0o644); err != nil {
					t.Fatalf("WriteFile marker: %v", err)
				}
			}

			_, err := svc.InitializeLocalRepository(ctx, &InitializeLocalRepositoryRequest{
				WorkspaceID: "ws-1", Name: "existing", ParentPath: parent,
			})
			if !errors.Is(err, ErrLocalRepositoryTargetExists) {
				t.Fatalf("error = %v, want ErrLocalRepositoryTargetExists", err)
			}
			if existingContent != "" {
				content, readErr := os.ReadFile(filepath.Join(target, "marker"))
				if readErr != nil || string(content) != existingContent {
					t.Fatalf("marker = %q, error %v; existing target was modified", content, readErr)
				}
			}
			assertNoInitializedRepositories(t, repo, "ws-1")
		})
	}
}

func TestServiceInitializeLocalRepositoryValidatesWorkspaceBeforeMutation(t *testing.T) {
	svc, _, repo := createTestService(t)
	parent := t.TempDir()
	target := filepath.Join(parent, "new-project")

	_, err := svc.InitializeLocalRepository(context.Background(), &InitializeLocalRepositoryRequest{
		WorkspaceID: "missing", Name: "new-project", ParentPath: parent,
	})
	if !errors.Is(err, repository.ErrWorkspaceNotFound) {
		t.Fatalf("error = %v, want ErrWorkspaceNotFound", err)
	}
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("target stat error = %v, want not exist", statErr)
	}
	assertNoInitializedRepositories(t, repo, "missing")
}

func TestServiceInitializeLocalRepositoryCleansUpGitFailure(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	parent := t.TempDir()
	t.Setenv("PATH", t.TempDir())

	_, err := svc.InitializeLocalRepository(ctx, &InitializeLocalRepositoryRequest{
		WorkspaceID: "ws-1", Name: "new-project", ParentPath: parent,
	})
	if err == nil || !strings.Contains(err.Error(), "initialize git repository") {
		t.Fatalf("error = %v, want Git initialization error", err)
	}
	if _, statErr := os.Stat(filepath.Join(parent, "new-project")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("target stat error = %v, want cleanup", statErr)
	}
	assertNoInitializedRepositories(t, repo, "ws-1")
}

func TestServiceInitializeLocalRepositoryDoesNotRemoveReplacedTarget(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	parent := t.TempDir()
	target := filepath.Join(parent, "new-project")
	movedTarget := filepath.Join(parent, "request-owned")
	replacementPath := ""
	wantErr := errors.New("git unavailable")

	_, err := svc.initializeLocalRepository(ctx, &InitializeLocalRepositoryRequest{
		WorkspaceID: "ws-1", Name: "new-project", ParentPath: parent,
	}, func(_ context.Context, stagingPath string, _ *os.File) error {
		replacementPath = stagingPath
		if renameErr := os.Rename(stagingPath, movedTarget); renameErr != nil {
			t.Fatalf("Rename target: %v", renameErr)
		}
		if mkdirErr := os.Mkdir(stagingPath, 0o755); mkdirErr != nil {
			t.Fatalf("Mkdir replacement: %v", mkdirErr)
		}
		if writeErr := os.WriteFile(filepath.Join(stagingPath, "keep"), []byte("replacement"), 0o644); writeErr != nil {
			t.Fatalf("Write replacement marker: %v", writeErr)
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if content, readErr := os.ReadFile(filepath.Join(replacementPath, "keep")); readErr != nil || string(content) != "replacement" {
		t.Fatalf("replacement marker = %q, error %v; cleanup removed or changed replacement", content, readErr)
	}
	if info, statErr := os.Stat(movedTarget); statErr != nil || !info.IsDir() {
		t.Fatalf("request-owned directory stat = %+v, error %v; want preserved directory", info, statErr)
	}
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("published target stat error = %v, want not exist", statErr)
	}
	assertNoInitializedRepositories(t, repo, "ws-1")
}

func TestServiceInitializeLocalRepositoryDoesNotPublishOverRacingTarget(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	parent := t.TempDir()
	target := filepath.Join(parent, "new-project")
	marker := filepath.Join(target, "keep")

	_, err := svc.initializeLocalRepository(ctx, &InitializeLocalRepositoryRequest{
		WorkspaceID: "ws-1", Name: "new-project", ParentPath: parent,
	}, func(context.Context, string, *os.File) error {
		if mkdirErr := os.Mkdir(target, 0o755); mkdirErr != nil {
			t.Fatalf("Mkdir racing target: %v", mkdirErr)
		}
		if writeErr := os.WriteFile(marker, []byte("racing target"), 0o644); writeErr != nil {
			t.Fatalf("Write racing target marker: %v", writeErr)
		}
		return nil
	})
	if !errors.Is(err, ErrLocalRepositoryTargetExists) {
		t.Fatalf("error = %v, want ErrLocalRepositoryTargetExists", err)
	}
	if content, readErr := os.ReadFile(marker); readErr != nil || string(content) != "racing target" {
		t.Fatalf("racing target marker = %q, error %v; publish modified existing target", content, readErr)
	}
	assertNoInitializedRepositories(t, repo, "ws-1")
}

type failingCreateRepository struct {
	repository.RepositoryEntityRepository
	err error
}

func (r failingCreateRepository) CreateRepository(context.Context, *models.Repository) error {
	return r.err
}

func TestServiceInitializeLocalRepositoryCleansUpPersistenceFailure(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	wantErr := errors.New("database unavailable")
	svc.repoEntities = failingCreateRepository{RepositoryEntityRepository: repo, err: wantErr}
	parent := t.TempDir()

	_, err := svc.InitializeLocalRepository(ctx, &InitializeLocalRepositoryRequest{
		WorkspaceID: "ws-1", Name: "new-project", ParentPath: parent,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if _, statErr := os.Stat(filepath.Join(parent, "new-project")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("target stat error = %v, want cleanup", statErr)
	}
	assertNoInitializedRepositories(t, repo, "ws-1")
}

func runGitOutput(t *testing.T, repositoryPath string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", repositoryPath}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func assertNoInitializedRepositories(t *testing.T, repo repository.RepositoryEntityRepository, workspaceID string) {
	t.Helper()
	repositories, err := repo.ListRepositories(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 0 {
		t.Fatalf("persisted repositories = %+v, want none", repositories)
	}
}
