package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLines(t *testing.T, path string, n int) {
	t.Helper()
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "line-%d\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestTail_LargerThanRequested(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	writeLines(t, path, 2000)

	svc := newTestService(t, dir)
	lines, err := svc.Tail(1000)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 1000 {
		t.Fatalf("len(lines) = %d, want 1000", len(lines))
	}
	if lines[0] != "line-1001" {
		t.Errorf("first line = %q, want line-1001", lines[0])
	}
	if lines[999] != "line-2000" {
		t.Errorf("last line = %q, want line-2000", lines[999])
	}
	// Order preserved.
	for i, want := 1001, 0; want < 1000; i, want = i+1, want+1 {
		if lines[want] != fmt.Sprintf("line-%d", i) {
			t.Fatalf("lines[%d] = %q, want line-%d", want, lines[want], i)
			break
		}
	}
}

func TestTail_SmallerThanRequested(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	writeLines(t, path, 500)

	svc := newTestService(t, dir)
	lines, err := svc.Tail(1000)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 500 {
		t.Fatalf("len(lines) = %d, want 500", len(lines))
	}
	if lines[0] != "line-1" {
		t.Errorf("first = %q, want line-1", lines[0])
	}
	if lines[499] != "line-500" {
		t.Errorf("last = %q, want line-500", lines[499])
	}
}

func TestTail_ZeroReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	writeLines(t, path, 100)

	svc := newTestService(t, dir)
	lines, err := svc.Tail(0)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("len(lines) = %d, want 0", len(lines))
	}
}

func TestTail_NegativeReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	writeLines(t, path, 10)

	svc := newTestService(t, dir)
	lines, err := svc.Tail(-5)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("len(lines) = %d, want 0 for negative n", len(lines))
	}
}

func TestTail_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir)
	lines, err := svc.Tail(100)
	if err != nil {
		t.Fatalf("Tail on missing file should not error, got %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("len(lines) = %d, want 0", len(lines))
	}
}

func TestTail_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	svc := newTestService(t, dir)
	lines, err := svc.Tail(100)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("len(lines) = %d, want 0", len(lines))
	}
}

func TestTail_FileWithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	// 3 lines but final has no trailing newline.
	content := "a\nb\nc"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := newTestService(t, dir)
	lines, err := svc.Tail(10)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3, got %v", len(lines), lines)
	}
	if lines[0] != "a" || lines[1] != "b" || lines[2] != "c" {
		t.Errorf("lines = %v, want [a b c]", lines)
	}
}

func TestTail_LongLineExceedsChunkSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	long := strings.Repeat("x", 10_000) // larger than 4KB chunk
	content := "first\n" + long + "\nlast\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := newTestService(t, dir)
	lines, err := svc.Tail(3)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	if lines[0] != "first" {
		t.Errorf("lines[0] = %q, want first", lines[0])
	}
	if lines[1] != long {
		t.Errorf("lines[1] truncated: len=%d, want %d", len(lines[1]), len(long))
	}
	if lines[2] != "last" {
		t.Errorf("lines[2] = %q, want last", lines[2])
	}
}

func TestOpen_PathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir)
	for _, name := range []string{"../etc/passwd", "../../foo.log", "subdir/x.log", "/etc/passwd", ""} {
		_, _, err := svc.Open(name)
		if err == nil {
			t.Errorf("Open(%q) returned no error, want rejection", name)
		}
	}
}

func TestOpen_NonexistentFileError(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir)
	_, _, err := svc.Open("does-not-exist.log")
	if err == nil {
		t.Error("Open() returned no error for nonexistent file")
	}
}

func TestOpen_OpensExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kandev.log")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	svc := newTestService(t, dir)
	f, size, err := svc.Open("kandev.log")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()
	if size != 5 {
		t.Errorf("size = %d, want 5", size)
	}
}
