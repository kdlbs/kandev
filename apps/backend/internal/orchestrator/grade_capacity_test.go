package orchestrator

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeGradedExecutionLister struct {
	rows []models.AdmittedExecution
}

func (f *fakeGradedExecutionLister) ListAdmittedSessionIDs(context.Context) ([]string, error) {
	ids := make([]string, 0, len(f.rows))
	for _, row := range f.rows {
		ids = append(ids, row.ID)
	}
	return ids, nil
}

func (f *fakeGradedExecutionLister) ListAdmittedExecutions(context.Context) ([]models.AdmittedExecution, error) {
	return append([]models.AdmittedExecution(nil), f.rows...), nil
}

func TestAllPolicyAdmissionUsesGradedPopulation(t *testing.T) {
	tests := []struct {
		name         string
		ceiling      int
		limits       map[string]int
		rows         []models.AdmittedExecution
		wantReason   string
		wantAdmitted bool
	}{
		{
			name:    "grade limit",
			ceiling: 3, limits: map[string]int{"work": 1},
			rows:       []models.AdmittedExecution{{ID: "active", Grade: "work", GradeKnown: true}},
			wantReason: ceilingReasonGradeLimit,
		},
		{
			name:    "total limit",
			ceiling: 1, limits: map[string]int{"work": 2, "control": 2},
			rows:       []models.AdmittedExecution{{ID: "active", Grade: "control", GradeKnown: true}},
			wantReason: ceilingReasonTotalLimit,
		},
		{
			name:    "both limits",
			ceiling: 1, limits: map[string]int{"work": 1},
			rows:       []models.AdmittedExecution{{ID: "active", Grade: "work", GradeKnown: true}},
			wantReason: ceilingReasonBothLimits,
		},
		{
			name:    "zero grade limit",
			ceiling: 5, limits: map[string]int{"work": 0},
			wantReason: ceilingReasonGradeLimit,
		},
		{
			name:    "unknown active grade",
			ceiling: 5, limits: map[string]int{"work": 2},
			rows:       []models.AdmittedExecution{{ID: "active", GradeKnown: false}},
			wantReason: ceilingReasonUnknownPopulation,
		},
		{
			name:    "unconfigured active grade",
			ceiling: 5, limits: map[string]int{"work": 2},
			rows:       []models.AdmittedExecution{{ID: "active", GradeKnown: true}},
			wantReason: ceilingReasonNeedsConfiguration,
		},
		{
			name:    "available total and grade capacity",
			ceiling: 2, limits: map[string]int{"work": 2},
			wantAdmitted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &fakeGradedExecutionLister{rows: tt.rows}
			controller := newSessionCeilingController(tt.ceiling, lister, zap.NewNop())
			controller.policy = sessioncapacity.PolicyAll
			controller.gradeLimits = tt.limits

			got := controller.admit(context.Background(), admissionRequest{
				taskID: "task", sessionID: "new-session", grade: "work", gradeKnown: true,
				origin: launchOriginAutomatic, seam: "grade-capacity-test",
			})
			if got.admitted != tt.wantAdmitted || got.reasonCode != tt.wantReason {
				t.Fatalf("admission = %+v, want admitted=%v reason=%q", got, tt.wantAdmitted, tt.wantReason)
			}
		})
	}
}

func TestAllPolicyAdmissionDeduplicatesReservationAliases(t *testing.T) {
	controller := newSessionCeilingController(10, &fakeGradedExecutionLister{}, zap.NewNop())
	controller.policy = sessioncapacity.PolicyAll
	controller.gradeLimits = map[string]int{"work": 4}
	controller.reservations = map[string]*ceilingReservation{
		"selected-start-a": {sessionID: "selected-session", grade: "work", gradeKnown: true},
		"selected-start-b": {sessionID: "selected-session", grade: "work", gradeKnown: true},
		"sessionless-a":    {grade: "work", gradeKnown: true},
		"sessionless-b":    {grade: "work", gradeKnown: true},
	}

	controller.mu.Lock()
	population, err := controller.readGradedPopulationLocked(context.Background(), "work")
	controller.mu.Unlock()
	if err != nil {
		t.Fatalf("readGradedPopulationLocked() error = %v", err)
	}
	if population.total != 3 || population.grade != 3 || population.grades["work"] != 3 {
		t.Fatalf("population total/grade/usage = %d/%d/%d, want 3/3/3", population.total, population.grade, population.grades["work"])
	}
	if len(population.counted) != 0 {
		t.Fatalf("persisted counted rows = %v, want none", population.counted)
	}

	decision := controller.admit(context.Background(), admissionRequest{
		taskID: "task", sessionID: "new-session", grade: "work", gradeKnown: true,
		origin: launchOriginAutomatic, seam: "grade-capacity-test",
	})
	if !decision.admitted {
		t.Fatalf("admission = %+v, want room after counting the aliased session once", decision)
	}
}
