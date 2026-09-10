package databasestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/system/storage/filescan"
)

func TestAnalyzeSQLiteFootprint(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "data", "named.db")
	writeSizedFile(t, databasePath, 11)
	writeSizedFile(t, databasePath+"-wal", 7)
	writeSizedFile(t, databasePath+"-shm", 5)
	writeSizedFile(t, databasePath+"-journal", 3)

	backupDir := filepath.Join(root, "data", "backups")
	writeSizedFile(t, filepath.Join(backupDir, "automatic.db"), 13)
	writeSizedFile(t, filepath.Join(backupDir, "manual", "snapshot.db"), 17)
	external := filepath.Join(root, "outside.bin")
	writeSizedFile(t, external, 29)
	if err := os.Symlink(external, filepath.Join(backupDir, "linked.bin")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	provider := New(Config{Driver: "sqlite", DatabasePath: databasePath})
	database, err := provider.AnalyzeDatabase(context.Background(), nil)
	if err != nil {
		t.Fatalf("AnalyzeDatabase: %v", err)
	}
	assertMeasured(t, database, databasePath, 26)

	backups, err := provider.AnalyzeBackups(context.Background(), nil)
	if err != nil {
		t.Fatalf("AnalyzeBackups: %v", err)
	}
	assertMeasured(t, backups, backupDir, 30)
	if !strings.Contains(backups.Warning, "linked.bin") {
		t.Fatalf("backup warning = %q, want skipped symlink", backups.Warning)
	}
}

func TestAnalyzeCustomRelativePathAndMissingBackups(t *testing.T) {
	root := t.TempDir()
	oldWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDir) })

	databasePath := filepath.Join(root, "relative", "custom.db")
	writeSizedFile(t, databasePath, 8)
	provider := New(Config{Driver: "sqlite", DatabasePath: filepath.Join("relative", "custom.db")})

	database, err := provider.AnalyzeDatabase(context.Background(), nil)
	if err != nil {
		t.Fatalf("AnalyzeDatabase: %v", err)
	}
	assertMeasured(t, database, databasePath, 8)

	backups, err := provider.AnalyzeBackups(context.Background(), nil)
	if err != nil {
		t.Fatalf("AnalyzeBackups: %v", err)
	}
	assertMeasured(t, backups, filepath.Join(root, "relative", "backups"), 0)
}

func TestAnalyzeNonSQLiteDoesNotReadFilesystem(t *testing.T) {
	provider := New(Config{Driver: "postgres", DatabasePath: filepath.Join(t.TempDir(), "missing.db")})

	for _, analyze := range []struct {
		name string
		fn   func(context.Context, func(filescan.Progress)) (Measurement, error)
	}{
		{name: "database", fn: func(ctx context.Context, notify func(filescan.Progress)) (Measurement, error) {
			return provider.AnalyzeDatabase(ctx, notify)
		}},
		{name: "backups", fn: func(ctx context.Context, notify func(filescan.Progress)) (Measurement, error) {
			return provider.AnalyzeBackups(ctx, notify)
		}},
	} {
		t.Run(analyze.name, func(t *testing.T) {
			measurement, err := analyze.fn(context.Background(), nil)
			if err != nil {
				t.Fatalf("analysis error: %v", err)
			}
			if measurement.Status != StatusNotApplicable {
				t.Fatalf("status = %q, want %q", measurement.Status, StatusNotApplicable)
			}
			if measurement.SizeBytes != nil || measurement.Path != "" || measurement.IncludedInTotal {
				t.Fatalf("non-SQLite measurement = %#v, want no local footprint", measurement)
			}
		})
	}
}

