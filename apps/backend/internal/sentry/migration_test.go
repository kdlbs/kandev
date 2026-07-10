package sentry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
)

// workspaceScopedConfigsDDL is PR #1572's one-config-per-workspace shape
// (workspace_id PRIMARY KEY, no id column) that migrateConfigsTableToInstances
// upgrades.
const workspaceScopedConfigsDDL = `
	CREATE TABLE sentry_configs (
		workspace_id TEXT PRIMARY KEY,
		auth_method TEXT NOT NULL,
		url TEXT NOT NULL DEFAULT 'https://sentry.io',
		last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);`

// workspaceScopedWatchesDDL is the pre-instance sentry_issue_watches shape
// (workspace-scoped, no sentry_instance_id) that migrateWatchesAddInstanceColumn
// rebuilds, plus its child dedup table.
const workspaceScopedWatchesDDL = `
	CREATE TABLE sentry_issue_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		base_branch TEXT NOT NULL DEFAULT '',
		filter_json TEXT NOT NULL DEFAULT '{}',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		executor_profile_id TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT 1,
		poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
		max_inflight_tasks INTEGER DEFAULT 5,
		last_polled_at DATETIME,
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE sentry_issue_watch_tasks (
		id TEXT PRIMARY KEY,
		issue_watch_id TEXT NOT NULL,
		issue_short_id TEXT NOT NULL,
		issue_url TEXT NOT NULL,
		task_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE(issue_watch_id, issue_short_id),
		FOREIGN KEY(issue_watch_id) REFERENCES sentry_issue_watches(id) ON DELETE CASCADE
	);`

func openMigrationDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedWorkspaceScopedSchema builds the pre-instance workspace-scoped tables and
// seeds configs for ws-1 (self-hosted) and ws-2 (SaaS), watches for ws-1, ws-2
// and ws-3 (which has NO config), plus one dedup task row for the ws-1 watch.
// Returns the ws-1/ws-2/ws-3 watch IDs.
func seedWorkspaceScopedSchema(t *testing.T, db *sqlx.DB) (w1, w2, w3 string) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(workspaceScopedConfigsDDL + workspaceScopedWatchesDDL); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO sentry_configs (workspace_id, auth_method, url, last_checked_at, last_ok, last_error, created_at, updated_at)
		VALUES ('ws-1', ?, 'https://sentry.example.com', ?, 1, 'healthy', ?, ?),
		       ('ws-2', ?, 'https://sentry.io', NULL, 0, '', ?, ?)`,
		AuthMethodAuthToken, now, now, now, AuthMethodAuthToken, now, now); err != nil {
		t.Fatalf("seed configs: %v", err)
	}
	w1, w2, w3 = uuid.New().String(), uuid.New().String(), uuid.New().String()
	for _, watch := range []struct{ id, ws string }{{w1, "ws-1"}, {w2, "ws-2"}, {w3, "ws-3"}} {
		if _, err := db.Exec(`
			INSERT INTO sentry_issue_watches (id, workspace_id, workflow_id, workflow_step_id, filter_json, created_at, updated_at)
			VALUES (?, ?, 'wf', 'step', '{"orgSlug":"acme","projectSlug":"fe"}', ?, ?)`,
			watch.id, watch.ws, now, now); err != nil {
			t.Fatalf("seed watch %s: %v", watch.ws, err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO sentry_issue_watch_tasks (id, issue_watch_id, issue_short_id, issue_url, task_id, created_at)
		VALUES (?, ?, 'PROJ-1', 'https://sentry.example.com/issues/PROJ-1', 'task-1', ?)`,
		uuid.New().String(), w1, now); err != nil {
		t.Fatalf("seed dedup task: %v", err)
	}
	return w1, w2, w3
}

