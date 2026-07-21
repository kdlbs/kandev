package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
)

// mockAgentProfileResolver returns a profile pointing to the mock-agent.
// When err is non-nil, ResolveProfile returns that error and a nil profile —
// simulating transient DB hiccups, network failures, or deleted profiles.
type mockAgentProfileResolver struct {
	cliPassthrough bool
	err            error
}

type blockingAgentProfileResolver struct {
	entered chan chan struct{}
}

type cancellableAgentProfileResolver struct {
	entered chan struct{}
	release chan struct{}
}

func (r *cancellableAgentProfileResolver) ResolveProfile(
	ctx context.Context,
	_ string,
) (*AgentProfileInfo, error) {
	close(r.entered)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
		return &AgentProfileInfo{AgentID: "mock-agent"}, nil
	}
}

func (r *blockingAgentProfileResolver) ResolveProfile(
	_ context.Context,
	profileID string,
) (*AgentProfileInfo, error) {
	release := make(chan struct{})
	r.entered <- release
	<-release
	return &AgentProfileInfo{
		ProfileID: profileID,
		AgentID:   "mock-agent",
	}, nil
}

func (m *mockAgentProfileResolver) ResolveProfile(_ context.Context, profileID string) (*AgentProfileInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &AgentProfileInfo{
		ProfileID:      profileID,
		ProfileName:    "Test Profile",
		AgentID:        "mock-agent",
		AgentName:      "mock-agent",
		Model:          "mock-fast",
		CLIPassthrough: m.cliPassthrough,
	}, nil
}

func TestStartAgentProcess_NotFound(t *testing.T) {
	mgr := newTestManager(t)
	err := mgr.StartAgentProcess(context.Background(), "non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent execution")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestStartAgentProcess_NonPassthrough_NoAgentctl(t *testing.T) {
	mgr := newTestManager(t)
	// Use a resolver that returns CLIPassthrough=false
	mgr.profileResolver = &mockAgentProfileResolver{cliPassthrough: false}

	execution := &AgentExecution{
		ID:             "exec-1",
		SessionID:      "sess-1",
		AgentProfileID: "profile-1",
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error for missing agentctl")
	}
	if !strings.Contains(err.Error(), "no agentctl client") {
		t.Errorf("expected 'no agentctl client' error, got: %v", err)
	}
}

func TestFailedConcurrentStartDoesNotReleaseLiveStartActivity(t *testing.T) {
	mgr := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	mgr.SetActivityCoordinator(coordinator)
	resolver := &blockingAgentProfileResolver{entered: make(chan chan struct{}, 2)}
	mgr.profileResolver = resolver
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "exec-shared", AgentProfileID: "profile-1",
	}); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	results := make(chan error, 2)
	go func() { results <- mgr.StartAgentProcess(context.Background(), "exec-shared") }()
	releaseFirst := <-resolver.entered
	go func() { results <- mgr.StartAgentProcess(context.Background(), "exec-shared") }()
	releaseSecond := <-resolver.entered

	close(releaseFirst)
	if err := <-results; err == nil {
		t.Fatal("first start returned nil, want missing agentctl error")
	}
	maintenance, _, maintenanceErr := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if maintenance != nil {
		maintenance.Release()
	}
	close(releaseSecond)
	if err := <-results; err == nil {
		t.Fatal("second start returned nil, want missing agentctl error")
	}
	if !errors.Is(maintenanceErr, activity.ErrBusy) {
		t.Fatalf("maintenance error = %v, want ErrBusy while second start is live", maintenanceErr)
	}
}

func TestTerminalInvalidationCancelsAndDrainsAdmittedStart(t *testing.T) {
	mgr := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	mgr.SetActivityCoordinator(coordinator)
	resolver := &cancellableAgentProfileResolver{
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	mgr.profileResolver = resolver
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "exec-invalidated", AgentProfileID: "profile-1",
	}); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	result := make(chan error, 1)
	go func() { result <- mgr.StartAgentProcess(context.Background(), "exec-invalidated") }()
	<-resolver.entered
	mgr.releaseActivity(executionActivityKey("exec-invalidated"))
	maintenance, _, maintenanceErr := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if maintenance != nil {
		maintenance.Release()
	}
	if !errors.Is(maintenanceErr, activity.ErrBusy) {
		close(resolver.release)
		<-result
		t.Fatalf("maintenance error = %v, want ErrBusy until invalidated start drains", maintenanceErr)
	}
	if err := <-result; !errors.Is(err, errExecutionActivityInvalidated) {
		t.Fatalf("StartAgentProcess error = %v, want claim invalidation", err)
	}
	maintenance, _, err := coordinator.TryAcquireMaintenance(context.Background(), 0)
	if err != nil {
		t.Fatalf("maintenance after start drained: %v", err)
	}
	maintenance.Release()
}

func TestStartAgentProcess_Passthrough_NotResumed(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockAgentProfileResolver{cliPassthrough: true}

	execution := &AgentExecution{
		ID:               "exec-1",
		SessionID:        "sess-1",
		AgentProfileID:   "profile-1",
		IsPassthrough:    true,
		isResumedSession: false,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error (no interactive runner)")
	}
	// startPassthroughSession path errors with "interactive runner not available for passthrough mode"
	if !strings.Contains(err.Error(), "interactive runner not available") {
		t.Errorf("expected 'interactive runner not available' error from startPassthroughSession, got: %v", err)
	}
}

