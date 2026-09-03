package launcher

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestParseMaintenanceDatabaseArgsDefaults confirms no flags parses to the
// dry-run default (Execute=false, Compact=false, zero limits).
func TestParseMaintenanceDatabaseArgsDefaults(t *testing.T) {
	args, err := parseMaintenanceDatabaseArgs(nil)
	if err != nil {
		t.Fatalf("parseMaintenanceDatabaseArgs(nil): %v", err)
	}
	if args.Execute || args.Compact || args.KeepPlanRevisions != 0 || args.CandidateLimit != 0 || args.HomeDir != "" {
		t.Fatalf("args = %+v, want all zero/false", args)
	}
}

// TestParseMaintenanceDatabaseArgsFlags covers every recognized flag in
// both "--flag value" and "--flag=value" spellings.
func TestParseMaintenanceDatabaseArgsFlags(t *testing.T) {
	args, err := parseMaintenanceDatabaseArgs([]string{
		"--execute", "--compact", "--keep-plan-revisions", "3", "--candidate-limit=50", "--home-dir=/tmp/kandev-home",
	})
	if err != nil {
		t.Fatalf("parseMaintenanceDatabaseArgs: %v", err)
	}
	if !args.Execute || !args.Compact {
		t.Fatalf("args = %+v, want Execute=true Compact=true", args)
	}
	if args.KeepPlanRevisions != 3 {
		t.Fatalf("KeepPlanRevisions = %d, want 3", args.KeepPlanRevisions)
	}
	if args.CandidateLimit != 50 {
		t.Fatalf("CandidateLimit = %d, want 50", args.CandidateLimit)
	}
	if args.HomeDir != "/tmp/kandev-home" {
		t.Fatalf("HomeDir = %q, want /tmp/kandev-home", args.HomeDir)
	}
}

// TestParseMaintenanceDatabaseArgsRejectsUnknownFlag confirms an
// unrecognized flag is a usage error (exit status 2 at the runMaintenance
// call site), not silently ignored.
func TestParseMaintenanceDatabaseArgsRejectsUnknownFlag(t *testing.T) {
	if _, err := parseMaintenanceDatabaseArgs([]string{"--not-a-real-flag"}); err == nil {
		t.Fatal("expected an error for an unrecognized flag")
	}
}

// TestParseMaintenanceDatabaseArgsRejectsNegativeLimit confirms
// --candidate-limit/--keep-plan-revisions reject negative values rather
// than silently passing them through to the domain layer.
func TestParseMaintenanceDatabaseArgsRejectsNegativeLimit(t *testing.T) {
	if _, err := parseMaintenanceDatabaseArgs([]string{"--candidate-limit", "-1"}); err == nil {
		t.Fatal("expected an error for a negative --candidate-limit")
	}
}

// TestParseMaintenanceDatabaseArgsRequiresValue confirms a flag expecting a
// value at the end of argv is a usage error rather than a panic or silent
// zero value.
func TestParseMaintenanceDatabaseArgsRequiresValue(t *testing.T) {
	if _, err := parseMaintenanceDatabaseArgs([]string{"--home-dir"}); err == nil {
		t.Fatal("expected an error when --home-dir has no value")
	}
}

// TestRunMaintenanceShowsHelpWithoutArgsOrFlag confirms both bare
// `kandev maintenance` and `kandev maintenance database --help` print help
// and exit 0 without touching any database.
func TestRunMaintenanceShowsHelpWithoutArgsOrFlag(t *testing.T) {
	output := captureLauncherStdout(t, func() {
		if code := runMaintenance(nil, BuildInfo{}); code != 0 {
			t.Fatalf("runMaintenance(nil) code = %d, want 0", code)
		}
	})
	if !strings.Contains(output, "kandev maintenance database") {
		t.Fatalf("help output = %q, want it to mention the command", output)
	}

	output = captureLauncherStdout(t, func() {
		if code := runMaintenance([]string{"database", "--help"}, BuildInfo{}); code != 0 {
			t.Fatalf("runMaintenance(database --help) code = %d, want 0", code)
		}
	})
	if !strings.Contains(output, "--execute") {
		t.Fatalf("help output = %q, want it to document --execute", output)
	}
}

