package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func fullSkill(id, workspaceID, slug string) *models.Skill {
	return &models.Skill{
		ID:            id,
		WorkspaceID:   workspaceID,
		Name:          "Review",
		Slug:          slug,
		Description:   "Reviews pull requests",
		SourceType:    models.SkillSourceTypeInline,
		SourceLocator: "skills/" + slug,
		Content:       "# Review\n",
		FileInventory: "[]",
		Version:       "v1",
		ContentHash:   "hash-1",
		ApprovalState: models.SkillApprovalStateApproved,
	}
}

func TestDeleteSkillTx_CommitRemoves(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	skill := fullSkill("skill-1", "ws-1", "review")
	if err := repo.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.DeleteSkillTx(ctx, tx, skill.ID); err != nil {
		t.Fatalf("DeleteSkillTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, err := repo.GetSkill(ctx, skill.ID); err == nil {
		t.Fatal("GetSkill: want error after committed delete, got nil")
	}
}

func TestDeleteSkillTx_RollbackKeepsRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	skill := fullSkill("skill-1", "ws-1", "review")
	if err := repo.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.DeleteSkillTx(ctx, tx, skill.ID); err != nil {
		t.Fatalf("DeleteSkillTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := repo.GetSkill(ctx, skill.ID); err != nil {
		t.Fatalf("GetSkill: want row to survive rollback, got error: %v", err)
	}
}
