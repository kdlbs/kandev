package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	"github.com/kandev/kandev/internal/office/service"
)

func TestBuildSkillManifest_DecisionSkillSeatBranches(t *testing.T) {
	tests := []struct {
		name           string
		desiredSkills  string
		available      []string
		wantSkillCount int
	}{
		{
			name:           "desired without decision seat",
			desiredSkills:  `["kandev-step-decision"]`,
			wantSkillCount: 0,
		},
		{
			name:           "decision seat auto injects skill",
			available:      []string{officeruntime.AvailableActionRecordStepDecision},
			wantSkillCount: 1,
		},
		{
			name:           "decision seat keeps desired skill once",
			desiredSkills:  `["kandev-step-decision"]`,
			available:      []string{officeruntime.AvailableActionRecordStepDecision},
			wantSkillCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()
			if err := svc.CreateSkill(ctx, &models.Skill{
				ID:          "decision-skill",
				WorkspaceID: "ws-1",
				Name:        "Step decision",
				Slug:        "kandev-step-decision",
				Content:     "decision skill",
				Version:     "0.42.0",
				ContentHash: "decision-skill-hash",
			}); err != nil {
				t.Fatalf("create skill: %v", err)
			}

			manifest := service.BuildSkillManifestForTest(
				service.NewSchedulerIntegration(svc, 0),
				ctx,
				&models.AgentInstance{
					ID:            "agent-1",
					WorkspaceID:   "ws-1",
					DesiredSkills: tt.desiredSkills,
				},
				"ws-1",
				tt.available...,
			)

			count := 0
			for _, skill := range manifest.Skills {
				if skill.Slug == "kandev-step-decision" {
					count++
				}
			}
			if count != tt.wantSkillCount {
				t.Fatalf("decision skill count = %d, want %d; manifest = %+v", count, tt.wantSkillCount, manifest.Skills)
			}
		})
	}
}