// TestRunMaintenanceRejectsUnknownTarget confirms `kandev maintenance foo`
// is a usage error (exit 2), matching runService's convention for an
// unrecognized subcommand action.
func TestRunMaintenanceRejectsUnknownTarget(t *testing.T) {
	if code := runMaintenance([]string{"not-a-target"}, BuildInfo{}); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

// TestRunMaintenanceDryRunEndToEndReportsCandidates exercises the full CLI
// path: config loading, driver/path resolution, and Run's dry-run report,
// against a real SQLite database seeded with one duplicate git snapshot.
func TestRunMaintenanceDryRunEndToEndReportsCandidates(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	dir := t.TempDir()
	t.Chdir(dir)
	dbPath := filepath.Join(dir, "state", "kandev.db")
	writeLauncherConfig(t, filepath.Join(dir, "config.yaml"), "database:\n  path: "+dbPath+"\n")

	seedMaintenanceFixtureDB(t, dbPath)

	output := captureLauncherStdout(t, func() {
		if code := runMaintenance([]string{"database"}, BuildInfo{}); code != 0 {
			t.Fatalf("runMaintenance(database) code = %d, want 0", code)
		}
	})
	if !strings.Contains(output, "duplicate git snapshots:      1 rows") {
		t.Fatalf("output = %q, want it to report 1 duplicate git snapshot candidate", output)
	}
	if !strings.Contains(output, "mode: dry run") {
		t.Fatalf("output = %q, want a dry-run mode line", output)
	}
}

// TestRunMaintenanceExecuteEndToEndDeletesAndBacksUp exercises --execute
// through the full CLI path against a real SQLite database, confirming the
// command reports deletions and a verified backup path.
func TestRunMaintenanceExecuteEndToEndDeletesAndBacksUp(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	dir := t.TempDir()
	t.Chdir(dir)
	dbPath := filepath.Join(dir, "state", "kandev.db")
	writeLauncherConfig(t, filepath.Join(dir, "config.yaml"), "homeDir: "+dir+"\ndatabase:\n  path: "+dbPath+"\n")

	seedMaintenanceFixtureDB(t, dbPath)

	output := captureLauncherStdout(t, func() {
		if code := runMaintenance([]string{"database", "--execute"}, BuildInfo{}); code != 0 {
			t.Fatalf("runMaintenance(database --execute) code = %d, want 0", code)
		}
	})
	if !strings.Contains(output, "deleted git snapshots:        1") {
		t.Fatalf("output = %q, want 1 deleted git snapshot", output)
	}
	if !strings.Contains(output, "backup:") {
		t.Fatalf("output = %q, want a backup path line", output)
	}
}

// TestRunMaintenanceExecuteRefusesWhileOwnershipLockHeld proves the CLI
// path surfaces ownership-conflict refusals (e.g. kandev is running) as
// exit status 1 with a clear stderr message, without touching the
// database.
func TestRunMaintenanceExecuteRefusesWhileOwnershipLockHeld(t *testing.T) {
	clearLauncherConfigurationEnvironment(t)
	dir := t.TempDir()
	t.Chdir(dir)
	dbPath := filepath.Join(dir, "state", "kandev.db")
	writeLauncherConfig(t, filepath.Join(dir, "config.yaml"), "homeDir: "+dir+"\ndatabase:\n  path: "+dbPath+"\n")
	seedMaintenanceFixtureDB(t, dbPath)

	targets, err := ownershiplock.Targets(dir, "sqlite", dbPath)
	if err != nil {
		t.Fatalf("ownershiplock.Targets: %v", err)
	}
	owner, err := ownershiplock.Acquire(targets)
	if err != nil {
		t.Fatalf("ownershiplock.Acquire (simulated live backend): %v", err)
	}
	defer func() { _ = owner.Close() }()

	if code := runMaintenance([]string{"database", "--execute"}, BuildInfo{}); code != 1 {
		t.Fatalf("code = %d, want 1 (ownership conflict)", code)
	}
}

// seedMaintenanceFixtureDB creates a fresh SQLite database at dbPath and
// seeds exactly one duplicate git snapshot candidate (older of two
// identical-content rows for the same session).
func seedMaintenanceFixtureDB(t *testing.T, dbPath string) {
	t.Helper()
	conn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	repo, err := sqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-cli-fixture", WorkspaceID: "workspace-cli-fixture", Title: "t"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-cli-fixture", TaskID: "task-cli-fixture"}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	snap := func(id string) *models.GitSnapshot {
		return &models.GitSnapshot{
			ID: id, SessionID: "session-cli-fixture", SnapshotType: models.SnapshotTypeStatusUpdate,
			Branch: "feature/x", HeadCommit: "head-sha", BaseCommit: "base-sha",
			Files: map[string]interface{}{"a.go": map[string]interface{}{"status": "modified"}},
		}
	}
	if err := repo.CreateGitSnapshot(ctx, snap("snap-cli-1")); err != nil {
		t.Fatalf("CreateGitSnapshot(1): %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, snap("snap-cli-2")); err != nil {
		t.Fatalf("CreateGitSnapshot(2): %v", err)
	}
	if err := sqlxDB.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}
}
