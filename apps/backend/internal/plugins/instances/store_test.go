package instances

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	userstore "github.com/kandev/kandev/internal/user/store"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "instances.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	pool := db.NewPool(sqlx.NewDb(conn, "sqlite3"), sqlx.NewDb(conn, "sqlite3"))
	t.Cleanup(func() { _ = pool.Close() })
	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func TestValidateScopeRequiresOnlyIdentifiersForScope(t *testing.T) {
	if err := ValidateScope(ScopeTask, ScopeIdentifiers{WorkspaceID: "workspace-1", TaskID: "task-1"}); err != nil {
		t.Fatalf("task scope unexpectedly invalid: %v", err)
	}
	if err := ValidateScope(ScopeTask, ScopeIdentifiers{TaskID: "task-1"}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("missing workspace error = %v, want ErrInvalidScope", err)
	}
	if err := ValidateScope(ScopeWorkspace, ScopeIdentifiers{WorkspaceID: "workspace-1", TaskID: "task-1"}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("mixed workspace error = %v, want ErrInvalidScope", err)
	}
}

func TestInstanceDataScopeIsIndependentAndDefaultsToPlacement(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, Instance{
		ID: "workspace-data", PluginID: "canvas-board", SourceKind: SourceLocalCanvas,
		ScopeKind: ScopeTask, DataScopeKind: ScopeWorkspace, WorkspaceID: "workspace-1",
		TaskID: "task-1", Status: StatusActive,
	}); err != nil {
		t.Fatalf("Create workspace-data task canvas: %v", err)
	}
	workspaceData, err := store.Get(ctx, "workspace-data")
	if err != nil {
		t.Fatalf("Get workspace-data task canvas: %v", err)
	}
	if workspaceData.ScopeKind != ScopeTask || workspaceData.EffectiveDataScopeKind() != ScopeWorkspace {
		t.Fatalf("scopes = placement %q, effective data %q, want task and workspace", workspaceData.ScopeKind, workspaceData.EffectiveDataScopeKind())
	}

	if err := store.Create(ctx, Instance{
		ID: "legacy", PluginID: "canvas-board", SourceKind: SourceLocalCanvas,
		ScopeKind: ScopeTask, WorkspaceID: "workspace-1", TaskID: "task-2", Status: StatusActive,
	}); err != nil {
		t.Fatalf("Create legacy task canvas: %v", err)
	}
	legacy, err := store.Get(ctx, "legacy")
	if err != nil {
		t.Fatalf("Get legacy task canvas: %v", err)
	}
	if legacy.DataScopeKind != "" || legacy.EffectiveDataScopeKind() != ScopeTask {
		t.Fatalf("legacy scopes = explicit %q, effective %q, want empty and task", legacy.DataScopeKind, legacy.EffectiveDataScopeKind())
	}
}

func TestInstanceRejectsUntrustedDataScopeCombinations(t *testing.T) {
	store := newTestStore(t)
	for _, instance := range []Instance{
		{ID: "installed", PluginID: "plugin", SourceKind: SourceInstalled, ScopeKind: ScopeTask, DataScopeKind: ScopeWorkspace, WorkspaceID: "workspace-1", TaskID: "task-1", Status: StatusActive},
		{ID: "session-data", PluginID: "canvas", SourceKind: SourceLocalCanvas, ScopeKind: ScopeTask, DataScopeKind: ScopeSession, WorkspaceID: "workspace-1", TaskID: "task-2", Status: StatusActive},
		{ID: "instance", PluginID: "canvas", SourceKind: SourceLocalCanvas, ScopeKind: ScopeTask, DataScopeKind: ScopeInstance, WorkspaceID: "workspace-1", TaskID: "task-3", Status: StatusActive},
	} {
		if err := store.Create(context.Background(), instance); !errors.Is(err, ErrInvalidScope) {
			t.Errorf("Create(%s) = %v, want ErrInvalidScope", instance.ID, err)
		}
	}
}

