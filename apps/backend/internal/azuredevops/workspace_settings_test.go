package azuredevops

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestDefaultQueryPresetsMatchAzureBrowseShortcuts(t *testing.T) {
	workItemIDs := make([]string, 0, len(DefaultWorkItemQueryPresets()))
	for _, preset := range DefaultWorkItemQueryPresets() {
		workItemIDs = append(workItemIDs, preset.ID)
	}
	pullRequestIDs := make([]string, 0, len(DefaultPullRequestQueryPresets()))
	for _, preset := range DefaultPullRequestQueryPresets() {
		pullRequestIDs = append(pullRequestIDs, preset.ID)
	}

	if want := []string{"recent", "assigned", "active", "created"}; !reflect.DeepEqual(workItemIDs, want) {
		t.Fatalf("work-item preset IDs = %v, want %v", workItemIDs, want)
	}
	if want := []string{"review-requested", "active", "completed", "created"}; !reflect.DeepEqual(pullRequestIDs, want) {
		t.Fatalf("pull-request preset IDs = %v, want %v", pullRequestIDs, want)
	}
}

func TestWorkspaceSettingsDefaultsAndActionOverrides(t *testing.T) {
	service, _, _ := newTestService(t, nil)
	ctx := context.Background()
	if _, err := service.SetConfigForWorkspace(ctx, "ws-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	defaults, err := service.GetWorkspaceSettings(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	if len(defaults.WorkItemActions) == 0 || len(defaults.PullRequestActions) == 0 {
		t.Fatalf("expected built-in actions, got %+v", defaults)
	}

	custom := []ActionPreset{{Label: "Triage", Hint: "Sort the report", PromptTemplate: "Triage {{url}}"}}
	updated, err := service.UpdateWorkspaceSettings(ctx, &UpdateWorkspaceSettingsRequest{
		WorkspaceID:        "ws-1",
		WorkItemActions:    &custom,
		WorkItemActionsSet: true,
	})
	if err != nil {
		t.Fatalf("update actions: %v", err)
	}
	if len(updated.WorkItemActions) != 1 || updated.WorkItemActions[0].ID == "" {
		t.Fatalf("custom actions were not normalized: %+v", updated.WorkItemActions)
	}
	if len(updated.PullRequestActions) != len(DefaultPullRequestActionPresets()) {
		t.Fatalf("untouched PR actions should keep defaults: %+v", updated.PullRequestActions)
	}
}

func TestWorkspaceSettingsPatchRequestPreservesFieldPresence(t *testing.T) {
	service, _, _ := newTestService(t, nil)
	ctx := context.Background()
	if _, err := service.SetConfigForWorkspace(ctx, "ws-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	var customRequest UpdateWorkspaceSettingsRequest
	if err := json.Unmarshal([]byte(`{"workItemActions":[{"label":"Triage","promptTemplate":"Triage {{url}}"}]}`), &customRequest); err != nil {
		t.Fatalf("decode custom patch: %v", err)
	}
	customRequest.WorkspaceID = "ws-1"
	if _, err := service.UpdateWorkspaceSettings(ctx, &customRequest); err != nil {
		t.Fatalf("apply custom patch: %v", err)
	}

	var omittedRequest UpdateWorkspaceSettingsRequest
	if err := json.Unmarshal([]byte(`{}`), &omittedRequest); err != nil {
		t.Fatalf("decode omitted patch: %v", err)
	}
	omittedRequest.WorkspaceID = "ws-1"
	omitted, err := service.UpdateWorkspaceSettings(ctx, &omittedRequest)
	if err != nil {
		t.Fatalf("apply omitted patch: %v", err)
	}
	if len(omitted.WorkItemActions) != 1 || omitted.WorkItemActions[0].Label != "Triage" {
		t.Fatalf("omitted work-item actions = %+v, want saved action", omitted.WorkItemActions)
	}

	var resetRequest UpdateWorkspaceSettingsRequest
	if err := json.Unmarshal([]byte(`{"workItemActions":null}`), &resetRequest); err != nil {
		t.Fatalf("decode null patch: %v", err)
	}
	resetRequest.WorkspaceID = "ws-1"
	reset, err := service.UpdateWorkspaceSettings(ctx, &resetRequest)
	if err != nil {
		t.Fatalf("apply null patch: %v", err)
	}
	if !reflect.DeepEqual(reset.WorkItemActions, DefaultWorkItemActionPresets()) {
		t.Fatalf("null work-item actions = %+v, want defaults", reset.WorkItemActions)
	}
}
