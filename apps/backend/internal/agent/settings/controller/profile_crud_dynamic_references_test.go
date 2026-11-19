package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentdto "github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

type profileUtilityDependencyChecker struct {
	refs []UtilityAgentReference
}

func (f *profileUtilityDependencyChecker) ListUtilityAgentsByAgentProfile(
	context.Context, string,
) ([]UtilityAgentReference, error) {
	return f.refs, nil
}

func (*profileUtilityDependencyChecker) ClearUtilityAgentProfileBindings(context.Context, string) error {
	return nil
}

func seedDynamicReference(
	t *testing.T,
	repo store.Repository,
	candidateID, dynamicName string,
) *models.AgentProfile {
	t.Helper()
	ctx := context.Background()
	dynamic := &models.AgentProfile{
		AgentID:          agents.DynamicAgentID,
		Name:             dynamicName,
		AgentDisplayName: "Dynamic",
	}
	if err := repo.CreateAgentProfile(ctx, dynamic); err != nil {
		t.Fatalf("seed dynamic profile: %v", err)
	}
	dynamicRepo, ok := repo.(store.DynamicProfileRepository)
	if !ok {
		t.Fatal("settings store does not implement the dynamic profile repository")
	}
	doc := &models.DynamicAgentProfile{ProfileID: dynamic.ID, Version: 1}
	routes := []models.DynamicAgentRoute{{
		DynamicProfileID:   dynamic.ID,
		Position:           0,
		ExecutionProfileID: candidateID,
		Enabled:            true,
		RulesJSON:          `{}`,
	}}
	if err := dynamicRepo.CreateDynamicAgentProfile(ctx, doc, routes); err != nil {
		t.Fatalf("seed dynamic route: %v", err)
	}
	return dynamic
}

func seedCandidateProfile(t *testing.T, repo store.Repository, name string) *models.AgentProfile {
	t.Helper()
	profile := &models.AgentProfile{AgentID: "agent-1", Name: name, Model: "model-a"}
	if err := repo.CreateAgentProfile(context.Background(), profile); err != nil {
		t.Fatalf("seed candidate profile: %v", err)
	}
	return profile
}

func TestDeleteProfile_IgnoresSoftDeletedDynamicReferences(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()

	candidate := seedCandidateProfile(t, repo, "Candidate")
	dynamic := seedDynamicReference(t, repo, candidate.ID, "Deleted selector")
	if err := repo.DeleteAgentProfile(ctx, dynamic.ID); err != nil {
		t.Fatalf("soft-delete dynamic profile: %v", err)
	}

	if _, err := ctrl.DeleteProfile(ctx, candidate.ID, false); err != nil {
		t.Fatalf("delete candidate profile: %v", err)
	}

	dynamicRepo := repo.(store.DynamicProfileRepository)
	_, routes, err := dynamicRepo.GetDynamicAgentProfile(ctx, dynamic.ID)
	if err != nil {
		t.Fatalf("read dynamic profile: %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("stale dynamic routes = %d, want 0", len(routes))
	}
}

func TestDeleteProfile_LiveDynamicReferenceBlocks(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()

	candidate := seedCandidateProfile(t, repo, "Candidate")
	dynamic := seedDynamicReference(t, repo, candidate.ID, "Live selector")

	_, err := ctrl.DeleteProfile(ctx, candidate.ID, false)
	var detail *ErrProfileInUseDetail
	if !errors.As(err, &detail) {
		t.Fatalf("delete err = %v, want ErrProfileInUseDetail", err)
	}
	if len(detail.DynamicProfiles) != 1 || detail.DynamicProfiles[0].ID != dynamic.ID {
		t.Fatalf("dynamic profiles = %#v, want the live reference", detail.DynamicProfiles)
	}
	if _, err := repo.GetAgentProfile(ctx, candidate.ID); err != nil {
		t.Fatalf("candidate profile should still exist: %v", err)
	}
}

func TestDeleteProfile_ForceDeletesWithLiveDynamicReference(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()

	candidate := seedCandidateProfile(t, repo, "Candidate")
	seedDynamicReference(t, repo, candidate.ID, "Live selector")

	if _, err := ctrl.DeleteProfile(ctx, candidate.ID, true); err != nil {
		t.Fatalf("force delete candidate profile: %v", err)
	}
	if _, err := repo.GetAgentProfileIncludingDeleted(ctx, candidate.ID); err != nil {
		t.Fatalf("candidate row should remain as a soft-deleted row: %v", err)
	}
	if deleted, err := repo.GetAgentProfile(ctx, candidate.ID); !isProfileNotFoundErr(err) {
		t.Fatalf("candidate should be soft-deleted, got %#v (err %v)", deleted, err)
	}
}

func TestDeleteProfile_ReportsDynamicAndSessionReferencesTogether(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()

	candidate := seedCandidateProfile(t, repo, "Candidate")
	dynamic := seedDynamicReference(t, repo, candidate.ID, "Live selector")
	ctrl.sessionChecker = &fakeSessionChecker{activeTasks: []agentdto.ActiveTaskInfo{
		{TaskID: "task-1", TaskTitle: "Live task"},
	}}

	_, err := ctrl.DeleteProfile(ctx, candidate.ID, false)
	var detail *ErrProfileInUseDetail
	if !errors.As(err, &detail) {
		t.Fatalf("delete err = %v, want ErrProfileInUseDetail", err)
	}
	if len(detail.DynamicProfiles) != 1 || detail.DynamicProfiles[0].ID != dynamic.ID {
		t.Fatalf("dynamic profiles = %#v, want the live reference", detail.DynamicProfiles)
	}
	if len(detail.ActiveSessions) != 1 || detail.ActiveSessions[0].TaskID != "task-1" {
		t.Fatalf("active sessions = %#v, want the live task", detail.ActiveSessions)
	}
}

func TestUpdateProfile_DisableIgnoresSoftDeletedDynamicReferences(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()

	candidate := seedCandidateProfile(t, repo, "Candidate")
	dynamic := seedDynamicReference(t, repo, candidate.ID, "Deleted selector")
	if err := repo.DeleteAgentProfile(ctx, dynamic.ID); err != nil {
		t.Fatalf("soft-delete dynamic profile: %v", err)
	}

	disabled := false
	result, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: candidate.ID, Enabled: &disabled})
	if err != nil {
		t.Fatalf("disable candidate profile: %v", err)
	}
	if result.Enabled {
		t.Fatal("profile should be disabled")
	}
}

