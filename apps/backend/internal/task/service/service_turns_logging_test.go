package service

import (
	"context"
	"errors"
	"testing"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestGetWorkspaceInfoForSession_MissingEnvironmentLogSeverity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-stale-environment", TaskID: "task-123",
		TaskEnvironmentID: "env-stale", State: models.TaskSessionStateCompleted,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	createTestEnvironment(t, repo, "env-current", "task-123")

	log, logs := newObservedServiceLogger(t)
	svc.logger = log

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-stale-environment")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession: %v", err)
	}
	if info.TaskEnvironmentID != "env-current" {
		t.Fatalf("TaskEnvironmentID = %q, want env-current", info.TaskEnvironmentID)
	}
	if entries := logs.FilterLevelExact(zapcore.WarnLevel).All(); len(entries) != 0 {
		t.Fatalf("stale environment warning count = %d, want 0", len(entries))
	}
	entries := logs.FilterMessage("failed to get task environment for session").All()
	if len(entries) != 1 {
		t.Fatalf("stale environment debug evidence count = %d, want 1", len(entries))
	}
	if entries[0].Level != zapcore.DebugLevel {
		t.Fatalf("stale environment log level = %s, want debug", entries[0].Level)
	}
}

func TestGetWorkspaceInfoForSession_UnexpectedEnvironmentErrorLogSeverity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-environment-error", TaskID: "task-123",
		TaskEnvironmentID: "env-unavailable", State: models.TaskSessionStateCompleted,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	createTestEnvironment(t, repo, "env-current", "task-123")
	svc.taskEnvironments = &workspaceInfoTaskEnvironmentRepo{
		TaskEnvironmentRepository: repo,
		directErr:                 errors.New("environment store unavailable"),
	}

	log, logs := newObservedServiceLogger(t)
	svc.logger = log

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-environment-error")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession: %v", err)
	}
	if info.TaskEnvironmentID != "env-current" {
		t.Fatalf("TaskEnvironmentID = %q, want env-current", info.TaskEnvironmentID)
	}
	entries := logs.FilterMessage("failed to get task environment for session").All()
	if len(entries) != 1 {
		t.Fatalf("unexpected environment warning count = %d, want 1", len(entries))
	}
	if entries[0].Level != zapcore.WarnLevel {
		t.Fatalf("unexpected environment log level = %s, want warn", entries[0].Level)
	}
}

type workspaceInfoTaskEnvironmentRepo struct {
	repository.TaskEnvironmentRepository
	directErr error
}

func (r *workspaceInfoTaskEnvironmentRepo) GetTaskEnvironment(context.Context, string) (*models.TaskEnvironment, error) {
	return nil, r.directErr
}

func newObservedServiceLogger(t *testing.T) (*commonlogger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return log, logs
}
