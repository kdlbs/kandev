package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestLaunchProfileFillsDefaultsAndRecoveryKeepsTheRecordedSnapshot(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	catalog := pluginExecutorTestCatalog(true)
	catalog.providers[0].ProfileSchema["properties"].(map[string]any)["tier"] = map[string]any{"type": "string", "default": "standard"}
	svc.SetExecutorProviderCatalog(catalog)
	svc.SetSecretStore(newPluginProfileSecretStore())

	executors, err := svc.ListExecutors(ctx)
	if err != nil {
		t.Fatalf("ListExecutors(): %v", err)
	}
	var remote *models.Executor
	for _, candidate := range executors {
		if candidate.Type == models.ExecutorTypePluginRemote {
			remote = candidate
		}
	}
	profile, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: remote.ID, Name: "Defaults", Config: map[string]string{"region": "eu-west-1"},
	})
	if err != nil {
		t.Fatalf("CreateExecutorProfile(): %v", err)
	}
	if _, stored := profile.Config["tier"]; stored {
		t.Fatalf("the stored profile must not be rewritten with defaults: %v", profile.Config)
	}
	seedPluginProfileInventory(t, repo, profile.ID, "unused-secret")

	launch, err := svc.ExecutorProviderProfileForLaunch(ctx, profile.ID, "environment-plugin-profile-secrets")
	if err != nil {
		t.Fatalf("ExecutorProviderProfileForLaunch(): %v", err)
	}
	if launch.Config["tier"] != "standard" {
		t.Fatalf("launch config = %v, want the default filled", launch.Config)
	}

	catalog.providers[0].ProfileSchema["properties"].(map[string]any)["tier"] = map[string]any{"type": "string", "default": "premium"}
	recovered, err := svc.ExecutorProviderProfileForRecovery(ctx, profile.ID, "task-plugin-profile-secrets", "environment-plugin-profile-secrets",
		catalog.providers[0].Identity, 7, launch.Config, nil)
	if err != nil {
		t.Fatalf("ExecutorProviderProfileForRecovery(): %v", err)
	}
	if recovered.Config["tier"] != "standard" {
		t.Fatalf("recovered config = %v; an upgraded default must not change an existing environment", recovered.Config)
	}
}
