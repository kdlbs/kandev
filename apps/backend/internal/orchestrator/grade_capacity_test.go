package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func applyGradeCapacity(t *testing.T, svc *Service, maximum, gradeMaximum int) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"enabled": true, "max_sessions": maximum, "policy": "all", "grade_limits": map[string]int{"A": gradeMaximum, "B": 1}})
	if err != nil {
		t.Fatal(err)
	}
	var effective sessioncapacity.Effective
	if err := json.Unmarshal(raw, &effective); err != nil {
		t.Fatal(err)
	}
	if target, ok := any(svc).(interface {
		SetExecutionCapacity(sessioncapacity.Effective)
	}); ok {
		target.SetExecutionCapacity(effective)
	} else {
		svc.SetSessionCapacity(maximum)
	}
}

func createGradeOwner(t *testing.T, repo *tasksqlite.Repository, id, grade string, active bool) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: id, Title: id, ExecutionGrade: grade}); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: id + "-session", TaskID: id}
	if active {
		session.State = models.TaskSessionStateRunning
		session.AdmittedExecutionGrade = grade
		session.AdmittedExecutionGradeKnown = true
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
}

func TestGradeCapacityANDMatrix(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		total, grade, active int
		wantAdmit            bool
		reason               string
	}{
		{"grade_full", 3, 0, 0, false, "ceiling_grade_limit"},
		{"total_full", 1, 3, 1, false, "ceiling_total_limit"},
		{"both_full", 1, 1, 1, false, "ceiling_both_limits"},
		{"both_free", 3, 2, 1, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo := newServiceWithCeiling(t, tc.total)
			svc.sessionCeiling.lister = repo
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
			applyGradeCapacity(t, svc, tc.total, tc.grade)
			if tc.active > 0 {
				createGradeOwner(t, repo, "active", "A", true)
			}
			createGradeOwner(t, repo, "incoming", "A", false)
			d := svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "incoming", sessionID: "incoming-session", origin: launchOriginManual})
			if d.admitted != tc.wantAdmit || d.reasonCode != tc.reason {
				t.Fatalf("decision=%+v want admit=%v reason=%q", d, tc.wantAdmit, tc.reason)
			}
		})
	}
}

func TestGradeCapacityLastSlotHasOneWinner(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 12)
	svc.sessionCeiling.lister = repo
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
	applyGradeCapacity(t, svc, 12, 1)
	const contenders = 12
	for i := range contenders {
		createGradeOwner(t, repo, fmt.Sprintf("contender-%d", i), "A", false)
	}
	start := make(chan struct{})
	results := make(chan admissionDecision, contenders)
	var wg sync.WaitGroup
	for i := range contenders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			id := fmt.Sprintf("contender-%d", i)
			results <- svc.sessionCeiling.admit(ctx, admissionRequest{taskID: id, sessionID: id + "-session", origin: launchOriginAutomatic})
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for d := range results {
		if d.admitted {
			winners++
		}
	}
	if winners != 1 || len(svc.sessionCeiling.reservations) != 1 {
		t.Fatalf("winners=%d reservations=%d", winners, len(svc.sessionCeiling.reservations))
	}
}

func TestGradeCapacityUnknownAndMissingAreDifferent(t *testing.T) {
	for _, tc := range []struct {
		name, grade, reason string
		legacy              bool
	}{
		{"missing_grade", "", "ceiling_needs_configuration", false},
		{"missing_limit", "Unconfigured", "ceiling_needs_configuration", false},
		{"legacy_active_unknown", "A", "ceiling_unknown_population", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo := newServiceWithCeiling(t, 4)
			svc.sessionCeiling.lister = repo
			svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
			applyGradeCapacity(t, svc, 4, 2)
			if tc.legacy {
				createGradeOwner(t, repo, "legacy", "", false)
				if err := repo.UpdateTaskSessionState(ctx, "legacy-session", models.TaskSessionStateRunning, ""); err != nil {
					t.Fatal(err)
				}
			}
			createGradeOwner(t, repo, "incoming", tc.grade, false)
			d := svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "incoming", sessionID: "incoming-session", origin: launchOriginManual})
			if d.admitted || d.reasonCode != tc.reason {
				t.Fatalf("decision=%+v want %s", d, tc.reason)
			}
		})
	}
}

func TestGradeCapacityBindRejectsChangedClassification(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 4)
	svc.sessionCeiling.lister = repo
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
	applyGradeCapacity(t, svc, 4, 2)
	createGradeOwner(t, repo, "binding", "A", false)
	reservation, deferred, err := svc.admitOrDeferSeam1(ctx, "binding", launchOriginAutomatic, map[string]any{})
	if err != nil || deferred {
		t.Fatalf("admission deferred=%v err=%v", deferred, err)
	}
	next := "B"
	if _, err := repo.UpdateTaskFieldsWithParentAdmission(ctx, "binding", models.TaskFieldUpdate{ExecutionGrade: &next}, nil); err != nil {
		t.Fatal(err)
	}
	if err := reservation.rebindToSession("binding-session", ctx); err == nil {
		t.Fatal("grade switched after capacity reservation")
	}
	stored, err := repo.GetTaskSession(ctx, "binding-session")
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.TaskSessionStateCreated || stored.AdmittedExecutionGradeKnown {
		t.Fatalf("changed classification started or persisted=%+v", stored)
	}
	reservation.releaseIfNotConsumed()
}

func TestGradeCapacityExistingOwnerSurvivesLimitReduction(t *testing.T) {
	ctx := context.Background()
	svc, repo := newServiceWithCeiling(t, 2)
	svc.sessionCeiling.lister = repo
	svc.SetProfileExecutionResolver(agentruntime.NewProfileExecutionResolver(gradeProfileStore(t), nil, false))
	applyGradeCapacity(t, svc, 2, 1)
	createGradeOwner(t, repo, "owner", "A", false)
	req := admissionRequest{taskID: "owner", sessionID: "owner-session", origin: launchOriginAutomatic}
	first := svc.sessionCeiling.admit(ctx, req)
	if !first.admitted {
		t.Fatalf("first=%+v", first)
	}
	applyGradeCapacity(t, svc, 1, 0)
	repeated := svc.sessionCeiling.admit(ctx, req)
	if !repeated.admitted || repeated.reservationKey != first.reservationKey || len(svc.sessionCeiling.reservations) != 1 {
		t.Fatalf("owner was replaced/count changed=%+v", repeated)
	}
	createGradeOwner(t, repo, "next", "A", false)
	next := svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "next", sessionID: "next-session", origin: launchOriginManual})
	if next.admitted || next.reasonCode != "ceiling_both_limits" {
		t.Fatalf("new owner entered after reduction=%+v", next)
	}
	svc.sessionCeiling.release(first.reservationKey)
	svc.sessionCeiling.release(first.reservationKey)
	applyGradeCapacity(t, svc, 1, 1)
	if last := svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "next", sessionID: "next-session", origin: launchOriginAutomatic}); !last.admitted {
		t.Fatalf("released prepared owner still counted=%+v", last)
	}
}
