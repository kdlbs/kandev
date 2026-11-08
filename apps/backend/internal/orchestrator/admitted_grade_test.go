package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/agents"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/task/models"
)

func gradeProfileStore(t *testing.T) settingsstore.Repository {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	repo, cleanup, err := settingsstore.Provide(conn, conn, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &settingsmodels.Agent{ID: "grade-agent", Name: "grade-agent"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAgent(ctx, &settingsmodels.Agent{ID: "dynamic-grade-agent", Name: agents.DynamicAgentID}); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []*settingsmodels.AgentProfile{
		{ID: "unconfigured-grade", AgentID: "grade-agent", Name: "Unconfigured", Enabled: true},
		{ID: "dynamic-grade", AgentID: "dynamic-grade-agent", Name: "Dynamic", Enabled: true, ExecutionGrade: "Logical-Dynamic"},
		{ID: "logical-grade", AgentID: "grade-agent", Name: "Logical", Enabled: true, ExecutionGrade: "Profile-A"},
		{ID: "office-grade", AgentID: "grade-agent", Name: "Office", Enabled: true, ExecutionAgentProfileID: "logical-grade", ExecutionGrade: "Identity-Wrong"},
	} {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

func storedAdmissionGrade(t *testing.T, session *models.TaskSession) string {
	t.Helper()
	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	grade, _ := fields["admitted_execution_grade"].(string)
	return grade
}

func TestAdmittedGradeActualSessionSelection(t *testing.T) {
	for _, tc := range []struct{ name, taskGrade, profile, want string }{
		{"task_override", "Task-A", "logical-grade", "Task-A"},
		{"logical_profile", "", "logical-grade", "Profile-A"},
		{"office_bound_profile", "", "office-grade", "Profile-A"},
		{"dynamic_without_candidate_selection", "", "dynamic-grade", "Logical-Dynamic"},
		{"observed_unconfigured", "", "unconfigured-grade", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo := newServiceWithCeiling(t, 5)
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
			task := &models.Task{ID: "actual-grade-task", Title: "Task", ExecutionGrade: tc.taskGrade}
			if err := repo.CreateTask(ctx, task); err != nil {
				t.Fatal(err)
			}
			session := &models.TaskSession{ID: "actual-grade-session", TaskID: task.ID, AgentProfileID: tc.profile, ExecutionProfileID: "concrete-candidate-wrong"}
			if err := repo.CreateTaskSession(ctx, session); err != nil {
				t.Fatal(err)
			}
			decision := svc.sessionCeiling.admit(ctx, admissionRequest{taskID: task.ID, sessionID: session.ID, origin: launchOriginAutomatic, seam: "launchPreparedSession"})
			if !decision.admitted {
				t.Fatalf("admission refused: %+v", decision)
			}
			stored, err := repo.GetTaskSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if grade := storedAdmissionGrade(t, stored); grade != tc.want || !stored.AdmittedExecutionGradeKnown {
				t.Fatalf("actual admitted grade=%q want %q", grade, tc.want)
			}
		})
	}
}

func TestAdmittedGradeSeam1BindsActualPreparedSession(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 5)
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
	task := &models.Task{ID: "bind-grade-task", Title: "Task"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	reservation, deferred, err := svc.admitOrDeferSeam1(ctx, task.ID, launchOriginAutomatic, map[string]any{})
	if err != nil || deferred {
		t.Fatalf("admission deferred=%v err=%v", deferred, err)
	}
	session := &models.TaskSession{ID: "bind-grade-session", TaskID: task.ID, AgentProfileID: "office-grade"}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	if err := reservation.rebindToSession(session.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if grade := storedAdmissionGrade(t, stored); grade != "Profile-A" {
		t.Fatalf("prepared binding grade=%q", grade)
	}
	reservation.releaseIfNotConsumed()
}

func TestAdmittedGradeActiveSnapshotSurvivesSettingsChange(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 5)
	profiles := gradeProfileStore(t)
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(profiles, nil, false))
	task := &models.Task{ID: "active-grade-task", Title: "Task"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "active-grade-session", TaskID: task.ID, AgentProfileID: "logical-grade"}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	req := admissionRequest{taskID: task.ID, sessionID: session.ID, origin: launchOriginAutomatic, seam: "launchPreparedSession"}
	if d := svc.sessionCeiling.admit(ctx, req); !d.admitted {
		t.Fatalf("admission=%+v", d)
	}
	if err := repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	svc.sessionCeiling.release(session.ID)
	task.ExecutionGrade = "Later-Task"
	if _, err := repo.UpdateTaskFieldsWithParentAdmission(ctx, task.ID, models.TaskFieldUpdate{ExecutionGrade: &task.ExecutionGrade}, nil); err != nil {
		t.Fatal(err)
	}
	profile, err := profiles.GetAgentProfile(ctx, "logical-grade")
	if err != nil {
		t.Fatal(err)
	}
	profile.ExecutionGrade = "Later-Profile"
	if err := profiles.UpdateAgentProfile(settingsstore.WithExecutionGradeUpdate(ctx, &profile.ExecutionGrade), profile); err != nil {
		t.Fatal(err)
	}
	currentTask, err := repo.GetTask(ctx, task.ID)
	if err != nil || currentTask.ExecutionGrade != "Later-Task" {
		t.Fatalf("task source not changed: task=%+v err=%v", currentTask, err)
	}
	currentProfile, err := profiles.GetAgentProfile(ctx, profile.ID)
	if err != nil || currentProfile.ExecutionGrade != "Later-Profile" {
		t.Fatalf("profile source not changed: profile=%+v err=%v", currentProfile, err)
	}
	if d := svc.sessionCeiling.admit(ctx, req); !d.admitted {
		t.Fatalf("active admission=%+v", d)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.AdmittedExecutionGradeKnown || stored.AdmittedExecutionGrade != "Profile-A" {
		t.Fatalf("active grade=%+v", stored)
	}
}

type admissionGradeRaceRepo struct {
	sessionExecutorStore
	writeGrade func(context.Context, string, string) error
}

func (r *admissionGradeRaceRepo) SetAdmittedExecutionGrade(ctx context.Context, id, grade string) error {
	return r.writeGrade(ctx, id, grade)
}

func TestAdmittedGradePersistenceRaceKeepsActiveUnknown(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 5)
	task := &models.Task{ID: "grade-write-race-task", Title: "Task", ExecutionGrade: "Task-A"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "grade-write-race-session", TaskID: task.ID}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	svc.repo = &admissionGradeRaceRepo{sessionExecutorStore: svc.repo, writeGrade: func(ctx context.Context, id, grade string) error {
		if err := repo.UpdateTaskSessionState(ctx, id, models.TaskSessionStateRunning, ""); err != nil {
			return err
		}
		return repo.SetAdmittedExecutionGrade(ctx, id, grade)
	}}
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
	decision := svc.sessionCeiling.admit(ctx, admissionRequest{taskID: task.ID, sessionID: session.ID, origin: launchOriginAutomatic})
	if decision.admitted || len(svc.sessionCeiling.reservations) != 0 {
		t.Fatalf("failed grade write retained admission=%+v", decision)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.TaskSessionStateRunning || stored.AdmittedExecutionGradeKnown {
		t.Fatalf("concurrent active snapshot overwritten=%+v", stored)
	}
}

func TestAdmittedGradeBindWriteFailureRetainsOwnerUntilCleanup(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 5)
	if err := repo.CreateTask(ctx, &models.Task{ID: "grade-bind-failure-task", Title: "Task", ExecutionGrade: "Task-A"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "grade-bind-failure-session", TaskID: "grade-bind-failure-task"}); err != nil {
		t.Fatal(err)
	}
	svc.repo = &admissionGradeRaceRepo{sessionExecutorStore: svc.repo, writeGrade: func(context.Context, string, string) error { return errors.New("snapshot write unavailable") }}
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
	reservation, deferred, err := svc.admitOrDeferSeam1(ctx, "grade-bind-failure-task", launchOriginAutomatic, map[string]any{})
	if err != nil || deferred {
		t.Fatalf("initial admission deferred=%v err=%v", deferred, err)
	}
	key := reservation.key
	if err := reservation.rebindToSession("grade-bind-failure-session", ctx); err == nil {
		t.Fatal("bind swallowed snapshot write failure")
	}
	if reservation.key != key || svc.sessionCeiling.reservations[key] == nil {
		t.Fatal("failed bind lost original reservation owner")
	}
	reservation.releaseIfNotConsumed()
	if len(svc.sessionCeiling.reservations) != 0 {
		t.Fatal("failed bind cleanup leaked reservation")
	}
}
