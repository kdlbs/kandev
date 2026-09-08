package logbundle

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackendCandidatesDiscoverNewestSegments(t *testing.T) {
	home := t.TempDir()
	logDir := filepath.Join(home, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("create log directory: %v", err)
	}
	for name, contents := range map[string]string{
		"backend-logs.log":                       "active",
		"backend-logs-2026-08-22-000001.log":     "first",
		"backend-logs-2026-08-22-000002.log":     "latest",
		"backend-logs-2026-08-23-000001.log":     "future",
		"backend-logs-2026-08-21.log":            "legacy",
		"backend-logs-2026-08-19-000001.log":     "expired",
		"backend-logs-not-a-segment.log":         "malformed",
		"backend-logs-2026-08-22-000003.log.tmp": "temporary",
	} {
		writeTestFile(t, filepath.Join(logDir, name), contents)
	}
	writeTestFile(t, filepath.Join(logDir, "notes.log"), "private")
	if err := os.Symlink(filepath.Join(logDir, "notes.log"), filepath.Join(logDir, "backend-logs-2026-08-22-000004.log")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	service := newTestService(t, Config{
		HomeDir: home,
		Now:     func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
	})
	candidates := service.backendCandidates()
	if got, want := len(candidates), 4; got != want {
		t.Fatalf("candidate count = %d, want %d", got, want)
	}
	want := []string{
		"backend-logs.log",
		"backend-logs-2026-08-22-000002.log",
		"backend-logs-2026-08-22-000001.log",
		"backend-logs-2026-08-21.log",
	}
	for index, candidate := range candidates {
		if candidate.Name != want[index] {
			t.Fatalf("candidate[%d] = %q, want %q", index, candidate.Name, want[index])
		}
	}
}

func TestBackendOnlyBundleKeepsNewestSegmentsWithinBudget(t *testing.T) {
	home := t.TempDir()
	logDir := filepath.Join(home, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("create log directory: %v", err)
	}
	writeTestFile(t, filepath.Join(logDir, "backend-logs.log"), "latest\n")
	writeTestFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000002.log"), "second\n")
	writeTestFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000001.log"), "first\n")
	writeTestFile(t, filepath.Join(logDir, "backend-logs-2026-08-21.log"), "yesterday\n")
	service := newTestService(t, Config{
		HomeDir: home,
		Now:     func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
	})

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	files, included, partial, warnings, err := service.addBackendFiles(writer, false, nil, int64(len("latest\nsecond\n")))
	if err != nil {
		t.Fatalf("add backend files: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	if !included || !partial || len(warnings) == 0 || len(files) != 2 {
		t.Fatalf("files=%#v included=%v partial=%v warnings=%v", files, included, partial, warnings)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	contents := make(map[string]string)
	for _, file := range reader.File {
		if file.Name == "manifest.json" {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		data, err := io.ReadAll(opened)
		_ = opened.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		contents[file.Name] = string(data)
	}
	if contents["backend/backend-logs.log"] != "latest\n" || contents["backend/backend-logs-2026-08-22-000002.log"] != "second\n" {
		t.Fatalf("newest archive contents = %#v", contents)
	}
	if _, ok := contents["backend/backend-logs-2026-08-22-000001.log"]; ok {
		t.Fatal("old segment was included after the archive budget was full")
	}
}

func TestBackendCandidateCollectionUsesOpenedHandleAcrossRotation(t *testing.T) {
	home := t.TempDir()
	logDir := filepath.Join(home, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("create log directory: %v", err)
	}
	path := filepath.Join(logDir, "backend-logs.log")
	writeTestFile(t, path, "old-content")
	service := newTestService(t, Config{
		HomeDir: home,
		Now:     func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) },
	})
	candidates := service.backendCandidates()
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	rotatedPath := path + ".rotated"
	open := func(candidatePath string) (*os.File, error) {
		source, err := openBackendCandidate(candidatePath)
		if err != nil {
			return nil, err
		}
		if err := os.Rename(candidatePath, rotatedPath); err != nil {
			_ = source.Close()
			return nil, err
		}
		writeTestFile(t, candidatePath, "new")
		return source, nil
	}
	file, included, err := addBackendCandidate(writer, candidates[0], int64(len("old-content")), open)
	if err != nil {
		t.Fatalf("add backend candidate: %v", err)
	}
	if !included || file.Length != int64(len("old-content")) {
		t.Fatalf("file=%#v included=%v", file, included)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	opened, err := reader.File[0].Open()
	if err != nil {
		t.Fatalf("open archived log: %v", err)
	}
	contents, err := io.ReadAll(opened)
	_ = opened.Close()
	if err != nil {
		t.Fatalf("read archived log: %v", err)
	}
	if got := string(contents); got != "old-content" {
		t.Fatalf("archived contents = %q, want old-content", got)
	}
}
