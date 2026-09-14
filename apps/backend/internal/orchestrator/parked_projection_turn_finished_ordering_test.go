package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeOrderingTurnService is a minimal TurnService whose CompleteTurn
// publishes events.TurnCompleted on the same bus the test observes,
// mirroring task/service.Service.publishTurnEvent's wire contract exactly
// (subject == events.TurnCompleted). backendapp/gateway.go subscribes to that
// subject and relays it into the session.turn_finished notification (AC-76);
// reproducing that hop too would need the full notification-service wiring
// the F7 disposition already ruled out for a runtime test (see
// TestParkedProjectionOrdering_ProbeCallSitesFollowTurnCompletion's source-
// text approach), but the ordering guarantee AC-76 actually depends on — the
// turn-completion signal fires exactly once and before the parked probe,
// regardless of outcome — is observable at this seam.
type fakeOrderingTurnService struct {
	bus       *recordingEventBus
	turnID    string
	completed bool
}

func (f *fakeOrderingTurnService) StartTurn(context.Context, string) (*models.Turn, error) {
	return nil, nil
}

func (f *fakeOrderingTurnService) ReserveTurn(
	context.Context,
	string,
	*models.PromptDispatchRecovery,
) (*models.Turn, error) {
	panic("fakeOrderingTurnService: ReserveTurn should not be called by ordering tests")
}

func (f *fakeOrderingTurnService) PublishReservedTurn(context.Context, *models.Turn) error {
	panic("fakeOrderingTurnService: PublishReservedTurn should not be called by ordering tests")
}

func (f *fakeOrderingTurnService) RollbackReservedTurn(context.Context, string, string) (bool, error) {
	panic("fakeOrderingTurnService: RollbackReservedTurn should not be called by ordering tests")
}

func (f *fakeOrderingTurnService) ReconcileUnpublishedPromptTurns(context.Context) (int, error) {
	return 0, nil
}

func (f *fakeOrderingTurnService) PatchTurnMetadata(
	context.Context,
	string,
	string,
	map[string]interface{},
) error {
	panic("fakeOrderingTurnService: PatchTurnMetadata should not be called by ordering tests")
}

func (f *fakeOrderingTurnService) CompleteTurn(ctx context.Context, turnID string) error {
	f.completed = true
	_ = f.bus.Publish(ctx, events.TurnCompleted, bus.NewEvent(events.TurnCompleted, "task-service", map[string]interface{}{
		"id": turnID,
	}))
	return nil
}

func (f *fakeOrderingTurnService) GetTurn(context.Context, string) (*models.Turn, error) {
	return nil, nil
}

// GetActiveTurn mirrors a repo-backed turn service: the turn stays active
// across any number of lookups until CompleteTurn actually closes it. An
// earlier revision cleared it on the first read instead, which happened to
// match handleCompleteStreamEvent's old single-lookup call pattern but broke
// once currentTurnIDForSession added an earlier GetActiveTurn probe ahead of
// the completion call — the probe silently "consumed" the turn before
// completion could see it.
func (f *fakeOrderingTurnService) GetActiveTurn(_ context.Context, sessionID string) (*models.Turn, error) {
	if f.completed {
		return nil, nil
	}
	return &models.Turn{ID: f.turnID, TaskSessionID: sessionID}, nil
}

func (f *fakeOrderingTurnService) UpdateTurn(context.Context, *models.Turn) error { return nil }

func (f *fakeOrderingTurnService) AbandonOpenTurns(context.Context, string) error { return nil }

func (f *fakeOrderingTurnService) MarkReservedTurnDispatchAttempted(context.Context, *models.Turn) error {
	return nil
}

// turnCompletedIndex returns the index of the first events.TurnCompleted
// publication on bus, and how many were published in total.
func turnCompletedIndex(recorder *recordingEventBus) (idx int, count int) {
	idx = -1
	for i, e := range recorder.events {
		if e.subject == events.TurnCompleted {
			count++
			if idx == -1 {
				idx = i
			}
		}
	}
	return idx, count
}

