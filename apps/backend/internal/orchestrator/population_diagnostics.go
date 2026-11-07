package orchestrator

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Diagnostic sampling is independent of admission, queue ownership and wakeups.
func newPopulationDiagnosticLogger(log *zap.Logger) *zap.Logger {
	if log == nil {
		log = zap.NewNop()
	}
	return log.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewSamplerWithOptions(core, time.Minute, 1, 0)
	}))
}

func populationReadFailure(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "read_cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "read_deadline"
	default:
		return "read_failed"
	}
}

// Only closed source/reason codes and UUID locators reach the existing log sink.
func (c *sessionCeilingController) logPopulationUnknown(source, reason, idKey, id string) {
	if c.populationLogger == nil {
		return
	}
	fields := []zap.Field{zap.String("source", source), zap.String("reason", reason)}
	id = strings.TrimPrefix(strings.TrimPrefix(id, "office-run:"), "launch-")
	if parsed, err := uuid.Parse(id); err == nil {
		fields = append(fields, zap.String(idKey, parsed.String()))
	}
	c.populationLogger.Warn("capacity population unknown: "+source+"."+reason, fields...)
}
