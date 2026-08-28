package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap/zapcore"
)

func TestPlanServiceMissingTaskWriteLogSeverity(t *testing.T) {
	svc, _, _ := createTestPlanService(t)
	log, logs := newObservedServiceLogger(t)
	svc.logger = log

	_, err := svc.CreatePlan(context.Background(), CreatePlanRequest{
		TaskID: "task-plan-service-missing", Content: "body",
	})
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("CreatePlan error = %v, want ErrTaskNotFound", err)
	}
	entries := logs.FilterMessage("write plan revision").All()
	if len(entries) != 1 {
		t.Fatalf("write-plan log count = %d, want 1", len(entries))
	}
	if entries[0].Level != zapcore.DebugLevel {
		t.Fatalf("write-plan log level = %s, want debug", entries[0].Level)
	}
	if errorsCount := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); errorsCount != 0 {
		t.Fatalf("error log count = %d, want 0 for missing task", errorsCount)
	}
}

func TestPlanServiceOtherWriteErrorLogSeverity(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	injected := errors.New("database unavailable")
	svc.repo = &planWriteErrorRepo{planRepo: repo, err: injected}
	log, logs := newObservedServiceLogger(t)
	svc.logger = log

	_, err := svc.CreatePlan(context.Background(), CreatePlanRequest{
		TaskID: "task-plan-service-error", Content: "body",
	})
	if !errors.Is(err, injected) {
		t.Fatalf("CreatePlan error = %v, want injected error", err)
	}
	entries := logs.FilterMessage("write plan revision").All()
	if len(entries) != 1 {
		t.Fatalf("write-plan log count = %d, want 1", len(entries))
	}
	if entries[0].Level != zapcore.ErrorLevel {
		t.Fatalf("write-plan log level = %s, want error", entries[0].Level)
	}
}

type planWriteErrorRepo struct {
	planRepo
	err error
}

func (r *planWriteErrorRepo) WritePlanRevision(
	context.Context,
	*models.TaskPlan,
	*models.TaskPlanRevision,
	*string,
) error {
	return r.err
}
