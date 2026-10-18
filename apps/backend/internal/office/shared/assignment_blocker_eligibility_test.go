package shared

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type assignmentBlockerLookup struct {
	stepID  string
	stepErr error
	listErr error
	ids     []string
}

func (l assignmentBlockerLookup) GetTaskWorkflowStepID(context.Context, string) (string, error) {
	return l.stepID, l.stepErr
}
func (l assignmentBlockerLookup) ListTaskBlockers(context.Context, string) ([]*models.TaskBlocker, error) {
	var out []*models.TaskBlocker
	for _, id := range l.ids {
		out = append(out, &models.TaskBlocker{BlockerTaskID: id})
	}
	return out, l.listErr
}
func (l assignmentBlockerLookup) IsTaskInTerminalStep(_ context.Context, id string) (bool, error) {
	if id == "unknown" {
		return false, errors.New("blocker unavailable")
	}
	return id == "terminal", nil
}

type assignmentBlockerSteps struct {
	step *wfmodels.WorkflowStep
	err  error
}

func (s assignmentBlockerSteps) GetStep(context.Context, string) (*wfmodels.WorkflowStep, error) {
	return s.step, s.err
}

// @covers AC-OFFICE-SCHEDULER-002.3 AC-OFFICE-SCHEDULER-002.4 AC-OFFICE-SCHEDULER-002.5 AC-OFFICE-SCHEDULER-002.8
func TestAssignmentBlockerEligibility(t *testing.T) {
	work := &wfmodels.WorkflowStep{Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}}}
	failure := errors.New("lookup unavailable")
	cases := []struct {
		name   string
		lookup assignmentBlockerLookup
		steps  AssignmentStepGetter
		want   bool
		warn   string
	}{
		{name: "no blockers", lookup: assignmentBlockerLookup{stepID: "work"}, steps: assignmentBlockerSteps{step: work}, want: true},
		{name: "terminal blockers", lookup: assignmentBlockerLookup{stepID: "work", ids: []string{"terminal", "terminal"}}, steps: assignmentBlockerSteps{step: work}, want: true},
		{name: "pending", lookup: assignmentBlockerLookup{stepID: "work", ids: []string{"pending"}}, steps: assignmentBlockerSteps{step: work}},
		{name: "mixed", lookup: assignmentBlockerLookup{stepID: "work", ids: []string{"terminal", "pending"}}, steps: assignmentBlockerSteps{step: work}},
		{name: "unbound pending", lookup: assignmentBlockerLookup{ids: []string{"pending"}}},
		{name: "step lookup error pending", lookup: assignmentBlockerLookup{stepErr: failure, ids: []string{"pending"}}, warn: "office.assignment_wake.step_lookup_failed"},
		{name: "nil getter pending", lookup: assignmentBlockerLookup{stepID: "work", ids: []string{"pending"}}, warn: "office.assignment_wake.no_step_getter"},
		{name: "getter error pending", lookup: assignmentBlockerLookup{stepID: "work", ids: []string{"pending"}}, steps: assignmentBlockerSteps{err: failure}, warn: "office.assignment_wake.step_getter_failed"},
		{name: "missing step pending", lookup: assignmentBlockerLookup{stepID: "work", ids: []string{"pending"}}, steps: assignmentBlockerSteps{}, warn: "office.assignment_wake.step_not_found"},
		{name: "step lookup error unblocked", lookup: assignmentBlockerLookup{stepErr: failure}, want: true, warn: "office.assignment_wake.step_lookup_failed"},
		{name: "blocker list error", lookup: assignmentBlockerLookup{listErr: failure}, want: true, warn: "office.assignment_wake.blockers_lookup_failed"},
		{name: "blocker state error", lookup: assignmentBlockerLookup{ids: []string{"unknown"}}, want: true, warn: "office.assignment_wake.blocker_lookup_failed"},
		{name: "unknown before pending", lookup: assignmentBlockerLookup{ids: []string{"unknown", "pending"}}, warn: "office.assignment_wake.blocker_lookup_failed"},
		{name: "pending before unknown", lookup: assignmentBlockerLookup{ids: []string{"pending", "unknown"}}},
		{name: "backlog despite list error", lookup: assignmentBlockerLookup{stepID: "backlog", listErr: failure}, steps: assignmentBlockerSteps{step: &wfmodels.WorkflowStep{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.InfoLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatal(err)
			}
			got := IsAssignmentWakeEligible(context.Background(), log, tc.lookup, tc.steps, "dependent", "policy-test")
			if got != tc.want {
				t.Fatalf("eligible=%t, want %t", got, tc.want)
			}
			if tc.warn != "" {
				entries := logs.FilterMessage(tc.warn).All()
				if len(entries) != 1 || entries[0].Level != zapcore.WarnLevel {
					t.Fatalf("missing Warn %q: %+v", tc.warn, logs.All())
				}
			}
		})
	}
}
