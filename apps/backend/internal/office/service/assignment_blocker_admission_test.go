package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/scheduler"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/runs/dedupkeys"
	runmodels "github.com/kandev/kandev/internal/runs/models"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	wfrepo "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type assignmentBlockerFixture struct {
	tasks     *tasksqlite.Repository
	workflows *wfrepo.Repository
	repo      *officesqlite.Repository
	svc       *service.Service
	scheduler *scheduler.SchedulerService
	eb        *bus.MemoryEventBus
	runs      *runsservice.Service
}

func blockerMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newAssignmentBlockerFixture(t *testing.T) *assignmentBlockerFixture {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:?_foreign_keys=on")
	blockerMust(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { blockerMust(t, db.Close()) })
	var foreignKeys int
	blockerMust(t, db.Get(&foreignKeys, "PRAGMA foreign_keys"))
	if foreignKeys != 1 {
		t.Fatal("foreign key enforcement is disabled")
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	blockerMust(t, err)
	settings, _, err := settingsstore.Provide(db, db, log)
	blockerMust(t, err)
	tasks, err := tasksqlite.NewWithDB(db, db, log)
	blockerMust(t, err)
	workflows, err := wfrepo.NewWithDB(db, db, log)
	blockerMust(t, err)
	repo, err := officesqlite.NewWithDB(db, db, log)
	blockerMust(t, err)
	blockerMust(t, tasks.CreateWorkspace(context.Background(), &taskmodels.Workspace{
		ID: "assignment-ws", Name: "Assignment blockers", OfficeWorkflowID: "assignment-workflow",
	}))
	blockerMust(t, settings.CreateAgent(context.Background(), &settingsmodels.Agent{ID: "assignment-cli", Name: "Assignment CLI"}))
	for _, id := range []string{"assignment-worker", "assignment-old"} {
		blockerMust(t, repo.CreateAgentInstance(context.Background(), &models.AgentInstance{
			ID: id, AgentID: "assignment-cli", WorkspaceID: "assignment-ws", Name: id, Status: models.AgentStatusIdle,
		}))
	}
	blockerMust(t, tasks.CreateWorkflow(context.Background(), &taskmodels.Workflow{ID: "assignment-workflow", WorkspaceID: "assignment-ws", Name: "Office"}))
	for position, id := range []string{"assignment-work", "assignment-backlog"} {
		step := &wfmodels.WorkflowStep{ID: id, WorkflowID: "assignment-workflow", Name: id, Position: position}
		if position == 0 {
			step.Events.OnEnter = []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}
		}
		blockerMust(t, workflows.CreateStep(context.Background(), step))
	}
	runs := runsservice.New(repo.RunsRepository(), nil, log, nil)
	svc := service.NewService(service.ServiceOptions{Repo: repo, Logger: log, CfgLoader: configloader.NewConfigLoader(t.TempDir())})
	svc.SetRunsService(runs)
	svc.SetWorkflowStepGetter(workflows)
	svc.SetSyncHandlers(true)
	eb := bus.NewMemoryEventBus(log)
	t.Cleanup(eb.Close)
	blockerMust(t, svc.RegisterEventSubscribers(eb))
	ss := scheduler.NewSchedulerService(repo, log, svc)
	ss.SetRunsService(runs)
	ss.SetWorkflowStepGetter(workflows)
	return &assignmentBlockerFixture{tasks: tasks, workflows: workflows, repo: repo, svc: svc, scheduler: ss, eb: eb, runs: runs}
}

func (f *assignmentBlockerFixture) task(t *testing.T, id, step string, state v1.TaskState, agent string) *taskmodels.Task {
	t.Helper()
	task := &taskmodels.Task{ID: id, WorkspaceID: "assignment-ws", WorkflowID: "assignment-workflow", WorkflowStepID: step,
		Title: id, State: state, AssigneeAgentProfileID: agent, Priority: "medium"}
	blockerMust(t, f.tasks.CreateTask(context.Background(), task))
	return task
}

