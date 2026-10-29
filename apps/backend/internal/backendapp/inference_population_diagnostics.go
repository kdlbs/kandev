package backendapp

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

func newInferencePopulationLogger(log *zap.Logger) *zap.Logger {
	if log == nil {
		log = zap.NewNop()
	}
	return log.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewSamplerWithOptions(core, time.Minute, 1, 0)
	}))
}

func inferencePopulationReadFailure(err error) string {
	switch {
	case errors.Is(err, agentruntime.ErrInferenceOwnershipUnsupported):
		return "ownership_unsupported"
	case errors.Is(err, context.Canceled):
		return "read_cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "read_deadline"
	default:
		return "read_failed"
	}
}

func inferencePopulationLocator(ref agentruntime.InferenceClientRef, status *utility.AttemptStatus) sessioncapacity.InferenceBinding {
	id := sessioncapacity.InferenceBinding{SessionID: ref.Identity.SessionID, ParentExecutionID: ref.Identity.ParentExecutionID}
	if ref.Client != nil {
		id.InstanceID = ref.Client.InferenceInstanceID()
	}
	if status != nil {
		id.OwnerID, id.AttemptID = status.OwnerID, status.AttemptID
	}
	return id
}

// Raw errors and arbitrary transport strings never enter diagnostic fields.
// Each closed source/reason pair emits at most once per minute, across all IDs.
func (b *sharedInferenceConsumer) logPopulationUnknown(source, reason string, id sessioncapacity.InferenceBinding) {
	if b.populationLogger == nil {
		return
	}
	fields := []zap.Field{zap.String("source", source), zap.String("reason", reason)}
	for _, field := range []struct{ key, value string }{
		{"instance_id", id.InstanceID}, {"owner_id", id.OwnerID}, {"attempt_id", id.AttemptID},
		{"session_id", id.SessionID}, {"execution_id", id.ParentExecutionID},
	} {
		if parsed, err := uuid.Parse(field.value); err == nil {
			fields = append(fields, zap.String(field.key, parsed.String()))
		}
	}
	b.populationLogger.Warn("capacity population unknown: "+source+"."+reason, fields...)
}
