package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/store"
)

func newTestPool(t *testing.T) *db.Pool {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(`
		CREATE TABLE conversation_session_streams (
			session_id TEXT PRIMARY KEY,
			watermark INTEGER NOT NULL
		)`); err != nil {
		t.Fatalf("create conversation journal schema: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return db.NewPool(conn, conn)
}

func TestProvideConstructsServiceUsingHomeDirPluginsSubdir(t *testing.T) {
	homeDir := t.TempDir()
	cfg := &config.Config{HomeDir: homeDir}

	svc, cleanup, err := Provide(cfg, newTestPool(t), newFakeSecretRevealer(), nil, testLogger(t))
	if err != nil {
		t.Fatalf("Provide() unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	// Install through the REAL runtime.Manager Provide wires — testPackage's
	// "server/plugin" is a fake shell script, not a real go-plugin binary,
	// so the spawn/handshake genuinely fails here. That's fine: this test
	// only asserts Provide() persists installed records under
	// <HomeDir>/plugins, which happens before activation is attempted.
	rec, err := svc.Install(context.Background(), testPackage(t, "kandev-plugin-slack", "1.0.0", false))
	if err == nil {
		t.Fatal("Install() expected a spawn error against the real runtime.Manager with a fake executable")
	}
	if rec == nil {
		t.Fatalf("Install() expected a persisted record despite the spawn failure, err: %v", err)
	}

	wantPath := filepath.Join(homeDir, "plugins", "kandev-plugin-slack.yml")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected installed record file at %s: %v", wantPath, err)
	}
}

func TestSetPluginsDirKeepsInstallRootWhenConversationStateFails(t *testing.T) {
	dir := t.TempDir()
	hostDir := filepath.Join(dir, ".host")
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		t.Fatalf("create host directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hostDir, "conversation-token.key"), []byte("corrupt"), 0o600); err != nil {
		t.Fatalf("write corrupt signing key: %v", err)
	}

	svc := NewService(store.NewFSStore(filepath.Join(dir, "store")), NewRegistry(), nil, testLogger(t))
	t.Cleanup(func() { _ = svc.Close() })

	if err := svc.SetPluginsDir(dir); err == nil {
		t.Fatal("SetPluginsDir() expected corrupt signing key error")
	}
	if svc.pluginsDir != dir {
		t.Fatalf("pluginsDir = %q, want %q after initialization failure", svc.pluginsDir, dir)
	}
}

func TestSetPluginsDirRemovesLegacyConversationFiles(t *testing.T) {
	dir := t.TempDir()
	hostDir := filepath.Join(dir, ".host")
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		t.Fatalf("create host directory: %v", err)
	}
	for _, name := range []string{"session-events.sqlite", "session-events.sqlite-wal", "session-events.sqlite-shm"} {
		if err := os.WriteFile(filepath.Join(hostDir, name), []byte("legacy"), 0o600); err != nil {
			t.Fatalf("write legacy file %s: %v", name, err)
		}
	}

	svc := NewService(store.NewFSStore(filepath.Join(dir, "store")), NewRegistry(), nil, testLogger(t))
	t.Cleanup(func() { _ = svc.Close() })
	if err := svc.SetPluginsDir(dir); err != nil {
		t.Fatalf("SetPluginsDir() unexpected error: %v", err)
	}
	for _, name := range []string{"session-events.sqlite", "session-events.sqlite-wal", "session-events.sqlite-shm"} {
		if _, err := os.Stat(filepath.Join(hostDir, name)); !os.IsNotExist(err) {
			t.Fatalf("legacy file %s still exists: %v", name, err)
		}
	}
}

func TestSetPluginsDirRejectsLegacyConversationSymlink(t *testing.T) {
	dir := t.TempDir()
	hostDir := filepath.Join(dir, ".host")
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		t.Fatalf("create host directory: %v", err)
	}
	target := filepath.Join(dir, "outside.sqlite")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	legacy := filepath.Join(hostDir, "session-events.sqlite")
	if err := os.Symlink(target, legacy); err != nil {
		t.Fatalf("create legacy symlink: %v", err)
	}

	svc := NewService(store.NewFSStore(filepath.Join(dir, "store")), NewRegistry(), nil, testLogger(t))
	t.Cleanup(func() { _ = svc.Close() })
	if err := svc.SetPluginsDir(dir); err == nil {
		t.Fatal("SetPluginsDir() accepted a legacy conversation symlink")
	}
	if _, err := os.Lstat(legacy); err != nil {
		t.Fatalf("legacy symlink was removed: %v", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read symlink target: %v", err)
	}
	if string(contents) != "keep" {
		t.Fatalf("symlink target changed to %q", contents)
	}
}

func TestProvideLoadsExistingInstallationsFromDisk(t *testing.T) {
	homeDir := t.TempDir()
	cfg := &config.Config{HomeDir: homeDir}

	// Pre-populate the plugins dir directly via the store, simulating a
	// prior process's installation surviving a restart.
	pluginsDir := filepath.Join(homeDir, "plugins")
	preexisting := store.NewFSStore(pluginsDir)
	if err := preexisting.Save(&store.Record{Manifest: *testManifest("kandev-plugin-jira"), Status: store.StatusDisabled}); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	svc, cleanup, err := Provide(cfg, newTestPool(t), newFakeSecretRevealer(), nil, testLogger(t))
	if err != nil {
		t.Fatalf("Provide() unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	got, err := svc.Get("kandev-plugin-jira")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.ID != "kandev-plugin-jira" {
		t.Fatalf("Get() ID = %q, want %q", got.ID, "kandev-plugin-jira")
	}
}

func TestProvideWiresStateStore(t *testing.T) {
	cfg := &config.Config{HomeDir: t.TempDir()}

	svc, cleanup, err := Provide(cfg, newTestPool(t), newFakeSecretRevealer(), nil, testLogger(t))
	if err != nil {
		t.Fatalf("Provide() unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	if svc.StateStore() == nil {
		t.Fatalf("StateStore() = nil, want a wired *state.Store")
	}

	// Sanity-check it's actually usable, not just non-nil.
	if err := svc.StateStore().Set(context.Background(), "kandev-plugin-slack", "instance", "", "k", []byte(`"v"`)); err != nil {
		t.Fatalf("StateStore().Set() unexpected error: %v", err)
	}
}

func TestProvideWiresRuntimeManager(t *testing.T) {
	cfg := &config.Config{HomeDir: t.TempDir()}

	svc, cleanup, err := Provide(cfg, newTestPool(t), newFakeSecretRevealer(), nil, testLogger(t))
	if err != nil {
		t.Fatalf("Provide() unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	if svc.Runtime() == nil {
		t.Fatalf("Runtime() = nil, want a wired runtime.Manager")
	}
}

func TestProvideCleanupDoesNotError(t *testing.T) {
	cfg := &config.Config{HomeDir: t.TempDir()}

	_, cleanup, err := Provide(cfg, newTestPool(t), newFakeSecretRevealer(), nil, testLogger(t))
	if err != nil {
		t.Fatalf("Provide() unexpected error: %v", err)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() unexpected error: %v", err)
	}
}

func TestProvideWithStoreErrorsKeepsRequiredStoreResultsIndependent(t *testing.T) {
	cfg := &config.Config{HomeDir: t.TempDir()}

	svc, cleanup, storeErrors := ProvideWithStoreErrors(cfg, nil, newFakeSecretRevealer(), nil, testLogger(t))
	if svc == nil {
		t.Fatal("ProvideWithStoreErrors() service = nil, want partially initialized service")
	}
	t.Cleanup(func() { _ = cleanup() })
	wantIDs := []string{
		"plugin-instances", "plugin-marketplace", "plugin-settings",
		"plugin-state", "plugin-instance-state", "plugin-user-state",
	}
	if len(storeErrors) != len(wantIDs) {
		t.Fatalf("store error set has %d entries, want %d: %v", len(storeErrors), len(wantIDs), storeErrors)
	}
	for _, id := range wantIDs {
		if err := storeErrors[id]; err == nil {
			t.Errorf("%s error = nil, want independent database-pool error", id)
		}
	}
}
