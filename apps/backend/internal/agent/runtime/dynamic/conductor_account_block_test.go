package dynamic

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

type accountBlockDownstream struct {
	launches int
	resumes  int
}

func (d *accountBlockDownstream) Launch(_ context.Context, request DownstreamLaunch) (DownstreamExecution, error) {
	d.launches++
	return DownstreamExecution{ID: "execution", ExecutionProfileID: request.ExecutionProfileID}, nil
}

func (d *accountBlockDownstream) Resume(context.Context, string, string) error {
	d.resumes++
	return nil
}

func (*accountBlockDownstream) Stop(context.Context, string, string) error { return nil }

func TestConductorAccountBlockedAfterSelectionCannotLaunch(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(map[bool]string{false: "new route", true: "selected route"}[selected], func(t *testing.T) {
			ctx := context.Background()
			engine := NewEngine()
			profile := Profile{ID: "dynamic-profile", Candidates: []Candidate{{ID: "candidate", Enabled: true}}}
			downstream := &accountBlockDownstream{}
			conductor := NewConductor(engine, conductorTestProfileLoader{profile}, downstream,
				WithContinuationBuilder(func(context.Context, ContinuationInput) (Continuation, error) {
					return Continuation{}, engine.HoldAccountBlock("candidate", routingerr.CodeAccountBlocked)
				}))
			var err error
			if selected {
				decision, selectErr := engine.Select("session", profile, 0, "")
				if selectErr != nil {
					t.Fatal(selectErr)
				}
				_, err = conductor.LaunchSelected(ctx, ConductorSelectedLaunch{
					SessionID: "session", LogicalProfileID: profile.ID, Decision: decision, Prompt: "hello",
				})
			} else {
				_, err = conductor.Launch(ctx, ConductorLaunch{SessionID: "session", LogicalProfileID: profile.ID, Prompt: "hello"})
			}
			if !errors.Is(err, ErrExecutionProfileHeld) || downstream.launches != 0 {
				t.Fatalf("held launch = %v, downstream calls = %d; want refusal before launch", err, downstream.launches)
			}
			if released, releaseErr := engine.ReleaseAccountBlock("candidate"); releaseErr != nil || !released {
				t.Fatalf("release = %v, %v", released, releaseErr)
			}
			conductor.continuationBuilder = nil
			state, _ := engine.State("session")
			_, err = conductor.LaunchSelected(ctx, ConductorSelectedLaunch{
				SessionID: "session", LogicalProfileID: profile.ID,
				Decision: RouteDecision{SessionID: "session", ExecutionProfileID: "candidate", Generation: state.Generation}, Prompt: "hello",
			})
			if err != nil || downstream.launches != 1 {
				t.Fatalf("released launch = %v, downstream calls = %d; want one", err, downstream.launches)
			}
		})
	}
}

func TestConductorAccountBlockedActiveExecutionCannotResume(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine()
	profile := Profile{ID: "dynamic-profile", Candidates: []Candidate{{ID: "candidate", Enabled: true}}}
	downstream := &accountBlockDownstream{}
	conductor := NewConductor(engine, conductorTestProfileLoader{profile}, downstream)
	if _, err := conductor.Launch(ctx, ConductorLaunch{SessionID: "session", LogicalProfileID: profile.ID, Prompt: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := engine.HoldAccountBlock("candidate", routingerr.CodeAccountBlocked); err != nil {
		t.Fatal(err)
	}
	if err := conductor.Resume(ctx, "session", "next turn"); !errors.Is(err, ErrExecutionProfileHeld) || downstream.resumes != 0 {
		t.Fatalf("held resume = %v, downstream calls = %d; want refusal", err, downstream.resumes)
	}
	if released, err := engine.ReleaseAccountBlock("candidate"); err != nil || !released {
		t.Fatalf("release = %v, %v", released, err)
	}
	if err := conductor.Resume(ctx, "session", "next turn"); err != nil || downstream.resumes != 1 {
		t.Fatalf("released resume = %v, downstream calls = %d; want one", err, downstream.resumes)
	}
}
