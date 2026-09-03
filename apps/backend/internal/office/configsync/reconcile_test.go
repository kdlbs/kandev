package configsync

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProj is a minimal comparable projection used to exercise the generic
// six-case apply engine without standing up a real office repository kind.
type testProj struct {
	Value string
}

// fakeEntities is an in-memory stand-in for one kind's persisted rows, keyed
// by both ID and name, mirroring the shape entityOps.list must reconstruct
// from a real repository.
type fakeEntities struct {
	byID   map[string]entityRow[testProj]
	nextID int
}

func newFakeEntities() *fakeEntities {
	return &fakeEntities{byID: map[string]entityRow[testProj]{}}
}

func (f *fakeEntities) seed(key string, proj testProj) string {
	f.nextID++
	id := fmt.Sprintf("id-%d", f.nextID)
	f.byID[id] = entityRow[testProj]{ID: id, Key: key, Projection: proj}
	return id
}

func (f *fakeEntities) ops() entityOps[testProj] {
	return entityOps[testProj]{
		kind: "widget",
		list: func(_ context.Context, _ string) ([]entityRow[testProj], error) {
			rows := make([]entityRow[testProj], 0, len(f.byID))
			for _, r := range f.byID {
				rows = append(rows, r)
			}
			return rows, nil
		},
		create: func(_ context.Context, _ *sqlx.Tx, _, key, _ string, proj testProj) (string, error) {
			return f.seed(key, proj), nil
		},
		update: func(_ context.Context, _ *sqlx.Tx, id string, proj testProj) error {
			row, ok := f.byID[id]
			if !ok {
				return fmt.Errorf("no such row %q", id)
			}
			row.Projection = proj
			f.byID[id] = row
			return nil
		},
		del: func(_ context.Context, _ *sqlx.Tx, id string) error {
			delete(f.byID, id)
			return nil
		},
	}
}

func setupReconcileTestStore(t *testing.T) (*Store, *sqlx.DB) {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	require.NoError(t, err)
	return store, db
}

func TestApplyKind_NewKeyWithNoManifestEntryIsCreated(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	fetched := []fetchedEntity[testProj]{{Key: "ceo", SourcePath: "agents/ceo.yml", Projection: testProj{Value: "v1"}}}
	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"ceo"}, res.Created)
	assert.Empty(t, res.Updated)
	assert.Empty(t, res.Deleted)
	assert.Empty(t, res.Warnings)
	id, ok := res.IDsByKey["ceo"]
	require.True(t, ok)

	entries, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "ceo", entries[0].EntityKey)
	assert.Equal(t, id, entries[0].EntityID)
	assert.Equal(t, "agents/ceo.yml", entries[0].SourcePath)
}

func TestApplyKind_ExistingKeyWithChangedProjectionIsUpdated(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	id := entities.seed("ceo", testProj{Value: "old"})
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "widget", "ceo", id, "agents/ceo.yml"))

	fetched := []fetchedEntity[testProj]{{Key: "ceo", SourcePath: "agents/ceo.yml", Projection: testProj{Value: "new"}}}
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)

	assert.Empty(t, res.Created)
	assert.Equal(t, []string{"ceo"}, res.Updated)
	assert.Equal(t, "new", entities.byID[id].Projection.Value)
}

func TestApplyKind_ExistingKeyWithUnchangedProjectionAndSamePathIsSkipped(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	id := entities.seed("ceo", testProj{Value: "same"})
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "widget", "ceo", id, "agents/ceo.yml"))

	fetched := []fetchedEntity[testProj]{{Key: "ceo", SourcePath: "agents/ceo.yml", Projection: testProj{Value: "same"}}}
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)

	assert.Empty(t, res.Created)
	assert.Empty(t, res.Updated, "unchanged projection at the same path must not count as a write")
}

func TestApplyKind_ExistingKeyWithUnchangedProjectionButMovedPathRefreshesManifestOnly(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	id := entities.seed("ceo", testProj{Value: "same"})
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "widget", "ceo", id, "agents/old.yml"))

	fetched := []fetchedEntity[testProj]{{Key: "ceo", SourcePath: "agents/new.yml", Projection: testProj{Value: "same"}}}
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)

	assert.Empty(t, res.Updated, "no owned field changed, so this is not a counted update")
	entries, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "agents/new.yml", entries[0].SourcePath)
}

func TestApplyKind_ForeignKeyLeavesBothUntouchedWithWarning(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	unmanagedID := entities.seed("ceo", testProj{Value: "hand-made"})

	fetched := []fetchedEntity[testProj]{{Key: "ceo", SourcePath: "agents/ceo.yml", Projection: testProj{Value: "fetched"}}}
	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)

	assert.Empty(t, res.Created)
	assert.Empty(t, res.Updated)
	require.Len(t, res.Warnings, 1)
	assert.Equal(t, "hand-made", entities.byID[unmanagedID].Projection.Value)
	assert.True(t, res.ForeignKeys["ceo"],
		"a Foreign-collision loser must be recorded so the agent kind's reports_to pass can distinguish it "+
			"from a name that appears nowhere (AC-OFFICE-CONFIG-SYNC-003.10b)")

	entries, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, entries, "a foreign key must not be recorded into the manifest")
}

