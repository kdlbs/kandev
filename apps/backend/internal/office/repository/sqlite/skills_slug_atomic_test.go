package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestNormalizeSkillSlugAtomicallyUpdatesAgentReferences(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	skill := &models.Skill{
		ID:          "skill-1",
		WorkspaceID: "ws-1",
		Name:        "Custom skill",
		Slug:        "custom-skill",
		SourceType:  "inline",
	}
	if err := repo.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	agent := &models.AgentInstance{
		ID:            "agent-1",
		WorkspaceID:   "ws-1",
		Name:          "Agent",
		DesiredSkills: `["custom-skill","other-skill","custom-skill"]`,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	changed, err := repo.NormalizeSkillSlug(ctx, "ws-1", skill.ID, "custom-skill", "kandev-custom-skill")
	if err != nil {
		t.Fatalf("normalize skill slug: %v", err)
	}
	if !changed {
		t.Fatal("normalize skill slug reported no change")
	}

	gotSkill, err := repo.GetSkillBySlug(ctx, "ws-1", "kandev-custom-skill")
	if err != nil {
		t.Fatalf("get normalized skill: %v", err)
	}
	if gotSkill.ID != skill.ID {
		t.Fatalf("normalized skill ID = %q, want %q", gotSkill.ID, skill.ID)
	}
	if _, err := repo.GetSkillBySlug(ctx, "ws-1", "custom-skill"); err == nil {
		t.Fatal("old skill slug still resolves after normalization")
	}

	gotAgent, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	var desired []string
	if err := json.Unmarshal([]byte(gotAgent.DesiredSkills), &desired); err != nil {
		t.Fatalf("decode desired_skills %q: %v", gotAgent.DesiredSkills, err)
	}
	want := []string{"kandev-custom-skill", "other-skill"}
	if len(desired) != len(want) || desired[0] != want[0] || desired[1] != want[1] {
		t.Fatalf("desired_skills = %v, want %v", desired, want)
	}
}

func TestNormalizeSkillSlugRollsBackWhenCanonicalSlugIsTaken(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	oldSkill := &models.Skill{
		ID:          "skill-old",
		WorkspaceID: "ws-1",
		Name:        "Custom skill",
		Slug:        "custom-skill",
		SourceType:  "inline",
	}
	takenSkill := &models.Skill{
		ID:          "skill-taken",
		WorkspaceID: "ws-1",
		Name:        "Canonical skill",
		Slug:        "kandev-custom-skill",
		SourceType:  "inline",
	}
	for _, skill := range []*models.Skill{oldSkill, takenSkill} {
		if err := repo.CreateSkill(ctx, skill); err != nil {
			t.Fatalf("create skill %s: %v", skill.ID, err)
		}
	}
	agent := &models.AgentInstance{
		ID:            "agent-1",
		WorkspaceID:   "ws-1",
		Name:          "Agent",
		DesiredSkills: `["custom-skill"]`,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	changed, err := repo.NormalizeSkillSlug(ctx, "ws-1", oldSkill.ID, "custom-skill", "kandev-custom-skill")
	if err == nil {
		t.Fatal("normalization should fail when the canonical slug is taken")
	}
	if changed {
		t.Fatal("normalization reported a change after a rolled-back transaction")
	}

	gotOld, err := repo.GetSkillBySlug(ctx, "ws-1", "custom-skill")
	if err != nil {
		t.Fatalf("old skill row was not preserved: %v", err)
	}
	if gotOld.ID != oldSkill.ID {
		t.Fatalf("old skill ID = %q, want %q", gotOld.ID, oldSkill.ID)
	}
	gotAgent, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if gotAgent.DesiredSkills != agent.DesiredSkills {
		t.Fatalf("desired_skills = %q after rollback, want %q", gotAgent.DesiredSkills, agent.DesiredSkills)
	}
}