func TestStartAgentProcess_Passthrough_Resumed(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockAgentProfileResolver{cliPassthrough: true}

	execution := &AgentExecution{
		ID:               "exec-1",
		SessionID:        "sess-1",
		AgentProfileID:   "profile-1",
		IsPassthrough:    true,
		isResumedSession: true,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error (no interactive runner)")
	}
	// ResumePassthroughSession returns "interactive runner not available" (without
	// "for passthrough mode"), while startPassthroughSession includes the suffix.
	// Assert we hit the resume path specifically.
	if !strings.Contains(err.Error(), "interactive runner not available") {
		t.Errorf("expected 'interactive runner not available' error, got: %v", err)
	}
	if strings.Contains(err.Error(), "for passthrough mode") {
		t.Errorf("expected ResumePassthroughSession path (no 'for passthrough mode' suffix), got: %v", err)
	}
}

// A session created in agent (ACP) mode must keep using the ACP launch path
// even after its profile has been toggled to CLIPassthrough — the
// session-snapshot wins so existing sessions don't get stranded.
func TestStartAgentProcess_AgentSession_IgnoresProfileToggleToPassthrough(t *testing.T) {
	mgr := newTestManager(t)
	// Profile currently advertises passthrough — simulates the post-toggle state.
	mgr.profileResolver = &mockAgentProfileResolver{cliPassthrough: true}

	execution := &AgentExecution{
		ID:             "exec-1",
		SessionID:      "sess-1",
		AgentProfileID: "profile-1",
		// Session snapshot: false (session was created when profile was ACP).
		IsPassthrough: false,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error (no agentctl client)")
	}
	// We must hit the ACP path, which errors on the missing agentctl client.
	// Passthrough errors look like "interactive runner not available [...]"
	// — seeing that here would mean the snapshot was ignored.
	if !strings.Contains(err.Error(), "no agentctl client") {
		t.Errorf("expected ACP path to fail on missing agentctl client, got: %v", err)
	}
}

// Mirror of the bug fix in the opposite direction: a session created in
// passthrough mode must keep using the passthrough path even if the profile
// is later toggled back to ACP.
func TestStartAgentProcess_PassthroughSession_IgnoresProfileToggleToAgent(t *testing.T) {
	mgr := newTestManager(t)
	// Profile currently advertises ACP — simulates a toggle away from passthrough.
	mgr.profileResolver = &mockAgentProfileResolver{cliPassthrough: false}

	execution := &AgentExecution{
		ID:             "exec-1",
		SessionID:      "sess-1",
		AgentProfileID: "profile-1",
		// Session snapshot: true (session was created when profile was passthrough).
		IsPassthrough: true,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error (no interactive runner)")
	}
	if !strings.Contains(err.Error(), "interactive runner not available") {
		t.Errorf("expected passthrough path to fail on missing interactive runner, got: %v", err)
	}
}

// When ResolveProfile errors on a passthrough resume, the routing decision
// must still honor the snapshot and stay on the passthrough branch. The
// downstream command builder may then surface the resolve error explicitly —
// what matters is that we do NOT silently fall through to the ACP path (the
// bug that would otherwise strand the session in the wrong launch mode).
func TestStartAgentProcess_PassthroughResume_SurvivesProfileResolveError(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockAgentProfileResolver{err: errors.New("transient DB error")}

	execution := &AgentExecution{
		ID:               "exec-1",
		SessionID:        "sess-1",
		AgentProfileID:   "profile-1",
		IsPassthrough:    true,
		isResumedSession: true,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error from the passthrough branch")
	}
	// ACP-path "no agentctl client" error would mean the snapshot was
	// ignored and routing silently picked the wrong branch.
	if strings.Contains(err.Error(), "no agentctl client") {
		t.Errorf("snapshot must keep us on the passthrough branch, got ACP error: %v", err)
	}
}

// A fresh passthrough launch (not a resume) needs the profile for command
// building, so a profile-resolve error must surface as an explicit error
// rather than silently falling through to ACP.
func TestStartAgentProcess_FreshPassthrough_SurfacesProfileResolveError(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockAgentProfileResolver{err: errors.New("transient DB error")}

	execution := &AgentExecution{
		ID:             "exec-1",
		SessionID:      "sess-1",
		AgentProfileID: "profile-1",
		IsPassthrough:  true,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected profile-resolve error to surface")
	}
	if !strings.Contains(err.Error(), "resolve profile") {
		t.Errorf("expected resolve-profile error, got: %v", err)
	}
	if strings.Contains(err.Error(), "agentctl client") {
		t.Errorf("snapshot must keep us on the passthrough branch, got ACP error: %v", err)
	}
}

// Sessionless launches (e.g. the legacy controller.LaunchAgent path that
// doesn't carry a SessionID) still fall back to live profile resolution so
// first-time launches reflect the current mode.
func TestStartAgentProcess_NoSession_FallsBackToLiveProfile(t *testing.T) {
	mgr := newTestManager(t)
	mgr.profileResolver = &mockAgentProfileResolver{cliPassthrough: true}

	execution := &AgentExecution{
		ID:             "exec-1",
		AgentProfileID: "profile-1",
		// No SessionID, no snapshot → use live profile state.
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	err := mgr.StartAgentProcess(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected error (no interactive runner)")
	}
	if !strings.Contains(err.Error(), "interactive runner not available") {
		t.Errorf("expected passthrough path via live fallback, got: %v", err)
	}
}
