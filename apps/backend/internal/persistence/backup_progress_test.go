package persistence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/startup"
)

func newBackupStepContext() (context.Context, *startup.Reporter) {
	r := startup.New(nil)
	ctx := startup.WithReporter(context.Background(), r)
	startup.BeginStep(ctx, startup.StepDatabaseBackup)
	return ctx, r
}

func TestReportBackupProgressAdvancesOnSizeIncrease(t *testing.T) {
	ctx, r := newBackupStepContext()
	path := filepath.Join(t.TempDir(), "staging.db")
	if err := os.WriteFile(path, make([]byte, 100), 0o600); err != nil {
		t.Fatalf("write staged file: %v", err)
	}

	lastSize, degraded := reportBackupProgress(ctx, path, 0, false)
	if lastSize != 100 {
		t.Fatalf("lastSize = %d, want 100", lastSize)
	}
	if degraded {
		t.Fatal("degraded = true, want false")
	}

	snap := r.Snapshot()
	if snap.Step == nil || snap.Step.Done == nil || *snap.Step.Done != 100 {
		t.Fatalf("Step = %+v, want Done=100", snap.Step)
	}

	if err := os.WriteFile(path, make([]byte, 100), 0o600); err != nil {
		t.Fatalf("rewrite staged file: %v", err)
	}
	lastSize, degraded = reportBackupProgress(ctx, path, lastSize, degraded)
	if lastSize != 100 {
		t.Fatalf("lastSize after unchanged size = %d, want 100 (no regression, no duplicate advance)", lastSize)
	}
	if degraded {
		t.Fatal("degraded = true after unchanged size, want false")
	}
}

func TestReportBackupProgressIgnoresMissingFile(t *testing.T) {
	ctx, r := newBackupStepContext()
	path := filepath.Join(t.TempDir(), "not-yet-created.db")

	lastSize, degraded := reportBackupProgress(ctx, path, 0, false)
	if lastSize != 0 {
		t.Fatalf("lastSize = %d, want 0 (VACUUM INTO has not created the file yet)", lastSize)
	}
	if degraded {
		t.Fatal("degraded = true on ENOENT, want false: not yet started is not a measurement failure")
	}
	if snap := r.Snapshot(); snap.Step == nil || snap.Step.Done == nil || *snap.Step.Done != 0 {
		t.Fatalf("Step = %+v, want Done=0 (no advance recorded)", r.Snapshot().Step)
	}
}

func TestReportBackupProgressDegradesOnceOnRealStatError(t *testing.T) {
	ctx, _ := newBackupStepContext()
	// Stat-ing a path under a regular file (not a directory) fails with
	// ENOTDIR, which os.IsNotExist reports as false - a genuine stat error
	// distinct from "VACUUM INTO has not created the file yet".
	regularFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(regularFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write regular file: %v", err)
	}
	path := filepath.Join(regularFile, "staging.db")

	lastSize, degraded := reportBackupProgress(ctx, path, 0, false)
	if !degraded {
		t.Fatal("degraded = false after a real stat error, want true")
	}
	if lastSize != 0 {
		t.Fatalf("lastSize = %d, want 0", lastSize)
	}

	// A second tick against the same failing path must not degrade again
	// (Degrade only warns once per activation; this call proves the helper
	// itself does not re-invoke it once already degraded).
	lastSize, degraded = reportBackupProgress(ctx, path, lastSize, degraded)
	if !degraded {
		t.Fatal("degraded flag reset itself, want it to stay true")
	}
	if lastSize != 0 {
		t.Fatalf("lastSize = %d, want 0", lastSize)
	}
}

func TestTrackBackupProgressReportsGrowthAndStopsCleanly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, r := newBackupStepContext()
		path := filepath.Join(t.TempDir(), "staging.db")

		stop := trackBackupProgress(ctx, path)

		if err := os.WriteFile(path, make([]byte, 42), 0o600); err != nil {
			t.Fatalf("write staged file: %v", err)
		}
		time.Sleep(backupProgressTickInterval)
		synctest.Wait()

		snap := r.Snapshot()
		if snap.Step == nil || snap.Step.Done == nil || *snap.Step.Done != 42 {
			t.Fatalf("Step = %+v, want Done=42 after one tick", snap.Step)
		}

		stop()
		synctest.Wait()
	})
}
