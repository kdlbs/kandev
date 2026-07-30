package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

type lockedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

type blockingWriteCloser struct {
	mu      sync.Mutex
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (w *blockingWriteCloser) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.once.Do(func() { close(w.started) })
	<-w.release
	return len(data), nil
}

func (w *blockingWriteCloser) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return nil
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func TestNewBackendLoggerUsesIndependentThresholds(t *testing.T) {
	homeDir := t.TempDir()
	stdout := &lockedBuffer{}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	log, err := NewBackendLogger(BackendLoggingConfig{
		HomeDir: homeDir, Level: "info", Format: "json", Stdout: stdout,
		Stderr: &lockedBuffer{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewBackendLogger: %v", err)
	}
	log.Info("file-only info")
	log.Warn("visible warning")
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fileLog := readLogFile(t, filepath.Join(homeDir, "logs", activeBackendLogName))
	if !strings.Contains(fileLog, "file-only info") || !strings.Contains(fileLog, "visible warning") {
		t.Fatalf("file log missing entries: %s", fileLog)
	}
	stdoutLog := stdout.String()
	if strings.Contains(stdoutLog, "file-only info") || !strings.Contains(stdoutLog, "visible warning") {
		t.Fatalf("stdout thresholds incorrect: %s", stdoutLog)
	}
	if !strings.Contains(stdoutLog, filepath.Join(homeDir, "logs", activeBackendLogName)) {
		t.Fatalf("startup output does not identify log path: %s", stdoutLog)
	}
}

func TestNewBackendLoggerDebugAndVerboseThresholds(t *testing.T) {
	tests := []struct {
		name           string
		level          string
		consoleLevel   string
		wantDebugFile  bool
		wantInfoStdout bool
	}{
		{name: "debug", level: "debug", wantDebugFile: true},
		{name: "verbose", level: "info", consoleLevel: "info", wantInfoStdout: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			homeDir := t.TempDir()
			stdout := &lockedBuffer{}
			log, err := NewBackendLogger(BackendLoggingConfig{
				HomeDir: homeDir, Level: test.level, Format: "json", ConsoleLevel: test.consoleLevel,
				Stdout: stdout, Stderr: &lockedBuffer{},
			})
			if err != nil {
				t.Fatalf("NewBackendLogger: %v", err)
			}
			log.Debug("debug entry")
			log.Info("info entry")
			_ = log.Close()
			fileLog := readLogFile(t, filepath.Join(homeDir, "logs", activeBackendLogName))
			if got := strings.Contains(fileLog, "debug entry"); got != test.wantDebugFile {
				t.Fatalf("debug file presence = %v, log = %s", got, fileLog)
			}
			if got := strings.Contains(stdout.String(), "info entry"); got != test.wantInfoStdout {
				t.Fatalf("info stdout presence = %v, log = %s", got, stdout.String())
			}
		})
	}
}

func TestNewBackendLoggerSurvivesUnavailableFileSink(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(homeDir, "logs"), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("seed conflicting log path: %v", err)
	}
	stdout := &lockedBuffer{}
	stderr := &lockedBuffer{}
	log, err := NewBackendLogger(BackendLoggingConfig{
		HomeDir: homeDir, Level: "info", Format: "json", Stdout: stdout, Stderr: stderr,
	})
	if err != nil {
		t.Fatalf("file failure prevented logger startup: %v", err)
	}
	log.Warn("stdout remains available")
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !strings.Contains(stdout.String(), "stdout remains available") {
		t.Fatalf("stdout fallback missing warning: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "retrying in 30s") {
		t.Fatalf("file activation failure was not reported: %s", stderr.String())
	}
	stats := log.SinkStatistics()
	if len(stats) != 2 || stats[0].Lost["warn"]["write_error"] != 1 {
		t.Fatalf("file sink statistics = %#v", stats)
	}
}

func TestNewBackendLoggerRetriesFileActivation(t *testing.T) {
	homeDir := t.TempDir()
	logPath := filepath.Join(homeDir, "logs")
	if err := os.WriteFile(logPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("seed conflicting log path: %v", err)
	}
	initialNow := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	var nowUnixNano atomic.Int64
	nowUnixNano.Store(initialNow.UnixNano())
	log, err := NewBackendLogger(BackendLoggingConfig{
		HomeDir: homeDir, Level: "info", Format: "json", Stdout: &lockedBuffer{},
		Stderr: &lockedBuffer{}, Now: func() time.Time { return time.Unix(0, nowUnixNano.Load()) },
	})
	if err != nil {
		t.Fatalf("NewBackendLogger: %v", err)
	}
	log.Info("before recovery")
	deadline := time.Now().Add(time.Second)
	for log.SinkStatistics()[0].Lost["info"]["write_error"] == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if log.SinkStatistics()[0].Lost["info"]["write_error"] != 1 {
		t.Fatal("initial file write failure was not observed")
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("remove conflicting path: %v", err)
	}
	nowUnixNano.Store(initialNow.Add(31 * time.Second).UnixNano())
	log.Info("after recovery")
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := readLogFile(t, filepath.Join(logPath, activeBackendLogName)); !strings.Contains(got, "after recovery") {
		t.Fatalf("recovered file log = %s", got)
	}
}

func TestBackendLoggerCloseDoesNotWaitOnBlockedFileWriter(t *testing.T) {
	writer := &blockingWriteCloser{started: make(chan struct{}), release: make(chan struct{})}
	fileSink := newAsyncSink(writer, asyncSinkConfig{
		Name: "file", MaxEntries: 3, MaxBytes: 30, MaxEntryBytes: 20,
	})
	stdoutSink := newAsyncSink(discardWriter{}, asyncSinkConfig{
		Name: "stdout", MaxEntries: 3, MaxBytes: 30, MaxEntryBytes: 20,
	})
	runtime := &backendLoggerRuntime{
		fileSink: fileSink, stdoutSink: stdoutSink, fileWriter: writer,
		stderr: &lockedBuffer{}, closeTimeout: 20 * time.Millisecond,
	}
	if !fileSink.Enqueue(zap.WarnLevel, []byte("warning")) {
		t.Fatal("warning was rejected")
	}
	<-writer.started

	done := make(chan error, 1)
	go func() { done <- runtime.Close() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Close succeeded despite blocked file writer")
		}
	case <-time.After(100 * time.Millisecond):
		close(writer.release)
		t.Fatal("Close exceeded bounded deadline")
	}
	close(writer.release)
}

