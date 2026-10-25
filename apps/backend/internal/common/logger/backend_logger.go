package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	textLogFormat       = "text"
	maxEncodedLogEntry  = 256 * 1024
	fileQueueEntries    = 8192
	fileQueueBytes      = 8 * 1024 * 1024
	stdoutQueueEntries  = 2048
	stdoutQueueBytes    = 2 * 1024 * 1024
	backendCloseTimeout = 2 * time.Second
)

type BackendLoggingConfig struct {
	HomeDir      string
	Level        string
	Format       string
	ConsoleLevel string
	Stdout       io.Writer
	Stderr       io.Writer
	Now          func() time.Time
}

type backendLoggerRuntime struct {
	fileSink     *asyncSink
	stdoutSink   *asyncSink
	fileWriter   io.Closer
	stderr       io.Writer
	closeTimeout time.Duration
	closeOnce    sync.Once
	closeErr     error
}

func NewBackendLogger(cfg BackendLoggingConfig) (*Logger, error) {
	cfg = backendConfigDefaults(cfg)
	fileLevel, err := parseLevel(cfg.Level)
	if err != nil {
		fileLevel = zapcore.InfoLevel
	}
	logDir, err := filepath.Abs(filepath.Join(cfg.HomeDir, "logs"))
	if err != nil {
		return nil, fmt.Errorf("resolve backend log path: %w", err)
	}
	_, _ = fmt.Fprintf(cfg.Stdout, "Backend logs: %s\n", filepath.Join(logDir, activeBackendLogName))

	fileWriter := newRetryDailyWriter(logDir, cfg.Now, cfg.Stderr)
	fileSink := newAsyncSink(fileWriter, asyncSinkConfig{
		Name: "file", MaxEntries: fileQueueEntries, MaxBytes: fileQueueBytes,
		ReservedEntries: 2048, ReservedBytes: 2 * 1024 * 1024, MaxEntryBytes: maxEncodedLogEntry,
	})
	stdoutSink := newAsyncSink(cfg.Stdout, asyncSinkConfig{
		Name: "stdout", MaxEntries: stdoutQueueEntries, MaxBytes: stdoutQueueBytes,
		ReservedEntries: 512, ReservedBytes: 512 * 1024, MaxEntryBytes: maxEncodedLogEntry,
	})
	runtime := &backendLoggerRuntime{
		fileSink: fileSink, stdoutSink: stdoutSink, fileWriter: fileWriter, stderr: cfg.Stderr,
		closeTimeout: backendCloseTimeout,
	}
	encoderConfig := backendEncoderConfig()
	fileCore := newAsyncCore(newLogEncoder(cfg.Format, encoderConfig), fileSink, fileLevel)
	stdoutLevel := zapcore.WarnLevel
	if cfg.ConsoleLevel == "info" {
		stdoutLevel = zapcore.InfoLevel
	}
	stdoutCore := newAsyncCore(newLogEncoder(cfg.Format, encoderConfig), stdoutSink, stdoutLevel)
	zapLogger := zap.New(zapcore.NewTee(fileCore, stdoutCore), zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	return &Logger{
		zap: zapLogger, sugar: zapLogger.Sugar(), backend: runtime,
		backendLogDir: logDir,
	}, nil
}

func backendConfigDefaults(cfg BackendLoggingConfig) BackendLoggingConfig {
	if cfg.HomeDir == "" {
		cfg.HomeDir = "."
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return cfg
}

func backendEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewProductionEncoderConfig()
	cfg.TimeKey = "timestamp"
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	return cfg
}

func newLogEncoder(format string, cfg zapcore.EncoderConfig) zapcore.Encoder {
	if format == "console" || format == textLogFormat {
		cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		return zapcore.NewConsoleEncoder(cfg)
	}
	return zapcore.NewJSONEncoder(cfg)
}

func (r *backendLoggerRuntime) Close() error {
	r.closeOnce.Do(func() {
		timeout := r.closeTimeout
		if timeout <= 0 {
			timeout = backendCloseTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		results := make(chan error, 2)
		go func() { results <- r.fileSink.Close(ctx) }()
		go func() { results <- r.stdoutSink.Close(ctx) }()
		for pending := 2; pending > 0; pending-- {
			select {
			case err := <-results:
				r.closeErr = errors.Join(r.closeErr, err)
			case <-ctx.Done():
				r.closeErr = errors.Join(r.closeErr, ctx.Err())
				pending = 1
			}
		}
		writerResult := make(chan error, 1)
		go func() { writerResult <- r.fileWriter.Close() }()
		select {
		case err := <-writerResult:
			r.closeErr = errors.Join(r.closeErr, err)
		case <-ctx.Done():
			r.closeErr = errors.Join(r.closeErr, ctx.Err())
		}
	})
	return r.closeErr
}

func (r *backendLoggerRuntime) Stats() []SinkStatistics {
	return []SinkStatistics{r.fileSink.Stats(), r.stdoutSink.Stats()}
}

func (r *backendLoggerRuntime) terminal(level, message string) {
	_ = r.Close()
	const maxTerminalMessage = 64 * 1024
	if len(message) > maxTerminalMessage {
		message = message[:maxTerminalMessage]
	}
	_, _ = fmt.Fprintf(r.stderr, "%s: %s\n", level, message)
}

type asyncCore struct {
	level   zapcore.LevelEnabler
	encoder zapcore.Encoder
	sink    *asyncSink
	fields  []zapcore.Field
}

func newAsyncCore(encoder zapcore.Encoder, sink *asyncSink, level zapcore.Level) zapcore.Core {
	return &asyncCore{level: zap.LevelEnablerFunc(func(candidate zapcore.Level) bool {
		return candidate >= level
	}), encoder: encoder, sink: sink}
}

func (c *asyncCore) Enabled(level zapcore.Level) bool {
	return c.level.Enabled(level)
}

func (c *asyncCore) With(fields []zapcore.Field) zapcore.Core {
	clone := *c
	clone.fields = append(append([]zapcore.Field(nil), c.fields...), fields...)
	return &clone
}

func (c *asyncCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func (c *asyncCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	encoder := c.encoder.Clone()
	for _, field := range c.fields {
		field.AddTo(encoder)
	}
	encoded, err := encoder.EncodeEntry(entry, fields)
	if err != nil {
		return err
	}
	c.sink.Enqueue(entry.Level, encoded.Bytes())
	encoded.Free()
	return nil
}

func (c *asyncCore) Sync() error { return nil }

type retryDailyWriter struct {
	mu        sync.Mutex
	logDir    string
	now       func() time.Time
	stderr    io.Writer
	writer    *dailyWriter
	nextRetry time.Time
	closed    bool
	stop      chan struct{}
	done      chan struct{}
}

func newRetryDailyWriter(logDir string, now func() time.Time, stderr io.Writer) *retryDailyWriter {
	writer := &retryDailyWriter{
		logDir: logDir, now: now, stderr: stderr, stop: make(chan struct{}), done: make(chan struct{}),
	}
	writer.activate()
	go writer.retryLoop()
	return writer
}

func (w *retryDailyWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if w.writer == nil && !w.now().Before(w.nextRetry) {
		w.activate()
	}
	if w.writer == nil {
		return 0, fmt.Errorf("backend log file unavailable")
	}
	written, err := w.writer.Write(data)
	if err != nil {
		_ = w.writer.Close()
		w.writer = nil
		w.nextRetry = w.now().Add(30 * time.Second)
		_, _ = fmt.Fprintf(w.stderr, "Backend log file write failed (%v); retrying in 30s\n", err)
	}
	return written, err
}

func (w *retryDailyWriter) activate() {
	writer, err := newDailyWriter(w.logDir, w.now)
	if err != nil {
		w.nextRetry = w.now().Add(30 * time.Second)
		_, _ = fmt.Fprintf(w.stderr, "Backend log file unavailable (%v); retrying in 30s\n", err)
		return
	}
	if err := writer.MaintenanceError(); err != nil {
		_, _ = fmt.Fprintf(w.stderr, "Backend log cleanup deferred: %v\n", err)
	}
	w.writer = writer
	w.nextRetry = time.Time{}
}

func (w *retryDailyWriter) retryLoop() {
	defer close(w.done)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			if !w.closed && w.writer == nil && !w.now().Before(w.nextRetry) {
				w.activate()
			}
			w.mu.Unlock()
		case <-w.stop:
			return
		}
	}
}

func (w *retryDailyWriter) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	close(w.stop)
	writer := w.writer
	w.mu.Unlock()
	<-w.done
	if writer == nil {
		return nil
	}
	return writer.Close()
}
