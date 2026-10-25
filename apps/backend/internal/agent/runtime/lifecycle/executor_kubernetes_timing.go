package lifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

const (
	kubernetesLaunchOutcomeSuccess  = "success"
	kubernetesLaunchOutcomeError    = "error"
	kubernetesLaunchOutcomeCanceled = "canceled"
	kubernetesLaunchOutcomeTimeout  = "timeout"
)

type kubernetesLaunchTiming struct {
	logger      *logger.Logger
	now         func() time.Time
	startedAt   time.Time
	attemptID   string
	instanceID  string
	taskID      string
	sessionID   string
	failedStage string
	failureSite string
}

func newKubernetesLaunchTiming(log *logger.Logger, req *ExecutorCreateRequest) *kubernetesLaunchTiming {
	return newKubernetesLaunchTimingWithClock(log, req, time.Now)
}

func newKubernetesLaunchTimingWithClock(
	log *logger.Logger,
	req *ExecutorCreateRequest,
	now func() time.Time,
) *kubernetesLaunchTiming {
	if now == nil {
		now = time.Now
	}
	timing := &kubernetesLaunchTiming{
		logger:    log,
		now:       now,
		startedAt: now(),
		attemptID: uuid.NewString(),
	}
	if req != nil {
		timing.instanceID = req.InstanceID
		timing.taskID = req.TaskID
		timing.sessionID = req.SessionID
	}
	return timing
}

func (t *kubernetesLaunchTiming) runStage(stage string, action func() error) error {
	startedAt := t.now()
	err := action()
	t.recordStage(stage, startedAt, err)
	return err
}

func (t *kubernetesLaunchTiming) recordStage(stage string, startedAt time.Time, err error) {
	if err != nil && t.failedStage == "" {
		t.failedStage = stage
	}
	if t.logger == nil {
		return
	}
	t.logger.Info("kubernetes.launch.stage", append(
		t.commonFields(t.elapsedMilliseconds(startedAt), kubernetesLaunchTimingOutcome(err)),
		zap.String("stage", stage),
	)...)
}

func (t *kubernetesLaunchTiming) complete(err error) {
	if t.logger == nil {
		return
	}
	fields := t.commonFields(t.elapsedMilliseconds(t.startedAt), kubernetesLaunchTimingOutcome(err))
	if t.failedStage != "" {
		fields = append(fields, zap.String("failed_stage", t.failedStage))
	} else if t.failureSite != "" {
		fields = append(fields, zap.String("failure_site", t.failureSite))
	}
	t.logger.Info("kubernetes.launch.completed", fields...)
}

func (t *kubernetesLaunchTiming) commonFields(duration int64, outcome string) []zap.Field {
	return []zap.Field{
		zap.String("attempt_id", t.attemptID),
		zap.String("instance_id", t.instanceID),
		zap.String("task_id", t.taskID),
		zap.String("session_id", t.sessionID),
		zap.String("mode", "fresh"),
		zap.Int64("duration_ms", duration),
		zap.String("outcome", outcome),
	}
}

func (t *kubernetesLaunchTiming) elapsedMilliseconds(startedAt time.Time) int64 {
	duration := t.now().Sub(startedAt).Milliseconds()
	if duration < 0 {
		return 0
	}
	return duration
}

func kubernetesLaunchTimingOutcome(err error) string {
	switch {
	case err == nil:
		return kubernetesLaunchOutcomeSuccess
	case errors.Is(err, context.DeadlineExceeded):
		return kubernetesLaunchOutcomeTimeout
	case errors.Is(err, context.Canceled):
		return kubernetesLaunchOutcomeCanceled
	default:
		return kubernetesLaunchOutcomeError
	}
}
