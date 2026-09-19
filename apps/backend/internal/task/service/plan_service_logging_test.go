package service

import (
	"context"
	"errors"
	"strings"
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

func TestPlanServiceHistoryReadErrorIsLogged(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	const taskID = "task-plan-history-read-log"
	seedTask(t, ctx, repo, taskID)
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: taskID, Content: strings.Repeat("x", planTruncationMinPriorChars+100),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	injected := errors.New("revision history unavailable")
	svc.repo = &planLatestRevisionReadErrorRepo{planRepo: repo, err: injected}
	log, logs := newObservedServiceLogger(t)
	svc.logger = log

	_, err = svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: taskID, Content: "short", CreatedBy: createdByAgent,
		AgentWrite: true, ExpectedVersion: created.Plan.WriteVersion,
		AllowTruncation: true, EvaluateTruncation: true, Mode: PlanWriteModeReplace,
	})
	if err == nil {
		t.Fatal("acknowledged replacement succeeded despite unavailable revision history")
	}
	var safety *PlanSafetyError
	if !errors.As(err, &safety) || safety.Code != PlanErrorHistoryUnavailable {
		t.Fatalf("error = %T %v, want history-unavailable PlanSafetyError", err, err)
	}
	entries := logs.FilterMessage("agent plan history read failed").All()
	if len(entries) != 1 {
		t.Fatalf("history-read log count = %d, want 1", len(entries))
	}
	if entries[0].Level != zapcore.WarnLevel {
		t.Fatalf("history-read log level = %s, want warn", entries[0].Level)
	}
	fields := entries[0].ContextMap()
	if fields["task_id"] != taskID {
		t.Fatalf("history-read task_id = %v, want %q", fields["task_id"], taskID)
	}
	if !strings.Contains(fields["error"].(string), injected.Error()) {
		t.Fatalf("history-read error = %v, want %q", fields["error"], injected)
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

type planLatestRevisionReadErrorRepo struct {
	planRepo
	err error
}

func (r *planLatestRevisionReadErrorRepo) GetLatestTaskPlanRevision(context.Context, string) (*models.TaskPlanRevision, error) {
	return nil, r.err
}
