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
		DisplayName:      skill.Name,
		Slug:             skill.Slug,
		LabelSource:      "captured",
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
	if got.DisplayName != "Review" || got.Slug != "review" || got.LabelSource != "captured" {
		t.Fatalf("snapshot label changed after source update: %#v", got)
	}
}

func TestRunSkillSnapshotLabelRetention(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateRunSkillSnapshots(ctx, []models.RunSkillSnapshot{{
		RunID: "run-2", SkillID: "skill-2", DisplayName: "Legacy Review", Slug: "legacy-review",
		LabelSource: "captured", Version: "v1", ContentHash: "hash", MaterializedPath: "/tmp/skills",
	}}); err != nil {
		t.Fatalf("CreateRunSkillSnapshots: %v", err)
	}
	if err := repo.CreateRunSkillSnapshots(ctx, []models.RunSkillSnapshot{{
		RunID: "run-2", SkillID: "skill-2", DisplayName: "Renamed Review", Slug: "renamed-review",
		LabelSource: "captured", Version: "v2", ContentHash: "hash-2", MaterializedPath: "/tmp/skills-2",
	}}); err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}
	snapshots, err := repo.ListRunSkillSnapshots(ctx, "run-2")
	if err != nil {
		t.Fatalf("ListRunSkillSnapshots: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].DisplayName != "Renamed Review" {
		t.Fatalf("snapshots = %#v, want one updated snapshot", snapshots)
	}
}
