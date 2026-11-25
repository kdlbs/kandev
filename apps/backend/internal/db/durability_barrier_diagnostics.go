package db

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

const (
	durabilityRunOperation = "durability_barrier"

	durabilityRunOutcomeSuccess = "success"
	durabilityRunOutcomeError   = "error"

	durabilityRunStageComplete        = "complete"
	durabilityRunStageWriterAcquire   = "writer_acquire"
	durabilityRunStageSetFull         = "set_full"
	durabilityRunStageBegin           = "begin"
	durabilityRunStageGenerationWrite = "generation_write"
	durabilityRunStageCommit          = "commit"
	durabilityRunStageRestoreNormal   = "restore_normal"

	durabilityRunErrorNone      = "none"
	durabilityRunErrorCanceled  = "context_canceled"
	durabilityRunErrorDeadline  = "deadline_exceeded"
	durabilityRunErrorOperation = "operation_failed"

	// durabilityRunSlowThreshold is fixed. A run that reaches it emits its
	// record even when it succeeds.
	durabilityRunSlowThreshold = time.Second
)

// durabilityRunDiagnostic records the phases of one actual barrier run. The
// record is local to the run goroutine: it excludes the coalescing window and
// no waiting caller contributes to it. A nil timing field means that phase was
// never reached, which is distinct from a measured zero.
type durabilityRunDiagnostic struct {
	log        *logger.Logger
	started    time.Time
	stage      string
	outcome    string
	errorClass string
	elapsed    time.Duration
	acquire    *time.Duration
	hold       *time.Duration
	begin      *time.Duration
	write      *time.Duration
	commit     *time.Duration
}

func newDurabilityRunDiagnostic(log *logger.Logger) *durabilityRunDiagnostic {
	return &durabilityRunDiagnostic{
		log:        log,
		started:    time.Now(),
		stage:      durabilityRunStageComplete,
		outcome:    durabilityRunOutcomeSuccess,
		errorClass: durabilityRunErrorNone,
	}
}

func (d *durabilityRunDiagnostic) recordAcquire(elapsed time.Duration) {
	d.acquire = &elapsed
}

// finish settles the record after the connection has been released.
func (d *durabilityRunDiagnostic) finish(err error) {
	if err != nil {
		d.outcome = durabilityRunOutcomeError
		d.errorClass = durabilityRunErrorClass(err)
	}
	d.elapsed = time.Since(d.started)
}

// logRun emits one bounded warning for a failed or slow run. A successful run
// below the threshold stays quiet. A nil logger or a nil record keeps the
// barrier silent.
func (d *durabilityRunDiagnostic) logRun(slowThreshold time.Duration) {
	if d == nil || d.log == nil {
		return
	}
	if d.outcome == durabilityRunOutcomeSuccess && d.elapsed < slowThreshold {
		return
	}
	fields := []zap.Field{
		zap.String("operation", durabilityRunOperation),
		zap.String("outcome", d.outcome),
		zap.String("stage", d.stage),
		zap.String("error_class", d.errorClass),
		zap.Float64("elapsed_ms", durationMilliseconds(d.elapsed)),
	}
	fields = appendPhaseDuration(fields, "acquire_ms", d.acquire)
	fields = appendPhaseDuration(fields, "hold_ms", d.hold)
	fields = appendPhaseDuration(fields, "begin_ms", d.begin)
	fields = appendPhaseDuration(fields, "write_ms", d.write)
	fields = appendPhaseDuration(fields, "commit_ms", d.commit)
	d.log.Warn("durability barrier run failed or was slow", fields...)
}

func durabilityRunErrorClass(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return durabilityRunErrorCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return durabilityRunErrorDeadline
	default:
		return durabilityRunErrorOperation
	}
}

func appendPhaseDuration(fields []zap.Field, key string, elapsed *time.Duration) []zap.Field {
	if elapsed == nil {
		return fields
	}
	return append(fields, zap.Float64(key, durationMilliseconds(*elapsed)))
}

func durationMilliseconds(elapsed time.Duration) float64 {
	return float64(elapsed) / float64(time.Millisecond)
}
