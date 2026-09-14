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

func TestPlanServiceRevertMissingTaskWriteLogSeverity(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-plan-revert-missing")

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-plan-revert-missing", Content: "body",
	})
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	revision, err := repo.GetLatestTaskPlanRevision(ctx, "task-plan-revert-missing")
	if err != nil {
		t.Fatalf("GetLatestTaskPlanRevision error = %v", err)
	}

	svc.repo = &planWriteErrorRepo{planRepo: repo, err: repoerrors.ErrTaskNotFound}
	log, logs := newObservedServiceLogger(t)
	svc.logger = log

	_, err = svc.RevertPlan(ctx, RevertPlanRequest{
		TaskID:           "task-plan-revert-missing",
		TargetRevisionID: revision.ID,
	})
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("RevertPlan error = %v, want ErrTaskNotFound", err)
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

func TestPlanServiceRevertOtherWriteErrorLogSeverity(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-plan-revert-error")

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: "task-plan-revert-error", Content: "body",
	})
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	revision, err := repo.GetLatestTaskPlanRevision(ctx, "task-plan-revert-error")
	if err != nil {
		t.Fatalf("GetLatestTaskPlanRevision error = %v", err)
	}
	injected := errors.New("database unavailable")
	svc.repo = &planWriteErrorRepo{planRepo: repo, err: injected}
	log, logs := newObservedServiceLogger(t)
	svc.logger = log

	_, err = svc.RevertPlan(ctx, RevertPlanRequest{
		TaskID:           "task-plan-revert-error",
		TargetRevisionID: revision.ID,
	})
	if !errors.Is(err, injected) {
		t.Fatalf("RevertPlan error = %v, want injected error", err)
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
	bool,
	bool,
) error {
	return r.err
}
