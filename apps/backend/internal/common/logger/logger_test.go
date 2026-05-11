package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

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
