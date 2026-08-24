package plugins

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// fakeAgentConversationCleanup is a minimal AgentConversationService double
// tracking, per plugin ID, how many managed conversations it "owns" across
// however many workspaces — exactly the shape DeleteAllForPlugin needs to
// prove without depending on the real internal/task/service implementation
// (that cross-workspace/provenance behavior is covered directly in
// internal/task/service/agent_conversations_dispatch_test.go). Ensure/
// Dispatch/Delete are unused by these tests and panic if called, so a test
// exercising the wrong method fails loudly instead of silently no-op'ing.
type fakeAgentConversationCleanup struct {
	mu        sync.Mutex
	owned     map[string]int // pluginID -> conversation count
	deleteErr error
	calls     int
}

func newFakeAgentConversationCleanup() *fakeAgentConversationCleanup {
	return &fakeAgentConversationCleanup{owned: map[string]int{}}
}

func (f *fakeAgentConversationCleanup) Ensure(context.Context, string, pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	panic("fakeAgentConversationCleanup.Ensure: not exercised by uninstall tests")
}

func (f *fakeAgentConversationCleanup) Dispatch(context.Context, string, string, string, string, string) (pluginsdk.AgentConversationDispatch, error) {
	panic("fakeAgentConversationCleanup.Dispatch: not exercised by uninstall tests")
}

func (f *fakeAgentConversationCleanup) Delete(context.Context, string, string, string) (int32, error) {
	panic("fakeAgentConversationCleanup.Delete: not exercised by uninstall tests")
}

func (f *fakeAgentConversationCleanup) DeleteAllForPlugin(_ context.Context, pluginID string) (int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.deleteErr != nil {
		return 0, f.deleteErr
	}
	count := f.owned[pluginID]
	delete(f.owned, pluginID)
	return int32(count), nil
}

func (f *fakeAgentConversationCleanup) seed(pluginID string, count int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.owned[pluginID] = count
}

func (f *fakeAgentConversationCleanup) remaining(pluginID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.owned[pluginID]
}

// TestServiceUninstallDeletesAgentConversations pins criterion 15: uninstall
// must remove every managed conversation the plugin owns (across whatever
// workspaces DeleteAllForPlugin found them in), not just plugin_state/
// user_state/secrets.
func TestServiceUninstallDeletesAgentConversations(t *testing.T) {
	svc, _, _ := newTestService(t)
	cleanup := newFakeAgentConversationCleanup()
	cleanup.seed("kandev-plugin-coordinator", 2) // e.g. one conversation per workspace
	svc.SetAgentConversations(cleanup)
	installTestPlugin(t, svc, "kandev-plugin-coordinator")

	if err := svc.Uninstall(context.Background(), "kandev-plugin-coordinator"); err != nil {
		t.Fatalf("Uninstall() unexpected error: %v", err)
	}

	if cleanup.calls != 1 {
		t.Fatalf("DeleteAllForPlugin calls = %d, want 1", cleanup.calls)
	}
	if got := cleanup.remaining("kandev-plugin-coordinator"); got != 0 {
		t.Fatalf("remaining owned conversations after uninstall = %d, want 0", got)
	}
}

// TestServiceUninstallLeavesOtherPluginsConversationsAlone is the
// provenance-safety regression at the plugin-lifecycle layer: uninstalling
// one plugin must call DeleteAllForPlugin scoped to ITS OWN id only, never
// touching another installed plugin's managed conversations.
func TestServiceUninstallLeavesOtherPluginsConversationsAlone(t *testing.T) {
	svc, _, _ := newTestService(t)
	cleanup := newFakeAgentConversationCleanup()
	cleanup.seed("kandev-plugin-coordinator", 1)
	cleanup.seed("kandev-plugin-other", 3)
	svc.SetAgentConversations(cleanup)
	installTestPlugin(t, svc, "kandev-plugin-coordinator")
	installTestPlugin(t, svc, "kandev-plugin-other")

	if err := svc.Uninstall(context.Background(), "kandev-plugin-coordinator"); err != nil {
		t.Fatalf("Uninstall() unexpected error: %v", err)
	}

	if got := cleanup.remaining("kandev-plugin-coordinator"); got != 0 {
		t.Fatalf("kandev-plugin-coordinator remaining = %d, want 0", got)
	}
	if got := cleanup.remaining("kandev-plugin-other"); got != 3 {
		t.Fatalf("kandev-plugin-other remaining = %d, want 3 (untouched)", got)
	}
}

