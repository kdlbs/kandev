package backendapp

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeTaskOriginGetter struct {
	task *models.Task
	err  error
}

func (f *fakeTaskOriginGetter) GetTask(_ context.Context, _ string) (*models.Task, error) {
	return f.task, f.err
}

func newObservedAdapter(t *testing.T, getter taskOriginGetter) (*automationTaskOriginLookupAdapter, *observer.ObservedLogs) {
	t.Helper()
	core, obs := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return &automationTaskOriginLookupAdapter{svc: getter, log: log}, obs
}

func TestAutomationTaskOriginLookupAdapter_ErrorLogsWarn(t *testing.T) {
	svcErr := errors.New("db unavailable")
	adapter, obs := newObservedAdapter(t, &fakeTaskOriginGetter{err: svcErr})

	wsID, isAuto, ok := adapter.TaskWorkspaceAndAutomationOrigin(context.Background(), "task-1")

	if ok || wsID != "" || isAuto {
		t.Fatalf("expected zero return on error; got wsID=%q isAuto=%v ok=%v", wsID, isAuto, ok)
	}
	entries := obs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry; got %d", len(entries))
	}
	if entries[0].Level != zap.WarnLevel {
		t.Errorf("expected Warn level; got %v", entries[0].Level)
	}
}

func TestAutomationTaskOriginLookupAdapter_NilTaskLogsDebug(t *testing.T) {
	adapter, obs := newObservedAdapter(t, &fakeTaskOriginGetter{task: nil, err: nil})

	wsID, isAuto, ok := adapter.TaskWorkspaceAndAutomationOrigin(context.Background(), "task-2")

	if ok || wsID != "" || isAuto {
		t.Fatalf("expected zero return on nil task; got wsID=%q isAuto=%v ok=%v", wsID, isAuto, ok)
	}
	entries := obs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry; got %d", len(entries))
	}
	if entries[0].Level != zap.DebugLevel {
		t.Errorf("expected Debug level; got %v", entries[0].Level)
	}
}

func TestAutomationTaskOriginLookupAdapter_FoundTaskNoLog(t *testing.T) {
	task := &models.Task{WorkspaceID: "ws-1", Origin: models.TaskOriginAutomationRun}
	adapter, obs := newObservedAdapter(t, &fakeTaskOriginGetter{task: task})

	wsID, isAuto, ok := adapter.TaskWorkspaceAndAutomationOrigin(context.Background(), "task-3")

	if !ok || wsID != "ws-1" || !isAuto {
		t.Fatalf("unexpected return: wsID=%q isAuto=%v ok=%v", wsID, isAuto, ok)
	}
	if obs.Len() != 0 {
		t.Errorf("expected no log entries on success; got %d", obs.Len())
	}
}
