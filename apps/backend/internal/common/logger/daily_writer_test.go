package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDailyWriterAppendsAndRollsByUTCDay(t *testing.T) {
	logDir := t.TempDir()
	now := time.Date(2026, 7, 29, 23, 59, 0, 0, time.FixedZone("local", 2*60*60))
	writer, err := newDailyWriter(logDir, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newDailyWriter: %v", err)
	}
	if _, err := writer.Write([]byte("day-one\n")); err != nil {
		t.Fatalf("write day one: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close day one: %v", err)
	}

	writer, err = newDailyWriter(logDir, func() time.Time { return now })
	if err != nil {
		t.Fatalf("restart same day: %v", err)
	}
	if _, err := writer.Write([]byte("same-day\n")); err != nil {
		t.Fatalf("write same day: %v", err)
	}
	now = time.Date(2026, 7, 30, 0, 1, 0, 0, time.UTC)
	if _, err := writer.Write([]byte("day-two\n")); err != nil {
		t.Fatalf("write day two: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close day two: %v", err)
	}

	archived := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-07-29-000001.log"))
	if archived != "day-one\nsame-day\n" {
		t.Fatalf("archived log = %q", archived)
	}
	if active := readLogFile(t, filepath.Join(logDir, "backend-logs.log")); active != "day-two\n" {
		t.Fatalf("active log = %q", active)
	}
}

func TestDailyWriterRetentionLeavesUnrelatedFiles(t *testing.T) {
	logDir := t.TempDir()
	for name := range map[string]bool{
		"backend-logs-2026-07-26.log": true,
		"backend-logs-2026-07-27.log": true,
		"backend-logs-2026-07-28.log": true,
		"backend-logs-2026-07-29.log": true,
		"backend-logs-not-a-day.log":  true,
		"another-service.log":         true,
	} {
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(name), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	writer, err := newDailyWriter(logDir, func() time.Time {
		return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("newDailyWriter: %v", err)
	}
	_ = writer.Close()

	for _, name := range []string{"backend-logs-2026-07-28.log", "backend-logs-2026-07-29.log",
		"backend-logs-not-a-day.log", "another-service.log"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); err != nil {
			t.Errorf("expected %s to remain: %v", name, err)
		}
	}
	for _, name := range []string{"backend-logs-2026-07-26.log", "backend-logs-2026-07-27.log"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, stat error = %v", name, err)
		}
	}
}

func TestDailyWriterResumesJournalWithoutDuplicatingDestination(t *testing.T) {
	logDir := t.TempDir()
	activePath := filepath.Join(logDir, activeBackendLogName)
	source := []byte("first\nsecond\nthird\n")
	if err := os.WriteFile(activePath, source, 0o600); err != nil {
		t.Fatalf("seed active log: %v", err)
	}
	info, err := os.Stat(activePath)
	if err != nil {
		t.Fatalf("stat active log: %v", err)
	}
	destination := "backend-logs-2026-07-29.log"
	prefix := source[:len("first\n")]
	if err := os.WriteFile(filepath.Join(logDir, destination), prefix, 0o600); err != nil {
		t.Fatalf("seed destination: %v", err)
	}
	journal := rolloverJournal{
		SourceDay: "2026-07-29", SourceSize: info.Size(),
		SourceModUnixNano: info.ModTime().UnixNano(), Destination: destination,
		DestinationStart: 0, CopiedOffset: int64(len(prefix)),
	}
	if err := writeJSONOwnerFile(filepath.Join(logDir, rolloverJournalName), journal); err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, backendDayMarkerName), []byte("2026-07-29"), 0o600); err != nil {
		t.Fatalf("seed marker: %v", err)
	}

	now := func() time.Time { return time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC) }
	writer, err := newDailyWriter(logDir, now)
	if err != nil {
		t.Fatalf("recover writer: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close recovered writer: %v", err)
	}
	writer, err = newDailyWriter(logDir, now)
	if err != nil {
		t.Fatalf("restart recovered writer: %v", err)
	}
	_ = writer.Close()

	if got := readLogFile(t, filepath.Join(logDir, destination)); got != string(source) {
		t.Fatalf("recovered destination = %q", got)
	}
	if _, err := os.Stat(filepath.Join(logDir, rolloverJournalName)); !os.IsNotExist(err) {
		t.Fatalf("rollover journal remains after recovery: %v", err)
	}
}

func TestDailyWriterRejectsEntryLargerThanSegment(t *testing.T) {
	logDir := t.TempDir()
	activePath := filepath.Join(logDir, activeBackendLogName)
	const dailyLimit = int64(len("full\n"))
	if err := os.WriteFile(activePath, []byte("full\n"), 0o600); err != nil {
		t.Fatalf("seed active log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, backendDayMarkerName),
		[]byte("2026-07-30"), 0o600); err != nil {
		t.Fatalf("seed day marker: %v", err)
	}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	writer, err := newDailyWriter(logDir, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newDailyWriter: %v", err)
	}
	defer func() { _ = writer.Close() }()
	if writer.maxBytes != backendLogSegmentBytes {
		t.Fatalf("default segment limit = %d, want %d", writer.maxBytes, backendLogSegmentBytes)
	}
	writer.maxBytes = dailyLimit

	if _, err := writer.Write([]byte("must-not-grow\n")); err == nil {
		t.Fatal("write beyond daily byte limit succeeded")
	}
	info, err := os.Stat(activePath)
	if err != nil {
		t.Fatalf("stat active log: %v", err)
	}
	if info.Size() != dailyLimit {
		t.Fatalf("active log size = %d, want %d", info.Size(), dailyLimit)
	}
	now = now.Add(24 * time.Hour)
	if _, err := writer.Write([]byte("next\n")); err != nil {
		t.Fatalf("next-day write: %v", err)
	}
	if active := readLogFile(t, activePath); active != "next\n" {
		t.Fatalf("next-day active log = %q", active)
	}
}

func readLogFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}
