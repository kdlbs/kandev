package configsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/office/models"
)

func skillMDFile(path, content string) *fetchedFile {
	return &fetchedFile{path: path, content: []byte(content)}
}

func TestBuildFetchedSkills_ReadsFrontmatterAndFallsBackNameToDirName(t *testing.T) {
	dirs := []skillFiles{
		{
			dirName: "reviewer", dirPath: "skills/reviewer",
			skillMD: skillMDFile("skills/reviewer/SKILL.md", "---\nname: Code Reviewer\ndescription: Reviews PRs\n---\nbody"),
		},
		{
			dirName: "no-frontmatter", dirPath: "skills/no-frontmatter",
			skillMD: skillMDFile("skills/no-frontmatter/SKILL.md", "just a body, no frontmatter"),
		},
	}
	fetched, warnings, _ := buildFetchedSkills(dirs)
	require.Len(t, fetched, 2)
	assert.Empty(t, warnings)

	byKey := map[string]fetchedSkill{}
	for _, f := range fetched {
		byKey[f.Key] = f
	}
	require.Contains(t, byKey, "reviewer")
	assert.Equal(t, "Code Reviewer", byKey["reviewer"].Proj.Name)
	assert.Equal(t, "Reviews PRs", byKey["reviewer"].Proj.Description)
	assert.Equal(t, models.SkillSourceTypeInline, byKey["reviewer"].Proj.SourceType)

	require.Contains(t, byKey, "no-frontmatter")
	assert.Equal(t, "no-frontmatter", byKey["no-frontmatter"].Proj.Name, "missing frontmatter name falls back to the directory name")
}

func TestBuildFetchedSkills_MissingSkillMDIsExcludedButNotWarnedHere(t *testing.T) {
	dirs := []skillFiles{
		{dirName: "empty-dir", dirPath: "skills/empty-dir", skillMD: nil},
	}
	fetched, warnings, _ := buildFetchedSkills(dirs)
	assert.Empty(t, fetched)
	assert.Empty(t, warnings, "the walk-phase warning for a missing SKILL.md is emitted by skillMissingDefinitionWarnings, not here")
}

func TestSkillMissingDefinitionWarnings_NamesDirectoryWithNoSkillMD(t *testing.T) {
	dirs := []skillFiles{
		{dirName: "empty-dir", dirPath: "skills/empty-dir", skillMD: nil},
	}
	warnings := skillMissingDefinitionWarnings(dirs)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "skills/empty-dir")
	assert.Contains(t, warnings[0], "SKILL.md")
}

func TestSkillMissingDefinitionWarnings_SilentWhenSkillMDPresent(t *testing.T) {
	dirs := []skillFiles{
		{dirName: "reviewer", dirPath: "skills/reviewer", skillMD: skillMDFile("skills/reviewer/SKILL.md", "body")},
	}
	assert.Empty(t, skillMissingDefinitionWarnings(dirs))
}

func TestSkillMissingDefinitionWarnings_SilentWhenSkillMDUnreadable(t *testing.T) {
	dirs := []skillFiles{
		{
			dirName: "reviewer", dirPath: "skills/reviewer",
			skillMDUnread: &unreadableFile{path: "skills/reviewer/SKILL.md", reason: "over size limit"},
		},
	}
	assert.Empty(t, skillMissingDefinitionWarnings(dirs), "an unreadable SKILL.md is warned by skillFetchWarnings instead")
}

func TestBuildFetchedSkills_KeyCollisionKeepsByteWiseFirstPath(t *testing.T) {
	dirs := []skillFiles{
		{dirName: "reviewer", dirPath: "skills/z/reviewer", skillMD: skillMDFile("skills/z/reviewer/SKILL.md", "---\nname: R\n---\n")},
		{dirName: "reviewer", dirPath: "skills/a/reviewer", skillMD: skillMDFile("skills/a/reviewer/SKILL.md", "---\nname: R\n---\n")},
	}
	fetched, warnings, _ := buildFetchedSkills(dirs)
	require.Len(t, fetched, 1)
	assert.Equal(t, "skills/a/reviewer", fetched[0].SourcePath)
	assert.NotEmpty(t, warnings)
}

