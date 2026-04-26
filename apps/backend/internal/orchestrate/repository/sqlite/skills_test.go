package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestSkill_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID:   "ws-1",
		Name:          "Go Testing",
		Slug:          "go-testing",
		Description:   "Write Go tests",
		SourceType:    "inline",
		Content:       "test content",
		FileInventory: "[]",
	}
	if err := repo.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetSkill(ctx, skill.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Slug != "go-testing" {
		t.Errorf("slug = %q, want %q", got.Slug, "go-testing")
	}

	skills, err := repo.ListSkills(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("list count = %d, want 1", len(skills))
	}

	skill.Name = "Updated Skill"
	if err := repo.UpdateSkill(ctx, skill); err != nil {
		t.Fatalf("update: %v", err)
	}

	if err := repo.DeleteSkill(ctx, skill.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	skills, _ = repo.ListSkills(ctx, "ws-1")
	if len(skills) != 0 {
		t.Errorf("list after delete = %d, want 0", len(skills))
	}
}
