package coordinator

import (
	"context"
	"errors"
	"testing"

	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakeStepReader is a test double for WorkflowStepReader. Most tests just
// set steps/err (returned on every call). stepsSeq additionally supports
// approve tests that need the graph to change between the validate-time and
// create-time checkpoints: when non-empty, successive calls consume it in
// order, pinned to the last entry once exhausted. err still takes priority
// over both, matching TestLoadStepGraph_PropagatesReadError.
type fakeStepReader struct {
	steps    []*workflowmodels.WorkflowStep
	err      error
	stepsSeq [][]*workflowmodels.WorkflowStep
	calls    int
}

func (f *fakeStepReader) ListStepsByWorkflow(context.Context, string) ([]*workflowmodels.WorkflowStep, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.stepsSeq) == 0 {
		return f.steps, nil
	}
	idx := f.calls
	if idx >= len(f.stepsSeq) {
		idx = len(f.stepsSeq) - 1
	}
	f.calls++
	return f.stepsSeq[idx], nil
}

func TestLoadStepGraph_MapsStepFields(t *testing.T) {
	reader := &fakeStepReader{steps: []*workflowmodels.WorkflowStep{
		{
			ID:              "start",
			IsStartStep:     true,
			AllowManualMove: false,
			PullFromStepID:  "",
		},
		{
			ID:              "manual",
			IsStartStep:     false,
			AllowManualMove: true,
			PullFromStepID:  "start",
		},
		{
			ID:             "auto",
			PullFromStepID: "manual",
			Events: workflowmodels.StepEvents{
				OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}},
			},
		},
	}}

	got, err := LoadStepGraph(context.Background(), reader, "wf-1")
	if err != nil {
		t.Fatalf("LoadStepGraph: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}

	byID := make(map[string]StepNode, len(got))
	for _, n := range got {
		byID[n.ID] = n
	}

	start := byID["start"]
	if !start.IsStart || start.AllowManualMove || start.AutoStartOnEnter || start.PullFromStepID != "" {
		t.Fatalf("start node = %+v, want IsStart=true and everything else zero", start)
	}

	manual := byID["manual"]
	if manual.IsStart || !manual.AllowManualMove || manual.AutoStartOnEnter || manual.PullFromStepID != "start" {
		t.Fatalf("manual node = %+v, want AllowManualMove=true, PullFromStepID=start", manual)
	}

	auto := byID["auto"]
	if !auto.AutoStartOnEnter || auto.PullFromStepID != "manual" {
		t.Fatalf("auto node = %+v, want AutoStartOnEnter=true, PullFromStepID=manual", auto)
	}
}

func TestLoadStepGraph_EmptyForWorkflowWithNoSteps(t *testing.T) {
	reader := &fakeStepReader{steps: nil}
	got, err := LoadStepGraph(context.Background(), reader, "wf-empty")
	if err != nil {
		t.Fatalf("LoadStepGraph: %v", err)
	}
	if got == nil {
		t.Fatal("LoadStepGraph returned nil, want an empty non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

func TestLoadStepGraph_PropagatesReadError(t *testing.T) {
	wantErr := errors.New("boom")
	reader := &fakeStepReader{err: wantErr}
	if _, err := LoadStepGraph(context.Background(), reader, "wf-1"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
