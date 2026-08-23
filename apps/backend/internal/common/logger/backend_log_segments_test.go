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

	_ = newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 5, 10)
	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000001.log")); got != "two-t" {
		t.Fatalf("converted first segment = %q, want two-t", got)
	}
	if got := readLogFile(t, filepath.Join(logDir, "backend-logs-2026-08-22-000002.log")); got != "hree\n" {
		t.Fatalf("converted second segment = %q, want hree", got)
	}
	if _, err := os.Stat(filepath.Join(logDir, "backend-logs-2026-08-22-000003.log")); !os.IsNotExist(err) {
		t.Fatalf("discarded third segment remains: %v", err)
	}
	if got := readLogFile(t, filepath.Join(logDir, activeBackendLogName)); got != "" {
		t.Fatalf("converted active log = %q, want empty", got)
	}
	if _, err := os.Stat(filepath.Join(logDir, conversionJournalName)); !os.IsNotExist(err) {
		t.Fatalf("conversion journal remains: %v", err)
	}
}

func TestDailyWriterRecoversOversizedConversionWithTailOffset(t *testing.T) {
	logDir := t.TempDir()
	source := []byte("one-two-three\n")
	journal := &conversionJournal{
		SourceDay: "2026-08-22", SourceSize: int64(len(source)), TailOffset: 4,
		BackupName: conversionSourceName,
		Outputs: []conversionOutput{
			{Name: "backend-logs-2026-08-22-000001.log", Offset: 0, Length: 5},
			{Name: "backend-logs-2026-08-22-000002.log", Offset: 5, Length: 5},
		},
	}
	if err := os.WriteFile(filepath.Join(logDir, conversionSourceName), source, 0o600); err != nil {
		t.Fatalf("seed conversion source: %v", err)
	}
	if err := writeJSONOwnerFile(filepath.Join(logDir, conversionJournalName), journal); err != nil {
		t.Fatalf("seed conversion journal: %v", err)
	}

	_ = newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 5, 10)
	for name, want := range map[string]string{
		"backend-logs-2026-08-22-000001.log": "two-t",
		"backend-logs-2026-08-22-000002.log": "hree\n",
	} {
		if got := readLogFile(t, filepath.Join(logDir, name)); got != want {
			t.Fatalf("recovered %s = %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(logDir, conversionSourceName)); !os.IsNotExist(err) {
		t.Fatalf("conversion source remains: %v", err)
	}
}

func TestBackendLogDayRetainedRejectsFutureDays(t *testing.T) {
	today := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	for day, want := range map[string]bool{
		"2026-08-20": true,
		"2026-08-21": true,
		"2026-08-22": true,
		"2026-08-23": false,
		"not-a-day":  false,
	} {
		if got := BackendLogDayRetained(day, today); got != want {
			t.Fatalf("BackendLogDayRetained(%q) = %v, want %v", day, got, want)
		}
	}
}

func TestDailyWriterConversionPrefersActiveTailOverClosedLogs(t *testing.T) {
	logDir := t.TempDir()
	source := []byte("one-two-three\n")
	if err := os.WriteFile(filepath.Join(logDir, "backend-logs-2026-08-21.log"), []byte("old-data"), 0o600); err != nil {
		t.Fatalf("seed old log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, activeBackendLogName), source, 0o600); err != nil {
		t.Fatalf("seed active log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, backendDayMarkerName), []byte("2026-08-22"), 0o600); err != nil {
		t.Fatalf("seed day marker: %v", err)
	}

	_ = newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 5, 15)
	if _, err := os.Stat(filepath.Join(logDir, "backend-logs-2026-08-21.log")); !os.IsNotExist(err) {
		t.Fatalf("older log was retained while converting active log: %v", err)
	}
	for name, want := range map[string]string{
		"backend-logs-2026-08-22-000001.log": "one-t",
		"backend-logs-2026-08-22-000002.log": "wo-th",
		"backend-logs-2026-08-22-000003.log": "ree\n",
	} {
		if got := readLogFile(t, filepath.Join(logDir, name)); got != want {
			t.Fatalf("converted %s = %q, want %q", name, got, want)
		}
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
	_ = newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 8, 100)
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
	_ = newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 5, 20)
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

func TestDailyWriterRecoversStaleConversionTemp(t *testing.T) {
	logDir := t.TempDir()
	source := []byte("one-two-three\n")
	if err := os.WriteFile(filepath.Join(logDir, conversionSourceName), source, 0o600); err != nil {
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
	if err := os.WriteFile(filepath.Join(logDir, journal.Outputs[0].Name+".tmp"), []byte("stale"), 0o600); err != nil {
		t.Fatalf("seed stale temporary output: %v", err)
	}

	_ = newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 5, 20)
	if got := readLogFile(t, filepath.Join(logDir, journal.Outputs[0].Name)); got != "one-t" {
		t.Fatalf("recovered first segment = %q", got)
	}
	if _, err := os.Stat(filepath.Join(logDir, journal.Outputs[0].Name+".tmp")); !os.IsNotExist(err) {
		t.Fatalf("stale temporary output remains: %v", err)
	}
}

func TestDailyWriterResumesInterruptedConversionTruncation(t *testing.T) {
	logDir := t.TempDir()
	journal := &conversionJournal{
		SourceDay: "2026-08-22", SourceSize: 14, TailOffset: 0,
		BackupName: conversionSourceName,
		Outputs: []conversionOutput{
			{Name: "backend-logs-2026-08-22-000001.log", Offset: 0, Length: 5},
			{Name: "backend-logs-2026-08-22-000002.log", Offset: 5, Length: 5},
			{Name: "backend-logs-2026-08-22-000003.log", Offset: 10, Length: 4},
		},
	}
	if err := os.WriteFile(filepath.Join(logDir, conversionSourceName), []byte("one-two-th"), 0o600); err != nil {
		t.Fatalf("seed partially truncated source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, journal.Outputs[2].Name), []byte("ree\n"), 0o600); err != nil {
		t.Fatalf("seed newest output: %v", err)
	}
	if err := writeJSONOwnerFile(filepath.Join(logDir, conversionJournalName), journal); err != nil {
		t.Fatalf("seed conversion journal: %v", err)
	}

	_ = newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 5, 20)
	for name, want := range map[string]string{
		"backend-logs-2026-08-22-000001.log": "one-t",
		"backend-logs-2026-08-22-000002.log": "wo-th",
		"backend-logs-2026-08-22-000003.log": "ree\n",
	} {
		if got := readLogFile(t, filepath.Join(logDir, name)); got != want {
			t.Fatalf("recovered %s = %q, want %q", name, got, want)
		}
	}
}

func TestDailyWriterPreservesFreshActiveLogAfterCompletedConversion(t *testing.T) {
	logDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(logDir, activeBackendLogName), []byte("new\n"), 0o600); err != nil {
		t.Fatalf("seed fresh active log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, backendDayMarkerName), []byte("2026-08-22"), 0o600); err != nil {
		t.Fatalf("seed active day marker: %v", err)
	}
	journal := &conversionJournal{
		SourceDay: "2026-08-22", SourceSize: 14, TailOffset: 0,
		BackupName: conversionSourceName,
		Outputs: []conversionOutput{
			{Name: "backend-logs-2026-08-22-000001.log", Offset: 0, Length: 5},
			{Name: "backend-logs-2026-08-22-000002.log", Offset: 5, Length: 5},
			{Name: "backend-logs-2026-08-22-000003.log", Offset: 10, Length: 4},
		},
	}
	for _, output := range journal.Outputs {
		contents := map[int64]string{0: "one-t", 5: "wo-th", 10: "ree\n"}[output.Offset]
		if err := os.WriteFile(filepath.Join(logDir, output.Name), []byte(contents), 0o600); err != nil {
			t.Fatalf("seed %s: %v", output.Name, err)
		}
	}
	if err := writeJSONOwnerFile(filepath.Join(logDir, conversionJournalName), journal); err != nil {
		t.Fatalf("seed conversion journal: %v", err)
	}

	_ = newTestDailyWriter(t, logDir, time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC), 5, 20)
	if got := readLogFile(t, filepath.Join(logDir, activeBackendLogName)); got != "new\n" {
		t.Fatalf("fresh active log = %q, want new entry", got)
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
	t.Cleanup(func() { _ = writer.Close() })
	if err := writer.prepare(); err != nil {
		t.Fatalf("prepare writer: %v", err)
	}
	return writer
}
