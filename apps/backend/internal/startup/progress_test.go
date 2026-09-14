package startup

import (
	"context"
	"testing"
)

func TestReporterStartsWithOpeningDatabase(t *testing.T) {
	reporter := New(nil)
	snapshot := reporter.Snapshot()
	if snapshot.Phase != OpeningDatabase {
		t.Fatalf("initial phase = %q, want %q", snapshot.Phase, OpeningDatabase)
	}
	if snapshot.ElapsedMS < 0 || snapshot.PhaseElapsedMS < 0 {
		t.Fatalf("negative startup durations: %#v", snapshot)
	}
}

func TestReporterTransitionsAndKeepsProtocolLabels(t *testing.T) {
	reporter := New(nil)
	ctx := WithReporter(context.Background(), reporter)
	SetPhase(ctx, BackingUpDatabase)
	snapshot := reporter.Snapshot()
	if snapshot.Phase != BackingUpDatabase {
		t.Fatalf("phase = %q, want %q", snapshot.Phase, BackingUpDatabase)
	}
	if snapshot.Phase.Label() != "Backing up database" {
		t.Fatalf("phase label = %q, want %q", snapshot.Phase.Label(), "Backing up database")
	}
	if Phase("private_error").Label() != "" {
		t.Fatal("unknown phase exposed a label")
	}
}
