package process

import (
	"errors"
	"os"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

// logDeliveryJournalOpenFailure emits one bounded diagnostic at the
// process-manager boundary. It carries the trusted journal path and owner
// identifiers for operators, but never request environments, prompts, event
// payloads, or credential values.
func logDeliveryJournalOpenFailure(logger interface {
	Warn(string, ...zap.Field)
}, cfgInstanceID, sessionID, path string, openErr error) {
	if openErr == nil {
		return
	}
	fields := []zap.Field{
		zap.String("instance_id", cfgInstanceID),
		zap.String("session_id", sessionID),
		zap.Int("pid", os.Getpid()),
		zap.String("reason", string(journal.OpenReasonUnavailable)),
	}
	var typed *journal.OpenError
	if errors.As(openErr, &typed) {
		fields[3] = zap.String("reason", typed.Reason().String())
		fields = append(fields,
			zap.String("journal_path", typed.Path()),
			zap.Duration("configured_timeout", typed.ConfiguredTimeout()),
			zap.Duration("elapsed", typed.Elapsed()),
		)
	} else if path != "" {
		fields = append(fields, zap.String("journal_path", path))
	}
	logger.Warn("delivery journal unavailable; instance creation will fail", fields...)
}