// TestMigration_WorkspaceConfigsToInstances pins acceptance (a): the
// workspace-scoped model upgrades to id-keyed instances, the per-workspace
// secret is rekeyed to the per-instance key, and every watch's
// sentry_instance_id is backfilled when its workspace has exactly one config.
func TestMigration_WorkspaceConfigsToInstances(t *testing.T) {
	db := openMigrationDB(t)
	w1, w2, w3 := seedWorkspaceScopedSchema(t, db)

	secrets := newFakeSecretStore()
	ctx := context.Background()
	_ = secrets.Set(ctx, SecretKeyForWorkspace("ws-1"), "tok", "tok-1")
	_ = secrets.Set(ctx, SecretKeyForWorkspace("ws-2"), "tok", "tok-2")

	svc, _, err := Provide(db, db, secrets, nil, logger.Default())
	if err != nil {
		t.Fatalf("Provide (migrate): %v", err)
	}
	store := svc.Store()

	// One instance per workspace config, host-derived names, health preserved.
	inst1List, _ := store.ListInstances(ctx, "ws-1")
	inst2List, _ := store.ListInstances(ctx, "ws-2")
	if len(inst1List) != 1 || len(inst2List) != 1 {
		t.Fatalf("expected one instance per workspace, got ws-1=%d ws-2=%d", len(inst1List), len(inst2List))
	}
	inst1, inst2 := inst1List[0], inst2List[0]
	if inst1.Name != "sentry.example.com" || inst1.URL != "https://sentry.example.com" || !inst1.LastOk {
		t.Errorf("ws-1 instance not migrated faithfully: %+v", inst1)
	}
	if inst2.Name != "sentry.io" {
		t.Errorf("ws-2 instance name = %q, want host-derived 'sentry.io'", inst2.Name)
	}

	// Watches backfilled to their workspace's instance; the config-less
	// workspace's watch stays unbound (NULL).
	gw1, _ := store.GetIssueWatch(ctx, w1)
	gw2, _ := store.GetIssueWatch(ctx, w2)
	gw3, _ := store.GetIssueWatch(ctx, w3)
	if gw1.SentryInstanceID != inst1.ID {
		t.Errorf("ws-1 watch bound to %q, want %q", gw1.SentryInstanceID, inst1.ID)
	}
	if gw2.SentryInstanceID != inst2.ID {
		t.Errorf("ws-2 watch bound to %q, want %q", gw2.SentryInstanceID, inst2.ID)
	}
	if gw3.SentryInstanceID != "" {
		t.Errorf("config-less workspace watch should stay unbound, got %q", gw3.SentryInstanceID)
	}

	// The dedup child row survived the watches table rebuild (FK preserved).
	ids, _ := store.ListIssueWatchTaskIDs(ctx, w1)
	if len(ids) != 1 || ids[0] != "task-1" {
		t.Errorf("expected dedup task preserved through rebuild, got %v", ids)
	}

	// Secret rekeyed from the workspace key to the per-instance key.
	if got, _ := secrets.Reveal(ctx, secretKeyForInstance(inst1.ID)); got != "tok-1" {
		t.Errorf("ws-1 secret not rekeyed to instance key, got %q", got)
	}
	if got, _ := secrets.Reveal(ctx, secretKeyForInstance(inst2.ID)); got != "tok-2" {
		t.Errorf("ws-2 secret not rekeyed to instance key, got %q", got)
	}
}

// TestMigration_RerunIsIdempotent pins acceptance (c): re-running the whole
// migration + secret rekey after a completed run changes nothing.
func TestMigration_RerunIsIdempotent(t *testing.T) {
	db := openMigrationDB(t)
	w1, _, _ := seedWorkspaceScopedSchema(t, db)
	secrets := newFakeSecretStore()
	ctx := context.Background()
	_ = secrets.Set(ctx, SecretKeyForWorkspace("ws-1"), "tok", "tok-1")
	_ = secrets.Set(ctx, SecretKeyForWorkspace("ws-2"), "tok", "tok-2")

	svc1, _, err := Provide(db, db, secrets, nil, logger.Default())
	if err != nil {
		t.Fatalf("first Provide: %v", err)
	}
	before, _ := svc1.Store().ListAllInstances(ctx)
	boundBefore, _ := svc1.Store().GetIssueWatch(ctx, w1)

	// Re-run: a fresh NewStore + secret rekey over the same (now-migrated) DB.
	store2, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("second NewStore: %v", err)
	}
	migrateInstanceSecrets(store2, secrets, logger.Default())

	after, _ := store2.ListAllInstances(ctx)
	if len(after) != len(before) || len(after) != 2 {
		t.Fatalf("instance count changed on rerun: before=%d after=%d", len(before), len(after))
	}
	if after[0].ID != before[0].ID || after[1].ID != before[1].ID {
		t.Error("instance IDs changed on rerun")
	}
	boundAfter, _ := store2.GetIssueWatch(ctx, w1)
	if boundAfter.SentryInstanceID != boundBefore.SentryInstanceID {
		t.Errorf("watch binding changed on rerun: %q -> %q", boundBefore.SentryInstanceID, boundAfter.SentryInstanceID)
	}
	if got, _ := secrets.Reveal(ctx, secretKeyForInstance(boundBefore.SentryInstanceID)); got != "tok-1" {
		t.Errorf("secret changed on rerun, got %q", got)
	}
}

