package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── Profile validation tests (blocker: Ensure must gate hidden task/session
// creation on a resolved, enabled profile — zero partial rows otherwise) ──

func TestEnsureRejectsMissingProfileWithZeroPartialRows(t *testing.T) {
	svc, deps := newACTestService()
	deps.profiles.markMissing("ghost-profile")

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
		AgentProfileID:  "ghost-profile",
	}

	desc, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusConfigurationRequired {
		t.Fatalf("status = %q, want %q", statusStr, AgentConversationStatusConfigurationRequired)
	}
	if desc.TaskID != "" || desc.SessionID != "" {
		t.Fatalf("expected empty descriptor, got %+v", desc)
	}
	if got := deps.tasks.count(); got != 0 {
		t.Fatalf("expected zero task rows, got %d", got)
	}
	if got := deps.sess.count(); got != 0 {
		t.Fatalf("expected zero session rows, got %d", got)
	}
	if published := deps.eventer.getPublished(); len(published) != 0 {
		t.Fatalf("expected no task.created event, got %d", len(published))
	}
}

func TestEnsureRejectsDisabledProfileWithZeroPartialRows(t *testing.T) {
	svc, deps := newACTestService()
	deps.profiles.markDisabled("disabled-profile")

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
		AgentProfileID:  "disabled-profile",
	}

	_, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusConfigurationRequired {
		t.Fatalf("status = %q, want %q", statusStr, AgentConversationStatusConfigurationRequired)
	}
	if got := deps.tasks.count(); got != 0 {
		t.Fatalf("expected zero task rows, got %d", got)
	}
}

func TestEnsureProfileValidatorReceivesConfiguredID(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
		AgentProfileID:  "profile-gpt4",
	}
	if _, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil || statusStr != AgentConversationStatusCreated {
		t.Fatalf("Ensure: status=%q err=%v", statusStr, err)
	}

	deps.profiles.mu.Lock()
	calls := append([]string(nil), deps.profiles.calls...)
	deps.profiles.mu.Unlock()
	if len(calls) != 1 || calls[0] != "profile-gpt4" {
		t.Fatalf("profile validator calls = %v, want [profile-gpt4]", calls)
	}
}

func TestEnsureWithoutProfileConfiguredSkipsValidation(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	_, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusCreated {
		t.Fatalf("status = %q, want created", statusStr)
	}
	deps.profiles.mu.Lock()
	calls := len(deps.profiles.calls)
	deps.profiles.mu.Unlock()
	if calls != 0 {
		t.Fatalf("expected no profile validation call when no profile is configured, got %d", calls)
	}
}

func TestEnsureRepairRefusedWhenBoundProfileBecomesInvalid(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
		AgentProfileID:  "profile-gpt4",
	}
	desc, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil || statusStr != AgentConversationStatusCreated {
		t.Fatalf("first Ensure: status=%q err=%v", statusStr, err)
	}

	// Simulate a partial-creation repair scenario, with the bound profile
	// having since become invalid (disabled or deleted after the task was
	// created).
	deps.sess.mu.Lock()
	delete(deps.sess.sessions, desc.SessionID)
	deps.sess.mu.Unlock()
	deps.profiles.markMissing("profile-gpt4")

	desc2, statusStr2, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if statusStr2 != AgentConversationStatusConfigurationRequired {
		t.Fatalf("status = %q, want %q", statusStr2, AgentConversationStatusConfigurationRequired)
	}
	if desc2.SessionID != "" {
		t.Fatalf("expected no session to be created during a refused repair, got %+v", desc2)
	}
	if got := deps.sess.count(); got != 0 {
		t.Fatalf("expected zero session rows after refused repair, got %d", got)
	}
}

// ── Dispatch tests (blocker: Dispatch must reach the real agent runtime,
// not just persist a message) ──────────────────────────────────────────

