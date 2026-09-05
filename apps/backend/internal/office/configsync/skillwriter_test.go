package configsync_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/office/configsync"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newSkillWriterTestRepo brings up a fully-schemaed office repository
// (including office_skills, with its real column defaults) so the sync-owned
// skill writer is tested against the same schema production code runs
// against, not a bespoke test-only table.
func newSkillWriterTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	repo, err := sqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	return repo
}

func TestCreateSkill_InsertsRowWithComputedHash(t *testing.T) {
	repo := newSkillWriterTestRepo(t)
	ctx := context.Background()

	proj := configsync.SkillProjection{
		Name:          "Code Reviewer",
		Description:   "Reviews pull requests",
		SourceType:    models.SkillSourceTypeInline,
		Content:       "# Reviewer\n",
		FileInventory: "[]",
	}
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	require.NoError(t, err)
	id, err := configsync.CreateSkill(ctx, tx, repo.Writer(), "ws-1", "reviewer", "skills/reviewer", proj)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NotEmpty(t, id)

	got, err := repo.GetSkill(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "ws-1", got.WorkspaceID)
	assert.Equal(t, "Code Reviewer", got.Name)
	assert.Equal(t, "reviewer", got.Slug)
	assert.Equal(t, "Reviews pull requests", got.Description)
	assert.Equal(t, models.SkillSourceTypeInline, got.SourceType)
	assert.Equal(t, "skills/reviewer", got.SourceLocator)
	assert.Equal(t, "# Reviewer\n", got.Content)
	assert.Equal(t, "[]", got.FileInventory)
	wantHash := models.SkillPackageContentHash(proj.Content, proj.FileInventory, "skills/reviewer")
	assert.Equal(t, wantHash, got.ContentHash)
	// Columns this writer does not own must still land at a usable default.
	assert.Equal(t, models.SkillApprovalStateApproved, got.ApprovalState)
}

func TestCreateSkill_RollbackDiscardsRow(t *testing.T) {
	repo := newSkillWriterTestRepo(t)
	ctx := context.Background()

	proj := configsync.SkillProjection{Name: "X", SourceType: models.SkillSourceTypeInline, FileInventory: "[]"}
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	require.NoError(t, err)
	id, err := configsync.CreateSkill(ctx, tx, repo.Writer(), "ws-1", "x", "skills/x", proj)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	_, err = repo.GetSkill(ctx, id)
	assert.Error(t, err)
}

func TestUpdateSkillProjection_MatchingLocatorAppliesAllSixColumns(t *testing.T) {
	repo := newSkillWriterTestRepo(t)
	ctx := context.Background()

	created := configsync.SkillProjection{
		Name: "Old Name", Description: "old", SourceType: models.SkillSourceTypeInline,
		Content: "old content", FileInventory: "[]",
	}
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	require.NoError(t, err)
	id, err := configsync.CreateSkill(ctx, tx, repo.Writer(), "ws-1", "x", "skills/x", created)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	updated := configsync.SkillProjection{
		Name: "New Name", Description: "new", SourceType: models.SkillSourceTypeInline,
		Content: "new content", FileInventory: `[{"path":"references/a.md"}]`,
	}
	tx2, err := repo.Writer().BeginTxx(ctx, nil)
	require.NoError(t, err)
	err = configsync.UpdateSkillProjection(ctx, tx2, repo.Writer(), id, "skills/x", updated)
	require.NoError(t, err)
	require.NoError(t, tx2.Commit())

	got, err := repo.GetSkill(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "New Name", got.Name)
	assert.Equal(t, "new", got.Description)
	assert.Equal(t, "new content", got.Content)
	assert.Equal(t, `[{"path":"references/a.md"}]`, got.FileInventory)
	// source_locator is never written by the update — it stays the value
	// used as the CAS guard (AC-OFFICE-CONFIG-SYNC-003.5d).
	assert.Equal(t, "skills/x", got.SourceLocator)
	wantHash := models.SkillPackageContentHash(updated.Content, updated.FileInventory, "skills/x")
	assert.Equal(t, wantHash, got.ContentHash)
}

func TestUpdateSkillProjection_LocatorChangedFailsWithoutWriting(t *testing.T) {
	repo := newSkillWriterTestRepo(t)
	ctx := context.Background()

	created := configsync.SkillProjection{
		Name: "Old Name", SourceType: models.SkillSourceTypeInline,
		Content: "old content", FileInventory: "[]",
	}
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	require.NoError(t, err)
	id, err := configsync.CreateSkill(ctx, tx, repo.Writer(), "ws-1", "x", "skills/x", created)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	// Simulate a concurrent writer moving the locator between read and write.
	_, err = repo.ExecRaw(ctx, `UPDATE office_skills SET source_locator = ? WHERE id = ?`, "skills/moved", id)
	require.NoError(t, err)

	updated := configsync.SkillProjection{
		Name: "New Name", SourceType: models.SkillSourceTypeInline,
		Content: "new content", FileInventory: "[]",
	}
	tx2, err := repo.Writer().BeginTxx(ctx, nil)
	require.NoError(t, err)
	err = configsync.UpdateSkillProjection(ctx, tx2, repo.Writer(), id, "skills/x", updated)
	assert.ErrorIs(t, err, configsync.ErrSkillLocatorChanged)
	require.NoError(t, tx2.Rollback())

	got, err := repo.GetSkill(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Old Name", got.Name, "skill must be left untouched when the CAS guard fails")
	assert.Equal(t, "skills/moved", got.SourceLocator)
}
