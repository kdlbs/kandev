package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestPreparedProfileGradeResolver(t *testing.T) {
	for _, grade := range []string{"Grade-A", ""} {
		profile := &models.AgentProfile{ID: "profile-grade", AgentID: "agent-grade", Model: "model-with-unrelated-name", ExecutionGrade: grade}
		repo := &MockRepository{
			GetAgentProfileFn: func(context.Context, string) (*models.AgentProfile, error) { return profile, nil },
			GetAgentFn: func(context.Context, string) (*models.Agent, error) {
				return &models.Agent{ID: "agent-grade", Name: "test-agent"}, nil
			},
		}
		info, err := NewStoreProfileResolver(repo, nil).ResolveProfile(context.Background(), profile.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(info)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got["ExecutionGrade"] != grade {
			t.Fatalf("resolved grade=%v want %q", got["ExecutionGrade"], grade)
		}
		if info.ProfileID != profile.ID || info.Model != profile.Model {
			t.Fatal("profile identity/model changed")
		}
	}
}
