package ownershiplock

import (
	"path/filepath"
	"testing"
)

func TestTargetsDefaultSQLiteUsesHomeOwnershipOnly(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home", ".", "nested", "..")

	targets, err := Targets(home, "sqlite", "")
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %#v, want one home target", targets)
	}
	if targets[0].Kind != TargetHome {
		t.Fatalf("target kind = %q, want %q", targets[0].Kind, TargetHome)
	}
	if targets[0].ResourcePath != filepath.Clean(filepath.Join(filepath.Dir(home), "home")) {
		t.Fatalf("resource path = %q, want canonical home", targets[0].ResourcePath)
	}
}

func TestTargetsExternalSQLiteAddsCanonicalDatabaseTarget(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	databaseDir := filepath.Join(t.TempDir(), "database")
	databasePath := filepath.Join(databaseDir, "..", "database", "kandev.db")

	targets, err := Targets(home, "sqlite", databasePath)
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %#v, want home and database targets", targets)
	}
	if targets[1].Kind != TargetDatabase {
		t.Fatalf("database target kind = %q, want %q", targets[1].Kind, TargetDatabase)
	}
	wantDatabase := filepath.Join(databaseDir, "kandev.db")
	if targets[1].ResourcePath != wantDatabase {
		t.Fatalf("database resource path = %q, want %q", targets[1].ResourcePath, wantDatabase)
	}
	if targets[1].LockPath != wantDatabase+".lock" {
		t.Fatalf("database lock path = %q, want %q", targets[1].LockPath, wantDatabase+".lock")
	}
}

func TestTargetsDoesNotDuplicateDatabaseInsideHome(t *testing.T) {
	home := t.TempDir()
	databasePath := filepath.Join(home, "data", "..", "data", "kandev.db")

	targets, err := Targets(filepath.Join(home, "."), "sqlite", databasePath)
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %#v, want one canonical home target", targets)
	}
}

func TestTargetsPostgresLocksOnlyHome(t *testing.T) {
	targets, err := Targets(t.TempDir(), "postgres", filepath.Join(t.TempDir(), "external.db"))
	if err != nil {
		t.Fatalf("Targets: %v", err)
	}
	if len(targets) != 1 || targets[0].Kind != TargetHome {
		t.Fatalf("targets = %#v, want only home target", targets)
	}
}
