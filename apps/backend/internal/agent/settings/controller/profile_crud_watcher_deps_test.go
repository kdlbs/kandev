package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentdto "github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

// fakeSessionChecker stubs the active-session lookup so the watcher-deps
// path is exercised independently of the existing active-session guard.
type fakeSessionChecker struct {
	activeTasks []agentdto.ActiveTaskInfo
}

func (f *fakeSessionChecker) HasActiveTaskSessionsByAgentProfile(context.Context, string) (bool, error) {
	return len(f.activeTasks) > 0, nil
}

func (f *fakeSessionChecker) DeleteEphemeralTasksByAgentProfile(context.Context, string) (int64, error) {
	return 0, nil
}

func (f *fakeSessionChecker) GetActiveTaskInfoByAgentProfile(context.Context, string) ([]agentdto.ActiveTaskInfo, error) {
	return f.activeTasks, nil
}

// fakeAutomationDependencyChecker returns a canned list of enabled automations
// bound to the profile.
type fakeAutomationDependencyChecker struct {
	refs         []AutomationReference
	err          error
	disableCalls int
}

func (f *fakeAutomationDependencyChecker) ListEnabledAutomationsByAgentProfile(
	context.Context, string,
) ([]AutomationReference, error) {
	return f.refs, f.err
}

func (f *fakeAutomationDependencyChecker) DisableAutomationsByAgentProfile(
	context.Context, string,
) ([]AutomationReference, error) {
	f.disableCalls++
	return f.refs, nil
}

// fakeWatcherDependencyChecker returns a canned list of referencing
// watchers and records disable invocations so tests can assert on the
// force-delete eager-disable contract.
type fakeWatcherDependencyChecker struct {
	refs            []WatcherReference
	err             error
	disableCalls    int
	disabledProfile string
	disabledCause   string
}

type fakeRoutingTierDependencyChecker struct {
	refs []RoutingTierReference
	err  error
}

func (f fakeRoutingTierDependencyChecker) ListRoutingTierReferencesByAgentProfile(context.Context, string) ([]RoutingTierReference, error) {
	return f.refs, f.err
}

func (f *fakeWatcherDependencyChecker) ListWatchersByAgentProfile(context.Context, string) ([]WatcherReference, error) {
	return f.refs, f.err
}

func (f *fakeWatcherDependencyChecker) DisableWatchersByAgentProfile(_ context.Context, profileID, cause string) ([]WatcherReference, error) {
	f.disableCalls++
	f.disabledProfile = profileID
	f.disabledCause = cause
	return f.refs, f.err
}

// TestDeleteProfile_BlocksOnReferencingWatchers is the regression guard for
// the UX hole: deleting a profile that linear/jira/github_issue/github_review
// watchers point at must surface a confirmation-detail error so the UI can
// say "this disables N watchers — continue?" instead of silently orphaning
// them.
func TestDeleteProfile_BlocksOnReferencingWatchers(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"test-agent": &testAgent{id: "test-agent", name: "test-agent", enabled: true}})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{{ID: "prof-1", AgentID: agent.ID, Name: "Kilo Profile"}}
	ctrl.repo = st
	ctrl.sessionChecker = &fakeSessionChecker{}
	ctrl.watcherDeps = &fakeWatcherDependencyChecker{refs: []WatcherReference{
		{ID: "linear-w1", Kind: "linear", Label: "ENG team backlog"},
		{ID: "github-w7", Kind: "github_issue", Label: "kdlbs/kandev bugs"},
	}}

	_, err := ctrl.DeleteProfile(context.Background(), "prof-1", false)

	var detail *ErrProfileInUseDetail
	if !errors.As(err, &detail) {
		t.Fatalf("expected ErrProfileInUseDetail, got %v", err)
	}
	if len(detail.Watchers) != 2 {
		t.Fatalf("expected 2 watcher refs, got %d: %+v", len(detail.Watchers), detail.Watchers)
	}
	if detail.Watchers[0].Kind != "linear" || detail.Watchers[1].Kind != "github_issue" {
		t.Errorf("unexpected watcher refs: %+v", detail.Watchers)
	}
}

// An enabled automation never appears in the active-session list — nothing is
// running — but its next firing would launch against a profile that is gone,
// and a schedule fails quietly, hours later, with nobody watching.
func TestDeleteProfile_BlocksOnEnabledAutomations(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"test-agent": &testAgent{id: "test-agent", name: "test-agent", enabled: true}})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{{ID: "prof-1", AgentID: agent.ID, Name: "Kilo Profile"}}
	ctrl.repo = st
	ctrl.sessionChecker = &fakeSessionChecker{}
	ctrl.automationDeps = &fakeAutomationDependencyChecker{refs: []AutomationReference{
		{ID: "auto-1", Name: "Nightly dependency sweep", WorkspaceID: "ws-1"},
	}}

	_, err := ctrl.DeleteProfile(context.Background(), "prof-1", false)

	var detail *ErrProfileInUseDetail
	if !errors.As(err, &detail) {
		t.Fatalf("expected ErrProfileInUseDetail, got %v", err)
	}
	if len(detail.Automations) != 1 || detail.Automations[0].Name != "Nightly dependency sweep" {
		t.Fatalf("expected the automation to be named, got %+v", detail.Automations)
	}
}

