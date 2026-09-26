package sqlite

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestExecutorCRUDRoundTripAndSoftDelete(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	executor := &models.Executor{ID: "executor-crud", Name: "Before", Type: models.ExecutorTypeLocalDocker, Status: "online", IsSystem: true, Resumable: true, Config: map[string]string{"image": "example:v1", "cpus": "2"}}
	if err := repo.CreateExecutor(ctx, executor); err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	got, err := repo.GetExecutor(ctx, executor.ID)
	if err != nil {
		t.Fatalf("GetExecutor: %v", err)
	}
	if got.Name != executor.Name || !got.IsSystem || !got.Resumable || !reflect.DeepEqual(got.Config, executor.Config) {
		t.Fatalf("GetExecutor = %+v, want round trip of %+v", got, executor)
	}

	executor.Name = "After"
	executor.Status = "offline"
	executor.IsSystem = false
	executor.Resumable = false
	executor.Config = map[string]string{"image": "example:v2"}
	if err := repo.UpdateExecutor(ctx, executor); err != nil {
		t.Fatalf("UpdateExecutor: %v", err)
	}
	listed, err := repo.ListExecutors(ctx)
	if err != nil {
		t.Fatalf("ListExecutors: %v", err)
	}
	var found *models.Executor
	for _, item := range listed {
		if item.ID == executor.ID {
			found = item
		}
	}
	if found == nil || found.Name != "After" || found.Status != "offline" || found.IsSystem || found.Resumable {
		t.Fatalf("updated executor not found in list: %+v", found)
	}

	if err := repo.DeleteExecutor(ctx, executor.ID); err != nil {
		t.Fatalf("DeleteExecutor: %v", err)
	}
	if _, err := repo.GetExecutor(ctx, executor.ID); !errors.Is(err, models.ErrExecutorNotFound) {
		t.Fatalf("GetExecutor after delete error = %v", err)
	}
	if err := repo.UpdateExecutor(ctx, executor); err == nil {
		t.Fatal("UpdateExecutor accepted a soft-deleted executor")
	}
	if err := repo.DeleteExecutor(ctx, executor.ID); err == nil {
		t.Fatal("DeleteExecutor accepted an already deleted executor")
	}
}

func TestRepositoryScriptsOrderUpdateDeleteAndConstraints(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-scripts")
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repository-scripts", WorkspaceID: "workspace-scripts", Name: "repo"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	for _, script := range []*models.RepositoryScript{
		{ID: "script-later", RepositoryID: "repository-scripts", Name: "Later", Command: "echo later", Position: 2},
		{ID: "script-first", RepositoryID: "repository-scripts", Name: "First", Command: "echo first", Position: 1},
	} {
		if err := repo.CreateRepositoryScript(ctx, script); err != nil {
			t.Fatalf("CreateRepositoryScript(%s): %v", script.ID, err)
		}
	}
	got, err := repo.GetRepositoryScript(ctx, "script-later")
	if err != nil || got.Command != "echo later" {
		t.Fatalf("GetRepositoryScript = %+v, %v", got, err)
	}
	got.Name, got.Command, got.Position = "Updated", "echo updated", 0
	if err := repo.UpdateRepositoryScript(ctx, got); err != nil {
		t.Fatalf("UpdateRepositoryScript: %v", err)
	}
	listed, err := repo.ListRepositoryScripts(ctx, "repository-scripts")
	if err != nil || strings.Join(repositoryScriptIDs(listed), ",") != "script-later,script-first" {
		t.Fatalf("ListRepositoryScripts = %v, %v", repositoryScriptIDs(listed), err)
	}
	grouped, err := repo.ListScriptsByRepositoryIDs(ctx, []string{"repository-scripts", "missing"})
	if err != nil || len(grouped["repository-scripts"]) != 2 {
		t.Fatalf("ListScriptsByRepositoryIDs = %v, %v", grouped, err)
	}
	empty, err := repo.ListScriptsByRepositoryIDs(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("ListScriptsByRepositoryIDs(nil) = %v, %v", empty, err)
	}
	if err := repo.CreateRepositoryScript(ctx, &models.RepositoryScript{ID: "orphan-script", RepositoryID: "missing", Name: "bad"}); err == nil {
		t.Fatal("CreateRepositoryScript accepted a missing repository")
	}
	if err := repo.DeleteRepositoryScript(ctx, "script-first"); err != nil {
		t.Fatalf("DeleteRepositoryScript: %v", err)
	}
	if err := repo.DeleteRepositoryScript(ctx, "script-first"); err == nil {
		t.Fatal("second DeleteRepositoryScript returned nil")
	}
	if err := repo.UpdateRepositoryScript(ctx, &models.RepositoryScript{ID: "missing"}); err == nil {
		t.Fatal("UpdateRepositoryScript accepted a missing script")
	}
}

func TestRepositoryLocalPathAndAtomicBindingReplacement(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-bindings")
	repository := &models.Repository{ID: "repository-bindings", WorkspaceID: "workspace-bindings", Name: "Before", LocalPath: "/tmp/repository-bindings"}
	bindings := []models.RepositorySecretBinding{{Key: "TOKEN", SecretID: "secret-one"}, {Key: "SSH_KEY", SecretID: "secret-two"}}
	if err := repo.CreateRepositoryWithSecretBindings(ctx, repository, bindings); err != nil {
		t.Fatalf("CreateRepositoryWithSecretBindings: %v", err)
	}
	got, err := repo.GetRepositoryByLocalPath(ctx, "workspace-bindings", "/tmp/repository-bindings")
	if err != nil || got == nil || got.ID != repository.ID || len(got.SecretBindings) != 2 {
		t.Fatalf("GetRepositoryByLocalPath = %+v, %v", got, err)
	}
	missing, err := repo.GetRepositoryByLocalPath(ctx, "workspace-bindings", "")
	if err != nil || missing != nil {
		t.Fatalf("GetRepositoryByLocalPath(empty) = %+v, %v", missing, err)
	}
	repository.Name = "After"
	replacement := []models.RepositorySecretBinding{{Key: "TOKEN", SecretID: "secret-three"}}
	if err := repo.UpdateRepositoryWithSecretBindings(ctx, repository, replacement); err != nil {
		t.Fatalf("UpdateRepositoryWithSecretBindings: %v", err)
	}
	got, err = repo.GetRepository(ctx, repository.ID)
	if err != nil || got.Name != "After" || len(got.SecretBindings) != 1 || got.SecretBindings[0].SecretID != "secret-three" {
		t.Fatalf("updated repository = %+v, %v", got, err)
	}
	duplicateKeys := []models.RepositorySecretBinding{{Key: "DUP", SecretID: "one"}, {Key: "DUP", SecretID: "two"}}
	repository.Name = "Must Roll Back"
	if err := repo.UpdateRepositoryWithSecretBindings(ctx, repository, duplicateKeys); err == nil {
		t.Fatal("duplicate binding keys were accepted")
	}
	got, err = repo.GetRepository(ctx, repository.ID)
	if err != nil || got.Name != "After" || len(got.SecretBindings) != 1 || got.SecretBindings[0].SecretID != "secret-three" {
		t.Fatalf("failed binding replacement was not atomic: %+v, %v", got, err)
	}
}

func repositoryScriptIDs(scripts []*models.RepositoryScript) []string {
	ids := make([]string, 0, len(scripts))
	for _, script := range scripts {
		ids = append(ids, script.ID)
	}
	return ids
}