// TestServiceUninstallFailsClosedWhenAgentConversationCleanupFails mirrors
// TestServiceUninstallFailsClosedWhenUserStateCleanupFails: a failed
// managed-conversation cleanup must abort uninstall (fail-visible — "failure
// is reported rather than silently orphaning data", criterion 15) rather
// than proceeding to remove the package/record and orphan the hidden
// conversations. The plugin stays installed and stopped so an operator can
// retry.
func TestServiceUninstallFailsClosedWhenAgentConversationCleanupFails(t *testing.T) {
	svc, fsStore, rt := newTestService(t)
	cleanup := newFakeAgentConversationCleanup()
	cleanup.seed("kandev-plugin-coordinator", 1)
	svc.SetAgentConversations(cleanup)
	rec := installTestPlugin(t, svc, "kandev-plugin-coordinator")

	cleanupErr := errors.New("agent conversation store unavailable")
	cleanup.deleteErr = cleanupErr
	deliverer := &fakeDeliverer{}
	svc.SetDeliverer(deliverer)

	err := svc.Uninstall(context.Background(), rec.ID)
	if err == nil || !strings.Contains(err.Error(), cleanupErr.Error()) {
		t.Fatalf("Uninstall() error = %v, want agent-conversation cleanup failure", err)
	}
	if cleanup.calls != 1 {
		t.Fatalf("DeleteAllForPlugin calls after failed uninstall = %d, want 1", cleanup.calls)
	}
	if !rt.stopped(rec.ID) {
		t.Fatal("Uninstall() did not stop the runtime before cleanup failure")
	}
	if _, err := svc.Get(rec.ID); err != nil {
		t.Fatalf("Get() after failed uninstall: %v, want installed record", err)
	}
	if _, err := fsStore.Get(rec.ID); err != nil {
		t.Fatalf("store.Get() after failed uninstall: %v, want installed record", err)
	}

	// Retry after the underlying failure resolves: uninstall completes and
	// the conversations are actually removed.
	cleanup.deleteErr = nil
	if err := svc.Uninstall(context.Background(), rec.ID); err != nil {
		t.Fatalf("retry Uninstall() error: %v", err)
	}
	if cleanup.calls != 2 {
		t.Fatalf("DeleteAllForPlugin calls after retry = %d, want 2", cleanup.calls)
	}
	if got := cleanup.remaining(rec.ID); got != 0 {
		t.Fatalf("remaining owned conversations after successful retry = %d, want 0", got)
	}
	if _, err := svc.Get(rec.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get() after successful retry = %v, want store.ErrNotFound", err)
	}
}

// TestServiceUninstallWithoutAgentConversationsWiredIsNoop proves a bare
// service (agent_conversation never wired — e.g. a Kandev boot where the
// capability is unavailable) still uninstalls cleanly rather than panicking
// or erroring on a nil dependency.
func TestServiceUninstallWithoutAgentConversationsWiredIsNoop(t *testing.T) {
	svc, _, _ := newTestService(t)
	rec := installTestPlugin(t, svc, "kandev-plugin-notes")

	if err := svc.Uninstall(context.Background(), rec.ID); err != nil {
		t.Fatalf("Uninstall() unexpected error with no agent-conversation service wired: %v", err)
	}
}

func TestServiceUninstallRevokesWorkspaceAgentPrincipalsBeforeRemovingPlugin(t *testing.T) {
	svc, fsStore, _ := newTestService(t)
	principals := &fakeWorkspaceAgentPrincipalSource{}
	svc.SetWorkspaceAgentPrincipalSource(principals)
	rec := installTestPlugin(t, svc, "kandev-plugin-coordinator")

	if err := svc.Uninstall(context.Background(), rec.ID); err != nil {
		t.Fatalf("Uninstall() unexpected error: %v", err)
	}
	if principals.revokedPluginID != rec.ID {
		t.Fatalf("revoked plugin ID = %q, want %q", principals.revokedPluginID, rec.ID)
	}
	if _, err := fsStore.Get(rec.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("plugin record after successful revocation/uninstall = %v, want not found", err)
	}
}

func TestServiceUninstallFailsClosedWhenPrincipalRevocationFails(t *testing.T) {
	svc, fsStore, rt := newTestService(t)
	principals := &fakeWorkspaceAgentPrincipalSource{revokeErr: errors.New("principal store unavailable")}
	svc.SetWorkspaceAgentPrincipalSource(principals)
	rec := installTestPlugin(t, svc, "kandev-plugin-coordinator")

	err := svc.Uninstall(context.Background(), rec.ID)
	if err == nil || !strings.Contains(err.Error(), "principal store unavailable") {
		t.Fatalf("Uninstall() error = %v, want principal revocation failure", err)
	}
	if !rt.stopped(rec.ID) {
		t.Fatal("Uninstall() must stop the runtime before revocation failure is returned")
	}
	if _, err := fsStore.Get(rec.ID); err != nil {
		t.Fatalf("plugin record after failed revocation = %v, want preserved", err)
	}
}
