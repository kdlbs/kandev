package orchestrator

import (
	"context"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestGradeQueueOldestEligibleKeepsOriginalOrder(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		blockedHead, all, cancelOld bool
		winner                      string
	}{
		{"same_grade_fifo", false, true, false, "z-old-B"},
		{"blocked_A_does_not_hold_B", true, true, false, "z-old-B"},
		{"legacy_automatic_keeps_id_order", false, false, false, "a-new-B"},
		{"cancelled_oldest_does_not_start", false, true, true, "a-new-B"},
	} {
		t.Run(tc.name, func(t *testing.T) { gradeQueueDrainCase(t, tc.blockedHead, tc.all, tc.cancelOld, tc.winner) })
	}
}

func gradeQueueDrainCase(t *testing.T, blockedHead, all, cancelOld bool, winner string) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	ids := []string{"z-old-B", "a-new-B"}
	if blockedHead {
		ids = append([]string{"0-blocked-A"}, ids...)
	}
	taskRepo := newMockTaskRepo()
	for _, id := range ids {
		sid := id + "-session"
		seedTaskAndSession(t, repo, id, sid, models.TaskSessionStateCreated)
		seedExecutorRunning(t, repo, sid, id, "exec-"+id)
		seedMockTaskState(taskRepo, id, v1.TaskStateInProgress)
		grade := "Queue-B"
		if id == "0-blocked-A" {
			grade = "Queue-A"
		}
		if _, err := repo.UpdateTaskFieldsWithParentAdmission(ctx, id, models.TaskFieldUpdate{ExecutionGrade: &grade}, nil); err != nil {
			t.Fatal(err)
		}
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{repoForExecutionLookup: repo})
	svc.sessionCeiling = newSessionCeilingController(1, repo, nil)
	profiles := gradeProfileStore(t)
	if err := profiles.CreateAgentProfile(ctx, &settingsmodels.AgentProfile{ID: "profile-1", AgentID: "grade-agent", Name: "Queue profile", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(profiles, nil, false))
	svc.SetExecutionCapacity(sessioncapacity.Effective{Enabled: true, MaxSessions: 1, Policy: sessioncapacity.PolicyAll, GradeLimits: map[string]int{"Queue-A": 0, "Queue-B": 0}})
	stamps := enqueueGradeQueueArrivals(t, svc, repo, ids)
	if cancelOld {
		if err := repo.UpdateTaskState(ctx, "z-old-B", v1.TaskStateCancelled); err != nil {
			t.Fatal(err)
		}
	}
	policy := sessioncapacity.PolicyAutomatic
	if all {
		policy = sessioncapacity.PolicyAll
	}
	svc.SetExecutionCapacity(sessioncapacity.Effective{Enabled: true, MaxSessions: 1, Policy: policy, GradeLimits: map[string]int{"Queue-A": 0, "Queue-B": 1}})
	svc.drainDeferredCeilingLaunches(ctx)
	for _, id := range ids {
		record := deferredLaunchOf(t, svc, id)
		pending := models.HasCeilingDeferredIntent(&models.Task{Metadata: map[string]any{models.MetaKeyDeferredLaunch: record}})
		wantPending := id != winner && (!cancelOld || id != "z-old-B")
		if pending != wantPending {
			t.Fatalf("wrong queue winner: task=%s pending=%v wantWinner=%s", id, pending, winner)
		}
		if pending && record[models.CeilingQueuedAtKey] != stamps[id] {
			t.Fatalf("retry reset original rank for %s: %v", id, record)
		}
	}
	if cancelOld {
		stopped, err := repo.GetTaskSession(ctx, "z-old-B-session")
		if err != nil {
			t.Fatal(err)
		}
		if models.IsAdmittedSessionState(stopped.State) {
			t.Fatal("cancelled queued task started its session")
		}
	}
	svc.drainDeferredCeilingLaunches(ctx)
	if record := deferredLaunchOf(t, svc, winner); models.HasCeilingDeferredIntent(&models.Task{Metadata: map[string]any{models.MetaKeyDeferredLaunch: record}}) {
		t.Fatal("duplicate drain restored a consumed intent")
	}
}

func enqueueGradeQueueArrivals(t *testing.T, svc *Service, repo *tasksqlite.Repository, ids []string) map[string]string {
	t.Helper()
	ctx := context.Background()
	stamps := map[string]string{}
	for i, id := range ids {
		execution, err := svc.StartCreatedSession(ctx, id, id+"-session", "profile-1", "original-"+id, false, false, true, nil, nil)
		if err != nil || execution != nil {
			t.Fatalf("enqueue %s: execution=%+v err=%v", id, execution, err)
		}
		record := deferredLaunchOf(t, svc, id)
		if record == nil {
			t.Fatalf("not queued: %s", id)
		}
		stamp := time.Date(2026, 10, 16, 0, 0, 0, (i+1)*100000000, time.UTC).Format(time.RFC3339Nano)
		record[models.CeilingQueuedAtKey] = stamp
		stamps[id] = stamp
		if err := repo.SetTaskMetadataKey(ctx, id, models.MetaKeyDeferredLaunch, record); err != nil {
			t.Fatal(err)
		}
	}
	return stamps
}