func TestApplyKind_RemovedUpstreamDeletesEntityAndManifestRow(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	id := entities.seed("intern", testProj{Value: "v"})
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "widget", "intern", id, "agents/intern.yml"))
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", nil, manifest, nil, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"intern"}, res.Deleted)
	_, stillPresent := entities.byID[id]
	assert.False(t, stillPresent)
	entries, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestApplyKind_ExemptKeyIsSkippedWithWarningAndManifestKept(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	id := entities.seed("intern", testProj{Value: "v"})
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "widget", "intern", id, "agents/intern.yml"))
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", nil, manifest, map[string]bool{"intern": true}, false)
	require.NoError(t, err)

	assert.Empty(t, res.Deleted)
	require.Len(t, res.Warnings, 1)
	_, stillPresent := entities.byID[id]
	assert.True(t, stillPresent, "an exempt key must be left in place")
	entries, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	assert.Len(t, entries, 1, "an exempt key's manifest row must be kept")
}

func TestApplyKind_CoarseExemptSkipsEveryDeletionThisRun(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	id := entities.seed("intern", testProj{Value: "v"})
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "widget", "intern", id, "agents/intern.yml"))
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", nil, manifest, nil, true)
	require.NoError(t, err)

	assert.Empty(t, res.Deleted)
	require.Len(t, res.Warnings, 1)
	_, stillPresent := entities.byID[id]
	assert.True(t, stillPresent)
}

func TestApplyKind_GoneOutOfBandDropsStaleManifestRowOnly(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	// The manifest still names an entity that no longer exists (deleted out
	// of band, e.g. through Office's own UI).
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "widget", "intern", "ghost-id", "agents/intern.yml"))
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", nil, manifest, nil, false)
	require.NoError(t, err)

	assert.Empty(t, res.Deleted, "gone-out-of-band is a manifest cleanup, not a counted deletion")
	assert.Empty(t, res.Warnings)
	entries, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestApplyKind_KeyBothFetchedAndManifestedButEntityGoneIsTreatedAsNew(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	// Manifest points at an entity ID that no longer exists among existing
	// rows (e.g. deleted out of band), while the key is also freshly
	// fetched this run. The documented Build assumption folds this into
	// decisionNew rather than a special case.
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "widget", "ceo", "ghost-id", "agents/ceo.yml"))
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	fetched := []fetchedEntity[testProj]{{Key: "ceo", SourcePath: "agents/ceo.yml", Projection: testProj{Value: "v1"}}}
	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"ceo"}, res.Created)
	entries, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, res.IDsByKey["ceo"], entries[0].EntityID)
}

func TestApplyKind_KeyBothFetchedAndManifestedButEntityGoneAndUnmanagedRowHoldsKeyIsForeign(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	// The manifest still names an entity that no longer exists (deleted out
	// of band), and a *different*, unmanaged entity was separately created
	// under the same key. The manifest's stale pointer must not blind the
	// collision check: this is AC-OFFICE-CONFIG-SYNC-003.7's Foreign case,
	// not a fresh create.
	unmanagedID := entities.seed("ceo", testProj{Value: "human-made"})
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "widget", "ceo", "ghost-id", "agents/ceo.yml"))
	manifest, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)

	fetched := []fetchedEntity[testProj]{{Key: "ceo", SourcePath: "agents/ceo.yml", Projection: testProj{Value: "v1"}}}
	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", fetched, manifest, nil, false)
	require.NoError(t, err)

	assert.Empty(t, res.Created, "must not silently create a duplicate")
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "ceo")
	assert.Len(t, entities.byID, 1, "the unmanaged row must be the only row left")
	unmanagedRow, stillPresent := entities.byID[unmanagedID]
	assert.True(t, stillPresent)
	assert.Equal(t, testProj{Value: "human-made"}, unmanagedRow.Projection, "must not be adopted or modified")
}

func TestApplyKind_MultipleFetchedKeysAppliedInSourcePathOrder(t *testing.T) {
	store, db := setupReconcileTestStore(t)
	entities := newFakeEntities()
	ctx := context.Background()

	fetched := []fetchedEntity[testProj]{
		{Key: "zed", SourcePath: "agents/z.yml", Projection: testProj{Value: "z"}},
		{Key: "ann", SourcePath: "agents/a.yml", Projection: testProj{Value: "a"}},
	}
	res, err := applyKind(ctx, db, store, entities.ops(), "ws-1", fetched, nil, nil, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"ann", "zed"}, res.Created)
}
