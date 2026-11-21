package backendapp

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/office/dashboard"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

type fakeSeatSetter struct {
	gotWorkspace string
	gotChanges   []workflowservice.SeatAuthorExclusionChange
	steps        []*workflowmodels.WorkflowStep
	err          error
}

func (f *fakeSeatSetter) SetSeatAuthorExclusion(
	_ context.Context, workspaceID string, changes []workflowservice.SeatAuthorExclusionChange,
) ([]*workflowmodels.WorkflowStep, error) {
	f.gotWorkspace, f.gotChanges = workspaceID, changes
	return f.steps, f.err
}

type fakeStepPublisher struct {
	eventType string
	steps     []*workflowmodels.WorkflowStep
}

func (f *fakeStepPublisher) PublishAll(_ context.Context, eventType string, steps []*workflowmodels.WorkflowStep) {
	f.eventType, f.steps = eventType, steps
}

func TestSeatExclusionWriterAdapterForwardsAndPublishes(t *testing.T) {
	updated := []*workflowmodels.WorkflowStep{{ID: "step-review"}}
	setter := &fakeSeatSetter{steps: updated}
	publisher := &fakeStepPublisher{}
	adapter := &seatExclusionWriterAdapter{workflow: setter, publisher: publisher}

	err := adapter.SetSeatAuthorExclusion(context.Background(), "ws-1",
		[]dashboard.SeatAuthorExclusionChange{{StepID: "step-review", Role: "reviewer", Enabled: true}})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	want := []workflowservice.SeatAuthorExclusionChange{{StepID: "step-review", Role: "reviewer", Enabled: true}}
	if setter.gotWorkspace != "ws-1" || !reflect.DeepEqual(setter.gotChanges, want) {
		t.Fatalf("forwarded %q %+v", setter.gotWorkspace, setter.gotChanges)
	}
	if publisher.eventType != events.WorkflowStepUpdated || !reflect.DeepEqual(publisher.steps, updated) {
		t.Fatalf("published %q %+v", publisher.eventType, publisher.steps)
	}
}

func TestSeatExclusionWriterAdapterMapsErrors(t *testing.T) {
	cases := []struct {
		err  error
		want error
	}{
		{fmt.Errorf("x: %w", workflowservice.ErrSeatRoleNotFound), dashboard.ErrSeatChangeInvalid},
		{fmt.Errorf("x: %w", workflowservice.ErrSeatStepNotInWorkspace), dashboard.ErrSeatChangeNotFound},
		{workflowservice.ErrNotVisible, dashboard.ErrSeatChangeNotFound},
		{workflowservice.ErrWorkflowReadOnly, dashboard.ErrSeatChangeReadOnly},
		{workflowservice.ErrWorkflowWorkspaceReadOnly, dashboard.ErrSeatChangeReadOnly},
		{fmt.Errorf("x: %w", workflowmodels.ErrWorkflowStepChanged), dashboard.ErrSeatChangeConflict},
	}
	for _, tc := range cases {
		publisher := &fakeStepPublisher{}
		adapter := &seatExclusionWriterAdapter{workflow: &fakeSeatSetter{err: tc.err}, publisher: publisher}
		err := adapter.SetSeatAuthorExclusion(context.Background(), "ws-1",
			[]dashboard.SeatAuthorExclusionChange{{StepID: "s", Role: "reviewer"}})
		if !errors.Is(err, tc.want) {
			t.Fatalf("error %v: got %v, want wrapping %v", tc.err, err, tc.want)
		}
		if publisher.steps != nil {
			t.Fatalf("published after a failed write: %+v", publisher.steps)
		}
	}
}
