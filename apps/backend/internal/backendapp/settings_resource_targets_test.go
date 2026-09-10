package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/settingscatalog"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type workspaceSettingsTaskStub struct {
	settingsTaskService
	workspace *taskmodels.Workspace
}

func (s workspaceSettingsTaskStub) ListWorkspaces(context.Context) ([]*taskmodels.Workspace, error) {
	return []*taskmodels.Workspace{s.workspace}, nil
}

func (s workspaceSettingsTaskStub) GetWorkspace(context.Context, string) (*taskmodels.Workspace, error) {
	return s.workspace, nil
}

func TestListedWorkspaceTargetsRoundTripThroughRegistryAndRead(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	workspace := &taskmodels.Workspace{ID: "workspace-1", Name: "Workspace one"}
	operations := &settingsOperations{
		registry: registry,
		deps:     settingsDomainDependencies{task: workspaceSettingsTaskStub{workspace: workspace}},
	}

	result, err := operations.listDomainSettingsResources(context.Background(), "workspace", nil, "", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	body, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want object", result)
	}
	resources, ok := body["resources"].([]map[string]any)
	if !ok || len(resources) != 1 {
		t.Fatalf("resources = %#v, want one resource", body["resources"])
	}
	target, ok := resources[0]["target"].(settingscatalog.ResourceTarget)
	if !ok {
		t.Fatalf("target = %#v, want settings target", resources[0]["target"])
	}
	if err := registry.ValidateTarget(target); err != nil {
		t.Fatalf("listed target failed registry validation: %v", err)
	}
	if _, err := operations.readDomainSettings(context.Background(), target, []string{"name"}); err != nil {
		t.Fatalf("listed target failed read round trip: %v", err)
	}
}
