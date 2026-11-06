package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// Embed the concrete repository so ledger, admission CAS, and effect capabilities
// remain present while failures are injected at their actual storage boundaries.
type prWorkflowFaultRepository struct {
	*sqliterepo.Repository
	activeFailures int32
	listFailures   int32
	emptyLists     int32
	commitFailures int32
	moveAway       bool
	activeCalls    atomic.Int32
	listCalls      atomic.Int32
	commitCalls    atomic.Int32
	beforeCommit   func()
}

func (r *prWorkflowFaultRepository) GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	if r.activeCalls.Add(1) <= r.activeFailures {
		return nil, errors.New("session read unavailable")
	}
	return r.Repository.GetActiveTaskSessionByTaskID(ctx, taskID)
}

func (r *prWorkflowFaultRepository) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	call := r.listCalls.Add(1)
	if call <= r.listFailures {
		return nil, errors.New("session list unavailable")
	}
	if call <= r.emptyLists {
		return nil, nil
	}
	return r.Repository.ListTaskSessions(ctx, taskID)
}

func (r *prWorkflowFaultRepository) UpdateTaskWithWorkflowStepAdmissionIfAtStep(
	ctx context.Context, task *models.Task, expectedStepID, targetStepID string, limit int,
) (bool, error) {
	if r.beforeCommit != nil {
		r.beforeCommit()
	}
	if r.commitCalls.Add(1) <= r.commitFailures {
		return false, errors.New("transition write unavailable")
	}
	if r.moveAway {
		current, err := r.GetTask(ctx, task.ID)
		if err != nil {
			return false, err
		}
		current.WorkflowStepID = "step_exec"
		if err := r.UpdateTask(ctx, current); err != nil {
			return false, err
		}
	}
	return r.Repository.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, expectedStepID, targetStepID, limit)
}

func registerPRWorkflowFixture(t *testing.T, f *prWorkflowFixture, repo *prWorkflowFaultRepository) {
	t.Helper()
	// An idle live session avoids unrelated running-to-waiting reconciliation
	// reads when asserting the session-selection retry counts below.
	session, err := f.repo.GetTaskSession(context.Background(), prWorkflowSessionID)
	if err == nil && session.State == models.TaskSessionStateRunning {
		setSessionState(t, context.Background(), f.repo, prWorkflowSessionID, models.TaskSessionStateWaitingForInput)
	} else if err != nil && !errors.Is(err, models.ErrTaskSessionNotFound) {
		t.Fatal(err)
	}
	f.svc.repo = repo
	f.svc.workflowStore.repo = repo
	f.svc.taskPRWorkflowRetryDelays = []time.Duration{0, 0}
	f.svc.eventBus = bus.NewMemoryEventBus(f.svc.logger)
	t.Cleanup(func() {
		f.svc.stopCIAutomationWorkers()
		f.svc.eventBus.Close()
	})
	f.svc.subscribeGitHubEvents()
}

func publishPRWorkflowFixture(t *testing.T, f *prWorkflowFixture) {
	t.Helper()
	at := time.Now().UTC()
	pr := linkedTaskPR(1, "merged")
	pr.MergedAt = &at
	f.github.taskPRs = []*github.TaskPR{pr}
	err := f.svc.eventBus.Publish(context.Background(), events.GitHubTaskPRUpdated,
		bus.NewEvent(events.GitHubTaskPRUpdated, "github", pr))
	if err != nil {
		t.Fatalf("publish PR update: %v", err)
	}
	f.svc.ciAutomationWorkers.Wait()
}

