package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyWriterRotatesFullActiveSegment(t *testing.T) {
	logDir := t.TempDir()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	writer, err := newDailyWriter(logDir, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newDailyWriter: %v", err)
	}
	defer func() { _ = writer.Close() }()
	writer.maxBytes = int64(len("first\n"))

	if _, err := writer.Write([]byte("first\n")); err != nil {
		t.Fatalf("write first segment: %v", err)
	}
	if _, err := writer.Write([]byte("next\n")); err != nil {
		t.Fatalf("write second segment: %v", err)
	}

	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000001.log")); got != "first\n" {
		t.Fatalf("closed segment = %q, want first entry", got)
	}
	if got := readLogFile(t, filepath.Join(logDir, activeBackendLogName)); got != "next\n" {
		t.Fatalf("active segment = %q, want next entry", got)
	}
}

func TestDailyWriterEvictsOldestClosedSegment(t *testing.T) {
	logDir := t.TempDir()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	writer := newTestDailyWriter(t, logDir, now, 4, 8)
	defer func() { _ = writer.Close() }()

	for _, entry := range []string{"one\n", "two\n", "tri\n"} {
		if _, err := writer.Write([]byte(entry)); err != nil {
			t.Fatalf("write %q: %v", entry, err)
		}
	}

	if _, err := os.Stat(filepath.Join(logDir, "backend-logs-2026-08-22-000001.log")); !os.IsNotExist(err) {
		t.Fatalf("oldest segment still exists, stat error = %v", err)
	}
	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000002.log")); got != "two\n" {
		t.Fatalf("retained closed segment = %q, want two", got)
	}
	if got := readLogFile(t, filepath.Join(logDir, activeBackendLogName)); got != "tri\n" {
		t.Fatalf("active segment = %q, want tri", got)
	}
}

func TestDailyWriterEvictsOldestSegmentAcrossUTCDays(t *testing.T) {
	logDir := t.TempDir()
	now := time.Date(2026, 8, 21, 23, 59, 0, 0, time.UTC)
	current := now
	writer := newTestDailyWriterWithClock(t, logDir, func() time.Time { return current }, 4, 8)
	if _, err := writer.Write([]byte("one\n")); err != nil {
		t.Fatalf("write one: %v", err)
	}
	if _, err := writer.Write([]byte("two\n")); err != nil {
		t.Fatalf("write two: %v", err)
	}
	current = current.Add(2 * time.Minute)
	if _, err := writer.Write([]byte("tri\n")); err != nil {
		t.Fatalf("write tri: %v", err)
	}
	defer func() { _ = writer.Close() }()

	if _, err := os.Stat(filepath.Join(logDir, "backend-logs-2026-08-21-000001.log")); !os.IsNotExist(err) {
		t.Fatalf("oldest cross-day segment remains: %v", err)
	}
	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-21-000002.log")); got != "two\n" {
		t.Fatalf("retained prior-day segment = %q, want two", got)
	}
	if got := readLogFile(t, filepath.Join(logDir, activeBackendLogName)); got != "tri\n" {
		t.Fatalf("active next-day segment = %q, want tri", got)
	}
}

func TestDailyWriterResumesSequenceAfterRestart(t *testing.T) {
	logDir := t.TempDir()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	writer := newTestDailyWriter(t, logDir, now, 4, 20)
	if _, err := writer.Write([]byte("one\n")); err != nil {
		t.Fatalf("write one: %v", err)
	}
	if _, err := writer.Write([]byte("two\n")); err != nil {
		t.Fatalf("write two: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close first writer: %v", err)
	}

	writer = newTestDailyWriter(t, logDir, now, 4, 20)
	defer func() { _ = writer.Close() }()
	if _, err := writer.Write([]byte("tri\n")); err != nil {
		t.Fatalf("write tri: %v", err)
	}

	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000001.log")); got != "one\n" {
		t.Fatalf("first sequence = %q, want one", got)
	}
	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000002.log")); got != "two\n" {
		t.Fatalf("second sequence = %q, want two", got)
	}
	if got := readLogFile(t, filepath.Join(logDir, activeBackendLogName)); got != "tri\n" {
		t.Fatalf("active after restart = %q, want tri", got)
	}
}

