package configsync

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/office/models"
)

// TestApplyKindCreatesOnly_MidKindFailurePreservesPriorWarnings proves
// applyKindCreatesOnly does not discard warnings already accumulated earlier
// in the same kind when a later key's write fails: forwardPass
// (reconcile_run.go) assigns this call's return value straight onto
// kr.<kind>, and partialWarnings only surfaces warnings from a non-nil
// kindApplyResult, so a `nil, err` return here erases every warning the kind
// produced before the failing key (AC-OFFICE-CONFIG-SYNC-004.5b requires the
// run's failure to retain warnings produced before it gave up).
func TestApplyKindCreatesOnly_MidKindFailurePreservesPriorWarnings(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	// An unmanaged row already occupies "ceo": fetching it again is
	// decisionForeign and produces a warning without erroring.
	entities.seed("ceo", testProj{Value: "hand-made"})

	failOn := "intern"
	ops := entities.ops()
	baseCreate := ops.create
	ops.create = func(ctx context.Context, tx *sqlx.Tx, workspaceID, key, sourcePath string, proj testProj) (string, error) {
		if key == failOn {
			return "", errors.New("forced create failure for test")
		}
		return baseCreate(ctx, tx, workspaceID, key, sourcePath, proj)
	}

	fetched := []fetchedEntity[testProj]{
		{Key: "ceo", SourcePath: "agents/a-ceo.yml", Projection: testProj{Value: "fetched"}},
		{Key: failOn, SourcePath: "agents/b-intern.yml", Projection: testProj{Value: "v1"}},
	}

	res, err := applyKindCreatesOnly(ctx, db, store, ops, "ws-1", fetched, nil)
	require.Error(t, err)
	require.NotNil(t, res, "a mid-kind failure must still return the partial result, not discard it")
	require.Len(t, res.Warnings, 1, "the Foreign warning produced before the failing key must survive")
	assert.Contains(t, res.Warnings[0], "ceo")
}

// TestApplySkillsCreatesOnly_MidKindFailurePreservesPriorWarnings is the
// skills-kind counterpart: applySkillsCreatesOnly has the same
// entityOps-free bespoke apply path (reconcile_skills.go) and must preserve
// the same guarantee.
func TestApplySkillsCreatesOnly_MidKindFailurePreservesPriorWarnings(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	// An unmanaged skill already occupies "reviewer": fetching it again is
	// decisionForeign and produces a warning without erroring.
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = CreateSkill(ctx, tx, repo.Writer(), "ws-1", "reviewer", "",
		SkillProjection{Name: "Human Made", SourceType: models.SkillSourceTypeInline, Content: "human", FileInventory: "[]"})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	_, err = repo.ExecRaw(ctx, `
		CREATE TRIGGER fail_skill_create BEFORE INSERT ON office_skills
		WHEN NEW.slug = 'zzz-fail'
		BEGIN
			SELECT RAISE(FAIL, 'forced creation failure for test');
		END;
	`)
	require.NoError(t, err)

	fetched := []fetchedSkill{
		{Key: "reviewer", SourcePath: "skills/a-reviewer", Proj: SkillProjection{
			Name: "Synced Name", SourceType: models.SkillSourceTypeInline, Content: "synced", FileInventory: "[]",
		}},
		{Key: "zzz-fail", SourcePath: "skills/b-zzz-fail", Proj: SkillProjection{
			Name: "Zzz", SourceType: models.SkillSourceTypeInline, Content: "z", FileInventory: "[]",
		}},
	}

	res, err := applySkillsCreatesOnly(ctx, repo, store, "ws-1", fetched, nil)
	require.Error(t, err)
	require.NotNil(t, res, "a mid-kind failure must still return the partial result, not discard it")
	require.Len(t, res.Warnings, 1, "the Foreign warning produced before the failing key must survive")
	assert.Contains(t, res.Warnings[0], "reviewer")
}
