package skills_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestValidateSkillUpdate_RejectsEmptySlug(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	skill := &models.Skill{WorkspaceID: "ws-1", Name: "Existing", Slug: "kandev-existing", SourceType: "inline"}
	if err := svc.ValidateAndPrepareSkill(ctx, skill); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	skill.Slug = ""
	if err := svc.ValidateSkillUpdate(ctx, skill, true); err == nil {
		t.Fatal("expected error for empty slug")
	}
	if skill.Slug != "" {
		t.Errorf("slug = %q, want unchanged empty string, not coerced", skill.Slug)
	}
}

func TestValidateSkillUpdate_HealsEmptyStoredSlugWhenNotRequested(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	// Bypasses ValidateAndPrepareSkill, mirroring config-import's CreateSkill
	// call, to reproduce a durable row with an empty stored slug.
	skill := &models.Skill{WorkspaceID: "ws-1", Name: "Existing Skill", SourceType: "inline"}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.ValidateSkillUpdate(ctx, skill, false); err != nil {
		t.Fatalf("update: %v", err)
	}
	if skill.Slug == "" {
		t.Error("slug still empty after healing an unrequested update")
	}
}

func TestValidateSkillUpdate_LeavesEmptyStoredSlugOnCollisionWhenNotRequested(t *testing.T) {
	svc := newTestSkillService(t)
	ctx := context.Background()

	occupant := &models.Skill{WorkspaceID: "ws-1", Name: "Existing Skill", SourceType: "inline"}
	if err := svc.ValidateAndPrepareSkill(ctx, occupant); err != nil {
		t.Fatalf("validate occupant: %v", err)
	}
	if err := svc.CreateSkill(ctx, occupant); err != nil {
		t.Fatalf("create occupant: %v", err)
	}

	// Bypasses ValidateAndPrepareSkill, mirroring config-import's CreateSkill
	// call, to reproduce a durable row with an empty stored slug whose
	// name-derived candidate collides with an existing skill.
	broken := &models.Skill{WorkspaceID: "ws-1", Name: "Existing Skill", SourceType: "inline"}
	if err := svc.CreateSkill(ctx, broken); err != nil {
		t.Fatalf("create broken: %v", err)
	}

	if err := svc.ValidateSkillUpdate(ctx, broken, false); err != nil {
		t.Fatalf("update: %v", err)
	}
	if broken.Slug != "" {
		t.Errorf("slug = %q, want left empty on collision", broken.Slug)
	}
}