func TestDailyWriterConvertsOversizedActiveLog(t *testing.T) {
	logDir := t.TempDir()
	day := "2026-08-22"
	source := []byte("one-two-three\n")
	if err := os.WriteFile(filepath.Join(logDir, activeBackendLogName), source, 0o600); err != nil {
		t.Fatalf("seed active log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, backendDayMarkerName), []byte(day), 0o600); err != nil {
		t.Fatalf("seed day marker: %v", err)
	}

	writer := newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 5, 15)
	defer func() { _ = writer.Close() }()
	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000001.log")); got != "one-t" {
		t.Fatalf("converted first segment = %q, want one-t", got)
	}
	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000002.log")); got != "wo-th" {
		t.Fatalf("converted second segment = %q, want wo-th", got)
	}
	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000003.log")); got != "ree\n" {
		t.Fatalf("converted third segment = %q, want ree", got)
	}
	if got := readLogFile(t, filepath.Join(logDir, activeBackendLogName)); got != "" {
		t.Fatalf("converted active log = %q, want empty", got)
	}
	if _, err := os.Stat(filepath.Join(logDir, conversionJournalName)); !os.IsNotExist(err) {
		t.Fatalf("conversion journal remains: %v", err)
	}
}

func TestDailyWriterCleansExpiredSegmentsAndLegacyFiles(t *testing.T) {
	logDir := t.TempDir()
	for name := range map[string]string{
		"backend-logs-2026-08-19.log":        "expired-legacy",
		"backend-logs-2026-08-20.log":        "oldest-retained",
		"backend-logs-2026-08-21-000001.log": "yesterday",
	} {
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(name), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	writer := newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 8, 100)
	defer func() { _ = writer.Close() }()
	if _, err := os.Stat(filepath.Join(logDir, "backend-logs-2026-08-19.log")); !os.IsNotExist(err) {
		t.Fatalf("expired legacy file remains: %v", err)
	}
	for _, name := range []string{"backend-logs-2026-08-20.log", "backend-logs-2026-08-21-000001.log"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); err != nil {
			t.Fatalf("retained file %s missing: %v", name, err)
		}
	}
}

func TestDailyWriterRecoversInterruptedConversion(t *testing.T) {
	logDir := t.TempDir()
	source := []byte("one-two-three\n")
	backup := filepath.Join(logDir, conversionSourceName)
	if err := os.WriteFile(backup, source, 0o600); err != nil {
		t.Fatalf("seed conversion source: %v", err)
	}
	journal := &conversionJournal{
		SourceDay: "2026-08-22", SourceSize: int64(len(source)), TailOffset: 0,
		BackupName: conversionSourceName,
		Outputs: []conversionOutput{
			{Name: "backend-logs-2026-08-22-000001.log", Offset: 0, Length: 5},
			{Name: "backend-logs-2026-08-22-000002.log", Offset: 5, Length: 5},
			{Name: "backend-logs-2026-08-22-000003.log", Offset: 10, Length: 4},
		},
	}
	if err := writeJSONOwnerFile(filepath.Join(logDir, conversionJournalName), journal); err != nil {
		t.Fatalf("seed conversion journal: %v", err)
	}
	writer := newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 5, 20)
	defer func() { _ = writer.Close() }()
	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000001.log")); got != "one-t" {
		t.Fatalf("recovered first segment = %q", got)
	}
	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000003.log")); got != "ree\n" {
		t.Fatalf("recovered last segment = %q", got)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("conversion source remains: %v", err)
	}
}

func newTestDailyWriter(t *testing.T, logDir string, now time.Time, segmentBytes, totalBytes int64) *dailyWriter {
	return newTestDailyWriterWithClock(t, logDir, func() time.Time { return now }, segmentBytes, totalBytes)
}

func newTestDailyWriterWithClock(
	t *testing.T, logDir string, now func() time.Time, segmentBytes, totalBytes int64,
) *dailyWriter {
	t.Helper()
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("create log directory: %v", err)
	}
	writer := &dailyWriter{
		logDir: logDir, now: now,
		maxBytes: segmentBytes, maxTotalBytes: totalBytes,
	}
	if err := writer.prepare(); err != nil {
		t.Fatalf("prepare writer: %v", err)
	}
	return writer
}
