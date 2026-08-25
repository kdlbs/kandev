package automation

import (
	"context"
	"testing"
)

// @covers AC-OFFICE-AUTOMATION-TARGETS-001.9
func TestCreateAutomationDefaultsEmptyRepositorySelectionToNone(t *testing.T) {
	store := setupTestStore(t)
	a := &Automation{WorkspaceID: "ws-1", Name: "Scratch", Enabled: true}

	if err := store.CreateAutomation(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if a.RepositoryMode != RepositoryModeNone {
		t.Fatalf("repository mode = %q, want %q", a.RepositoryMode, RepositoryModeNone)
	}
}

// @covers AC-OFFICE-AUTOMATION-TARGETS-001.9
func TestValidateAutomationTargetRejectsWorkspaceDefault(t *testing.T) {
	err := validateAutomationTarget(TaskModeAutomationRun, RepositoryModeWorkspaceDefault, "", nil)
	if err == nil {
		t.Fatal("expected workspace_default to be rejected")
	}
}

// @covers AC-OFFICE-AUTOMATION-TARGETS-001.10
func TestCreateAutomationPersistsRepositoryBaseBranches(t *testing.T) {
	store := setupTestStore(t)
	a := &Automation{
		WorkspaceID: "ws-1",
		Name:        "Branches",
		Enabled:     true,
		Repositories: []AutomationRepository{
			{RepositoryID: "repo-b", BaseBranch: "release/2"},
			{RepositoryID: "repo-a", BaseBranch: "main"},
		},
	}

	if err := store.CreateAutomation(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetAutomation(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []AutomationRepository{
		{RepositoryID: "repo-b", BaseBranch: "release/2"},
		{RepositoryID: "repo-a", BaseBranch: "main"},
	}
	if len(got.Repositories) != len(want) {
		t.Fatalf("repositories = %#v, want %#v", got.Repositories, want)
	}
	for i := range want {
		if got.Repositories[i] != want[i] {
			t.Fatalf("repositories[%d] = %#v, want %#v", i, got.Repositories[i], want[i])
		}
	}
}

// @covers AC-OFFICE-AUTOMATION-TARGETS-001.10
func TestUpdateAutomationReplacesRepositoryBaseBranches(t *testing.T) {
	store := setupTestStore(t)
	a := &Automation{
		WorkspaceID: "ws-1",
		Name:        "Branches",
		Enabled:     true,
		Repositories: []AutomationRepository{
			{RepositoryID: "repo-a", BaseBranch: "main"},
		},
	}
	if err := store.CreateAutomation(context.Background(), a); err != nil {
		t.Fatal(err)
	}

	repositories := []AutomationRepository{{RepositoryID: "repo-a", BaseBranch: "release/3"}}
	if err := store.UpdateAutomation(context.Background(), a.ID, &UpdateAutomationRequest{
		Repositories: repositories,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetAutomation(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Repositories) != 1 || got.Repositories[0] != repositories[0] {
		t.Fatalf("repositories = %#v, want %#v", got.Repositories, repositories)
	}
}
