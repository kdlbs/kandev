package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	officepause "github.com/kandev/kandev/internal/office/pause"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	"github.com/kandev/kandev/internal/office/scheduler"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
	officewakeup "github.com/kandev/kandev/internal/office/wakeup"
	runmodels "github.com/kandev/kandev/internal/runs/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	escalationBlocker = "dependent-prerequisite-0"
	escalationParent  = "coordinator-parent"
	escalationRunner  = "assignment-old"
	escalationCEO     = "assignment-ceo"
)

type escalationFixture struct {
	*assignmentBlockerFixture
	ts               *taskservice.Service
	wakeupDispatcher *officewakeup.Dispatcher
}

// newEscalationFixture builds an Office dependent held by one in-progress
// blocker, with the failure subscriber registered on the real bus and state
// writes going through the real task service publisher.
func newEscalationFixture(t *testing.T) *escalationFixture {
	t.Helper()
	f := newAssignmentBlockerFixture(t)
	wakeupDispatcher := officewakeup.NewDispatcher(f.repo, f.repo, logger.Default())
	wakeupDispatcher.SetRunQueuer(f.svc)
	f.svc.SetWakeupRequestDispatcher(wakeupDispatcher)
	blockerMust(t, f.scheduler.RegisterBlockerFailureSubscriber(f.eb))
	f.task(t, "dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
	f.blockers(t, "dependent", v1.TaskStateInProgress)
	return &escalationFixture{assignmentBlockerFixture: f, ts: f.completionService(t), wakeupDispatcher: wakeupDispatcher}
}

func (f *escalationFixture) withParentRunner(t *testing.T) {
	t.Helper()
	f.task(t, escalationParent, "assignment-work", v1.TaskStateInProgress, escalationRunner)
	blockerMust(t, f.repo.UpdateTaskParentID(context.Background(), escalationBlocker, escalationParent))
}

func (f *escalationFixture) withCEO(t *testing.T) {
	t.Helper()
	blockerMust(t, f.repo.CreateAgentInstance(context.Background(), &models.AgentInstance{
		ID: escalationCEO, AgentID: "assignment-cli", WorkspaceID: "assignment-ws", Name: "CEO",
		Role: models.AgentRoleCEO, Status: models.AgentStatusIdle,
	}))
}

func (f *escalationFixture) setState(t *testing.T, id string, state v1.TaskState) {
	t.Helper()
	_, err := f.ts.UpdateTaskState(context.Background(), id, state)
	blockerMust(t, err)
}

func (f *escalationFixture) escalations(t *testing.T) []*runmodels.Run {
	t.Helper()
	runs, err := f.svc.ListRuns(context.Background(), "assignment-ws")
	blockerMust(t, err)
	var found []*runmodels.Run
	for _, run := range runs {
		if run.Reason == service.RunReasonTaskBlockerFailed {
			found = append(found, run)
		}
	}
	return found
}

func (f *escalationFixture) wantEscalations(t *testing.T, want int, when string) []*runmodels.Run {
	t.Helper()
	runs := f.escalations(t)
	if len(runs) != want {
		t.Fatalf("%s: escalation runs=%d, want %d", when, len(runs), want)
	}
	return runs
}

type escalationPayload struct {
	TaskID           string                                      `json:"task_id"`
	BlockerTaskID    string                                      `json:"blocker_task_id"`
	BlockerState     string                                      `json:"blocker_state"`
	HeldDependentIDs []string                                    `json:"held_dependent_ids"`
	Alerts           map[string]officewakeup.BlockerFailureAlert `json:"blocker_failure_alerts"`
}

func decodeEscalation(t *testing.T, run *runmodels.Run) escalationPayload {
	t.Helper()
	var p escalationPayload
	blockerMust(t, json.Unmarshal([]byte(run.Payload), &p))
	return p
}

func decodeEscalationAlerts(t *testing.T, run *runmodels.Run) map[string]officewakeup.BlockerFailureAlert {
	t.Helper()
	var payload escalationPayload
	encoded := run.ContextSnapshot
	if encoded == "" {
		encoded = run.Payload
	}
	blockerMust(t, json.Unmarshal([]byte(encoded), &payload))
	if len(payload.Alerts) == 0 {
		blockerMust(t, json.Unmarshal([]byte(run.Payload), &payload))
	}
	return payload.Alerts
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.1 AC-OFFICE-BLOCKER-POLICY-002.3
func TestOfficeBlockerFailureEscalatesToParentRunner(t *testing.T) {
	for _, failure := range []v1.TaskState{v1.TaskStateFailed, v1.TaskStateCancelled} {
		t.Run(string(failure), func(t *testing.T) {
			f := newEscalationFixture(t)
			f.withParentRunner(t)
			f.withCEO(t)
			f.task(t, "second-dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
			blockerMust(t, f.repo.CreateTaskBlocker(context.Background(),
				&models.TaskBlocker{TaskID: "second-dependent", BlockerTaskID: escalationBlocker}))

			f.setState(t, escalationBlocker, failure)

			run := f.wantEscalations(t, 1, "failure transition")[0]
			if run.AgentProfileID != escalationRunner {
				t.Fatalf("recipient=%q, want parent runner %q", run.AgentProfileID, escalationRunner)
			}
			p := decodeEscalation(t, run)
			if p.TaskID != escalationParent || p.BlockerTaskID != escalationBlocker || p.BlockerState != string(failure) {
				t.Fatalf("payload=%+v", p)
			}
			if len(p.HeldDependentIDs) != 2 || p.HeldDependentIDs[0] != "dependent" || p.HeldDependentIDs[1] != "second-dependent" {
				t.Fatalf("held dependents=%v", p.HeldDependentIDs)
			}
		})
	}
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.1
func TestOfficeBlockerFailureFallsBackToCEO(t *testing.T) {
	cases := map[string]func(t *testing.T, f *escalationFixture){
		"no_parent": func(*testing.T, *escalationFixture) {},
		"parent_without_runner": func(t *testing.T, f *escalationFixture) {
			f.task(t, escalationParent, "assignment-work", v1.TaskStateInProgress, "")
			blockerMust(t, f.repo.UpdateTaskParentID(context.Background(), escalationBlocker, escalationParent))
		},
		"parent_terminal": func(t *testing.T, f *escalationFixture) {
			f.task(t, escalationParent, "assignment-work", v1.TaskStateCancelled, escalationRunner)
			blockerMust(t, f.repo.UpdateTaskParentID(context.Background(), escalationBlocker, escalationParent))
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			f := newEscalationFixture(t)
			f.withCEO(t)
			arrange(t, f)
			f.setState(t, escalationBlocker, v1.TaskStateFailed)
			run := f.wantEscalations(t, 1, "failure transition")[0]
			if run.AgentProfileID != escalationCEO {
				t.Fatalf("recipient=%q, want CEO", run.AgentProfileID)
			}
			if p := decodeEscalation(t, run); p.TaskID != "" || p.BlockerTaskID != escalationBlocker {
				t.Fatalf("CEO escalation payload=%+v", p)
			}
		})
	}
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.2 AC-OFFICE-BLOCKER-POLICY-002.5 AC-OFFICE-BLOCKER-POLICY-002.7
func TestOfficeBlockerFailureNoEscalation(t *testing.T) {
	cases := map[string]func(t *testing.T, f *escalationFixture){
		"no_recipient": func(t *testing.T, f *escalationFixture) {
			f.setState(t, escalationBlocker, v1.TaskStateFailed)
		},
		"dependent_terminal": func(t *testing.T, f *escalationFixture) {
			f.withCEO(t)
			blockerMust(t, f.tasks.UpdateTaskState(context.Background(), "dependent", v1.TaskStateCancelled))
			f.setState(t, escalationBlocker, v1.TaskStateFailed)
		},
		"dependent_archived": func(t *testing.T, f *escalationFixture) {
			f.withCEO(t)
			blockerMust(t, f.tasks.ArchiveTask(context.Background(), "dependent"))
			f.setState(t, escalationBlocker, v1.TaskStateFailed)
		},
		"dependent_kanban": func(t *testing.T, f *escalationFixture) {
			f.withCEO(t)
			work, _ := f.completionSteps(t, true)
			task, err := f.tasks.GetTask(context.Background(), "dependent")
			blockerMust(t, err)
			task.WorkflowID, task.WorkflowStepID = "kanban", work
			blockerMust(t, f.tasks.UpdateTask(context.Background(), task))
			f.setState(t, escalationBlocker, v1.TaskStateFailed)
		},
		"failed_to_cancelled": func(t *testing.T, f *escalationFixture) {
			blockerMust(t, f.tasks.UpdateTaskState(context.Background(), escalationBlocker, v1.TaskStateFailed))
			f.withCEO(t)
			f.setState(t, escalationBlocker, v1.TaskStateCancelled)
		},
		"stale_event": func(t *testing.T, f *escalationFixture) {
			f.withCEO(t)
			f.publishState(t, escalationBlocker, "IN_PROGRESS", "FAILED", "2026-11-11T00:00:00Z")
		},
		"success": func(t *testing.T, f *escalationFixture) {
			f.withCEO(t)
			f.setState(t, escalationBlocker, v1.TaskStateCompleted)
		},
	}
	for name, act := range cases {
		t.Run(name, func(t *testing.T) {
			f := newEscalationFixture(t)
			act(t, f)
			f.wantEscalations(t, 0, name)
		})
	}
}

func TestOfficeBlockerFailurePauseReplayAndStaleDiscard(t *testing.T) {
	for _, recoveryTick := range []bool{false, true} {
		name := "resume_hook"
		if recoveryTick {
			name = "recovery_tick"
		}
		t.Run(name, func(t *testing.T) {
			f := newEscalationFixture(t)
			f.withParentRunner(t)
			pauseSvc := officepause.NewService(f.repo, f.svc, f.tasks, logger.Default())
			f.svc.SetPauseGate(pauseSvc)
			f.wakeupDispatcher.SetPauseGate(pauseSvc)
			if !recoveryTick {
				pauseSvc.SetAssignmentReplayer(f.svc)
			}
			_, err := pauseSvc.Pause(context.Background(), "assignment-ws", "test", "user-1", "user")
			blockerMust(t, err)

			f.setState(t, escalationBlocker, v1.TaskStateFailed)
			f.wantEscalations(t, 0, "paused failure remains deferred")
			requests, err := f.repo.ListQueuedWakeupRequestsForWorkspaceAndSource(
				context.Background(), "assignment-ws", officewakeup.SourceBlockerFailure, 0)
			blockerMust(t, err)
			if len(requests) != 1 {
				t.Fatalf("queued blocker requests=%d, want 1", len(requests))
			}

			_, err = pauseSvc.Resume(context.Background(), "assignment-ws", "test complete", "user-1", "user")
			blockerMust(t, err)
			if recoveryTick {
				blockerMust(t, f.svc.ReplayPendingDeferredAssignments(context.Background()))
			}
			f.wantEscalations(t, 1, "failure replayed after resume")
			request, err := f.repo.GetWakeupRequest(context.Background(), requests[0].ID)
			blockerMust(t, err)
			if request.Status != officesqlite.WakeupStatusClaimed {
				t.Fatalf("replayed request status=%q, want claimed", request.Status)
			}
		})
	}

	t.Run("recovered before resume", func(t *testing.T) {
		f := newEscalationFixture(t)
		pauseSvc := officepause.NewService(f.repo, f.svc, f.tasks, logger.Default())
		f.svc.SetPauseGate(pauseSvc)
		f.wakeupDispatcher.SetPauseGate(pauseSvc)
		pauseSvc.SetAssignmentReplayer(f.svc)
		_, err := pauseSvc.Pause(context.Background(), "assignment-ws", "test", "user-1", "user")
		blockerMust(t, err)
		f.setState(t, escalationBlocker, v1.TaskStateFailed)
		f.setState(t, escalationBlocker, v1.TaskStateInProgress)
		_, err = pauseSvc.Resume(context.Background(), "assignment-ws", "test complete", "user-1", "user")
		blockerMust(t, err)
		f.wantEscalations(t, 0, "recovered blocker is discarded")
		requests, err := f.repo.ListQueuedWakeupRequestsForWorkspaceAndSource(
			context.Background(), "assignment-ws", officewakeup.SourceBlockerFailure, 0)
		blockerMust(t, err)
		if len(requests) != 0 {
			t.Fatalf("queued blocker requests=%d, want stale request resolved", len(requests))
		}
	})

	t.Run("queued occurrence follows reassignment", func(t *testing.T) {
		f := newEscalationFixture(t)
		f.withParentRunner(t)
		pauseSvc := officepause.NewService(f.repo, f.svc, f.tasks, logger.Default())
		f.svc.SetPauseGate(pauseSvc)
		f.wakeupDispatcher.SetPauseGate(pauseSvc)
		pauseSvc.SetAssignmentReplayer(f.svc)
		_, err := pauseSvc.Pause(context.Background(), "assignment-ws", "test", "user-1", "user")
		blockerMust(t, err)
		f.setState(t, escalationBlocker, v1.TaskStateFailed)
		stored, err := f.tasks.GetTask(context.Background(), escalationBlocker)
		blockerMust(t, err)
		committedAt := stored.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")

		_, err = f.repo.UpdateTaskAssignee(context.Background(), escalationParent, "assignment-worker")
		blockerMust(t, err)
		f.publishState(t, escalationBlocker, "IN_PROGRESS", "FAILED", committedAt)

		requests, err := f.repo.ListQueuedWakeupRequestsForWorkspaceAndSource(
			context.Background(), "assignment-ws", officewakeup.SourceBlockerFailure, 0)
		blockerMust(t, err)
		if len(requests) != 1 || requests[0].AgentProfileID != "assignment-worker" {
			t.Fatalf("queued requests=%+v, want one request for reassigned coordinator", requests)
		}
		_, err = pauseSvc.Resume(context.Background(), "assignment-ws", "test complete", "user-1", "user")
		blockerMust(t, err)
		run := f.wantEscalations(t, 1, "reassigned coordinator receives alert")[0]
		if run.AgentProfileID != "assignment-worker" {
			t.Fatalf("recipient=%q, want assignment-worker", run.AgentProfileID)
		}
	})
}

func (f *escalationFixture) publishState(t *testing.T, id, oldState, newState, updatedAt string) {
	t.Helper()
	data := map[string]any{"task_id": id, "old_state": oldState, "new_state": newState, "updated_at": updatedAt}
	blockerMust(t, f.eb.Publish(context.Background(), events.TaskStateChanged,
		bus.NewEvent(events.TaskStateChanged, "escalation-test", data)))
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.4
func TestOfficeBlockerFailureOccurrenceIdentity(t *testing.T) {
	f := newEscalationFixture(t)
	f.withCEO(t)
	f.setState(t, escalationBlocker, v1.TaskStateFailed)
	f.wantEscalations(t, 1, "first failure")

	stored, err := f.tasks.GetTask(context.Background(), escalationBlocker)
	blockerMust(t, err)
	committedAt := stored.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")
	// Metadata changes after the transition must not change its occurrence
	// identity. Replayed event data carries the timestamp committed with the
	// failure, while the task row now has a newer updated_at.
	_, err = f.repo.UpdateTaskAssignee(context.Background(), escalationBlocker, "assignment-worker")
	blockerMust(t, err)
	f.publishState(t, escalationBlocker, "IN_PROGRESS", "FAILED", committedAt)
	f.publishState(t, escalationBlocker, "IN_PROGRESS", "FAILED", committedAt)
	f.wantEscalations(t, 1, "duplicate delivery after metadata edit")

	f.setState(t, escalationBlocker, v1.TaskStateInProgress)
	f.setState(t, escalationBlocker, v1.TaskStateFailed)
	run := f.wantEscalations(t, 1, "refail after retry is coalesced")[0]
	if len(decodeEscalationAlerts(t, run)) != 2 {
		t.Fatalf("coalesced blocker alerts=%v, want both failure occurrences", decodeEscalationAlerts(t, run))
	}
}

func TestOfficeBlockerFailureOccurrenceIdentityIgnoresRecipient(t *testing.T) {
	f := newEscalationFixture(t)
	f.withParentRunner(t)
	f.setState(t, escalationBlocker, v1.TaskStateFailed)
	f.wantEscalations(t, 1, "first failure")

	stored, err := f.tasks.GetTask(context.Background(), escalationBlocker)
	blockerMust(t, err)
	committedAt := stored.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")
	_, err = f.repo.UpdateTaskAssignee(context.Background(), escalationParent, "assignment-worker")
	blockerMust(t, err)
	f.publishState(t, escalationBlocker, "IN_PROGRESS", "FAILED", committedAt)
	f.wantEscalations(t, 1, "duplicate after parent reassignment")
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.1 AC-OFFICE-BLOCKER-POLICY-002.4
func TestOfficeBlockerFailureDistinctBlockersKeepSeparateWakes(t *testing.T) {
	f := newEscalationFixture(t)
	f.withCEO(t)
	f.task(t, "other-prerequisite-0", "assignment-work", v1.TaskStateInProgress, "")
	blockerMust(t, f.repo.CreateTaskBlocker(context.Background(),
		&models.TaskBlocker{TaskID: "dependent", BlockerTaskID: "other-prerequisite-0"}))
	// A dangling edge to a deleted dependent must not suppress the escalation.
	blockerMust(t, f.repo.CreateTaskBlocker(context.Background(),
		&models.TaskBlocker{TaskID: "deleted-dependent", BlockerTaskID: "other-prerequisite-0"}))
	f.setState(t, escalationBlocker, v1.TaskStateFailed)
	f.setState(t, "other-prerequisite-0", v1.TaskStateFailed)
	runs := f.wantEscalations(t, 1, "two blockers coalesce into one wake")
	seen := map[string]bool{}
	for _, alert := range decodeEscalationAlerts(t, runs[0]) {
		seen[alert.BlockerTaskID] = true
	}
	if !seen[escalationBlocker] || !seen["other-prerequisite-0"] {
		t.Fatalf("escalated blockers=%v", seen)
	}
}

func TestOfficeBlockerFailureMergesIntoExistingAgentRun(t *testing.T) {
	for _, status := range []string{"queued", "claimed"} {
		t.Run(status, func(t *testing.T) {
			f := newEscalationFixture(t)
			f.withParentRunner(t)
			f.withCEO(t)
			inflight := &runmodels.Run{
				ID: "existing-agent-run", AgentProfileID: escalationRunner,
				Reason: service.RunReasonAgentError, Payload: `{"agent_profile_id":"assignment-old"}`,
				Status: runmodels.RunStatus(status), ContextSnapshot: `{"existing_context":"keep me","task_id":"prior-task"}`,
			}
			blockerMust(t, f.repo.CreateRun(context.Background(), inflight))

			f.setState(t, escalationBlocker, v1.TaskStateFailed)
			runs, err := f.svc.ListRuns(context.Background(), "assignment-ws")
			blockerMust(t, err)
			if len(runs) != 1 || runs[0].ID != inflight.ID {
				t.Fatalf("runs=%v, want only existing in-flight run", runIDs(runs))
			}
			if runs[0].Reason != service.RunReasonAgentError {
				t.Fatalf("existing run reason=%q, want agent_error preserved", runs[0].Reason)
			}
			var snapshot map[string]any
			blockerMust(t, json.Unmarshal([]byte(runs[0].ContextSnapshot), &snapshot))
			if snapshot["task_id"] != "prior-task" {
				t.Fatalf("merged context task_id=%v, want existing target prior-task", snapshot["task_id"])
			}
			alerts := decodeEscalationAlerts(t, runs[0])
			if len(alerts) != 1 {
				t.Fatalf("merged alerts=%v, want one blocker failure", alerts)
			}
			prompt := service.BuildPrompt(service.BuildPromptContextWithSnapshotForTest(
				f.svc, context.Background(), runs[0].Reason, runs[0].Payload, runs[0].ContextSnapshot))
			for _, want := range []string{escalationBlocker, "dependent", escalationParent} {
				if !strings.Contains(prompt, want) {
					t.Errorf("existing run prompt missing %q:\n%s", want, prompt)
				}
			}
		})
	}
}

func runIDs(runs []*runmodels.Run) []string {
	ids := make([]string, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

// officeStatus changes a task's status through the Office dashboard, the path
// the Office UI and the agent runtime status action share. It writes through
// the task repository and never publishes task.state_changed.
func (f *escalationFixture) officeStatus(t *testing.T, id, status, actorAgentID string) {
	t.Helper()
	log := logger.Default()
	dash := dashboard.NewDashboardService(f.repo, log, shared.NewActivityLogger(f.repo, log), f.svc, nil)
	dash.SetTaskStateWriter(f.tasks)
	dash.SetReactivityApplier(scheduler.NewDashboardReactivityAdapter(f.scheduler))
	blockerMust(t, dash.UpdateTaskStatusAsAgent(context.Background(), officeruntime.TaskStatusUpdate{
		TaskID: id, NewStatus: status, ActorAgentID: actorAgentID,
	}))
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.3 AC-OFFICE-BLOCKER-POLICY-002.4
func TestOfficeBlockerFailureEscalatesFromOfficeStatusPath(t *testing.T) {
	for name, actor := range map[string]string{"user": "", "agent_runtime": "assignment-worker"} {
		t.Run(name, func(t *testing.T) {
			f := newEscalationFixture(t)
			f.withParentRunner(t)
			f.officeStatus(t, escalationBlocker, "cancelled", actor)
			run := f.wantEscalations(t, 1, "office cancel")[0]
			p := decodeEscalation(t, run)
			if run.AgentProfileID != escalationRunner || p.TaskID != escalationParent ||
				p.BlockerState != string(v1.TaskStateCancelled) || len(p.HeldDependentIDs) != 1 {
				t.Fatalf("recipient=%q payload=%+v", run.AgentProfileID, p)
			}
			f.officeStatus(t, escalationBlocker, "cancelled", actor)
			f.wantEscalations(t, 1, "repeated office cancel")
		})
	}
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.5
func TestOfficeBlockerFailureOfficeCancelOfFailedBlockerDoesNotEscalate(t *testing.T) {
	f := newEscalationFixture(t)
	blockerMust(t, f.tasks.UpdateTaskState(context.Background(), escalationBlocker, v1.TaskStateFailed))
	f.withCEO(t)
	f.officeStatus(t, escalationBlocker, "cancelled", "")
	f.wantEscalations(t, 0, "failed to cancelled through office")
}

// withTree makes the blocker and an in-tree dependent children of a tree root
// whose runner would be the recipient while the root is live.
func (f *escalationFixture) withTree(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	f.task(t, "tree-root", "assignment-work", v1.TaskStateInProgress, escalationRunner)
	f.task(t, "tree-dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
	for _, child := range []string{escalationBlocker, "tree-dependent"} {
		blockerMust(t, f.repo.UpdateTaskParentID(ctx, child, "tree-root"))
	}
	blockerMust(t, f.repo.CreateTaskBlocker(ctx,
		&models.TaskBlocker{TaskID: "tree-dependent", BlockerTaskID: escalationBlocker}))
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.1 AC-OFFICE-BLOCKER-POLICY-002.3
func TestOfficeBlockerFailureTreeCancelEscalatesOutsideDependents(t *testing.T) {
	f := newEscalationFixture(t)
	f.withCEO(t)
	f.withTree(t)
	_, err := f.svc.CancelTaskTree(context.Background(), "tree-root", "user:test")
	blockerMust(t, err)
	run := f.wantEscalations(t, 1, "tree cancel")[0]
	p := decodeEscalation(t, run)
	if run.AgentProfileID != escalationCEO || p.TaskID != "" || p.BlockerTaskID != escalationBlocker ||
		p.BlockerState != string(v1.TaskStateCancelled) {
		t.Fatalf("recipient=%q payload=%+v", run.AgentProfileID, p)
	}
	if len(p.HeldDependentIDs) != 1 || p.HeldDependentIDs[0] != "dependent" {
		t.Fatalf("held dependents=%v, want only the dependent outside the tree", p.HeldDependentIDs)
	}
}

func TestOfficeBlockerFailureTreeCancelCoalescesEveryMemberAlert(t *testing.T) {
	f := newEscalationFixture(t)
	f.withCEO(t)
	f.withTree(t)
	f.task(t, "second-tree-blocker", "assignment-work", v1.TaskStateInProgress, "")
	f.task(t, "second-outside-dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
	blockerMust(t, f.repo.UpdateTaskParentID(context.Background(), "second-tree-blocker", "tree-root"))
	blockerMust(t, f.repo.CreateTaskBlocker(context.Background(), &models.TaskBlocker{
		TaskID: "second-outside-dependent", BlockerTaskID: "second-tree-blocker",
	}))

	_, err := f.svc.CancelTaskTree(context.Background(), "tree-root", "user:test")
	blockerMust(t, err)
	runs := f.wantEscalations(t, 1, "tree blocker failures coalesce")[0]
	alerts := decodeEscalationAlerts(t, runs)
	seen := make(map[string]bool, len(alerts))
	for _, alert := range alerts {
		seen[alert.BlockerTaskID] = true
	}
	if !seen[escalationBlocker] || !seen["second-tree-blocker"] {
		t.Fatalf("tree cancellation alerts=%v, want both blocker members", seen)
	}
}

// @covers AC-OFFICE-BLOCKER-POLICY-002.5
func TestOfficeBlockerFailureTreeHoldWithoutFailureTransition(t *testing.T) {
	cases := map[string]func(t *testing.T, f *escalationFixture){
		"pause": func(t *testing.T, f *escalationFixture) {
			_, err := f.svc.PauseTaskTree(context.Background(), "tree-root")
			blockerMust(t, err)
		},
		"cancel_failed_blocker": func(t *testing.T, f *escalationFixture) {
			blockerMust(t, f.tasks.UpdateTaskState(context.Background(), escalationBlocker, v1.TaskStateFailed))
			_, err := f.svc.CancelTaskTree(context.Background(), "tree-root", "user:test")
			blockerMust(t, err)
		},
	}
	for name, act := range cases {
		t.Run(name, func(t *testing.T) {
			f := newEscalationFixture(t)
			f.withTree(t)
			f.withCEO(t)
			act(t, f)
			f.wantEscalations(t, 0, name)
		})
	}
}