func TestDispatchDeliversThroughRuntime(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{
		WorkspaceID:     "ws-1",
		ConversationKey: "coordinator",
	}
	desc, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	dispatch, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check the board", "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// A freshly ensured session is CREATED (never launched); a real
	// dispatch through the runtime starts it. The pre-fix implementation
	// always reported "sent" because it only ever inserted a message row —
	// this assertion is the regression guard for that gap.
	if dispatch.Status != "started" {
		t.Fatalf("status = %q, want started", dispatch.Status)
	}
	if dispatch.Descriptor.TaskID == "" {
		t.Fatal("expected descriptor TaskID")
	}

	if deps.dispatcher.callCount() != 1 {
		t.Fatalf("expected 1 call to reach the runtime dispatcher, got %d", deps.dispatcher.callCount())
	}
	call := deps.dispatcher.lastCall()
	if call.taskID != desc.TaskID {
		t.Fatalf("dispatcher taskID = %q, want %q", call.taskID, desc.TaskID)
	}
	if call.sessionID != desc.SessionID {
		t.Fatalf("dispatcher sessionID = %q, want %q", call.sessionID, desc.SessionID)
	}
	if call.text != "Check the board" {
		t.Fatalf("dispatcher text = %q", call.text)
	}
	if call.source != "plugin:plugin-coordinator" {
		t.Fatalf("dispatcher source = %q", call.source)
	}
}

func TestDispatchWithIdleSessionSendsRatherThanStarts(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	desc, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	deps.sess.setState(desc.SessionID, models.TaskSessionStateWaitingForInput)

	dispatch, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check again", "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if dispatch.Status != "sent" {
		t.Fatalf("status = %q, want sent", dispatch.Status)
	}
}

func TestDispatchWithoutDispatcherReturnsUnavailable(t *testing.T) {
	tasks := &acFakeTaskRepo{}
	sess := newACFakeSessionRepo()
	profiles := newACFakeProfileRepo()
	state := newACFakeStateRepo()
	eventer := &acFakeEventBus{}
	svc := NewAgentConversationService(tasks, sess, profiles, state, eventer)
	// Deliberately do not call SetDispatcher — models a not-yet-wired boot
	// window (backendapp wires this only once the orchestrator exists).

	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	_, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if code := status.Code(err); code != codes.Unavailable {
		t.Fatalf("got code %v, want Unavailable", code)
	}
}

func TestDispatchBeforeEnsure(t *testing.T) {
	svc, _ := newACTestService()

	_, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Hello", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if code := status.Code(err); code != codes.NotFound {
		t.Fatalf("got code %v, want NotFound", code)
	}
}

func TestDispatchWithOccurrenceKeyDeduplicates(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	key := "wake:cycle:2026-08-17T10:00:00Z"
	first, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check", key)
	if err != nil {
		t.Fatalf("first Dispatch: %v", err)
	}
	if first.Status != "started" {
		t.Fatalf("first status = %q, want started", first.Status)
	}

	second, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check again", key)
	if err != nil {
		t.Fatalf("second Dispatch: %v", err)
	}
	if second.Status != "duplicate_occurrence" {
		t.Fatalf("second status = %q, want duplicate_occurrence", second.Status)
	}

	// Only one call must have reached the runtime — a duplicate occurrence
	// must never fire the agent twice.
	if deps.dispatcher.callCount() != 1 {
		t.Fatalf("expected 1 runtime dispatch, got %d", deps.dispatcher.callCount())
	}
}

func TestDispatchOccurrenceKeysAreScopedPerConversation(t *testing.T) {
	svc, deps := newACTestService()

	specA := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	specB := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-2", ConversationKey: "coordinator"}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", specA); err != nil {
		t.Fatalf("Ensure ws-1: %v", err)
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", specB); err != nil {
		t.Fatalf("Ensure ws-2: %v", err)
	}

	key := "wake:cycle:same-timestamp"
	dispatchA, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "A", key)
	if err != nil || dispatchA.Status != "started" {
		t.Fatalf("dispatch ws-1: status=%q err=%v", dispatchA.Status, err)
	}
	dispatchB, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-2", "coordinator", "B", key)
	if err != nil || dispatchB.Status != "started" {
		t.Fatalf("dispatch ws-2 (same occurrence key, different workspace) should not be treated as a duplicate: status=%q err=%v", dispatchB.Status, err)
	}
	if deps.dispatcher.callCount() != 2 {
		t.Fatalf("expected 2 runtime dispatches, got %d", deps.dispatcher.callCount())
	}
}