func (f *assignmentBlockerFixture) blockers(t *testing.T, taskID string, states ...v1.TaskState) {
	t.Helper()
	for i, state := range states {
		id := fmt.Sprintf("%s-prerequisite-%d", taskID, i)
		f.task(t, id, "assignment-work", state, "")
		blockerMust(t, f.repo.CreateTaskBlocker(context.Background(), &models.TaskBlocker{TaskID: taskID, BlockerTaskID: id}))
	}
}

func (f *assignmentBlockerFixture) assignment(t *testing.T, subject, taskID, agent string) {
	t.Helper()
	generation := f.generation(t, taskID)
	blockerMust(t, f.eb.Publish(context.Background(), subject, bus.NewEvent(subject, "assignment-test", service.TaskUpdatedData{
		TaskID: taskID, WorkspaceID: "assignment-ws", AssigneeAgentProfileID: agent, AssignmentGeneration: &generation,
	})))
}

func (f *assignmentBlockerFixture) taskRuns(t *testing.T, taskID, reason string) []*runmodels.Run {
	t.Helper()
	runs, err := f.svc.ListRuns(context.Background(), "assignment-ws")
	blockerMust(t, err)
	var found []*runmodels.Run
	for _, run := range runs {
		var payload map[string]any
		blockerMust(t, json.Unmarshal([]byte(run.Payload), &payload))
		if payload["task_id"] == taskID && run.Reason == reason {
			if run.AgentProfileID != "assignment-worker" {
				t.Fatalf("run assigned to %q", run.AgentProfileID)
			}
			found = append(found, run)
		}
	}
	return found
}

// @covers AC-OFFICE-SCHEDULER-002.1 AC-OFFICE-SCHEDULER-002.2 AC-OFFICE-SCHEDULER-002.5 AC-OFFICE-SCHEDULER-002.6
func TestOfficeAssignmentBlockerAdmission(t *testing.T) {
	cases := []struct {
		name, step string
		states     []v1.TaskState
		want       int
	}{
		{"pending", "assignment-work", []v1.TaskState{v1.TaskStateInProgress}, 0},
		{"mixed", "assignment-work", []v1.TaskState{v1.TaskStateCompleted, v1.TaskStateInProgress}, 0},
		{"no_blocker", "assignment-work", nil, 1},
		{"completed", "assignment-work", []v1.TaskState{v1.TaskStateCompleted}, 1},
		{"backlog", "assignment-backlog", nil, 0},
		{"failed", "assignment-work", []v1.TaskState{v1.TaskStateFailed}, 1},
		{"cancelled", "assignment-work", []v1.TaskState{v1.TaskStateCancelled}, 1},
		{"archived_pending", "assignment-work", []v1.TaskState{v1.TaskStateInProgress}, 0},
		{"archived_completed", "assignment-work", []v1.TaskState{v1.TaskStateCompleted}, 1},
		{"missing_blocker", "assignment-work", nil, 1},
	}
	for _, subject := range []string{events.TaskCreated, events.TaskUpdated} {
		for _, fallback := range []bool{false, true} {
			if fallback && subject != events.TaskCreated {
				continue
			}
			for _, tc := range cases {
				t.Run(fmt.Sprintf("%s/fallback_%t/%s", subject, fallback, tc.name), func(t *testing.T) {
					f := newAssignmentBlockerFixture(t)
					f.task(t, "dependent", tc.step, v1.TaskStateTODO, "assignment-worker")
					f.blockers(t, "dependent", tc.states...)
					if tc.name == "archived_pending" || tc.name == "archived_completed" {
						blockerMust(t, f.tasks.ArchiveTask(context.Background(), "dependent-prerequisite-0"))
					}
					if tc.name == "missing_blocker" {
						blockerMust(t, f.repo.CreateTaskBlocker(context.Background(), &models.TaskBlocker{TaskID: "dependent", BlockerTaskID: "missing"}))
					}
					generation := f.generation(t, "dependent")
					agent := "assignment-worker"
					if fallback {
						agent = ""
					}
					f.assignment(t, subject, "dependent", agent)
					runs := f.taskRuns(t, "dependent", service.RunReasonTaskAssigned)
					if len(runs) != tc.want {
						t.Errorf("persisted assignment runs=%d, want %d", len(runs), tc.want)
					}
					stored, err := f.tasks.GetTask(context.Background(), "dependent")
					blockerMust(t, err)
					if stored.AssigneeAgentProfileID != "assignment-worker" || f.generation(t, "dependent") != generation {
						t.Fatalf("runner/generation changed: %+v", stored)
					}
					if len(runs) == 1 && !fallback {
						want := dedupkeys.AssignmentKey("dependent", "assignment-worker", generation)
						if runs[0].IdempotencyKey == nil || *runs[0].IdempotencyKey != want {
							t.Fatalf("assignment key=%v, want %s", runs[0].IdempotencyKey, want)
						}
					}
				})
			}
		}
	}
}