// The confirmation exists so the user can proceed knowingly; force is how they
// say so.
func TestDeleteProfile_ForceBypassesAutomationCheck(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"test-agent": &testAgent{id: "test-agent", name: "test-agent", enabled: true}})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{{ID: "prof-1", AgentID: agent.ID, Name: "Kilo Profile"}}
	ctrl.repo = st
	ctrl.sessionChecker = &fakeSessionChecker{}
	automationDeps := &fakeAutomationDependencyChecker{refs: []AutomationReference{
		{ID: "auto-1", Name: "Nightly dependency sweep", WorkspaceID: "ws-1"},
	}}
	ctrl.automationDeps = automationDeps

	if _, err := ctrl.DeleteProfile(context.Background(), "prof-1", true); err != nil {
		t.Fatalf("force delete should proceed past the automation check, got %v", err)
	}
	// Proceeding is not the whole contract: an automation left enabled would
	// keep firing on its schedule into a profile that no longer exists.
	if automationDeps.disableCalls != 1 {
		t.Fatalf("force delete must disable the orphaned automations, got %d call(s)",
			automationDeps.disableCalls)
	}
}

// Fails closed. A watcher this lookup misses is disabled anyway by the force
// pass; an automation is not, so silently treating "lookup failed" as "no
// automations" would orphan exactly the references this check exists to catch.
func TestDeleteProfile_AutomationLookupFailureBlocks(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"test-agent": &testAgent{id: "test-agent", name: "test-agent", enabled: true}})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{{ID: "prof-1", AgentID: agent.ID, Name: "Kilo Profile"}}
	ctrl.repo = st
	ctrl.sessionChecker = &fakeSessionChecker{}
	ctrl.automationDeps = &fakeAutomationDependencyChecker{err: errors.New("store down")}

	_, err := ctrl.DeleteProfile(context.Background(), "prof-1", false)
	if err == nil {
		t.Fatal("a failed automation lookup must refuse the deletion, not wave it through")
	}
	var detail *ErrProfileInUseDetail
	if errors.As(err, &detail) {
		t.Fatalf("a lookup failure is an error, not an in-use conflict: %v", err)
	}
}

func TestDeleteProfile_BlocksOnRoutingTierReferencesEvenWithForce(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"test-agent": &testAgent{id: "test-agent", name: "test-agent", enabled: true}})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{{ID: "prof-1", AgentID: agent.ID, Name: "Kilo Profile"}}
	ctrl.repo = st
	ctrl.sessionChecker = &fakeSessionChecker{}
	ctrl.routingTierDeps = fakeRoutingTierDependencyChecker{refs: []RoutingTierReference{
		{WorkspaceID: "ws-1", ProviderID: "codex-acp", Tier: "balanced"},
	}}

	_, err := ctrl.DeleteProfile(context.Background(), "prof-1", true)

	var detail *ErrProfileInUseDetail
	if !errors.As(err, &detail) {
		t.Fatalf("expected ErrProfileInUseDetail, got %v", err)
	}
	if len(detail.RoutingTiers) != 1 {
		t.Fatalf("expected 1 routing tier ref, got %+v", detail.RoutingTiers)
	}
	if detail.RoutingTiers[0].Tier != "balanced" {
		t.Errorf("unexpected routing tier ref: %+v", detail.RoutingTiers[0])
	}
}

// TestDeleteProfile_ForceBypassesWatcherCheck pins the override knob: when
// the user has already confirmed in the UI (force=true), DeleteProfile
// proceeds even though watchers reference the profile. The watchers will
// self-heal on their next poll via the dispatch coordinator's pre-flight.
func TestDeleteProfile_ForceBypassesWatcherCheck(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"test-agent": &testAgent{id: "test-agent", name: "test-agent", enabled: true}})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{{ID: "prof-1", AgentID: agent.ID, Name: "Kilo Profile"}}
	ctrl.repo = st
	ctrl.sessionChecker = &fakeSessionChecker{}
	deps := &fakeWatcherDependencyChecker{refs: []WatcherReference{{ID: "linear-w1", Kind: "linear"}}}
	ctrl.watcherDeps = deps

	if _, err := ctrl.DeleteProfile(context.Background(), "prof-1", true); err != nil {
		t.Fatalf("force=true must bypass the watcher check, got %v", err)
	}
	// Force-delete must NOT rely on the lazy preflight — it must eagerly
	// disable each referencing watcher with the deletion cause so the UI
	// reflects the change immediately, before any external event fires.
	if deps.disableCalls != 1 {
		t.Errorf("expected DisableWatchersByAgentProfile to fire once, got %d", deps.disableCalls)
	}
	if deps.disabledProfile != "prof-1" {
		t.Errorf("disabled profile = %q, want %q", deps.disabledProfile, "prof-1")
	}
	// The cause must carry the human-readable profile name (not just the UUID)
	// so the settings banner is legible and matches the lazy preflight's cause.
	if !strings.Contains(deps.disabledCause, "Kilo Profile") {
		t.Errorf("disable cause %q must include the profile name", deps.disabledCause)
	}
}

// TestDeleteProfile_NoWatchersStillSucceeds pins the negative case: when no
// watchers reference the profile, the existing happy path is preserved.
func TestDeleteProfile_NoWatchersStillSucceeds(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"test-agent": &testAgent{id: "test-agent", name: "test-agent", enabled: true}})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	st.profiles[agent.ID] = []*models.AgentProfile{{ID: "prof-1", AgentID: agent.ID, Name: "Kilo Profile"}}
	ctrl.repo = st
	ctrl.sessionChecker = &fakeSessionChecker{}
	ctrl.watcherDeps = &fakeWatcherDependencyChecker{} // empty refs

	if _, err := ctrl.DeleteProfile(context.Background(), "prof-1", false); err != nil {
		t.Fatalf("no watchers should not block delete, got %v", err)
	}
}
