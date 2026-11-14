package agents

import (
	"context"
	"errors"
	"reflect"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/models"
)

const (
	nameWorkspaceA = "name-workspace-a"
	nameWorkspaceB = "name-workspace-b"
	nameAlpha      = "Alpha"
	nameBeta       = "Beta"
	nameSavedIcon  = "saved-icon"
)

// @covers AC-OFFICE-AGENT-NAMES-001.1, AC-OFFICE-AGENT-NAMES-001.2,
// AC-OFFICE-AGENT-NAMES-001.3, AC-OFFICE-AGENT-NAMES-001.4, AC-OFFICE-AGENT-NAMES-001.5
func TestAgentNameUniqueness_Service(t *testing.T) {
	tests := []struct {
		name           string
		requestedName  string
		otherWorkspace string
		deleteOther    bool
		wantConflict   bool
	}{
		{name: "occupied_name_with_parent", requestedName: nameAlpha, wantConflict: true},
		{name: "unchanged_name", requestedName: nameBeta},
		{name: "unused_name", requestedName: "Gamma"},
		{name: "cross_workspace_name", requestedName: nameAlpha, otherWorkspace: nameWorkspaceB},
		{name: "case_is_distinct", requestedName: "alpha"},
		{name: "whitespace_is_retained", requestedName: "Alpha "},
		{name: "deleted_name_is_available", requestedName: nameAlpha, deleteOther: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newTestAgentService(t)
			otherWorkspace := tt.otherWorkspace
			if otherWorkspace == "" {
				otherWorkspace = nameWorkspaceA
			}
			other := createNameTestAgent(t, svc, otherWorkspace, nameAlpha)
			target := createNameTestAgent(t, svc, nameWorkspaceA, nameBeta)
			if tt.deleteOther {
				if err := repo.DeleteAgentInstance(context.Background(), other.ID); err != nil {
					t.Fatalf("delete other agent: %v", err)
				}
			}
			candidate := *target
			candidate.Name, candidate.Icon = tt.requestedName, nameSavedIcon
			if tt.wantConflict {
				candidate.ReportsTo = other.ID
			}
			err := svc.UpdateAgentInstance(context.Background(), &candidate)
			if tt.wantConflict {
				assertNameConflict(t, err)
				assertNameAgentUnchanged(t, svc, target)
			} else {
				if err != nil {
					t.Fatalf("update agent: %v", err)
				}
				assertNameAgentSaved(t, svc, target.ID, tt.requestedName)
			}
			if !tt.deleteOther {
				assertNameAgentUnchanged(t, svc, other)
			}
		})
	}
	t.Run("duplicate_create", testNameDuplicateCreate)
	t.Run("global_profile_name_is_available", testNameGlobalProfile)
}

func createNameTestAgent(t *testing.T, svc *AgentService, workspace, name string) *models.AgentInstance {
	t.Helper()
	return createAndGetAgent(t, svc, svc.repo, &models.AgentInstance{
		WorkspaceID: workspace, Name: name, Role: models.AgentRoleWorker,
	})
}

func assertNameConflict(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrAgentNameAlreadyExists) {
		t.Errorf("name conflict error = %v, want occupied-name rejection", err)
	}
}

func assertNameAgentUnchanged(t *testing.T, svc *AgentService, before *models.AgentInstance) {
	t.Helper()
	after, err := svc.repo.GetAgentInstance(context.Background(), before.ID)
	if err != nil {
		t.Fatalf("reread agent: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("persisted agent changed: before=%+v after=%+v", before, after)
	}
}

func assertNameAgentSaved(t *testing.T, svc *AgentService, id, name string) {
	t.Helper()
	stored, err := svc.repo.GetAgentInstance(context.Background(), id)
	if err != nil {
		t.Fatalf("reread saved agent: %v", err)
	}
	if stored.ID != id || stored.Name != name || stored.Icon != nameSavedIcon {
		t.Errorf("saved identity = %q/%q/%q, want %q/%q/%q",
			stored.ID, stored.Name, stored.Icon, id, name, nameSavedIcon)
	}
}

func testNameDuplicateCreate(t *testing.T) {
	svc, repo := newTestAgentService(t)
	original := createNameTestAgent(t, svc, nameWorkspaceA, nameAlpha)
	err := svc.CreateAgentInstance(context.Background(), &models.AgentInstance{
		WorkspaceID: nameWorkspaceA, Name: nameAlpha, Role: models.AgentRoleWorker,
	})
	assertNameConflict(t, err)
	assertNameAgentUnchanged(t, svc, original)
	rows, err := repo.ListAgentInstances(context.Background(), nameWorkspaceA)
	if err != nil || len(rows) != 1 {
		t.Errorf("agents after duplicate create = %d, err=%v; want 1", len(rows), err)
	}
}

func testNameGlobalProfile(t *testing.T) {
	svc, _, profiles := newTestAgentServiceWithProfileStore(t)
	ctx := context.Background()
	provider := &settingsmodels.Agent{ID: "name-test-provider", Name: "claude-acp"}
	if err := profiles.CreateAgent(ctx, provider); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	profile := &settingsmodels.AgentProfile{
		ID: "global-name-profile", AgentID: provider.ID, Name: nameAlpha, Enabled: true,
	}
	if err := profiles.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatalf("create global profile: %v", err)
	}
	target := createNameTestAgent(t, svc, nameWorkspaceA, nameBeta)
	candidate := *target
	candidate.Name, candidate.Icon = nameAlpha, nameSavedIcon
	if err := svc.UpdateAgentInstance(ctx, &candidate); err != nil {
		t.Fatalf("rename alongside global profile: %v", err)
	}
	assertNameAgentSaved(t, svc, target.ID, nameAlpha)
	storedProfile, err := profiles.GetAgentProfile(ctx, profile.ID)
	if err != nil || storedProfile.Name != nameAlpha || storedProfile.WorkspaceID != "" {
		t.Errorf("global profile changed: profile=%+v err=%v", storedProfile, err)
	}
}