func TestSkillDeletionExemptions_UnreadableRefsAloneDoNotExemptAnything(t *testing.T) {
	// AC-OFFICE-CONFIG-SYNC-003.6a's ambiguity does not exist when SKILL.md
	// itself is readable: the directory's identity is known regardless of
	// whether its reference files could be read. Only skillMDUnread and a
	// parse failure (unparsedDirs) create the ambiguity that justifies
	// exempting anything.
	dirs := []skillFiles{{
		dirName: "known", dirPath: "skills/known",
		skillMD:        skillMDFile("skills/known/SKILL.md", "---\nname: known\n---\n"),
		unreadableRefs: []unreadableFile{{path: "skills/known/references/big.md", reason: "over size limit"}},
	}}
	exemptKeys, coarse := skillDeletionExemptions(dirs, nil, nil)
	assert.False(t, coarse, "a known-identity skill's unreadable reference file must not trigger the whole-kind fallback")
	assert.Empty(t, exemptKeys)
}

func TestSkillDeletionExemptions_UnreadableSkillMDMatchingManifestExemptsOnlyThatKey(t *testing.T) {
	dirs := []skillFiles{{
		dirName: "flaky", dirPath: "skills/flaky",
		skillMDUnread: &unreadableFile{path: "skills/flaky/SKILL.md", reason: "network error"},
	}}
	manifest := []ManifestEntry{{EntityKey: "flaky", SourcePath: "skills/flaky"}}
	exemptKeys, coarse := skillDeletionExemptions(dirs, nil, manifest)
	assert.False(t, coarse)
	assert.True(t, exemptKeys["flaky"])
}

func TestSkillDeletionExemptions_UnreadableSkillMDMatchingNoManifestEntryIsCoarse(t *testing.T) {
	dirs := []skillFiles{{
		dirName: "new-name", dirPath: "skills/new-name",
		skillMDUnread: &unreadableFile{path: "skills/new-name/SKILL.md", reason: "network error"},
	}}
	manifest := []ManifestEntry{{EntityKey: "old-name", SourcePath: "skills/old-name"}}
	exemptKeys, coarse := skillDeletionExemptions(dirs, nil, manifest)
	assert.True(t, coarse)
	assert.Empty(t, exemptKeys)
}

func TestSkillDeletionExemptions_UnparsedDirMatchingManifestExemptsOnlyThatKey(t *testing.T) {
	manifest := []ManifestEntry{{EntityKey: "broken", SourcePath: "skills/broken"}}
	exemptKeys, coarse := skillDeletionExemptions(nil, []string{"skills/broken"}, manifest)
	assert.False(t, coarse)
	assert.True(t, exemptKeys["broken"])
}

func TestSkillDeletionExemptions_UnparsedDirMatchingNoManifestEntryIsCoarse(t *testing.T) {
	manifest := []ManifestEntry{{EntityKey: "old-name", SourcePath: "skills/old-name"}}
	exemptKeys, coarse := skillDeletionExemptions(nil, []string{"skills/new-name"}, manifest)
	assert.True(t, coarse)
	assert.Empty(t, exemptKeys)
}

func TestApplySkills_CreateRecordsManifestAndInlineSourceType(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedSkill{{
		Key: "reviewer", SourcePath: "skills/reviewer",
		Proj: SkillProjection{Name: "Reviewer", Description: "d", SourceType: models.SkillSourceTypeInline, Content: "body", FileInventory: "[]"},
	}}
	res, err := applySkills(ctx, repo, store, "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	require.Equal(t, []string{"reviewer"}, res.Created)

	skills, err := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "reviewer", skills[0].Slug)
	assert.Equal(t, models.SkillSourceTypeInline, skills[0].SourceType, "R5-F2: sync always writes inline, materializeInline ignores source_locator")
	assert.Equal(t, "skills/reviewer", skills[0].SourceLocator)

	entries, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, skills[0].ID, entries[0].EntityID)
}

func TestApplySkills_UpdateOnlyWritesWhenProjectionChanges(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedSkill{{
		Key: "reviewer", SourcePath: "skills/reviewer",
		Proj: SkillProjection{Name: "Reviewer", SourceType: models.SkillSourceTypeInline, Content: "old", FileInventory: "[]"},
	}}
	_, err := applySkills(ctx, repo, store, "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applySkills(ctx, repo, store, "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)
	assert.Empty(t, res.Updated, "identical content must not count as a write")

	fetched[0].Proj.Content = "new"
	res, err = applySkills(ctx, repo, store, "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"reviewer"}, res.Updated)

	skills, err := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "new", skills[0].Content)
}

