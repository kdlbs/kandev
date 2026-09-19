package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-REMOTE-OPTIONS-002.1
func TestRepositoryCheckoutOptionsMetadata(t *testing.T) {
	var input TaskRepositoryInput
	if err := json.Unmarshal([]byte(`{"checkout_options":{"version":1,"download_mode":"on_demand","sparse_directories":["extensions/example"]}}`), &input); err != nil {
		t.Fatal(err)
	}
	metadata, err := buildTaskRepositoryMetadata(input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(metadata["checkout_options"])
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"version":1,"download_mode":"on_demand","sparse_directories":["extensions/example"]}` {
		t.Fatalf("checkout options lost in metadata: %s", encoded)
	}
}

func TestRepositoryCheckoutOptionsPreservedOnUpdate(t *testing.T) {
	svc, _, repo := createTestService(t)
	setupTestTask(t, repo)
	ctx := context.Background()
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "remote-options", WorkspaceID: "ws-1", Name: "Remote", SourceType: "github", Provider: "github", ProviderOwner: "acme", ProviderName: "repo", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]interface{}{}
	if err := models.PutRepositoryCheckoutOptions(metadata, &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadOnDemand, SparseDirectories: []string{"app"}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "attachment", TaskID: "task-123", RepositoryID: "remote-options", BaseBranch: "main", Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.UpdateTask(ctx, "task-123", &UpdateTaskRequest{Repositories: []TaskRepositoryInput{{RepositoryID: "remote-options"}}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListTaskRepositories(ctx, "task-123")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	options, err := models.GetRepositoryCheckoutOptions(rows[0].Metadata)
	if err != nil || options == nil || options.SparseDirectories[0] != "app" {
		t.Fatalf("options after update = %+v, err %v", options, err)
	}
}

func TestRepositoryCheckoutOptionsRejectLocalRepository(t *testing.T) {
	svc, _, repo := createTestService(t)
	setupTestTask(t, repo)
	ctx := context.Background()
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "local-options", WorkspaceID: "ws-1", Name: "Local", SourceType: "local", LocalPath: t.TempDir(), DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.resolveTaskRepositoryRows(ctx, "ws-1", []TaskRepositoryInput{{RepositoryID: "local-options", BaseBranch: "main", CheckoutOptions: &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadOnDemand}}})
	if err == nil {
		t.Fatal("remote checkout options accepted for a user-owned local repository")
	}
}

// @covers AC-TASKS-REMOTE-OPTIONS-003.2
func TestRepositoryCheckoutOptionsRejectInvalid(t *testing.T) {
	for _, options := range []string{
		`{"version":2,"download_mode":"standard"}`,
		`{"version":1,"download_mode":"shallow"}`,
		`{"version":1,"download_mode":"standard","sparse_directories":["../outside"]}`,
		`{"version":1,"download_mode":"standard","sparse_directories":["/outside"]}`,
	} {
		t.Run(options, func(t *testing.T) {
			var input TaskRepositoryInput
			if err := json.Unmarshal([]byte(`{"checkout_options":`+options+`}`), &input); err != nil {
				t.Fatal(err)
			}
			if _, err := buildTaskRepositoryMetadata(input); err == nil {
				t.Fatal("invalid checkout options accepted")
			}
		})
	}
}

func TestRepositoryCheckoutCapabilitiesRejectExecutorCredentials(t *testing.T) {
	svc, _, repo := createTestService(t)
	setupTestTask(t, repo)
	executor := createTestExecutor(t, svc, "Checkout runner", models.ExecutorTypeWorktree)
	profile, err := svc.CreateExecutorProfile(context.Background(), &CreateExecutorProfileRequest{ExecutorID: executor.ID, Name: "Default", McpPolicy: "allow"})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetRepositoryCheckoutCredentialPolicy(func(context.Context, string) (bool, error) { return false, nil })
	capabilities, err := svc.GetRepositoryCheckoutCapabilities(context.Background(), "ws-1", TaskRepositoryInput{RemoteURL: "https://github.com/acme/repo"}, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if capabilities.OnDemand || capabilities.Sparse || capabilities.Reason != "credentials_unsupported" {
		t.Fatalf("unexpected credential capability: %+v", capabilities)
	}
}
