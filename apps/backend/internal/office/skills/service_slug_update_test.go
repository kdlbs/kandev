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
	if err := svc.ValidateSkillUpdate(ctx, skill); err == nil {
		t.Fatal("expected error for empty slug")
	}
	if skill.Slug != "" {
		t.Errorf("slug = %q, want unchanged empty string, not coerced", skill.Slug)
	}
}
