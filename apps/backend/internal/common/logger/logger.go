// Package logger provides structured logging using go.uber.org/zap.
package logger

import (
	"context"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/kandev/kandev/internal/common/logger/buffer"
)

// Context keys for extracting values from context.
type contextKey string

const (
	CorrelationIDKey contextKey = "correlation_id"
	RequestIDKey     contextKey = "request_id"
)

// LoggingConfig holds the configuration for the logger.
//
// This struct is constructed in Go (not unmarshaled via viper/mapstructure),
// so it does not carry struct tags. Callers map their own config types
// (e.g. config.LoggingConfig) into these fields explicitly.
type LoggingConfig struct {
	Level      string // debug, info, warn, error
	Format     string // json, console
	OutputPath string // stdout, stderr, or file path

	// Rotation options - apply only when OutputPath is a file path
	// (ignored for stdout/stderr). Backed by lumberjack.
	MaxSizeMB  int  // rotate when file exceeds this size; 0 = lumberjack default (100MB)
	MaxBackups int  // max number of rotated files to retain; 0 = unlimited
	MaxAgeDays int  // max age of rotated files; 0 = unlimited
	Compress   bool // gzip rotated files
}

// Logger wraps zap.Logger to provide structured logging with helper methods.
type Logger struct {
	zap     *zap.Logger
	sugar   *zap.SugaredLogger
	fields  []zap.Field
	rotator *lumberjack.Logger // non-nil only when OutputPath is a file path
}

var (
	defaultLogger     *Logger
	defaultLoggerOnce sync.Once
)

// Default returns the global default logger.
// This logger is initialized with default settings (info level, text format for terminals, stdout).
func Default() *Logger {
	defaultLoggerOnce.Do(func() {
		var err error
		defaultLogger, err = NewLogger(LoggingConfig{
			Level:      "info",
			Format:     detectLogFormat(),
			OutputPath: "stdout",
		})
		if err != nil {
			// Fallback to a basic logger if configuration fails
			zapLogger, _ := zap.NewProduction()
			defaultLogger = &Logger{
				zap:   zapLogger,
				sugar: zapLogger.Sugar(),
			}
		}
	})
	return defaultLogger
}

// SetDefault sets the global default logger.
func SetDefault(logger *Logger) {
	defaultLogger = logger
}

// NewLogger creates a new Logger with the given configuration.
func NewLogger(cfg LoggingConfig) (*Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder

	var encoder zapcore.Encoder
	// Accept both "console" and "text" as aliases for human-readable format
	if cfg.Format == "console" || cfg.Format == "text" {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	var (
		writeSyncer zapcore.WriteSyncer
		rotator     *lumberjack.Logger
	)
	switch cfg.OutputPath {
	case "", "stdout":
		writeSyncer = zapcore.AddSync(os.Stdout)
	case "stderr":
		writeSyncer = zapcore.AddSync(os.Stderr)
	default:
		rotator = &lumberjack.Logger{
			Filename:   cfg.OutputPath,
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		}
		writeSyncer = zapcore.AddSync(rotator)
	}

	outputCore := zapcore.NewCore(encoder, writeSyncer, level)
	bufferCore := buffer.NewCore(buffer.Default(), level)
	core := zapcore.NewTee(outputCore, bufferCore)
	zapLogger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return &Logger{
		zap:     zapLogger,
		sugar:   zapLogger.Sugar(),
		rotator: rotator,
	}, nil
}

func parseLevel(level string) (zapcore.Level, error) {
	var l zapcore.Level
	err := l.UnmarshalText([]byte(level))
	return l, err
}

// detectLogFormat returns the appropriate log format based on environment.
// Returns "json" if running in Kubernetes or other production environments.
// Returns "text" for terminal/development use (human-readable console format).
func detectLogFormat() string {
	// Check if running in Kubernetes
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "json"
	}

	// Check for explicit production environment
	if env := os.Getenv("KANDEV_ENV"); env == "production" || env == "prod" {
		return "json"
	}

	// Default to text format for terminal use (more readable than JSON)
	return "text"
}

// Sync flushes any buffered log entries.
func (l *Logger) Sync() error {
	return l.zap.Sync()
}

// Close flushes pending log entries and releases the file handle held by the
// rotating file sink, if any. Safe to call on a stdout/stderr logger (no-op).
// Call before process shutdown to ensure the final batch reaches disk and the
// rotation cycle can complete cleanly.
func (l *Logger) Close() error {
	_ = l.zap.Sync()
	if l.rotator != nil {
		return l.rotator.Close()
	}
	return nil
}

// WithFields returns a new Logger with the given fields added.
// The rotator reference is propagated so Close() on a derived logger
// still releases the underlying file handle.
func (l *Logger) WithFields(fields ...zap.Field) *Logger {
	z := l.zap.With(fields...)
	return &Logger{
		zap:     z,
		sugar:   z.Sugar(),
		fields:  append(l.fields, fields...),
		rotator: l.rotator,
	}
}

// WithContext returns a new Logger with context values (correlation ID, etc.) added.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := []zap.Field{}

	if correlationID, ok := ctx.Value(CorrelationIDKey).(string); ok && correlationID != "" {
		fields = append(fields, zap.String("correlation_id", correlationID))
	}
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok && requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}

	if len(fields) == 0 {
		return l
	}
	return l.WithFields(fields...)
}

// WithError returns a new Logger with the error field added.
func (l *Logger) WithError(err error) *Logger {
	return l.WithFields(zap.Error(err))
}

// WithTaskID returns a new Logger with the task_id field added.
func (l *Logger) WithTaskID(taskID string) *Logger {
	return l.WithFields(zap.String("task_id", taskID))
}

// WithAgentID returns a new Logger with the agent_id field added.
func (l *Logger) WithAgentID(agentID string) *Logger {
	return l.WithFields(zap.String("agent_id", agentID))
}

// Debug logs a message at debug level with optional structured fields.
func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.zap.Debug(msg, fields...)
}

// Info logs a message at info level with optional structured fields.
func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
}

// Warn logs a message at warn level with optional structured fields.
func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.zap.Warn(msg, fields...)
}

// Error logs a message at error level with optional structured fields.
func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
}

// Fatal logs a message at fatal level with optional structured fields,
// then calls os.Exit(1).
func (l *Logger) Fatal(msg string, fields ...zap.Field) {
	l.zap.Fatal(msg, fields...)
}

// Zap returns the underlying zap.Logger for advanced use cases.
func (l *Logger) Zap() *zap.Logger {
	return l.zap
}

// Sugar returns the underlying zap.SugaredLogger for printf-style logging.
func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.sugar
}

// BufferSnapshot returns a copy of the entries currently held in the
// process-wide log ring buffer used by Improve Kandev reports.
func (l *Logger) BufferSnapshot() []buffer.Entry {
	return buffer.Default().Snapshot()
}