func TestNewLogger_StdoutHasNoRotator(t *testing.T) {
	for _, path := range []string{"", "stdout", "stderr"} {
		t.Run(path, func(t *testing.T) {
			log, err := NewLogger(LoggingConfig{Level: "info", Format: "json", OutputPath: path})
			if err != nil {
				t.Fatalf("NewLogger: %v", err)
			}
			if log.rotator != nil {
				t.Fatalf("expected rotator to be nil for %q output, got %#v", path, log.rotator)
			}
			if err := log.Close(); err != nil {
				t.Fatalf("Close on stdout/stderr logger should be a no-op, got %v", err)
			}
		})
	}
}

func TestNewLogger_FileOutputUsesRotator(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "kandev.log")

	log, err := NewLogger(LoggingConfig{
		Level:      "info",
		Format:     "json",
		OutputPath: logPath,
		MaxSizeMB:  10,
		MaxBackups: 3,
		MaxAgeDays: 7,
		Compress:   true,
	})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if log.rotator == nil {
		t.Fatal("expected rotator to be configured for file output")
	}

	if log.rotator.Filename != logPath {
		t.Errorf("Filename: got %q, want %q", log.rotator.Filename, logPath)
	}
	if log.rotator.MaxSize != 10 {
		t.Errorf("MaxSize: got %d, want 10", log.rotator.MaxSize)
	}
	if log.rotator.MaxBackups != 3 {
		t.Errorf("MaxBackups: got %d, want 3", log.rotator.MaxBackups)
	}
	if log.rotator.MaxAge != 7 {
		t.Errorf("MaxAge: got %d, want 7", log.rotator.MaxAge)
	}
	if !log.rotator.Compress {
		t.Error("Compress: got false, want true")
	}

	log.Info("hello", zap.String("k", "v"))
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("log file missing entry; got %q", string(data))
	}
}

func TestWithFields_PropagatesRotator(t *testing.T) {
	dir := t.TempDir()
	log, err := NewLogger(LoggingConfig{
		Level:      "info",
		Format:     "json",
		OutputPath: filepath.Join(dir, "kandev.log"),
	})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	derived := log.WithFields(zap.String("k", "v"))
	if derived.rotator != log.rotator {
		t.Fatalf("WithFields dropped rotator: got %p, want %p", derived.rotator, log.rotator)
	}

	// Derived helpers all funnel through WithFields, so spot-check one.
	if log.WithTaskID("t1").rotator != log.rotator {
		t.Fatal("WithTaskID dropped rotator")
	}
}

func TestLoggerClose_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	log, err := NewLogger(LoggingConfig{
		Level:      "info",
		Format:     "json",
		OutputPath: filepath.Join(dir, "kandev.log"),
	})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("second Close should be a no-op, got %v", err)
	}
}