func TestExistingInstanceTableMigratesWithTaskDataScopeFallback(t *testing.T) {
	connection, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "legacy-instances.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	connection.SetMaxOpenConns(1)
	pool := db.NewPool(sqlx.NewDb(connection, "sqlite3"), sqlx.NewDb(connection, "sqlite3"))
	t.Cleanup(func() { _ = pool.Close() })
	if _, err := pool.Writer().Exec(`CREATE TABLE plugin_instances (
		id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL, source_kind TEXT NOT NULL, scope_kind TEXT NOT NULL,
		workspace_id TEXT NOT NULL DEFAULT '', task_id TEXT NOT NULL DEFAULT '', session_id TEXT NOT NULL DEFAULT '',
		repository_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, active_release_id TEXT NOT NULL DEFAULT '',
		grant_generation INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy plugin_instances schema: %v", err)
	}
	created := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Writer().Exec(`INSERT INTO plugin_instances
		(id, plugin_id, source_kind, scope_kind, workspace_id, task_id, status, created_at, updated_at)
		VALUES ('legacy', 'canvas', 'local_canvas', 'task', 'workspace-1', 'task-1', 'active', ?, ?)`, created, created); err != nil {
		t.Fatalf("insert legacy instance: %v", err)
	}
	store, err := NewStore(pool)
	if err != nil {
		t.Fatalf("NewStore migrates legacy table: %v", err)
	}
	instance, err := store.Get(context.Background(), "legacy")
	if err != nil {
		t.Fatalf("Get legacy instance: %v", err)
	}
	if instance.DataScopeKind != "" || instance.EffectiveDataScopeKind() != ScopeTask {
		t.Fatalf("legacy scopes = explicit %q, effective %q, want empty and task", instance.DataScopeKind, instance.EffectiveDataScopeKind())
	}
}

func TestEnableWorkspaceDataRequiresCurrentOwnerReview(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO workspaces (id, owner_id) VALUES ('workspace-1', 'owner-1')`); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	if err := store.Create(ctx, Instance{
		ID: "instance-1", PluginID: "canvas", SourceKind: SourceLocalCanvas,
		ScopeKind: ScopeTask, WorkspaceID: "workspace-1", TaskID: "task-1", Status: StatusActive,
	}); err != nil {
		t.Fatalf("create task canvas: %v", err)
	}
	declared := json.RawMessage(`{"reads":["tasks"]}`)
	if err := store.CreateRelease(ctx, Release{
		ID: "release-1", PluginID: "canvas", InstanceID: "instance-1", PackageDigest: "digest-1",
		SourceKind: SourceLocalCanvas, ManifestJSON: json.RawMessage(`{}`), DeclaredPermissionsJSON: declared,
		ArtifactPath: "releases/one", ArtifactBytes: 1, ValidationStatus: ValidationValid,
	}); err != nil {
		t.Fatalf("create active release: %v", err)
	}
	if err := store.AddGrant(ctx, Grant{InstanceID: "instance-1", PermissionKind: "api_read", Resource: "tasks", ScopeCeiling: ScopeTask, ApprovedBy: "owner-1"}); err != nil {
		t.Fatalf("add task grant: %v", err)
	}
	if err := store.AddGrant(ctx, Grant{InstanceID: "instance-1", PermissionKind: "api_write", Resource: "messages", ScopeCeiling: ScopeTask, ApprovedBy: "owner-1"}); err != nil {
		t.Fatalf("add stale task grant: %v", err)
	}
	if err := store.SetActiveRelease(ctx, "instance-1", "release-1"); err != nil {
		t.Fatalf("activate release: %v", err)
	}
	instanceBeforeReview, err := store.Get(ctx, "instance-1")
	if err != nil {
		t.Fatalf("get instance before review: %v", err)
	}

	workspaceGrant := []Grant{{InstanceID: "instance-1", PermissionKind: "api_read", Resource: "tasks", ScopeCeiling: ScopeWorkspace}}
	apply := func(userID string, generation int64, grants []Grant) error {
		return store.WithTransaction(ctx, func(tx *sqlx.Tx) error {
			return store.EnableWorkspaceDataReviewedTx(ctx, tx, "instance-1", "workspace-1", userID, grants, "release-1", permissionDigest(string(declared)), generation)
		})
	}
	if err := apply("owner-2", instanceBeforeReview.GrantGeneration, workspaceGrant); !errors.Is(err, ErrWorkspaceOwnerRequired) {
		t.Fatalf("foreign owner confirmation = %v, want ErrWorkspaceOwnerRequired", err)
	}
	if err := apply("owner-1", instanceBeforeReview.GrantGeneration+1, workspaceGrant); !errors.Is(err, ErrStaleWorkspaceDataReview) {
		t.Fatalf("stale review confirmation = %v, want ErrStaleWorkspaceDataReview", err)
	}
	undeclared := append(append([]Grant(nil), workspaceGrant...), Grant{InstanceID: "instance-1", PermissionKind: "api_write", Resource: "messages", ScopeCeiling: ScopeWorkspace})
	if err := apply("owner-1", instanceBeforeReview.GrantGeneration, undeclared); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("undeclared grant confirmation = %v, want ErrInvalidScope", err)
	}
	if err := apply("owner-1", instanceBeforeReview.GrantGeneration, workspaceGrant); err != nil {
		t.Fatalf("owner confirmation: %v", err)
	}

	instance, err := store.Get(ctx, "instance-1")
	if err != nil {
		t.Fatalf("get upgraded instance: %v", err)
	}
	if instance.ScopeKind != ScopeTask || instance.DataScopeKind != ScopeWorkspace || instance.ActiveReleaseID != "release-1" || instance.GrantGeneration != instanceBeforeReview.GrantGeneration+1 {
		t.Fatalf("upgraded instance = %+v, want task placement, workspace data, same release, generation increment", instance)
	}
	grants, err := store.ListGrants(ctx, "instance-1")
	if err != nil {
		t.Fatalf("list upgraded grants: %v", err)
	}
	if len(grants) != 1 || grants[0].ScopeCeiling != ScopeWorkspace || grants[0].ApprovedBy != "owner-1" {
		t.Fatalf("upgraded grants = %+v, want one owner-approved workspace task grant", grants)
	}
}