// @covers AC-TASKS-WORKFLOW-PR-EVENTS-001.4
func TestRegisteredPRWorkflowSessionReadRetry(t *testing.T) {
	cases := []struct {
		name          string
		activeFailure int32
		listFailure   int32
		emptyList     int32
		terminal      bool
		noSession     bool
		wantActive    int32
		wantList      int32
	}{
		{name: "both reads fail then recover", activeFailure: 1, listFailure: 1, wantActive: 2, wantList: 1},
		{name: "active error and empty fallback then recover", activeFailure: 1, emptyList: 1, wantActive: 2, wantList: 1},
		{name: "terminal list error then recover", listFailure: 1, terminal: true, wantActive: 2, wantList: 2},
		{name: "healthy fallback after active error", activeFailure: 1, wantActive: 1, wantList: 1},
		{name: "genuine empty task skips", noSession: true, wantActive: 1, wantList: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPRWorkflowFixture(t, prMergedAcceptEvents())
			ctx := context.Background()
			if tc.terminal {
				setSessionState(t, ctx, f.repo, prWorkflowSessionID, models.TaskSessionStateCompleted)
			}
			if tc.noSession {
				session, err := f.repo.GetTaskSession(ctx, prWorkflowSessionID)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.repo.DeleteTaskSession(ctx, session); err != nil {
					t.Fatal(err)
				}
			}
			repo := &prWorkflowFaultRepository{Repository: f.repo,
				activeFailures: tc.activeFailure, listFailures: tc.listFailure, emptyLists: tc.emptyList}
			registerPRWorkflowFixture(t, f, repo)
			publishPRWorkflowFixture(t, f)
			wantStep, wantHistory := "step_done", 1
			if tc.noSession {
				wantStep, wantHistory = "step_accept", 0
			}
			if got := f.taskStep(t); got != wantStep {
				t.Fatalf("registered handler step = %s, want %s after storage recovery", got, wantStep)
			}
			if !tc.terminal && !tc.noSession {
				f.waitOnEnter(t)
			}
			if got := len(f.recorder.Calls()); got != wantHistory {
				t.Errorf("history rows = %d, want %d", got, wantHistory)
			}
			if repo.activeCalls.Load() != tc.wantActive || repo.listCalls.Load() != tc.wantList {
				t.Errorf("session reads = %d/%d, want %d/%d", repo.activeCalls.Load(), repo.listCalls.Load(), tc.wantActive, tc.wantList)
			}
		})
	}
}

// @covers AC-TASKS-WORKFLOW-PR-EVENTS-001.4, AC-TASKS-WORKFLOW-PR-EVENTS-001.6
func TestRegisteredPRWorkflowCommitRetry(t *testing.T) {
	for _, tc := range []struct {
		name        string
		terminal    bool
		failures    int32
		moveAway    bool
		wantCalls   int32
		wantStep    string
		wantHistory int
	}{
		{name: "terminal commit recovers", terminal: true, failures: 1, wantCalls: 2, wantStep: "step_done", wantHistory: 1},
		{name: "live commit recovers", failures: 1, wantCalls: 2, wantStep: "step_done", wantHistory: 1},
		{name: "healthy terminal commit", terminal: true, wantCalls: 1, wantStep: "step_done", wantHistory: 1},
		{name: "terminal stale CAS is not retried", terminal: true, moveAway: true, wantCalls: 1, wantStep: "step_exec"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPRWorkflowFixture(t, prMergedAcceptEvents())
			if tc.terminal {
				setSessionState(t, context.Background(), f.repo, prWorkflowSessionID, models.TaskSessionStateCompleted)
			}
			repo := &prWorkflowFaultRepository{Repository: f.repo, commitFailures: tc.failures, moveAway: tc.moveAway}
			registerPRWorkflowFixture(t, f, repo)
			publishPRWorkflowFixture(t, f)
			if got := f.taskStep(t); got != tc.wantStep {
				t.Fatalf("registered handler step = %s, want %s after commit recovery", got, tc.wantStep)
			}
			if !tc.terminal {
				f.waitOnEnter(t)
			}
			if repo.commitCalls.Load() != tc.wantCalls || len(f.recorder.Calls()) != tc.wantHistory {
				t.Errorf("commits/history = %d/%d, want %d/%d", repo.commitCalls.Load(), len(f.recorder.Calls()), tc.wantCalls, tc.wantHistory)
			}
			if tc.terminal {
				session, err := f.repo.GetTaskSession(context.Background(), prWorkflowSessionID)
				if err != nil || session.State != models.TaskSessionStateCompleted {
					t.Fatalf("terminal session changed: session=%+v error=%v", session, err)
				}
			}
		})
	}
}