func TestDispatchConcurrentOccurrenceClaimIsAtomic(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	const attempts = 20
	key := "wake:cycle:concurrent"
	results := make([]pluginsdk.AgentConversationDispatch, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			d, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check", key)
			if err != nil {
				t.Errorf("dispatch %d: %v", idx, err)
				return
			}
			results[idx] = d
		}(i)
	}
	wg.Wait()

	started := 0
	duplicate := 0
	for _, r := range results {
		switch r.Status {
		case "started":
			started++
		case "duplicate_occurrence":
			duplicate++
		default:
			t.Fatalf("unexpected status %q", r.Status)
		}
	}
	if started != 1 {
		t.Fatalf("expected exactly 1 winning dispatch, got %d", started)
	}
	if duplicate != attempts-1 {
		t.Fatalf("expected %d duplicates, got %d", attempts-1, duplicate)
	}
	if deps.dispatcher.callCount() != 1 {
		t.Fatalf("expected exactly 1 runtime dispatch under concurrency, got %d", deps.dispatcher.callCount())
	}
}

func TestDispatchWithBusySession(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	desc, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	deps.sess.setState(desc.SessionID, models.TaskSessionStateRunning)

	dispatch, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check", "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if dispatch.Status != "skipped_busy" {
		t.Fatalf("status = %q, want skipped_busy", dispatch.Status)
	}
	if deps.dispatcher.callCount() != 0 {
		t.Fatal("a busy session must never reach the runtime dispatcher")
	}
}

func TestDispatchWithStartingSessionIsAlsoBusy(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	desc, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	deps.sess.setState(desc.SessionID, models.TaskSessionStateStarting)

	dispatch, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check", "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if dispatch.Status != "skipped_busy" {
		t.Fatalf("status = %q, want skipped_busy", dispatch.Status)
	}
	if deps.dispatcher.callCount() != 0 {
		t.Fatal("a starting session must never reach the runtime dispatcher")
	}
}

func TestDispatchRejectsEmptyArguments(t *testing.T) {
	svc, _ := newACTestService()

	tests := []struct {
		name, plugin, ws, key, text string
	}{
		{"empty plugin", "", "ws-1", "key", "hello"},
		{"empty workspace", "p", "", "key", "hello"},
		{"empty key", "p", "ws-1", "", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Dispatch(context.Background(), tt.plugin, tt.ws, tt.key, tt.text, "")
			if err == nil {
				t.Fatal("expected error")
			}
			if code := status.Code(err); code != codes.InvalidArgument {
				t.Fatalf("got %v, want InvalidArgument", code)
			}
		})
	}
}

func TestDispatchPropagatesRuntimeDeliveryFailure(t *testing.T) {
	svc, deps := newACTestService()

	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	deps.dispatcher.err = errors.New("agent runtime unavailable")

	_, err := svc.Dispatch(context.Background(), "plugin-coordinator", "ws-1", "coordinator", "Check", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── Delete tests ────────────────────────────────────────────────────────

func TestDeleteRemovesConversation(t *testing.T) {
	svc, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	if _, _, err := svc.Ensure(context.Background(), "plugin-coordinator", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	count, err := svc.Delete(context.Background(), "plugin-coordinator", "ws-1", "coordinator")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("deleted count = %d, want 1", count)
	}

	_, statusStr, err := svc.Ensure(context.Background(), "plugin-coordinator", spec)
	if err != nil {
		t.Fatalf("Ensure after delete: %v", err)
	}
	if statusStr != AgentConversationStatusCreated {
		t.Fatalf("status = %q, want created", statusStr)
	}
}

func TestDeleteOnlyOwnedConversations(t *testing.T) {
	svc, _ := newACTestService()

	spec := pluginsdk.AgentConversationSpec{WorkspaceID: "ws-1", ConversationKey: "coordinator"}
	if _, _, err := svc.Ensure(context.Background(), "plugin-a", spec); err != nil {
		t.Fatalf("plugin-a Ensure: %v", err)
	}
	if _, _, err := svc.Ensure(context.Background(), "plugin-b", spec); err != nil {
		t.Fatalf("plugin-b Ensure: %v", err)
	}

	count, err := svc.Delete(context.Background(), "plugin-a", "ws-1", "coordinator")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("deleted = %d, want 1", count)
	}

	_, statusStr, err := svc.Ensure(context.Background(), "plugin-b", spec)
	if err != nil {
		t.Fatalf("plugin-b Ensure: %v", err)
	}
	if statusStr != AgentConversationStatusExists {
		t.Fatalf("plugin-b status = %q, want exists", statusStr)
	}
}

func TestDeleteIdempotent(t *testing.T) {
	svc, _ := newACTestService()

	count, err := svc.Delete(context.Background(), "plugin-nonexistent", "ws-1", "key")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("deleted = %d, want 0", count)
	}
}
