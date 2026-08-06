package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

// The tests in this file guard one invariant: a profile delete must never
// return with the agent_profile row gone and an automation still enabled and
// bound to it. That state is silent and permanent — an automation is a standing
// schedule with nobody watching it, and unlike a watcher it has no preflight to
// re-resolve the profile and disable itself on the next poll.
//
// They assert against modelled state rather than call counts on purpose. The
// defect the reviewer found was never "the disable call is missing"; it was that
// the call could fail, or never happen at all, after the row had already gone.
// Counting calls would have passed against the buggy ordering.

// automationDeleteWorld models the two rows this bug desynchronises, plus the
// order the controller touched them in.
type automationDeleteWorld struct {
	profileDeleted    bool
	automationEnabled bool
	// disableErr, when set, is what DisableAutomationsByAgentProfile returns —
	// the injected failure the reviewer asked for.
	disableErr error
	// afterDisable runs once the disable has taken effect, so a test can slam
	// the door (cancel the context) in the window between the two operations.
	afterDisable func()
	steps        []string
}

func (w *automationDeleteWorld) record(step string) { w.steps = append(w.steps, step) }

// assertNothingStranded is the whole point of the file.
func (w *automationDeleteWorld) assertNothingStranded(t *testing.T) {
	t.Helper()
	if w.profileDeleted && w.automationEnabled {
		t.Fatalf("stranded automation: the profile row was deleted while an automation bound to it is still enabled (steps: %v)", w.steps)
	}
}

// worldAutomationDeps is an AutomationDependencyChecker backed by the world
// model, so a successful disable actually flips the automation off.
type worldAutomationDeps struct{ w *automationDeleteWorld }

func (d *worldAutomationDeps) ListEnabledAutomationsByAgentProfile(
	context.Context, string,
) ([]AutomationReference, error) {
	if !d.w.automationEnabled {
		return nil, nil
	}
	return []AutomationReference{{ID: "auto-1", Name: "Nightly dependency sweep", WorkspaceID: "ws-1"}}, nil
}

func (d *worldAutomationDeps) DisableAutomationsByAgentProfile(
	ctx context.Context, _ string,
) ([]AutomationReference, error) {
	d.w.record("disable_automations")
	// A real store call fails on a dead context before it touches any row.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.w.disableErr != nil {
		return nil, d.w.disableErr
	}
	if !d.w.automationEnabled {
		return nil, nil
	}
	d.w.automationEnabled = false
	if d.w.afterDisable != nil {
		d.w.afterDisable()
	}
	return []AutomationReference{{ID: "auto-1", Name: "Nightly dependency sweep", WorkspaceID: "ws-1"}}, nil
}

// worldWatcherDeps records when the watcher pass ran so the eager-after-delete
// ordering can be pinned alongside the new automation ordering.
type worldWatcherDeps struct{ w *automationDeleteWorld }

func (d *worldWatcherDeps) ListWatchersByAgentProfile(context.Context, string) ([]WatcherReference, error) {
	return []WatcherReference{{ID: "linear-w1", Kind: "linear"}}, nil
}

func (d *worldWatcherDeps) DisableWatchersByAgentProfile(
	context.Context, string, string,
) ([]WatcherReference, error) {
	d.w.record("disable_watchers")
	return []WatcherReference{{ID: "linear-w1", Kind: "linear"}}, nil
}

// ctxAwareStore layers real context handling and world bookkeeping over the
// shared fakeStore, which ignores ctx entirely. A cancellation test running
// against a store that cannot be cancelled would prove nothing.
type ctxAwareStore struct {
	*fakeStore
	w *automationDeleteWorld
}

func (s *ctxAwareStore) DeleteAgentProfile(ctx context.Context, id string) error {
	s.w.record("delete_profile")
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.fakeStore.DeleteAgentProfile(ctx, id); err != nil {
		return err
	}
	s.w.profileDeleted = true
	return nil
}

// newAutomationDeleteFixture wires a controller holding one profile that one
// enabled automation and one watcher point at.
func newAutomationDeleteFixture(t *testing.T) (*Controller, *automationDeleteWorld) {
	t.Helper()
	world := &automationDeleteWorld{automationEnabled: true}
	ctrl := newTestController(map[string]agents.Agent{
		"test-agent": &testAgent{id: "test-agent", name: "test-agent", enabled: true},
	})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{{ID: "prof-1", AgentID: agent.ID, Name: "Kilo Profile"}}
	ctrl.repo = &ctxAwareStore{fakeStore: st, w: world}
	ctrl.sessionChecker = &fakeSessionChecker{}
	ctrl.automationDeps = &worldAutomationDeps{w: world}
	ctrl.watcherDeps = &worldWatcherDeps{w: world}
	return ctrl, world
}

