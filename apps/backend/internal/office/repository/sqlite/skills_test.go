package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestRunSkillSnapshotsRemainStableAfterSkillUpdate(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	skill := &models.Skill{
		ID:            "skill-1",
		WorkspaceID:   "ws-1",
		Name:          "Review",
		Slug:          "review",
		SourceType:    "inline",
		Content:       "original",
		Version:       "v1",
		ContentHash:   "hash-original",
		ApprovalState: "approved",
	}
	if err := repo.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if err := repo.CreateRunSkillSnapshots(ctx, []models.RunSkillSnapshot{{
		RunID:            "run-1",
		SkillID:          skill.ID,
		Version:          skill.Version,
		ContentHash:      skill.ContentHash,
		MaterializedPath: "/tmp/run-1/skills/review",
	}}); err != nil {
		t.Fatalf("CreateRunSkillSnapshots: %v", err)
	}

	skill.Content = "updated"
	skill.Version = "v2"
	skill.ContentHash = "hash-updated"
	if err := repo.UpdateSkill(ctx, skill); err != nil {
		t.Fatalf("UpdateSkill: %v", err)
	}

	snapshots, err := repo.ListRunSkillSnapshots(ctx, "run-1")
	if err != nil {
		t.Fatalf("ListRunSkillSnapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snapshots))
	}
	got := snapshots[0]
	if got.Version != "v1" || got.ContentHash != "hash-original" {
		t.Fatalf("snapshot changed after source update: %#v", got)
	}
}
