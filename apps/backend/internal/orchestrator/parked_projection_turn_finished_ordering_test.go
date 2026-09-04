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
	bus         *recordingEventBus
	turnID      string
	activeGiven bool
}

func (f *fakeOrderingTurnService) StartTurn(context.Context, string) (*models.Turn, error) {
	return nil, nil
}

func (f *fakeOrderingTurnService) CompleteTurn(ctx context.Context, turnID string) error {
	_ = f.bus.Publish(ctx, events.TurnCompleted, bus.NewEvent(events.TurnCompleted, "task-service", map[string]interface{}{
		"id": turnID,
	}))
	return nil
}

func (f *fakeOrderingTurnService) GetTurn(context.Context, string) (*models.Turn, error) {
	return nil, nil
}

func (f *fakeOrderingTurnService) GetActiveTurn(_ context.Context, sessionID string) (*models.Turn, error) {
	if f.activeGiven {
		return nil, nil
	}
	f.activeGiven = true
	return &models.Turn{ID: f.turnID, TaskSessionID: sessionID}, nil
}

func (f *fakeOrderingTurnService) UpdateTurn(context.Context, *models.Turn) error { return nil }

func (f *fakeOrderingTurnService) AbandonOpenTurns(context.Context, string) error { return nil }

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
	}{
		{"parked", true, []executor.ProbeResult{executor.ProbeResultLive}},
		{"un-parked (probe settled)", true, []executor.ProbeResult{executor.ProbeResultSettled}},
		{"unknown probe", true, []executor.ProbeResult{executor.ProbeResultUnknown}},
		{"no recogniser (never attested)", false, nil},
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
			if parkedIdx == -1 {
				// Some outcomes never touch the projection at all (e.g. no
				// attestation means the probe is never called, AC-40a) — there
				// is nothing to order the turn-completion signal against.
				return
			}
			if turnIdx >= parkedIdx {
				t.Fatalf("events.TurnCompleted (index %d) must precede the parked-projection event (index %d)", turnIdx, parkedIdx)
			}
		})
	}
}
