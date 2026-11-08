package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

func assertExecutionGrade(t *testing.T, value any, want string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["execution_grade"] != want {
		t.Fatalf("execution_grade=%v want %q", got["execution_grade"], want)
	}
}

func TestExecutionGradeProfileRoundTrip(t *testing.T) {
	ctrl, repo, ctx, agentID, _ := newProviderTestController(t)
	var create dto.ProfileCreateRequest
	if err := json.Unmarshal([]byte(`{"name":"Grade","execution_grade":"Tier-A"}`), &create); err != nil {
		t.Fatal(err)
	}
	create.AgentID = agentID
	profile, err := ctrl.CreateProfile(ctx, CreateProfileRequestFromDTO(create))
	if err != nil {
		t.Fatal(err)
	}
	assertExecutionGrade(t, profile, "Tier-A")
	for _, tc := range []struct{ raw, want string }{
		{`{}`, "Tier-A"}, {`{"execution_grade":null}`, "Tier-A"},
		{`{"execution_grade":""}`, ""}, {`{"execution_grade":"界Tier"}`, "界Tier"},
	} {
		var patch dto.ProfileUpdateRequest
		if err := json.Unmarshal([]byte(tc.raw), &patch); err != nil {
			t.Fatal(err)
		}
		patch.ID = profile.ID
		saved, err := ctrl.UpdateProfile(ctx, UpdateProfileRequestFromDTO(patch))
		if err != nil {
			t.Fatal(err)
		}
		assertExecutionGrade(t, saved, tc.want)
		stored, err := repo.GetAgentProfile(ctx, profile.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertExecutionGrade(t, stored, tc.want)
	}
	clone, err := ctrl.DuplicateProfile(ctx, DuplicateProfileRequest{ID: profile.ID})
	if err != nil {
		t.Fatal(err)
	}
	assertExecutionGrade(t, clone, "界Tier")
}

func TestExecutionGradeResponseUsesOwnCommit(t *testing.T) {
	ctrl, repo, ctx, agentID, _ := newProviderTestController(t)
	profile, err := ctrl.CreateProfile(ctx, CreateProfileRequest{AgentID: agentID, Name: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	later := "Later-Grade"
	ctrl.repo = &enabledIntentAfterWriteStore{
		Repository: repo,
		afterWrite: func(ctx context.Context) error {
			current, err := repo.GetAgentProfile(ctx, profile.ID)
			if err != nil {
				return err
			}
			return repo.UpdateAgentProfileWithEnabledIntent(store.WithExecutionGradeUpdate(ctx, &later), current, nil)
		},
	}
	own := "Own-Grade"
	result, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: profile.ID, ExecutionGrade: &own})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExecutionGrade != own || stored.ExecutionGrade != later {
		t.Fatalf("own response=%q later stored=%q", result.ExecutionGrade, stored.ExecutionGrade)
	}
}

func TestExecutionGradeDynamicRollback(t *testing.T) {
	ctrl, repo, profile := enabledIntentDynamicFixture(t)
	grade := "Before"
	if _, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: profile.ID, ExecutionGrade: &grade}); err != nil {
		t.Fatal(err)
	}
	profile.Dynamic.Version++
	grade = "Rejected"
	if _, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: profile.ID, ExecutionGrade: &grade, Dynamic: profile.Dynamic}); err != store.ErrDynamicProfileVersionConflict {
		t.Fatalf("stale dynamic error=%v", err)
	}
	stored, err := repo.GetAgentProfile(context.Background(), profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ExecutionGrade != "Before" {
		t.Fatalf("failed transaction leaked grade=%q", stored.ExecutionGrade)
	}
}