// TestApplySkills_LocatorChangedConcurrentlyWarnsAndLeavesRowUntouched
// exercises updateSkillEntityIfChanged directly with a deliberately stale
// existing.SourceLocator. applySkills itself re-reads office_skills fresh at
// the start of every call (via ListSkills), so within one synchronous call
// there is no way for a real concurrent write to land between that read and
// the guarded UPDATE — the race this CAS guards against spans two separate
// sync runs (or a sync run and Office's own UI), not two lines of one call.
func TestApplySkills_LocatorChangedConcurrentlyWarnsAndLeavesRowUntouched(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedSkill{{
		Key: "reviewer", SourcePath: "skills/reviewer",
		Proj: SkillProjection{Name: "Old Name", SourceType: models.SkillSourceTypeInline, Content: "old", FileInventory: "[]"},
	}}
	res, err := applySkills(ctx, repo, store, "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	id := res.IDsByKey["reviewer"]

	// A writer outside this run (another sync run, or Office's own package
	// materialization) moves the locator after this run's stale read below.
	_, err = repo.ExecRaw(ctx, `UPDATE office_skills SET source_locator = ? WHERE id = ?`, "skills/moved", id)
	require.NoError(t, err)

	stale := &models.Skill{
		ID: id, SourceLocator: "skills/reviewer",
		Name: "Old Name", SourceType: models.SkillSourceTypeInline, Content: "old", FileInventory: "[]",
	}
	updated := fetchedSkill{
		Key: "reviewer", SourcePath: "skills/reviewer",
		Proj: SkillProjection{Name: "New Name", SourceType: models.SkillSourceTypeInline, Content: "new", FileInventory: "[]"},
	}
	changed, warning, err := updateSkillEntityIfChanged(ctx, repo, store, "ws-1", updated, stale, "skills/reviewer")
	require.NoError(t, err)
	assert.False(t, changed)
	assert.NotEmpty(t, warning)

	skills, err := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "Old Name", skills[0].Name, "CAS failure must leave the row untouched for the next run")
}

func TestApplySkills_StaleManifestAndUnmanagedRowHoldingKeyIsForeign(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	// A different, unmanaged skill already occupies the "reviewer" slug —
	// created directly, never through sync (no manifest entry).
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	require.NoError(t, err)
	unmanagedID, err := CreateSkill(ctx, tx, repo.Writer(), "ws-1", "reviewer", "",
		SkillProjection{Name: "Human Made", SourceType: models.SkillSourceTypeInline, Content: "human", FileInventory: "[]"})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	// The manifest still names an entity that no longer exists (deleted out
	// of band), separate from the unmanaged row above.
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", kindSkill, "reviewer", "ghost-id", "skills/reviewer"))
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	fetched := []fetchedSkill{{
		Key: "reviewer", SourcePath: "skills/reviewer",
		Proj: SkillProjection{Name: "Synced Name", SourceType: models.SkillSourceTypeInline, Content: "synced", FileInventory: "[]"},
	}}
	res, err := applySkills(ctx, repo, store, "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)

	assert.Empty(t, res.Created, "must not silently create a duplicate")
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "reviewer")

	skills, err := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, unmanagedID, skills[0].ID)
	assert.Equal(t, "Human Made", skills[0].Name, "must not be adopted or modified")
}

func TestApplySkills_DeletionDatabaseFailureIsReturnedNotWarned(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedSkill{{
		Key: "reviewer", SourcePath: "skills/reviewer",
		Proj: SkillProjection{Name: "Reviewer", SourceType: models.SkillSourceTypeInline, FileInventory: "[]"},
	}}
	_, err := applySkills(ctx, repo, store, "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	_, err = repo.ExecRaw(ctx, `
		CREATE TRIGGER fail_skill_delete BEFORE DELETE ON office_skills
		WHEN OLD.slug = 'reviewer'
		BEGIN
			SELECT RAISE(FAIL, 'forced deletion failure for test');
		END;
	`)
	require.NoError(t, err)

	res, err := applySkills(ctx, repo, store, "ws-1", nil, manifest, nil, false)
	require.Error(t, err, "a real DB failure deleting a managed skill must be a run failure, not a swallowed warning")
	assert.Empty(t, res.Deleted)

	skills, listErr := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, listErr)
	require.Len(t, skills, 1, "the row must still exist since the delete never committed")
}

func TestApplySkills_RemovedUpstreamDeletesEntity(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedSkill{{
		Key: "reviewer", SourcePath: "skills/reviewer",
		Proj: SkillProjection{Name: "Reviewer", SourceType: models.SkillSourceTypeInline, FileInventory: "[]"},
	}}
	_, err := applySkills(ctx, repo, store, "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applySkills(ctx, repo, store, "ws-1", nil, manifest, nil, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"reviewer"}, res.Deleted)

	skills, err := repo.ListSkills(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, skills)
}