// The ordering is the fix. Disabling after the delete leaves a window — however
// short — in which the profile is gone and the automation is not yet off, and
// anything that interrupts the request inside that window makes the state
// permanent.
func TestDeleteProfile_DisablesAutomationsBeforeDeletingTheRow(t *testing.T) {
	ctrl, world := newAutomationDeleteFixture(t)

	if _, err := ctrl.DeleteProfile(context.Background(), "prof-1", true); err != nil {
		t.Fatalf("force delete should succeed, got %v", err)
	}

	want := []string{"disable_automations", "delete_profile", "disable_watchers"}
	if len(world.steps) != len(want) {
		t.Fatalf("steps = %v, want %v", world.steps, want)
	}
	for i := range want {
		if world.steps[i] != want[i] {
			t.Fatalf("steps = %v, want %v", world.steps, want)
		}
	}
	world.assertNothingStranded(t)
}

// The regression the reviewer asked for. With the disable running after the
// delete and its error swallowed, this ended with the row gone and the
// automation still firing on schedule into nothing.
func TestDeleteProfile_AutomationDisableFailureAbortsTheDelete(t *testing.T) {
	ctrl, world := newAutomationDeleteFixture(t)
	world.disableErr = errors.New("automation store down")

	_, err := ctrl.DeleteProfile(context.Background(), "prof-1", true)

	if err == nil {
		t.Fatal("a failed automation disable must abort the delete, not proceed without it")
	}
	if !errors.Is(err, world.disableErr) {
		t.Errorf("error must carry the underlying store failure, got %v", err)
	}
	world.assertNothingStranded(t)
	if world.profileDeleted {
		t.Error("profile row was deleted despite the automation disable failing")
	}
	if !world.automationEnabled {
		t.Error("the automation should be untouched — the delete never happened")
	}
	// The live row must survive so a retry has something to delete.
	if _, getErr := ctrl.repo.GetAgentProfile(context.Background(), "prof-1"); getErr != nil {
		t.Errorf("profile should still be retrievable after the aborted delete, got %v", getErr)
	}
}

// Cancellation in the window between the two operations is the version of this
// bug that needs no store outage to reproduce — the user closing the tab is
// enough. The disable has landed by then, so the worst outcome is an automation
// disabled against a profile that is still there: visible, and one toggle back.
func TestDeleteProfile_ContextCancelledAfterAutomationDisableStrandsNothing(t *testing.T) {
	ctrl, world := newAutomationDeleteFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	world.afterDisable = cancel

	_, err := ctrl.DeleteProfile(ctx, "prof-1", true)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("delete should surface the cancellation, got %v", err)
	}
	world.assertNothingStranded(t)
	if world.profileDeleted {
		t.Error("the profile row must not be deleted under a cancelled context")
	}
	if world.automationEnabled {
		t.Error("the automation disable landed before the cancellation and must have stuck")
	}
}

// Cancellation before anything runs. Both rows are left exactly as they were,
// and the request must have stopped at the automation gate — reaching the
// delete first would mean the gate is downstream of the destructive step again.
func TestDeleteProfile_ContextCancelledBeforeAutomationDisableChangesNothing(t *testing.T) {
	ctrl, world := newAutomationDeleteFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ctrl.DeleteProfile(ctx, "prof-1", true)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("delete should surface the cancellation, got %v", err)
	}
	world.assertNothingStranded(t)
	if world.profileDeleted || !world.automationEnabled {
		t.Errorf("nothing should have changed, got deleted=%v automation_enabled=%v",
			world.profileDeleted, world.automationEnabled)
	}
	if len(world.steps) != 1 || world.steps[0] != "disable_automations" {
		t.Errorf("a dead context must be refused at the automation gate, steps = %v", world.steps)
	}
}

// Watchers keep the opposite ordering on purpose, and it must stay that way:
// disabling them before the delete would strand them disabled against a live
// profile whenever the delete fails, which is a real regression the automation
// fix must not drag along. Their safety net is the dispatch coordinator's
// preflight, which automations do not have.
func TestDeleteProfile_WatcherDisableStillRunsOnlyAfterTheRowIsDeleted(t *testing.T) {
	ctrl, world := newAutomationDeleteFixture(t)
	world.disableErr = errors.New("automation store down")

	if _, err := ctrl.DeleteProfile(context.Background(), "prof-1", true); err == nil {
		t.Fatal("expected the aborted delete for this fixture")
	}

	for _, step := range world.steps {
		if step == "disable_watchers" {
			t.Fatal("watchers must not be disabled when the row was never deleted")
		}
	}
}