func TestAnalyzeMissingAndUnreadableFiles(t *testing.T) {
	root := t.TempDir()
	missingDatabase := filepath.Join(root, "missing.db")
	provider := New(Config{Driver: "sqlite", DatabasePath: missingDatabase})

	database, err := provider.AnalyzeDatabase(context.Background(), nil)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing database error = %v, want not-exist error", err)
	}
	if database.Status != StatusUnavailable || database.SizeBytes != nil || database.IncludedInTotal {
		t.Fatalf("missing database measurement = %#v, want unavailable", database)
	}

	databasePath := filepath.Join(root, "database.db")
	writeSizedFile(t, databasePath, 1)
	if err := os.WriteFile(filepath.Join(root, "backups"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider = New(Config{Driver: "sqlite", DatabasePath: databasePath})
	backups, err := provider.AnalyzeBackups(context.Background(), nil)
	if err == nil {
		t.Fatal("backup file path unexpectedly measured successfully")
	}
	if backups.Status != StatusUnavailable || backups.SizeBytes != nil || backups.IncludedInTotal {
		t.Fatalf("unreadable backup root = %#v, want unavailable", backups)
	}
}

func TestAnalyzeRejectsDatabaseAndBackupSymlinks(t *testing.T) {
	root := t.TempDir()
	databaseTarget := filepath.Join(root, "database-target.db")
	databasePath := filepath.Join(root, "database.db")
	writeSizedFile(t, databaseTarget, 4)
	if err := os.Symlink(databaseTarget, databasePath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	provider := New(Config{Driver: "sqlite", DatabasePath: databasePath})
	database, err := provider.AnalyzeDatabase(context.Background(), nil)
	if err == nil || database.Status != StatusUnavailable {
		t.Fatalf("database symlink measurement = %#v, error = %v; want unavailable", database, err)
	}

	backupTarget := filepath.Join(root, "backups-target")
	writeSizedFile(t, filepath.Join(backupTarget, "snapshot.db"), 6)
	backupPath := filepath.Join(root, "backups")
	if err := os.Symlink(backupTarget, backupPath); err != nil {
		t.Skipf("backup symlinks unavailable: %v", err)
	}
	regularDatabase := filepath.Join(root, "regular.db")
	writeSizedFile(t, regularDatabase, 4)
	provider = New(Config{Driver: "sqlite", DatabasePath: regularDatabase})
	backups, err := provider.AnalyzeBackups(context.Background(), nil)
	if err == nil || backups.Status != StatusUnavailable {
		t.Fatalf("backup symlink measurement = %#v, error = %v; want unavailable", backups, err)
	}
}

func TestAnalyzeOverlappingRootsRetainsMeasurementButExcludesTotal(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "database.db")
	writeSizedFile(t, databasePath, 4)
	writeSizedFile(t, filepath.Join(root, "backups", "snapshot.db"), 6)
	provider := New(Config{
		Driver:        "sqlite",
		DatabasePath:  databasePath,
		ExistingRoots: []string{root},
	})

	database, err := provider.AnalyzeDatabase(context.Background(), nil)
	if err != nil {
		t.Fatalf("AnalyzeDatabase: %v", err)
	}
	if database.Status != StatusMeasured || database.SizeBytes == nil || *database.SizeBytes != 4 {
		t.Fatalf("database measurement = %#v, want measured 4 bytes", database)
	}
	if database.IncludedInTotal || database.Reason != ReasonOverlapsExistingSource {
		t.Fatalf("database attribution = %#v, want overlap exclusion", database)
	}

	backups, err := provider.AnalyzeBackups(context.Background(), nil)
	if err != nil {
		t.Fatalf("AnalyzeBackups: %v", err)
	}
	if backups.Status != StatusMeasured || backups.SizeBytes == nil || *backups.SizeBytes != 6 {
		t.Fatalf("backup measurement = %#v, want measured 6 bytes", backups)
	}
	if backups.IncludedInTotal || backups.Reason != ReasonOverlapsExistingSource {
		t.Fatalf("backup attribution = %#v, want overlap exclusion", backups)
	}
}

func TestAnalyzeAdditionalRootsExcludeTotal(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "database.db")
	writeSizedFile(t, databasePath, 4)
	provider := New(Config{Driver: "sqlite", DatabasePath: databasePath})

	database, err := provider.AnalyzeDatabase(context.Background(), nil, root)
	if err != nil {
		t.Fatalf("AnalyzeDatabase: %v", err)
	}
	if database.Status != StatusMeasured || database.SizeBytes == nil || *database.SizeBytes != 4 {
		t.Fatalf("database measurement = %#v, want measured 4 bytes", database)
	}
	if database.IncludedInTotal || database.Reason != ReasonOverlapsExistingSource {
		t.Fatalf("database attribution = %#v, want additional-root overlap exclusion", database)
	}
}

func TestAnalyzeCancellationDoesNotReturnMeasurement(t *testing.T) {
	provider := New(Config{Driver: "sqlite", DatabasePath: filepath.Join(t.TempDir(), "missing.db")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	measurement, err := provider.AnalyzeDatabase(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want context.Canceled", err)
	}
	if measurement.Status != StatusUnavailable || measurement.SizeBytes != nil {
		t.Fatalf("cancelled measurement = %#v, want unavailable without bytes", measurement)
	}
}

func TestMissingOptionalSidecarResultIsIgnored(t *testing.T) {
	if !isMissingOptionalSidecar(1, os.ErrNotExist) {
		t.Fatal("missing sidecar result was not recognized")
	}
	if isMissingOptionalSidecar(0, os.ErrNotExist) {
		t.Fatal("missing primary database result was treated as optional")
	}
	if isMissingOptionalSidecar(1, errors.New("permission denied")) {
		t.Fatal("non-missing sidecar failure was ignored")
	}
}

func TestAnalyzeDoesNotModifyFiles(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "database.db")
	writeSizedFile(t, databasePath, 4)
	backupPath := filepath.Join(root, "backups", "snapshot.db")
	writeSizedFile(t, backupPath, 6)
	databaseBefore := fileBytes(t, databasePath)
	backupBefore := fileBytes(t, backupPath)

	provider := New(Config{Driver: "sqlite", DatabasePath: databasePath})
	if _, err := provider.AnalyzeDatabase(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.AnalyzeBackups(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := fileBytes(t, databasePath); got != databaseBefore {
		t.Fatalf("database changed from %d to %d bytes", databaseBefore, got)
	}
	if got := fileBytes(t, backupPath); got != backupBefore {
		t.Fatalf("backup changed from %d to %d bytes", backupBefore, got)
	}
}

func assertMeasured(t *testing.T, measurement Measurement, wantPath string, wantBytes int64) {
	t.Helper()
	if measurement.Status != StatusMeasured {
		t.Fatalf("status = %q, want %q: %#v", measurement.Status, StatusMeasured, measurement)
	}
	if measurement.Path != filepath.Clean(wantPath) {
		t.Fatalf("path = %q, want %q", measurement.Path, filepath.Clean(wantPath))
	}
	if measurement.SizeBytes == nil || *measurement.SizeBytes != wantBytes {
		t.Fatalf("size = %v, want %d", measurement.SizeBytes, wantBytes)
	}
	if !measurement.IncludedInTotal {
		t.Fatalf("measurement = %#v, want included in total", measurement)
	}
}

func writeSizedFile(t *testing.T, path string, size int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(size); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func fileBytes(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}