// firstParkedEventIndex returns the index of the first published event that
// carries a parked_on_background_work key, or -1 if none was published.
func firstParkedEventIndex(recorder *recordingEventBus) int {
	for i, e := range recorder.events {
		data, ok := e.event.Data.(map[string]interface{})
		if !ok {
			continue
		}
		if _, ok := data["parked_on_background_work"]; ok {
			return i
		}
	}
	return -1
}

func runTurnFinishedOrderingCase(
	t *testing.T, attested bool, probeResults []executor.ProbeResult,
) (*recordingEventBus, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t-ordering", "s-ordering", "step1")

	// handleCompleteStreamEvent defers the running->waiting transition to a
	// later READY event when the session is still RUNNING at complete time
	// (avoids racing READY); seed STARTING so this test exercises the
	// positive path that owns the transition itself, same as the office/
	// cancellation call sites already covered elsewhere in this file.
	session, err := repo.GetTaskSession(ctx, "s-ordering")
	if err != nil {
		t.Fatalf("load seeded session: %v", err)
	}
	session.State = models.TaskSessionStateStarting
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("seed STARTING state: %v", err)
	}

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t-ordering", v1.TaskStateInProgress)
	svc := createTestService(repo, newMockStepGetter(), taskRepo)
	recorder := &recordingEventBus{}
	svc.eventBus = recorder
	svc.SetTurnService(&fakeOrderingTurnService{bus: recorder, turnID: "turn-1"})
	svc.SetBackgroundProbe(&spyBackgroundProbe{results: probeResults})
	if attested {
		svc.setObservedDetachedLaunch("s-ordering")
	}

	svc.handleCompleteStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "t-ordering", SessionID: "s-ordering",
		Data: &lifecycle.AgentStreamEventData{Type: agentEventComplete},
	})

	updated, err := repo.GetTaskSession(ctx, "s-ordering")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	return recorder, updated
}

// AC-76: whatever this spec does to the parked projection, the turn-
// completion signal (events.TurnCompleted, which backendapp/gateway.go
// relays into session.turn_finished) is delivered exactly once and strictly
// before any parked-projection event — across every outcome this spec can
// produce: parked, un-parked (settled probe), an indeterminate probe, and an
// unattested (no-recogniser) turn that never probes at all.
func TestHandleCompleteStreamEvent_TurnCompletedPrecedesParkedEvent(t *testing.T) {
	cases := []struct {
		name         string
		attested     bool
		probeResults []executor.ProbeResult
		// wantParkedEvent is true only when the case actually transitions
		// parked from false to true — the projection only publishes on
		// change (D8), so "un-parked"/"unknown probe" legitimately publish
		// nothing (parked stays false throughout) and are not a regression.
		wantParkedEvent bool
	}{
		{"parked", true, []executor.ProbeResult{executor.ProbeResultLive}, true},
		{"un-parked (probe settled)", true, []executor.ProbeResult{executor.ProbeResultSettled}, false},
		{"unknown probe", true, []executor.ProbeResult{executor.ProbeResultUnknown}, false},
		{"no recogniser (never attested)", false, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder, session := runTurnFinishedOrderingCase(t, tc.attested, tc.probeResults)

			if session.State != models.TaskSessionStateWaitingForInput {
				t.Fatalf("expected the session to settle into WAITING_FOR_INPUT, got %q", session.State)
			}

			turnIdx, turnCount := turnCompletedIndex(recorder)
			if turnCount != 1 {
				t.Fatalf("expected events.TurnCompleted published exactly once, got %d", turnCount)
			}
			parkedIdx := firstParkedEventIndex(recorder)
			if !tc.wantParkedEvent {
				// Nothing transitions parked to true in this case, so the
				// projection publishes no event (D8: publish only on
				// change) — there is nothing to order the turn-completion
				// signal against.
				if parkedIdx != -1 {
					t.Fatalf("case %q should never publish a parked-projection event, got one at index %d", tc.name, parkedIdx)
				}
				return
			}
			// A regression that stopped publishing the parked-projection
			// event entirely must fail this test, not silently no-op.
			if parkedIdx == -1 {
				t.Fatalf("expected a parked-projection event to be published for case %q, got none", tc.name)
			}
			if turnIdx >= parkedIdx {
				t.Fatalf("events.TurnCompleted (index %d) must precede the parked-projection event (index %d)", turnIdx, parkedIdx)
			}
		})
	}
}
