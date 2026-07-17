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