func TestUpdateProfile_DisableLiveDynamicReferenceBlocks(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()

	candidate := seedCandidateProfile(t, repo, "Candidate")
	seedDynamicReference(t, repo, candidate.ID, "Live selector")

	disabled := false
	_, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: candidate.ID, Enabled: &disabled})
	var detail *ErrProfileInUseDetail
	if !errors.As(err, &detail) {
		t.Fatalf("disable err = %v, want ErrProfileInUseDetail", err)
	}
	if len(detail.DynamicProfiles) != 1 {
		t.Fatalf("dynamic profiles = %#v, want 1", detail.DynamicProfiles)
	}
}

func TestUpdateProfile_DisableReportsDynamicAndUtilityReferencesTogether(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()

	candidate := seedCandidateProfile(t, repo, "Candidate")
	dynamic := seedDynamicReference(t, repo, candidate.ID, "Live selector")
	utility := UtilityAgentReference{ID: "utility-1", Name: "Commit message"}
	ctrl.SetUtilityDependencyChecker(&profileUtilityDependencyChecker{refs: []UtilityAgentReference{utility}})

	disabled := false
	_, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: candidate.ID, Enabled: &disabled})
	var detail *ErrProfileInUseDetail
	if !errors.As(err, &detail) {
		t.Fatalf("disable err = %v, want ErrProfileInUseDetail", err)
	}
	if len(detail.DynamicProfiles) != 1 || detail.DynamicProfiles[0].Name != dynamic.Name {
		t.Fatalf("dynamic profiles = %#v, want %q", detail.DynamicProfiles, dynamic.Name)
	}
	if len(detail.UtilityAgents) != 1 || detail.UtilityAgents[0].Name != utility.Name {
		t.Fatalf("utility agents = %#v, want %q", detail.UtilityAgents, utility.Name)
	}

	stored, err := repo.GetAgentProfile(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("read candidate profile: %v", err)
	}
	if !stored.Enabled {
		t.Fatal("blocked disable changed the stored enabled flag")
	}
}

func TestUpdateProfile_DisableForcePassesWithLiveDynamicReference(t *testing.T) {
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()

	candidate := seedCandidateProfile(t, repo, "Candidate")
	seedDynamicReference(t, repo, candidate.ID, "Live selector")

	disabled := false
	result, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID:      candidate.ID,
		Enabled: &disabled,
		Force:   true,
	})
	if err != nil {
		t.Fatalf("force disable candidate profile: %v", err)
	}
	if result.Enabled {
		t.Fatal("profile should be disabled")
	}
}