func TestVerifyWorkspaceOwnerAllowsAuthDisabledUnownedWorkspace(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO workspaces (id, owner_id) VALUES ('workspace-1', '')`); err != nil {
		t.Fatalf("insert unowned workspace: %v", err)
	}
	if err := store.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		return verifyWorkspaceOwnerTx(ctx, tx, "workspace-1", userstore.DefaultUserID)
	}); err != nil {
		t.Fatalf("default user review of unowned workspace: %v", err)
	}
	if err := store.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		return verifyWorkspaceOwnerTx(ctx, tx, "workspace-1", "member-1")
	}); !errors.Is(err, ErrWorkspaceOwnerRequired) {
		t.Fatalf("ordinary user review of unowned workspace = %v, want ErrWorkspaceOwnerRequired", err)
	}
}

func TestCreateInstanceAdmissionIsAtomicAtTaskAndWorkspaceLimits(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		created int
		lastErr error
	)
	for i := 0; i < MaxTaskInstances+5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			instance := Instance{
				ID:          "instance-" + time.Now().Format("150405.000000000") + "-" + string(rune('a'+i)),
				PluginID:    "canvas-board",
				SourceKind:  SourceLocalCanvas,
				ScopeKind:   ScopeTask,
				WorkspaceID: "workspace-1",
				TaskID:      "task-1",
				Status:      StatusActive,
			}
			if err := store.Create(ctx, instance); err != nil {
				mu.Lock()
				lastErr = err
				mu.Unlock()
				return
			}
			mu.Lock()
			created++
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if created != MaxTaskInstances {
		t.Fatalf("created = %d, want %d (last error %v)", created, MaxTaskInstances, lastErr)
	}
	count, err := store.CountActive(ctx, ScopeTask, "task-1")
	if err != nil {
		t.Fatalf("CountActive: %v", err)
	}
	if count != MaxTaskInstances {
		t.Fatalf("task count = %d, want %d", count, MaxTaskInstances)
	}
}

func TestArchiveRestoreUsesTheSameAdmission(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < MaxTaskInstances-1; i++ {
		if err := store.Create(ctx, Instance{ID: "instance-" + string(rune('a'+i)), PluginID: "p", SourceKind: SourceLocalCanvas, ScopeKind: ScopeTask, WorkspaceID: "w", TaskID: "t", Status: StatusActive}); err != nil {
			t.Fatalf("Create(%d): %v", i, err)
		}
	}
	if err := store.Archive(ctx, "instance-a"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if err := store.Restore(ctx, "instance-a"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if err := store.Create(ctx, Instance{ID: "instance-z", PluginID: "p", SourceKind: SourceLocalCanvas, ScopeKind: ScopeTask, WorkspaceID: "w", TaskID: "t", Status: StatusActive}); err != nil {
		t.Fatalf("fill task limit: %v", err)
	}
	if err := store.Archive(ctx, "instance-b"); err != nil {
		t.Fatalf("second Archive: %v", err)
	}
	if err := store.Restore(ctx, "instance-b"); !errors.Is(err, ErrTaskCanvasLimit) {
		t.Fatalf("second Restore = %v, want ErrTaskCanvasLimit", err)
	}
}

func TestReserveBytesRejectsWorkspaceAndInstallationOverages(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first, err := store.ReserveBytes(ctx, "workspace-1", 8, 10, 100)
	if err != nil {
		t.Fatalf("first reservation: %v", err)
	}
	if _, err := store.ReserveBytes(ctx, "workspace-1", 3, 10, 100); !errors.Is(err, ErrWorkspaceStorageLimit) {
		t.Fatalf("workspace overage = %v, want ErrWorkspaceStorageLimit", err)
	}
	if _, err := store.ReserveBytes(ctx, "workspace-2", 93, 100, 100); !errors.Is(err, ErrInstallationStorageLimit) {
		t.Fatalf("installation overage = %v, want ErrInstallationStorageLimit", err)
	}
	if err := store.ReleaseBytes(ctx, first.ID); err != nil {
		t.Fatalf("release reservation: %v", err)
	}
}

func TestReserveBytesCountsRetainedReleaseArtifactsAfterReservationRelease(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, Instance{
		ID:          "instance-1",
		PluginID:    "canvas-board",
		SourceKind:  SourceLocalCanvas,
		ScopeKind:   ScopeWorkspace,
		WorkspaceID: "workspace-1",
		Status:      StatusActive,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.CreateRelease(ctx, Release{
		ID:               "release-1",
		PluginID:         "canvas-board",
		InstanceID:       "instance-1",
		PackageDigest:    "digest-1",
		SourceKind:       SourceLocalCanvas,
		SourceActorKind:  "agent",
		ArtifactPath:     "releases/digest-1",
		ArtifactBytes:    8,
		ValidationStatus: ValidationValid,
	}); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}

	reservation, err := store.ReserveBytes(ctx, "workspace-1", 2, 10, 10)
	if err != nil {
		t.Fatalf("reserve remaining capacity: %v", err)
	}
	if err := store.ReleaseBytes(ctx, reservation.ID); err != nil {
		t.Fatalf("release reservation: %v", err)
	}
	if _, err := store.ReserveBytes(ctx, "workspace-1", 3, 10, 10); !errors.Is(err, ErrWorkspaceStorageLimit) {
		t.Fatalf("workspace reservation after release = %v, want ErrWorkspaceStorageLimit", err)
	}
	if _, err := store.ReserveBytes(ctx, "workspace-2", 3, 10, 10); !errors.Is(err, ErrInstallationStorageLimit) {
		t.Fatalf("installation reservation after release = %v, want ErrInstallationStorageLimit", err)
	}
}

func TestReserveBytesDoesNotDoubleCountSharedRetainedArtifacts(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for _, instance := range []Instance{
		{ID: "instance-1", PluginID: "canvas-board", SourceKind: SourceLocalCanvas, ScopeKind: ScopeWorkspace, WorkspaceID: "workspace-1", Status: StatusActive},
		{ID: "instance-2", PluginID: "canvas-board", SourceKind: SourceLocalCanvas, ScopeKind: ScopeWorkspace, WorkspaceID: "workspace-2", Status: StatusActive},
	} {
		if err := store.Create(ctx, instance); err != nil {
			t.Fatalf("Create(%s): %v", instance.ID, err)
		}
	}
	for _, release := range []Release{
		{ID: "release-1", PluginID: "canvas-board", InstanceID: "instance-1", PackageDigest: "digest-shared", SourceKind: SourceLocalCanvas, SourceActorKind: "agent", ArtifactPath: "releases/shared", ArtifactBytes: 8, ValidationStatus: ValidationValid},
		{ID: "release-2", PluginID: "canvas-board", InstanceID: "instance-1", PackageDigest: "digest-shared", SourceKind: SourceLocalCanvas, SourceActorKind: "agent", ArtifactPath: "releases/shared", ArtifactBytes: 8, ValidationStatus: ValidationValid},
		{ID: "release-3", PluginID: "canvas-board", InstanceID: "instance-2", PackageDigest: "digest-shared", SourceKind: SourceLocalCanvas, SourceActorKind: "agent", ArtifactPath: "releases/shared", ArtifactBytes: 8, ValidationStatus: ValidationValid},
	} {
		if err := store.CreateRelease(ctx, release); err != nil {
			t.Fatalf("CreateRelease(%s): %v", release.ID, err)
		}
	}

	workspaceReservation, err := store.ReserveBytes(ctx, "workspace-1", 2, 10, 10)
	if err != nil {
		t.Fatalf("reserve shared workspace capacity: %v", err)
	}
	if err := store.ReleaseBytes(ctx, workspaceReservation.ID); err != nil {
		t.Fatalf("release workspace reservation: %v", err)
	}
	installationReservation, err := store.ReserveBytes(ctx, "workspace-2", 2, 10, 10)
	if err != nil {
		t.Fatalf("reserve shared installation capacity: %v", err)
	}
	if err := store.ReleaseBytes(ctx, installationReservation.ID); err != nil {
		t.Fatalf("release installation reservation: %v", err)
	}
}

func TestPruneReleasesFreesQuotaAfterRemovingSupersededArtifact(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, Instance{
		ID:          "instance-1",
		PluginID:    "canvas-board",
		SourceKind:  SourceLocalCanvas,
		ScopeKind:   ScopeWorkspace,
		WorkspaceID: "workspace-1",
		Status:      StatusActive,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, release := range []Release{
		{ID: "release-old", PluginID: "canvas-board", InstanceID: "instance-1", PackageDigest: "digest-old", SourceKind: SourceLocalCanvas, SourceActorKind: "agent", ArtifactPath: "releases/old", ArtifactBytes: 4, ValidationStatus: ValidationValid, CreatedAt: base.Add(time.Second)},
		{ID: "release-prior", PluginID: "canvas-board", InstanceID: "instance-1", PackageDigest: "digest-prior", SourceKind: SourceLocalCanvas, SourceActorKind: "agent", ArtifactPath: "releases/prior", ArtifactBytes: 4, ValidationStatus: ValidationValid, CreatedAt: base.Add(2 * time.Second)},
		{ID: "release-active", PluginID: "canvas-board", InstanceID: "instance-1", PackageDigest: "digest-active", SourceKind: SourceLocalCanvas, SourceActorKind: "agent", ArtifactPath: "releases/active", ArtifactBytes: 4, ValidationStatus: ValidationValid, CreatedAt: base.Add(3 * time.Second)},
	} {
		if err := store.CreateRelease(ctx, release); err != nil {
			t.Fatalf("CreateRelease(%s): %v", release.ID, err)
		}
	}
	if err := store.SetActiveRelease(ctx, "instance-1", "release-active"); err != nil {
		t.Fatalf("SetActiveRelease: %v", err)
	}
	if _, err := store.ReserveBytes(ctx, "workspace-1", 1, 10, 10); !errors.Is(err, ErrWorkspaceStorageLimit) {
		t.Fatalf("reservation before prune = %v, want ErrWorkspaceStorageLimit", err)
	}
	if err := store.PruneReleases(ctx, "instance-1"); err != nil {
		t.Fatalf("PruneReleases: %v", err)
	}
	reservation, err := store.ReserveBytes(ctx, "workspace-1", 1, 10, 10)
	if err != nil {
		t.Fatalf("reservation after prune: %v", err)
	}
	if err := store.ReleaseBytes(ctx, reservation.ID); err != nil {
		t.Fatalf("release reservation: %v", err)
	}
}

func TestReconcileArtifactsMarksRetainedReleaseUnavailable(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, Instance{ID: "instance-1", PluginID: "canvas-board", SourceKind: SourceLocalCanvas, ScopeKind: ScopeWorkspace, WorkspaceID: "workspace-1", Status: StatusActive}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.CreateRelease(ctx, Release{ID: "release-1", PluginID: "canvas-board", InstanceID: "instance-1", PackageDigest: "digest-1", SourceKind: SourceLocalCanvas, SourceActorKind: "agent", ArtifactPath: "releases/digest-1", ValidationStatus: ValidationValid}); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	marked, err := store.ReconcileArtifacts(ctx, func(path, digest string, bytes int64) (ArtifactCheck, error) {
		if path != "releases/digest-1" || digest != "digest-1" || bytes != 0 {
			t.Fatalf("checker arguments = %q, %q, %d", path, digest, bytes)
		}
		return ArtifactCheck{Reason: "missing"}, nil
	})
	if err != nil || marked != 1 {
		t.Fatalf("ReconcileArtifacts() = %d, %v; want one mark", marked, err)
	}
	release, err := store.GetRelease(ctx, "release-1")
	if err != nil {
		t.Fatalf("GetRelease: %v", err)
	}
	if release.ValidationStatus != ValidationUnavailable || release.ValidationError != "missing" {
		t.Fatalf("release after reconcile = %+v", release)
	}
}

func TestCleanupJobsCanBeClaimedRetriedAndCompleted(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.AddCleanupJob(ctx, CleanupJob{ID: "cleanup-1", InstanceID: "instance-1", ArtifactPath: "releases/artifact"}); err != nil {
		t.Fatalf("AddCleanupJob: %v", err)
	}

	claimed, ok, err := store.ClaimCleanupJob(ctx, time.Now().UTC())
	if err != nil || !ok {
		t.Fatalf("ClaimCleanupJob() = %+v, %v, %t; want a claim", claimed, err, ok)
	}
	if claimed.Status != CleanupRunning || claimed.Attempts != 1 {
		t.Fatalf("claimed job = %+v, want running attempt 1", claimed)
	}
	if _, ok, err := store.ClaimCleanupJob(ctx, time.Now().UTC()); err != nil || ok {
		t.Fatalf("second ClaimCleanupJob() = %v, %t; want no duplicate claim", err, ok)
	}

	next := time.Now().UTC().Add(time.Hour)
	if err := store.RetryCleanupJob(ctx, claimed.ID, next, errors.New("remove failed")); err != nil {
		t.Fatalf("RetryCleanupJob: %v", err)
	}
	if _, ok, err := store.ClaimCleanupJob(ctx, time.Now().UTC()); err != nil || ok {
		t.Fatalf("early retry claim = %v, %t; want no claim", err, ok)
	}
	retried, ok, err := store.ClaimCleanupJob(ctx, next)
	if err != nil || !ok || retried.Attempts != 2 {
		t.Fatalf("retry claim = %+v, %v, %t; want attempt 2", retried, err, ok)
	}
	if err := store.CompleteCleanupJob(ctx, retried.ID); err != nil {
		t.Fatalf("CompleteCleanupJob: %v", err)
	}
	jobs, err := store.ListCleanupJobs(ctx)
	if err != nil {
		t.Fatalf("ListCleanupJobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Status != CleanupCompleted || jobs[0].LastError != "remove failed" {
		t.Fatalf("cleanup jobs = %+v, want one completed job with diagnostic", jobs)
	}
}

func TestReleaseActivationRejectsBroaderGrants(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, Instance{ID: "instance-1", PluginID: "canvas-board", SourceKind: SourceLocalCanvas, ScopeKind: ScopeTask, WorkspaceID: "workspace-1", TaskID: "task-1", Status: StatusActive}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	declared := json.RawMessage(`{"reads":["tasks"],"shared_state":false,"external_origins":["https://api.example.com"]}`)
	if err := store.CreateRelease(ctx, Release{ID: "release-1", PluginID: "canvas-board", InstanceID: "instance-1", PackageDigest: "digest-1", SourceKind: SourceLocalCanvas, SourceActorKind: "agent", ManifestJSON: json.RawMessage(`{}`), DeclaredPermissionsJSON: declared, ArtifactPath: "releases/digest-1", ArtifactBytes: 1, ValidationStatus: ValidationValid}); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := store.AddGrant(ctx, Grant{InstanceID: "instance-1", PermissionKind: "api_read", Resource: "tasks", ScopeCeiling: ScopeTask, ApprovedBy: "user-1"}); err != nil {
		t.Fatalf("AddGrant: %v", err)
	}
	if err := store.AddGrant(ctx, Grant{InstanceID: "instance-1", PermissionKind: "network", NetworkOrigin: "https://api.example.com", ScopeCeiling: ScopeWorkspace, ApprovedBy: "user-1"}); err != nil {
		t.Fatalf("AddGrant network preapproval: %v", err)
	}
	if err := store.ActivateRelease(ctx, "instance-1", "release-1"); !errors.Is(err, ErrInvalidRelease) {
		t.Fatalf("ActivateRelease with broader grant = %v, want ErrInvalidRelease", err)
	}
	instance, err := store.Get(ctx, "instance-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if instance.ActiveReleaseID != "" {
		t.Fatalf("active release = %q after rejected activation", instance.ActiveReleaseID)
	}
}

func TestReleaseActivationContractsRemovedPermissions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, Instance{ID: "instance-1", PluginID: "canvas-board", SourceKind: SourceLocalCanvas, ScopeKind: ScopeTask, WorkspaceID: "workspace-1", TaskID: "task-1", Status: StatusActive}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.AddGrant(ctx, Grant{InstanceID: "instance-1", PermissionKind: "api_read", Resource: "tasks", ScopeCeiling: ScopeTask, ApprovedBy: "user-1"}); err != nil {
		t.Fatalf("AddGrant: %v", err)
	}
	if err := store.CreateRelease(ctx, Release{ID: "release-1", PluginID: "canvas-board", InstanceID: "instance-1", PackageDigest: "digest-1", SourceKind: SourceLocalCanvas, ManifestJSON: json.RawMessage(`{}`), DeclaredPermissionsJSON: json.RawMessage(`{}`), ArtifactPath: "releases/digest-1", ArtifactBytes: 1, ValidationStatus: ValidationValid}); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := store.ActivateRelease(ctx, "instance-1", "release-1"); err != nil {
		t.Fatalf("ActivateRelease: %v", err)
	}
	instance, err := store.Get(ctx, "instance-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if instance.ActiveReleaseID != "release-1" {
		t.Fatalf("active release = %q, want release-1", instance.ActiveReleaseID)
	}
}

func TestCreateReleaseIfActiveReleaseTxRejectsChangedAuthority(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, Instance{
		ID: "instance-authority", PluginID: "canvas-board", SourceKind: SourceLocalCanvas,
		ScopeKind: ScopeTask, WorkspaceID: "workspace-1", TaskID: "task-1", Status: StatusActive,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.CreateRelease(ctx, Release{
		ID: "release-authority-base", PluginID: "canvas-board", InstanceID: "instance-authority",
		PackageDigest: "digest-base", SourceKind: SourceLocalCanvas, SourceActorKind: "agent",
		ManifestJSON: json.RawMessage(`{}`), DeclaredPermissionsJSON: json.RawMessage(`{}`),
		ArtifactPath: "releases/base", ArtifactBytes: 1, ValidationStatus: ValidationValid,
	}); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := store.SetActiveRelease(ctx, "instance-authority", "release-authority-base"); err != nil {
		t.Fatalf("SetActiveRelease: %v", err)
	}
	captured, err := store.Get(ctx, "instance-authority")
	if err != nil {
		t.Fatalf("capture publish authority: %v", err)
	}
	expected := captured.PublishAuthority()
	if err := store.SetScope(ctx, "instance-authority", ScopeWorkspace, ScopeIdentifiers{WorkspaceID: "workspace-1"}); err != nil {
		t.Fatalf("promote scope: %v", err)
	}

	err = store.WithTransaction(ctx, func(tx *sqlx.Tx) error {
		return store.CreateReleaseIfAuthorityTx(ctx, tx, "instance-authority", expected, Release{
			ID: "release-authority-stale", PluginID: "canvas-board", InstanceID: "instance-authority",
			PackageDigest: "digest-stale", SourceKind: SourceLocalCanvas, SourceActorKind: "agent",
			ManifestJSON: json.RawMessage(`{}`), DeclaredPermissionsJSON: json.RawMessage(`{}`),
			ArtifactPath: "releases/stale", ArtifactBytes: 1, ValidationStatus: ValidationValid,
		})
	})
	if !errors.Is(err, ErrStaleCanvasPublish) {
		t.Fatalf("stale task authority error = %v, want ErrStaleCanvasPublish", err)
	}
}

func TestActivateReleaseRejectsArchivedInstance(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, Instance{
		ID: "instance-archived-activate", PluginID: "canvas-board", SourceKind: SourceLocalCanvas,
		ScopeKind: ScopeWorkspace, WorkspaceID: "workspace-1", Status: StatusArchived,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.CreateRelease(ctx, Release{
		ID: "release-archived-activate", PluginID: "canvas-board", InstanceID: "instance-archived-activate",
		PackageDigest: "digest-archived-activate", SourceKind: SourceLocalCanvas, SourceActorKind: "agent",
		ManifestJSON: json.RawMessage(`{}`), DeclaredPermissionsJSON: json.RawMessage(`{}`),
		ArtifactPath: "releases/archived-activate", ArtifactBytes: 1, ValidationStatus: ValidationValid,
	}); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := store.ActivateRelease(ctx, "instance-archived-activate", "release-archived-activate"); err == nil {
		t.Fatal("archived instance was reactivated without Restore")
	}
}

func TestApproveReleaseRejectsArchivedInstance(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Create(ctx, Instance{
		ID: "instance-archived-approve", PluginID: "canvas-board", SourceKind: SourceLocalCanvas,
		ScopeKind: ScopeTask, WorkspaceID: "workspace-1", TaskID: "task-1", Status: StatusArchived,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.CreateRelease(ctx, Release{
		ID: "release-archived-approve", PluginID: "canvas-board", InstanceID: "instance-archived-approve",
		PackageDigest: "digest-archived-approve", SourceKind: SourceLocalCanvas, SourceActorKind: "agent",
		ManifestJSON: json.RawMessage(`{}`), DeclaredPermissionsJSON: json.RawMessage(`{"reads":["tasks"]}`),
		ArtifactPath: "releases/archived-approve", ArtifactBytes: 1, ValidationStatus: ValidationPendingPermission,
	}); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := store.ApproveRelease(ctx, "instance-archived-approve", "release-archived-approve", "user-1", []Grant{{
		PermissionKind: "api_read", Resource: "tasks", ScopeCeiling: ScopeTask,
	}}); err == nil {
		t.Fatal("archived instance was approved and activated without Restore")
	}
}
