package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestGenerateSkillSlug(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Code Review", "code-review"},
		{"  Go Testing  ", "go-testing"},
		{"Deploy Runbook!!!", "deploy-runbook"},
		{"MCP--server", "mcp-server"},
		{"test", "test"},
		{"", "skill"},
		{"Already-Kebab", "already-kebab"},
		{"UPPERCASE NAME", "uppercase-name"},
		{"special@#$chars", "special-chars"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.GenerateSlug(tt.name)
			if got != tt.want {
				t.Errorf("GenerateSlug(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestValidateAndPrepareSkill_AutoGeneratesSlug(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Code Review",
		SourceType:  "inline",
		Content:     "# Review code",
	}
	if err := svc.ValidateAndPrepareSkill(ctx, skill); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if skill.Slug != "code-review" {
		t.Errorf("slug = %q, want %q", skill.Slug, "code-review")
	}
}

func TestValidateAndPrepareSkill_RejectsEmptyName(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		SourceType:  "inline",
	}
	err := svc.ValidateAndPrepareSkill(ctx, skill)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateAndPrepareSkill_RejectsDuplicateSlug(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	s1 := &models.Skill{WorkspaceID: "ws-1", Name: "Test Skill", SourceType: "inline"}
	if err := svc.ValidateAndPrepareSkill(ctx, s1); err != nil {
		t.Fatalf("validate first: %v", err)
	}
	if err := svc.CreateSkill(ctx, s1); err != nil {
		t.Fatalf("create first: %v", err)
	}

	s2 := &models.Skill{WorkspaceID: "ws-1", Name: "Test Skill", SourceType: "inline"}
	err := svc.ValidateAndPrepareSkill(ctx, s2)
	if err == nil {
		t.Fatal("expected duplicate slug error")
	}
}

func TestValidateAndPrepareSkill_RejectsSameSlugInSameWorkspace(t *testing.T) {
	// With filesystem-backed config, all skills live in a single "default" workspace.
	// Duplicate slugs are always rejected.
	svc := newTestService(t)
	ctx := context.Background()

	s1 := &models.Skill{WorkspaceID: "ws-1", Name: "Test Skill", SourceType: "inline"}
	if err := svc.ValidateAndPrepareSkill(ctx, s1); err != nil {
		t.Fatalf("validate first: %v", err)
	}
	if err := svc.CreateSkill(ctx, s1); err != nil {
		t.Fatalf("create first: %v", err)
	}

	s2 := &models.Skill{WorkspaceID: "ws-1", Name: "Test Skill", SourceType: "inline"}
	err := svc.ValidateAndPrepareSkill(ctx, s2)
	if err == nil {
		t.Fatal("expected duplicate slug error in same workspace")
	}
}

func TestValidateAndPrepareSkill_RejectsInvalidSourceType(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Bad Source",
		SourceType:  "ftp",
	}
	err := svc.ValidateAndPrepareSkill(ctx, skill)
	if err == nil {
		t.Fatal("expected error for invalid source type")
	}
}

func TestListSkillsWithUsage(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	skill := &models.Skill{WorkspaceID: "ws-1", Name: "My Skill", Slug: "my-skill", SourceType: "inline"}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}

	skills, err := svc.ListSkillsWithUsage(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("count = %d, want 1", len(skills))
	}
	if skills[0].UsedByCount != 0 {
		t.Errorf("used_by_count = %d, want 0", skills[0].UsedByCount)
	}
}