// TestMigration_CrashAfterConfigsRebuild pins the other half of acceptance (c):
// a crash after the configs table was rebuilt to the id-keyed shape but before
// the watches table gained its FK column. The next boot must complete the watch
// rebuild + backfill against the already-migrated configs.
func TestMigration_CrashAfterConfigsRebuild(t *testing.T) {
	db := openMigrationDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Simulate the post-configs-rebuild / pre-watches-rebuild state: configs are
	// already id-keyed, watches are still workspace-scoped without the FK column.
	inst1, inst2 := uuid.New().String(), uuid.New().String()
	if _, err := db.Exec(`CREATE TABLE sentry_configs (` + sentryConfigsColumns + `)`); err != nil {
		t.Fatalf("create id-keyed configs: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO sentry_configs (id, workspace_id, name, auth_method, url, last_ok, last_error, created_at, updated_at)
		VALUES (?, 'ws-1', 'sentry.example.com', ?, 'https://sentry.example.com', 0, '', ?, ?),
		       (?, 'ws-2', 'sentry.io', ?, 'https://sentry.io', 0, '', ?, ?)`,
		inst1, AuthMethodAuthToken, now, now, inst2, AuthMethodAuthToken, now, now); err != nil {
		t.Fatalf("seed id-keyed configs: %v", err)
	}
	w1, w2, w3 := uuid.New().String(), uuid.New().String(), uuid.New().String()
	if _, err := db.Exec(workspaceScopedWatchesDDL); err != nil {
		t.Fatalf("seed workspace-scoped watches: %v", err)
	}
	for _, watch := range []struct{ id, ws string }{{w1, "ws-1"}, {w2, "ws-2"}, {w3, "ws-3"}} {
		if _, err := db.Exec(`
			INSERT INTO sentry_issue_watches (id, workspace_id, workflow_id, workflow_step_id, filter_json, created_at, updated_at)
			VALUES (?, ?, 'wf', 'step', '{}', ?, ?)`, watch.id, watch.ws, now, now); err != nil {
			t.Fatalf("seed watch %s: %v", watch.ws, err)
		}
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("resume migration: %v", err)
	}

	// Configs are left untouched (no second rebuild → IDs preserved).
	gi1, _ := store.GetInstance(ctx, inst1)
	if gi1 == nil {
		t.Fatal("existing id-keyed config was clobbered")
	}
	// Watches now carry the FK column, backfilled from the existing configs.
	if gw1, _ := store.GetIssueWatch(ctx, w1); gw1.SentryInstanceID != inst1 {
		t.Errorf("ws-1 watch bound to %q, want %q", gw1.SentryInstanceID, inst1)
	}
	if gw2, _ := store.GetIssueWatch(ctx, w2); gw2.SentryInstanceID != inst2 {
		t.Errorf("ws-2 watch bound to %q, want %q", gw2.SentryInstanceID, inst2)
	}
	if gw3, _ := store.GetIssueWatch(ctx, w3); gw3.SentryInstanceID != "" {
		t.Errorf("config-less workspace watch should stay unbound, got %q", gw3.SentryInstanceID)
	}
}

// experimentalWatchesDDL is the unmerged-#1469 experimental shape:
// sentry_issue_watches carries sentry_instance_id as NOT NULL DEFAULT ” with
// NO foreign key. It only ever existed on dev machines that ran that build;
// migrateWatchesAddInstanceColumn must still converge it to the nullable-FK
// shape rather than skip it because the column name happens to be present.
const experimentalWatchesDDL = `
	CREATE TABLE sentry_issue_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		sentry_instance_id TEXT NOT NULL DEFAULT '',
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		base_branch TEXT NOT NULL DEFAULT '',
		filter_json TEXT NOT NULL DEFAULT '{}',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		executor_profile_id TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT 1,
		poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
		max_inflight_tasks INTEGER DEFAULT 5,
		last_polled_at DATETIME,
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE sentry_issue_watch_tasks (
		id TEXT PRIMARY KEY,
		issue_watch_id TEXT NOT NULL,
		issue_short_id TEXT NOT NULL,
		issue_url TEXT NOT NULL,
		task_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE(issue_watch_id, issue_short_id),
		FOREIGN KEY(issue_watch_id) REFERENCES sentry_issue_watches(id) ON DELETE CASCADE
	);`

// TestMigration_ExperimentalNonNullInstanceColumnRebuilt pins the hardened
// guard: a watches table that already has sentry_instance_id but as
// NOT NULL DEFAULT ” with no foreign key is rebuilt into the nullable-FK shape
// (not skipped), converting the ” sentinel to a real backfill / NULL.
func TestMigration_ExperimentalNonNullInstanceColumnRebuilt(t *testing.T) {
	db := openMigrationDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	inst1 := uuid.New().String()
	if _, err := db.Exec(`CREATE TABLE sentry_configs (` + sentryConfigsColumns + `)`); err != nil {
		t.Fatalf("create id-keyed configs: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO sentry_configs (id, workspace_id, name, auth_method, url, last_ok, last_error, created_at, updated_at)
		VALUES (?, 'ws-1', 'sentry.example.com', ?, 'https://sentry.example.com', 0, '', ?, ?)`,
		inst1, AuthMethodAuthToken, now, now); err != nil {
		t.Fatalf("seed configs: %v", err)
	}
	if _, err := db.Exec(experimentalWatchesDDL); err != nil {
		t.Fatalf("seed experimental watches: %v", err)
	}
	w1, w3 := uuid.New().String(), uuid.New().String()
	// Experimental rows store the '' sentinel for the unbound case.
	for _, watch := range []struct{ id, ws string }{{w1, "ws-1"}, {w3, "ws-3"}} {
		if _, err := db.Exec(`
			INSERT INTO sentry_issue_watches (id, workspace_id, sentry_instance_id, workflow_id, workflow_step_id, filter_json, created_at, updated_at)
			VALUES (?, ?, '', 'wf', 'step', '{}', ?, ?)`, watch.id, watch.ws, now, now); err != nil {
			t.Fatalf("seed watch %s: %v", watch.ws, err)
		}
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("migrate experimental schema: %v", err)
	}

	// Column is now nullable and backed by the ON DELETE RESTRICT FK to id.
	nullable, err := store.columnIsNullable("sentry_issue_watches", "sentry_instance_id")
	if err != nil || !nullable {
		t.Errorf("sentry_instance_id nullable=%v err=%v, want nullable", nullable, err)
	}
	hasFK, err := store.hasForeignKey("sentry_issue_watches", "sentry_instance_id", "sentry_configs", "id", "RESTRICT")
	if err != nil || !hasFK {
		t.Errorf("sentry_instance_id FK present=%v err=%v, want RESTRICT FK to id", hasFK, err)
	}

	// Rows re-backfilled: ws-1 to its sole instance, config-less ws-3 to NULL.
	if gw1, _ := store.GetIssueWatch(ctx, w1); gw1.SentryInstanceID != inst1 {
		t.Errorf("ws-1 watch bound to %q, want %q", gw1.SentryInstanceID, inst1)
	}
	if gw3, _ := store.GetIssueWatch(ctx, w3); gw3.SentryInstanceID != "" {
		t.Errorf("config-less watch should be unbound, got %q", gw3.SentryInstanceID)
	}

	// FK now enforced: deleting an in-use instance is blocked.
	if _, err := db.Exec(`DELETE FROM sentry_configs WHERE id = ?`, inst1); err == nil {
		t.Error("expected FK RESTRICT to block deleting an in-use instance after rebuild")
	}
}

// TestMigration_RekeyDoesNotSeedLaterSecretlessInstance ensures a completed
// legacy secret migration cannot grant an ordinary later instance the stale
// workspace or singleton token on a subsequent process start.
func TestMigration_RekeyDoesNotSeedLaterSecretlessInstance(t *testing.T) {
	db := openMigrationDB(t)
	seedWorkspaceScopedSchema(t, db)
	ctx := context.Background()
	secrets := newFakeSecretStore()
	if err := secrets.Set(ctx, SecretKeyForWorkspace("ws-1"), "tok", "workspace-token"); err != nil {
		t.Fatalf("seed workspace token: %v", err)
	}
	if err := secrets.Set(ctx, SecretKey, "tok", "singleton-token"); err != nil {
		t.Fatalf("seed singleton token: %v", err)
	}

	svc, _, err := Provide(db, db, secrets, nil, logger.Default())
	if err != nil {
		t.Fatalf("first Provide: %v", err)
	}
	later := testInstance("ws-1", "Later")
	if err := svc.Store().CreateInstance(ctx, later); err != nil {
		t.Fatalf("create later secretless instance: %v", err)
	}

	if _, _, err := Provide(db, db, secrets, nil, logger.Default()); err != nil {
		t.Fatalf("restart Provide: %v", err)
	}
	if exists, err := secrets.Exists(ctx, secretKeyForInstance(later.ID)); err != nil {
		t.Fatalf("check later instance secret: %v", err)
	} else if exists {
		t.Fatal("later secretless instance inherited a stale legacy token")
	}
}

// TestMigration_RekeyRetriesOriginalInstanceDespiteLaterAddition ensures a
// transient secret-store failure on the original migrated instance's rekey
// does not permanently strand it once a later, unrelated instance is added to
// the same workspace before the next restart: only the later instance is
// excluded from ever inheriting the legacy token, not the legitimate original.
func TestMigration_RekeyRetriesOriginalInstanceDespiteLaterAddition(t *testing.T) {
	db := openMigrationDB(t)
	seedWorkspaceScopedSchema(t, db)
	ctx := context.Background()
	secrets := newFakeSecretStore()
	if err := secrets.Set(ctx, SecretKeyForWorkspace("ws-1"), "tok", "workspace-token"); err != nil {
		t.Fatalf("seed workspace token: %v", err)
	}

	secrets.setErr = errors.New("secret store unavailable")
	svc, _, err := Provide(db, db, secrets, nil, logger.Default())
	if err != nil {
		t.Fatalf("first Provide: %v", err)
	}
	instances, err := svc.Store().ListInstances(ctx, "ws-1")
	if err != nil || len(instances) != 1 {
		t.Fatalf("list ws-1 instances: %v %+v", err, instances)
	}
	original := instances[0]
	if exists, _ := secrets.Exists(ctx, secretKeyForInstance(original.ID)); exists {
		t.Fatal("expected the first rekey attempt to fail, but the instance key already exists")
	}

	later := testInstance("ws-1", "Later")
	if err := svc.Store().CreateInstance(ctx, later); err != nil {
		t.Fatalf("create later secretless instance: %v", err)
	}

	secrets.setErr = nil
	if _, _, err := Provide(db, db, secrets, nil, logger.Default()); err != nil {
		t.Fatalf("restart Provide: %v", err)
	}

	if exists, err := secrets.Exists(ctx, secretKeyForInstance(original.ID)); err != nil || !exists {
		t.Fatalf("expected the original instance to recover its legacy token once the transient failure cleared, exists=%v err=%v", exists, err)
	}
	if exists, err := secrets.Exists(ctx, secretKeyForInstance(later.ID)); err != nil || exists {
		t.Fatalf("later secretless instance must still never inherit the legacy token, exists=%v err=%v", exists, err)
	}
}

// TestMigration_MultipleInstancesLeaveLegacyWatchUnbound ensures the watch
// rebuild preserves the unbound fallback when a workspace has several possible
// instance targets rather than guessing the oldest one.
func TestMigration_MultipleInstancesLeaveLegacyWatchUnbound(t *testing.T) {
	db := openMigrationDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	first, second := uuid.New().String(), uuid.New().String()
	if _, err := db.Exec(`CREATE TABLE sentry_configs (` + sentryConfigsColumns + `)`); err != nil {
		t.Fatalf("create id-keyed configs: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO sentry_configs (id, workspace_id, name, auth_method, url, last_ok, last_error, created_at, updated_at)
		VALUES (?, 'ws-1', 'First', ?, 'https://first.example.com', 0, '', ?, ?),
		       (?, 'ws-1', 'Second', ?, 'https://second.example.com', 0, '', ?, ?)`,
		first, AuthMethodAuthToken, now, now,
		second, AuthMethodAuthToken, now.Add(time.Minute), now.Add(time.Minute)); err != nil {
		t.Fatalf("seed instances: %v", err)
	}
	if _, err := db.Exec(workspaceScopedWatchesDDL); err != nil {
		t.Fatalf("create legacy watches: %v", err)
	}
	watchID := uuid.New().String()
	if _, err := db.Exec(`
		INSERT INTO sentry_issue_watches (id, workspace_id, workflow_id, workflow_step_id, filter_json, created_at, updated_at)
		VALUES (?, 'ws-1', 'wf', 'step', '{}', ?, ?)`, watchID, now, now); err != nil {
		t.Fatalf("seed legacy watch: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("migrate watches: %v", err)
	}
	watch, err := store.GetIssueWatch(ctx, watchID)
	if err != nil {
		t.Fatalf("get migrated watch: %v", err)
	}
	if watch.SentryInstanceID != "" {
		t.Fatalf("multi-instance legacy watch bound to %q, want unbound", watch.SentryInstanceID)
	}
}

// TestMigration_ExperimentalBindingSurvivesRebuild ensures a valid explicit
// binding from the experimental no-FK shape is not overwritten by backfill.
func TestMigration_ExperimentalBindingSurvivesRebuild(t *testing.T) {
	db := openMigrationDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	first, selected := uuid.New().String(), uuid.New().String()
	if _, err := db.Exec(`CREATE TABLE sentry_configs (` + sentryConfigsColumns + `)`); err != nil {
		t.Fatalf("create id-keyed configs: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO sentry_configs (id, workspace_id, name, auth_method, url, last_ok, last_error, created_at, updated_at)
		VALUES (?, 'ws-1', 'First', ?, 'https://first.example.com', 0, '', ?, ?),
		       (?, 'ws-1', 'Selected', ?, 'https://selected.example.com', 0, '', ?, ?)`,
		first, AuthMethodAuthToken, now, now,
		selected, AuthMethodAuthToken, now.Add(time.Minute), now.Add(time.Minute)); err != nil {
		t.Fatalf("seed instances: %v", err)
	}
	if _, err := db.Exec(experimentalWatchesDDL); err != nil {
		t.Fatalf("create experimental watches: %v", err)
	}
	watchID := uuid.New().String()
	if _, err := db.Exec(`
		INSERT INTO sentry_issue_watches (id, workspace_id, sentry_instance_id, workflow_id, workflow_step_id, filter_json, created_at, updated_at)
		VALUES (?, 'ws-1', ?, 'wf', 'step', '{}', ?, ?)`, watchID, selected, now, now); err != nil {
		t.Fatalf("seed experimental watch: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("migrate watches: %v", err)
	}
	watch, err := store.GetIssueWatch(ctx, watchID)
	if err != nil {
		t.Fatalf("get migrated watch: %v", err)
	}
	if watch.SentryInstanceID != selected {
		t.Fatalf("experimental binding = %q, want %q", watch.SentryInstanceID, selected)
	}
}