// @covers AC-OFFICE-SCHEDULER-002.5 AC-OFFICE-SCHEDULER-002.6
func TestOfficeAssignmentBlockerMutation(t *testing.T) {
	for _, blocked := range []bool{true, false} {
		for _, same := range []bool{false, true} {
			t.Run(fmt.Sprintf("blocked_%t/same_%t", blocked, same), func(t *testing.T) {
				f := newAssignmentBlockerFixture(t)
				previous := "assignment-old"
				if same {
					previous = "assignment-worker"
				}
				f.task(t, "dependent", "assignment-work", v1.TaskStateTODO, previous)
				if blocked {
					f.blockers(t, "dependent", v1.TaskStateInProgress)
				}
				generation, err := f.repo.UpdateTaskAssignee(context.Background(), "dependent", "assignment-worker")
				blockerMust(t, err)
				next := "assignment-worker"
				result, err := f.scheduler.ApplyTaskMutation(context.Background(), &scheduler.TaskSnapshot{
					ID: "dependent", WorkspaceID: "assignment-ws", State: "TODO", AssigneeAgentProfileID: previous,
				}, scheduler.TaskMutation{NewAssigneeID: &next, AssignmentGeneration: &generation, ActorID: "operator", ActorType: "user"})
				blockerMust(t, err)
				if (result.InterruptSessionID != "") == same {
					t.Fatalf("interrupt=%q, same=%t", result.InterruptSessionID, same)
				}
				runs := f.taskRuns(t, "dependent", service.RunReasonTaskAssigned)
				want := 1
				if blocked {
					want = 0
				}
				if len(runs) != want {
					t.Errorf("persisted mutation assignment runs=%d, want %d", len(runs), want)
				}
				stored, err := f.tasks.GetTask(context.Background(), "dependent")
				blockerMust(t, err)
				if stored.AssigneeAgentProfileID != next || f.generation(t, "dependent") != generation {
					t.Fatalf("assignment write changed: %+v", stored)
				}
				if len(runs) == 1 {
					key := dedupkeys.AssignmentKey("dependent", next, generation)
					if runs[0].IdempotencyKey == nil || *runs[0].IdempotencyKey != key {
						t.Fatalf("key=%v, want %s", runs[0].IdempotencyKey, key)
					}
					var payload map[string]any
					blockerMust(t, json.Unmarshal([]byte(runs[0].Payload), &payload))
					if payload["actor_id"] != "operator" || payload["actor_type"] != "user" {
						t.Fatalf("actor lost: %v", payload)
					}
				}
			})
		}
	}
}

func (f *assignmentBlockerFixture) generation(t *testing.T, taskID string) int64 {
	t.Helper()
	var generation int64
	blockerMust(t, f.repo.ReaderDB().Get(&generation, `SELECT assignment_generation FROM tasks WHERE id = ?`, taskID))
	return generation
}
