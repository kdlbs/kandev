package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestService(t *testing.T, logDir string) *Service {
	t.Helper()
	return NewService(logDir, "kandev.log", nil)
}

func TestList_EmptyDirReturnsEmptySlice(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir)

	files, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if files == nil {
		t.Fatal("List() returned nil, want non-nil empty slice")
	}
	if len(files) != 0 {
		t.Errorf("len(files) = %d, want 0", len(files))
	}
}

func TestList_MissingDirReturnsEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	svc := newTestService(t, dir)

	files, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("len(files) = %d, want 0", len(files))
	}
}

func TestList_EmptyLogDirConfigReturnsEmpty(t *testing.T) {
	svc := NewService("", "kandev.log", nil)
	files, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("len(files) = %d, want 0 for empty logDir", len(files))
	}
}

func TestList_SortsNewestFirstAndMarksCurrent(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "kandev-2026-01-01T00-00-00.000.log")
	newer := filepath.Join(dir, "kandev-2026-05-01T00-00-00.000.log.gz")
	current := filepath.Join(dir, "kandev.log")

	for _, p := range []string{older, newer, current} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	// Force distinct mtimes: older < newer < current.
	base := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(older, base, base); err != nil {
		t.Fatalf("chtimes older: %v", err)
	}
	if err := os.Chtimes(newer, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}
	if err := os.Chtimes(current, base.Add(90*time.Minute), base.Add(90*time.Minute)); err != nil {
		t.Fatalf("chtimes current: %v", err)
	}

	svc := newTestService(t, dir)
	files, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}

	// Newest first ordering.
	if files[0].Name != "kandev.log" {
		t.Errorf("files[0] = %q, want kandev.log", files[0].Name)
	}
	if files[1].Name != "kandev-2026-05-01T00-00-00.000.log.gz" {
		t.Errorf("files[1] = %q", files[1].Name)
	}
	if files[2].Name != "kandev-2026-01-01T00-00-00.000.log" {
		t.Errorf("files[2] = %q", files[2].Name)
	}

	// Exactly one Current.
	currentCount := 0
	for _, f := range files {
		if f.Current {
			currentCount++
			if f.Name != "kandev.log" {
				t.Errorf("Current=true on wrong file: %q", f.Name)
			}
		}
	}
	if currentCount != 1 {
		t.Errorf("currentCount = %d, want 1", currentCount)
	}
}

func TestList_IgnoresNeighboringNonLogFiles(t *testing.T) {
	// Simulate logging.outputPath pointing at a shared directory: List must
	// only surface the active log + lumberjack rotations, never unrelated
	// neighbours like ".env" or "config.yaml".
	dir := t.TempDir()
	for _, name := range []string{
		"kandev.log",
		"kandev-2026-01-01T00-00-00.000.log",
		"kandev-2026-05-01T00-00-00.000.log.gz",
		".env",
		"config.yaml",
		"secrets",
		"kandev",                         // no extension
		"other.log",                      // different base
		"kandev-suffixed.txt",            // wrong extension
		"kandevish-2026-01-01.000.log",   // base mismatch (no leading "kandev-")
		"kandev-cli.log",                 // matching prefix but not a timestamp
		"kandev-2026-01-01.log",          // timestamp missing time part
		"kandev-2026-01-01T00-00-00.log", // timestamp missing milliseconds
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	svc := newTestService(t, dir)
	files, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	got := map[string]bool{}
	for _, f := range files {
		got[f.Name] = true
	}
	want := []string{
		"kandev.log",
		"kandev-2026-01-01T00-00-00.000.log",
		"kandev-2026-05-01T00-00-00.000.log.gz",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("List() missing %q", w)
		}
	}
	for _, ban := range []string{
		".env", "config.yaml", "secrets", "kandev", "other.log",
		"kandev-suffixed.txt", "kandevish-2026-01-01.000.log",
		"kandev-cli.log", "kandev-2026-01-01.log", "kandev-2026-01-01T00-00-00.log",
	} {
		if got[ban] {
			t.Errorf("List() leaked %q (not a log file)", ban)
		}
	}
	if len(files) != len(want) {
		t.Errorf("len(files) = %d, want %d", len(files), len(want))
	}
}

func TestOpen_RejectsNeighboringNonLogFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=hunter2"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := newTestService(t, dir)
	if _, _, err := svc.Open(".env"); err == nil {
		t.Fatal("Open(\".env\") returned nil, want allow-list rejection")
	}
}

func TestList_IgnoresSubdirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kandev.log"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	svc := newTestService(t, dir)
	files, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1 (subdir filtered)", len(files))
	}
	if files[0].Name != "kandev.log" {
		t.Errorf("files[0] = %q", files[0].Name)
	}
}

func TestOpen_ReadsRegularFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kandev.log"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := newTestService(t, dir)

	f, size, err := svc.Open("kandev.log")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if size != 5 {
		t.Errorf("size = %d, want 5", size)
	}
}

func TestOpen_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.log")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}
	svc := newTestService(t, dir)

	if _, _, err := svc.Open("link.log"); err == nil {
		t.Fatal("Open(symlink) returned nil, want not-a-regular-file error")
	}
}

func TestOpen_RejectsTraversalAndSeparators(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir)

	for _, name := range []string{
		"../etc/passwd",
		"sub/file.log",
		"\\windows\\style.log",
		".",
		"..",
		"",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := svc.Open(name); err == nil {
				t.Fatalf("Open(%q) returned nil, want error", name)
			}
		})
	}
}

func TestContainedPath_ReturnedPathIsInsideLogDir(t *testing.T) {
	// containedPath's escape-rejection branch is architecturally unreachable
	// for single-segment clean names (filepath.Join of a non-empty rootAbs
	// and a bare filename without separators always stays inside the root).
	// This test exercises the happy path and verifies the postcondition:
	// the returned absolute path must be a child of the log directory.
	dir := t.TempDir()
	svc := newTestService(t, dir)

	// A clean filename that would resolve outside the directory after
	// filepath.Join cannot exist by construction (a single segment without
	// separators always stays inside the directory). Document this by
	// exercising the happy path and verifying the returned path is under dir.
	got, err := svc.containedPath("kandev.log")
	if err != nil {
		t.Fatalf("containedPath: %v", err)
	}
	rootAbs, _ := filepath.Abs(dir)
	rel, err := filepath.Rel(rootAbs, got)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Errorf("containedPath = %q escapes %q", got, rootAbs)
	}
}
