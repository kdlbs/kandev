package orchestrator

// AC-F8's three named overlapping-guard-pair scenarios
// (docs/specs/workflow-on-agent-error-dispatch/spec.md:904-916), split into
// their own file because the sibling coverage test file is already near the
// repo's 800-effective-line test-file limit.

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func namedPairBaseStep() *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}}},
	}
}

// AC-A5 (silent) precedes AC-B2 (WARNING): a sessionless event never reads
// the task, so an unloadable task id produces no AC-B2 WARNING.
func TestDispatchKanbanAgentErrorTrigger_SessionlessEventOnUnloadableTaskNeverWarns(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

	svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "does-not-exist", SessionID: ""})

	if decisions.clearCalls != 0 {
		t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorTaskLoadFailed); len(got) != 0 {
		t.Errorf("got %d %q WARNING(s) for a sessionless event, want 0 (AC-A5 must skip before any task read)",
			len(got), msgAgentErrorTaskLoadFailed)
	}
	if all := logs.All(); len(all) != 0 {
		t.Errorf("expected no log records at all, got %+v", all)
	}
}

// AC-A8 (DEBUG) precedes AC-F4 (silent): a user-initiated cancel on an
// archived task still logs its own DEBUG, because the marker check returns
// before the archived-task shape guard ever runs.
func TestDispatchKanbanAgentErrorTrigger_UserInitiatedOnArchivedTaskStillLogsDebug(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.ArchiveTask(ctx, "t1"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = namedPairBaseStep()
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })

	svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", UserInitiated: true,
	})

	if decisions.clearCalls != 0 {
		t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorUserInitiated); len(got) != 1 {
		t.Fatalf("got %d %q DEBUG record(s), want 1 (AC-A8 must fire before AC-F4's archived check)",
			len(got), msgAgentErrorUserInitiated)
	}
}

// AC-F6 (WARNING) precedes AC-B2 (WARNING): a session reload failure on a
// task that is also unloadable still produces exactly one WARNING, and it is
// AC-F6's — the task is never read once the session read has already failed,
// pinning the session-before-task order.
func TestDispatchKanbanAgentErrorTrigger_SessionReadFailureOnUnloadableTaskIsOneWarning(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = namedPairBaseStep()
	decisions := &spyDecisionStore{}
	svc, logs := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) { s.engineDecisions = decisions })
	double := &agentErrorSessionAndTaskReadErrorRepo{
		sessionExecutorStore: svc.repo,
		sessionErr:           errors.New("session db timeout"),
		taskErr:              errors.New("task db timeout"),
	}
	svc.repo = double

	svc.dispatchKanbanAgentErrorTrigger(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1"})

	if decisions.clearCalls != 0 {
		t.Errorf("clearCalls = %d, want 0", decisions.clearCalls)
	}
	if got := filterLogs(logs, msgAgentErrorSessionReloadFailed); len(got) != 1 {
		t.Fatalf("got %d %q WARNING(s), want 1 (AC-F6)", len(got), msgAgentErrorSessionReloadFailed)
	}
	if got := filterLogs(logs, msgAgentErrorTaskLoadFailed); len(got) != 0 {
		t.Errorf("got %d %q WARNING(s), want 0 (AC-B2 must not also fire)", len(got), msgAgentErrorTaskLoadFailed)
	}
	if double.getTaskCalls != 0 {
		t.Errorf("GetTask called %d time(s), want 0 (task must never be read once the session read has failed)",
			double.getTaskCalls)
	}
}

type agentErrorSessionAndTaskReadErrorRepo struct {
	sessionExecutorStore
	sessionErr   error
	taskErr      error
	getTaskCalls int
}

func (r *agentErrorSessionAndTaskReadErrorRepo) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return nil, r.sessionErr
}

func (r *agentErrorSessionAndTaskReadErrorRepo) GetTask(context.Context, string) (*models.Task, error) {
	r.getTaskCalls++
	return nil, r.taskErr
}
