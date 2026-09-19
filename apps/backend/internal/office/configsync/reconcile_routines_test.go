package configsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/office/models"
)

func TestBuildFetchedRoutines_ParsesStemWarningsAndConcurrencyPolicy(t *testing.T) {
	files := []fetchedFile{
		{path: "routines/Nightly.yml", content: []byte("name: Nightly\ntask_template: run tests\nconcurrency_policy: coalesce_if_active\n")},
		{path: "routines/mismatch.yml", content: []byte("name: Other\n")},
		{path: "routines/broken.yml", content: []byte("name:\n  - not-a-string\n")},
	}
	fetched, warnings, _ := buildFetchedRoutines(files)

	require.Len(t, fetched, 2)
	byKey := map[string]fetchedEntity[routineProjection]{}
	for _, f := range fetched {
		byKey[f.Key] = f
	}
	require.Contains(t, byKey, "Nightly")
	assert.Equal(t, "run tests", byKey["Nightly"].Projection.TaskTemplate)
	assert.Equal(t, models.ConcurrencyPolicyCoalesceIfActive, byKey["Nightly"].Projection.ConcurrencyPolicy)

	assert.Contains(t, warnings, stemMismatchWarning(kindRoutine, "routines/mismatch.yml", "Other"))
	assert.Len(t, warnings, 2, "one stem-mismatch warning plus one parse-failure warning")
}

func TestBuildFetchedRoutines_KeyCollisionKeepsByteWiseFirstPath(t *testing.T) {
	files := []fetchedFile{
		{path: "routines/z-dup.yml", content: []byte("name: Nightly\n")},
		{path: "routines/a-dup.yml", content: []byte("name: Nightly\n")},
	}
	fetched, warnings, _ := buildFetchedRoutines(files)
	require.Len(t, fetched, 1)
	assert.Equal(t, "routines/a-dup.yml", fetched[0].SourcePath)
	assert.NotEmpty(t, warnings)
}

func TestRoutineOps_CreateDefaultsStatusActiveAndRecordsManifest(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[routineProjection]{{
		Key: "Nightly", SourcePath: "routines/nightly.yml",
		Projection: routineProjection{Description: "d", TaskTemplate: "run tests", ConcurrencyPolicy: models.ConcurrencyPolicySkipIfActive},
	}}
	res, err := applyKind(ctx, repo.Writer(), store, routineOps(repo), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	require.Equal(t, []string{"Nightly"}, res.Created)

	routines, err := repo.ListRoutines(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, routines, 1)
	assert.Equal(t, "active", routines[0].Status)
	assert.Equal(t, "run tests", routines[0].TaskTemplate)
}

func TestRoutineOps_UpdateOnlyWritesWhenProjectionChanges(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[routineProjection]{{
		Key: "Nightly", SourcePath: "routines/nightly.yml",
		Projection: routineProjection{TaskTemplate: "old", ConcurrencyPolicy: models.ConcurrencyPolicySkipIfActive},
	}}
	_, err := applyKind(ctx, repo.Writer(), store, routineOps(repo), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, repo.Writer(), store, routineOps(repo), "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)
	assert.Empty(t, res.Updated)

	fetched[0].Projection.TaskTemplate = "new"
	res, err = applyKind(ctx, repo.Writer(), store, routineOps(repo), "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"Nightly"}, res.Updated)

	routines, err := repo.ListRoutines(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, routines, 1)
	assert.Equal(t, "new", routines[0].TaskTemplate)
}

func TestRoutineOps_RemovedUpstreamDeletesEntity(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[routineProjection]{{Key: "Nightly", SourcePath: "routines/nightly.yml"}}
	_, err := applyKind(ctx, repo.Writer(), store, routineOps(repo), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, repo.Writer(), store, routineOps(repo), "ws-1", nil, manifest, nil, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"Nightly"}, res.Deleted)

	routines, err := repo.ListRoutines(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, routines)
}
