package configsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/office/models"
)

func TestBuildFetchedProjects_ParsesStemWarningsAndCollisions(t *testing.T) {
	files := []fetchedFile{
		{path: "projects/Website.yml", content: []byte("name: Website\ncolor: blue\nbudget_cents: 500\n")},
		{path: "projects/mismatch.yml", content: []byte("name: Other\n")},
		{path: "projects/broken.yml", content: []byte("name:\n  - not-a-string\n")},
	}
	fetched, warnings, _ := buildFetchedProjects(files)

	require.Len(t, fetched, 2)
	byKey := map[string]fetchedEntity[projectProjection]{}
	for _, f := range fetched {
		byKey[f.Key] = f
	}
	require.Contains(t, byKey, "Website")
	assert.Equal(t, "blue", byKey["Website"].Projection.Color)
	assert.Equal(t, 500, byKey["Website"].Projection.BudgetCents)

	assert.Contains(t, warnings, stemMismatchWarning(kindProject, "projects/mismatch.yml", "Other"))
	assert.Len(t, warnings, 2, "one stem-mismatch warning plus one parse-failure warning")
}

func TestBuildFetchedProjects_KeyCollisionKeepsByteWiseFirstPath(t *testing.T) {
	files := []fetchedFile{
		{path: "projects/z-dup.yml", content: []byte("name: Website\n")},
		{path: "projects/a-dup.yml", content: []byte("name: Website\n")},
	}
	fetched, warnings, _ := buildFetchedProjects(files)
	require.Len(t, fetched, 1)
	assert.Equal(t, "projects/a-dup.yml", fetched[0].SourcePath)
	assert.NotEmpty(t, warnings)
}

func TestProjectOps_CreateDefaultsStatusActiveAndRecordsManifest(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[projectProjection]{{
		Key: "Website", SourcePath: "projects/website.yml",
		Projection: projectProjection{Description: "d", Color: "blue", BudgetCents: 100},
	}}
	res, err := applyKind(ctx, repo.Writer(), store, projectOps(repo), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	require.Equal(t, []string{"Website"}, res.Created)

	projects, err := repo.ListProjects(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, models.ProjectStatusActive, projects[0].Status)
	assert.Equal(t, "blue", projects[0].Color)
}

func TestProjectOps_UpdateOnlyWritesWhenProjectionChanges(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[projectProjection]{{
		Key: "Website", SourcePath: "projects/website.yml",
		Projection: projectProjection{Description: "d", Color: "blue"},
	}}
	_, err := applyKind(ctx, repo.Writer(), store, projectOps(repo), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	// Re-run unchanged: no update counted.
	res, err := applyKind(ctx, repo.Writer(), store, projectOps(repo), "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)
	assert.Empty(t, res.Updated)

	// Change color: one update counted.
	fetched[0].Projection.Color = "red"
	res, err = applyKind(ctx, repo.Writer(), store, projectOps(repo), "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"Website"}, res.Updated)

	projects, err := repo.ListProjects(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "red", projects[0].Color)
}

func TestProjectOps_RemovedUpstreamDeletesEntity(t *testing.T) {
	repo, store := newReconcileTestRepo(t)
	ctx := context.Background()

	fetched := []fetchedEntity[projectProjection]{{Key: "Website", SourcePath: "projects/website.yml"}}
	_, err := applyKind(ctx, repo.Writer(), store, projectOps(repo), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, repo.Writer(), store, projectOps(repo), "ws-1", nil, manifest, nil, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"Website"}, res.Deleted)

	projects, err := repo.ListProjects(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, projects)
}
