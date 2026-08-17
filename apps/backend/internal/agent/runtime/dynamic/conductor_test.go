package dynamic

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

type conductorTestProfileLoader struct {
	profile Profile
}

func (l conductorTestProfileLoader) LoadDynamicProfile(context.Context, string) (Profile, error) {
	return l.profile, nil
}

type conductorTestDownstream struct {
	launches []DownstreamLaunch
}

func (d *conductorTestDownstream) Launch(_ context.Context, launch DownstreamLaunch) (DownstreamExecution, error) {
	d.launches = append(d.launches, launch)
	if len(d.launches) == 1 {
		return DownstreamExecution{}, &routingerr.Error{
			Code:            routingerr.CodeProviderUnavailable,
			FallbackAllowed: true,
		}
	}
	return DownstreamExecution{ID: "execution-second"}, nil
}

func (*conductorTestDownstream) Resume(context.Context, string, string) error { return nil }

func (*conductorTestDownstream) Stop(context.Context, string, string) error { return nil }

func TestConductorFallsBackAfterClassifiedLaunchFailure(t *testing.T) {
	profile := Profile{
		ID: "dynamic-profile",
		Candidates: []Candidate{
			{
				ID:         "candidate-first",
				Enabled:    true,
				BindingKey: "first",
				Rules:      map[string]Action{string(routingerr.CodeProviderUnavailable): ActionTryNext},
			},
			{ID: "candidate-second", Enabled: true, BindingKey: "second"},
		},
	}
	downstream := &conductorTestDownstream{}
	conductor := NewConductor(NewEngine(), conductorTestProfileLoader{profile: profile}, downstream)

	result, err := conductor.Launch(context.Background(), ConductorLaunch{
		SessionID:        "session-1",
		LogicalProfileID: profile.ID,
		Prompt:           "hello",
		PriorACPSession:  "acp-first",
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Decision.ExecutionProfileID != "candidate-second" || result.Decision.Generation != 2 {
		t.Fatalf("final decision = %#v", result.Decision)
	}
	if result.Execution.ID != "execution-second" {
		t.Fatalf("execution = %#v", result.Execution)
	}
	if len(downstream.launches) != 2 {
		t.Fatalf("launch count = %d, want 2", len(downstream.launches))
	}
	if got := downstream.launches[0].Decision.ExecutionProfileID; got != "candidate-first" {
		t.Fatalf("first decision profile = %q", got)
	}
	if got := downstream.launches[1].Decision.ExecutionProfileID; got != "candidate-second" {
		t.Fatalf("fallback decision profile = %q", got)
	}
	if downstream.launches[1].Decision.Generation != 2 {
		t.Fatalf("fallback generation = %d, want 2", downstream.launches[1].Decision.Generation)
	}
	if got := downstream.launches[0].PriorACPSession; got != "acp-first" {
		t.Fatalf("first prior ACP session = %q, want acp-first", got)
	}
	if got := downstream.launches[1].PriorACPSession; got != "" {
		t.Fatalf("fallback prior ACP session = %q, want empty", got)
	}
}

type restoringConductorTestDownstream struct {
	loaded  DownstreamExecution
	resumed DownstreamExecution
}

func (d *restoringConductorTestDownstream) Launch(context.Context, DownstreamLaunch) (DownstreamExecution, error) {
	return d.loaded, nil
}

func (d *restoringConductorTestDownstream) LoadExecution(context.Context, string) (DownstreamExecution, bool, error) {
	return d.loaded, true, nil
}

func (d *restoringConductorTestDownstream) Resume(_ context.Context, executionID, _ string) error {
	d.resumed.ID = executionID
	return nil
}

func (*restoringConductorTestDownstream) Stop(context.Context, string, string) error { return nil }

func TestConductorRestoresActiveExecutionBeforeResume(t *testing.T) {
	downstream := &restoringConductorTestDownstream{loaded: DownstreamExecution{ID: "execution-restored"}}
	conductor := NewConductor(NewEngine(), conductorTestProfileLoader{}, downstream)
	if err := conductor.Resume(context.Background(), "session-restored", "continue"); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if downstream.resumed.ID != "execution-restored" {
		t.Fatalf("resumed execution = %q, want execution-restored", downstream.resumed.ID)
	}
}

func TestConductorDoesNotFallbackForUnclassifiedLaunchFailure(t *testing.T) {
	profile := Profile{
		ID:         "dynamic-profile",
		Candidates: []Candidate{{ID: "candidate-first", Enabled: true}},
	}
	failure := errors.New("workspace failed")
	conductor := NewConductor(
		NewEngine(), conductorTestProfileLoader{profile: profile},
		failingConductorTestDownstream{err: failure},
	)

	_, err := conductor.Launch(context.Background(), ConductorLaunch{
		SessionID:        "session-2",
		LogicalProfileID: profile.ID,
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Launch error = %v", err)
	}
}

func TestConductorWalksEveryCandidateAndPersistsFailureContext(t *testing.T) {
	profile := Profile{
		ID: "dynamic-profile",
		Candidates: []Candidate{
			{ID: "candidate-first", Enabled: true, BindingKey: "first", Rules: map[string]Action{
				string(routingerr.CodeProviderUnavailable): ActionTryNext,
			}},
			{ID: "candidate-second", Enabled: true, BindingKey: "second", Rules: map[string]Action{
				string(routingerr.CodeProviderUnavailable): ActionTryNext,
			}},
			{ID: "candidate-third", Enabled: true, BindingKey: "third"},
		},
	}
	downstream := &multiFailureConductorTestDownstream{failures: 2}
	store := &conductorContinuationStore{}
	conductor := NewConductor(
		NewEngine(), conductorTestProfileLoader{profile: profile}, downstream,
		WithContinuationPersistence(store),
	)

	result, err := conductor.Launch(context.Background(), ConductorLaunch{
		SessionID: "session-three", LogicalProfileID: profile.ID,
		Prompt:       "continue the work",
		Continuation: ContinuationInput{TaskDescription: "ship the change"},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if result.Decision.ExecutionProfileID != "candidate-third" || result.Decision.Generation != 3 {
		t.Fatalf("final decision = %#v", result.Decision)
	}
	if len(downstream.launches) != 3 {
		t.Fatalf("launch count = %d, want 3", len(downstream.launches))
	}
	if downstream.launches[1].Continuation.FailureReason == "" || downstream.launches[2].Continuation.FailureReason == "" {
		t.Fatalf("fallback continuation did not retain failure context: %#v", downstream.launches)
	}
	if len(store.saved) != 2 || store.saved[0].Generation != 2 || store.saved[1].Generation != 3 {
		t.Fatalf("persisted continuations = %#v, want generations 2 and 3", store.saved)
	}
}

func TestConductorDoesNotFallbackAfterPostStartLaunchFailure(t *testing.T) {
	profile := Profile{
		ID: "dynamic-profile",
		Candidates: []Candidate{
			{ID: "candidate-first", Enabled: true, Rules: map[string]Action{
				string(routingerr.CodeProviderUnavailable): ActionTryNext,
			}},
			{ID: "candidate-second", Enabled: true},
		},
	}
	failure := &routingerr.Error{Code: routingerr.CodeProviderUnavailable, Phase: routingerr.PhaseStreaming, FallbackAllowed: true}
	conductor := NewConductor(NewEngine(), conductorTestProfileLoader{profile: profile}, failingConductorTestDownstream{err: failure})
	_, err := conductor.Launch(context.Background(), ConductorLaunch{SessionID: "session-post-start", LogicalProfileID: profile.ID})
	if !errors.Is(err, failure) {
		t.Fatalf("Launch error = %v, want %v", err, failure)
	}
}

type multiFailureConductorTestDownstream struct {
	launches []DownstreamLaunch
	failures int
}

func (d *multiFailureConductorTestDownstream) Launch(_ context.Context, launch DownstreamLaunch) (DownstreamExecution, error) {
	d.launches = append(d.launches, launch)
	if len(d.launches) <= d.failures {
		return DownstreamExecution{}, &routingerr.Error{Code: routingerr.CodeProviderUnavailable, Phase: routingerr.PhaseProcessStart, FallbackAllowed: true}
	}
	return DownstreamExecution{ID: "execution-third"}, nil
}

func (*multiFailureConductorTestDownstream) Resume(context.Context, string, string) error { return nil }
func (*multiFailureConductorTestDownstream) Stop(context.Context, string, string) error   { return nil }

type conductorContinuationStore struct{ saved []ContinuationRecord }

func (s *conductorContinuationStore) SaveRouteContinuation(_ context.Context, record ContinuationRecord) error {
	s.saved = append(s.saved, record)
	return nil
}

type failingConductorTestDownstream struct{ err error }

func (d failingConductorTestDownstream) Launch(context.Context, DownstreamLaunch) (DownstreamExecution, error) {
	return DownstreamExecution{}, d.err
}

func (failingConductorTestDownstream) Resume(context.Context, string, string) error { return nil }

func (failingConductorTestDownstream) Stop(context.Context, string, string) error { return nil }
