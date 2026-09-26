package sqlite_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestUpdateSkillConfigFields_UpdatesOnlyOwnedColumns proves the
// column-scoped writer touches only name, description, source_type, content,
// and the hash derived from content. It leaves approval_state and package
// metadata, which a config import does not own, exactly as they were.
func TestUpdateSkillConfigFields_UpdatesOnlyOwnedColumns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	skill := &models.Skill{
		ID:                      "skill-1",
		WorkspaceID:             "ws-1",
		Name:                    "Code Review",
		Slug:                    "code-review",
		Description:             "reviews diffs",
		SourceType:              "inline",
		SourceLocator:           "local://code-review",
		Content:                 "# Code Review\n",
		FileInventory:           `["SKILL.md"]`,
		Version:                 "v1",
		ContentHash:             "hash-v1",
		ApprovalState:           "approved",
		CreatedByAgentProfileID: "agent-1",
		IsSystem:                false,
		SystemVersion:           "",
		DefaultForRoles:         `["cto"]`,
	}
	if err := repo.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	fields := sqlite.SkillConfigFields{
		Name:        "Deep Review",
		Description: "reads everything",
		SourceType:  models.SkillSourceType("git"),
		Content:     "# Deep Review\n",
	}
	if err := repo.UpdateSkillConfigFields(ctx, skill.ID, fields); err != nil {
		t.Fatalf("UpdateSkillConfigFields: %v", err)
	}

	got, err := repo.GetSkill(ctx, skill.ID)
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}

	// Owned columns landed.
	if got.Name != fields.Name {
		t.Errorf("Name = %q, want %q", got.Name, fields.Name)
	}
	if got.Description != fields.Description {
		t.Errorf("Description = %q, want %q", got.Description, fields.Description)
	}
	if got.SourceType != fields.SourceType {
		t.Errorf("SourceType = %q, want %q", got.SourceType, fields.SourceType)
	}
	if got.Content != fields.Content {
		t.Errorf("Content = %q, want %q", got.Content, fields.Content)
	}

	// Everything else stayed exactly as seeded.
	if got.Slug != skill.Slug {
		t.Errorf("Slug changed: got %q, want %q", got.Slug, skill.Slug)
	}
	if got.SourceLocator != skill.SourceLocator {
		t.Errorf("SourceLocator changed: got %q, want %q", got.SourceLocator, skill.SourceLocator)
	}
	if got.FileInventory != skill.FileInventory {
		t.Errorf("FileInventory changed: got %q, want %q", got.FileInventory, skill.FileInventory)
	}
	if got.Version != skill.Version {
		t.Errorf("Version changed: got %q, want %q", got.Version, skill.Version)
	}
	wantHashBytes := sha256.Sum256([]byte(fields.Content + "\x00" + skill.FileInventory + "\x00" + skill.SourceLocator))
	wantHash := hex.EncodeToString(wantHashBytes[:])
	if got.ContentHash != wantHash {
		t.Errorf("ContentHash = %q, want %q for updated content", got.ContentHash, wantHash)
	}
	if got.ApprovalState != skill.ApprovalState {
		t.Errorf("ApprovalState changed: got %q, want %q", got.ApprovalState, skill.ApprovalState)
	}
	if got.DefaultForRoles != skill.DefaultForRoles {
		t.Errorf("DefaultForRoles changed: got %q, want %q", got.DefaultForRoles, skill.DefaultForRoles)
	}
}
