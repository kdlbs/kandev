package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// The conversations.probe action is the fixture's end-to-end exercise of the
// agent_conversation RPCs (Ensure, Dispatch with a unique occurrence key, and
// Delete). It shipped without a caller: no Go test and no e2e spec invoked it,
// so the handler and the SDK manager plumbing behind it were never run. These
// tests drive the action directly so the sequence is exercised, the delete is
// actually issued, and a failure in any of the three steps is reported rather
// than folded into a success response.

// convHost is a Host that also offers an agent-conversation manager, which is
// what a host does for a plugin holding the agent_conversation capability.
type convHost struct {
	fakeHost
	mgr *recordingConvManager
}

func (h *convHost) AgentConversations() pluginsdk.AgentConversationManager { return h.mgr }

type recordingConvManager struct {
	ensureSpec    pluginsdk.AgentConversationSpec
	dispatchKey   string
	dispatchOcc   string
	dispatchText  string
	deleteWS      string
	deleteConvKey string
	deleteCalls   int

	ensureErr   error
	dispatchErr error
	deleteErr   error
	deleteCount int32
}

func (m *recordingConvManager) Ensure(_ context.Context, spec pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	m.ensureSpec = spec
	if m.ensureErr != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", m.ensureErr
	}
	return pluginsdk.AgentConversationDescriptor{
		TaskID:          "task-conv-1",
		SessionID:       "session-conv-1",
		WorkspaceID:     spec.WorkspaceID,
		ConversationKey: spec.ConversationKey,
		AgentProfileID:  "profile-1",
	}, "created", nil
}

func (m *recordingConvManager) Dispatch(_ context.Context, workspaceID, conversationKey, text, occurrenceKey string) (pluginsdk.AgentConversationDispatch, error) {
	m.dispatchKey = conversationKey
	m.dispatchOcc = occurrenceKey
	m.dispatchText = text
	if m.dispatchErr != nil {
		return pluginsdk.AgentConversationDispatch{}, m.dispatchErr
	}
	return pluginsdk.AgentConversationDispatch{
		SessionID: "session-conv-1",
		Status:    "queued",
		Descriptor: pluginsdk.AgentConversationDescriptor{
			TaskID:      "task-conv-1",
			SessionID:   "session-conv-1",
			WorkspaceID: workspaceID,
		},
	}, nil
}

func (m *recordingConvManager) Delete(_ context.Context, workspaceID, conversationKey string) (int32, error) {
	m.deleteCalls++
	m.deleteWS = workspaceID
	m.deleteConvKey = conversationKey
	if m.deleteErr != nil {
		return 0, m.deleteErr
	}
	return m.deleteCount, nil
}

var _ pluginsdk.AgentConversationManager = (*recordingConvManager)(nil)
var _ pluginsdk.AgentConversationHost = (*convHost)(nil)

func newConvProbePlugin(t *testing.T) (*fixturePlugin, *recordingConvManager) {
	t.Helper()
	p := &fixturePlugin{dataDir: t.TempDir()}
	mgr := &recordingConvManager{deleteCount: 1}
	p.SetHost(&convHost{mgr: mgr})
	return p, mgr
}

func probeResult(t *testing.T, resp *pluginsdk.PluginActionResponse) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("probe response was not JSON: %v (body=%s)", err, resp.Body)
	}
	return out
}

func TestConversationProbe_RunsEnsureDispatchAndDelete(t *testing.T) {
	p, mgr := newConvProbePlugin(t)

	resp, err := p.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{
		ActionKey: conversationProbeActionKey,
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "ws-probe"},
	})
	if err != nil {
		t.Fatalf("conversations.probe: %v", err)
	}

	if mgr.ensureSpec.WorkspaceID != "ws-probe" {
		t.Errorf("Ensure workspace = %q, want ws-probe", mgr.ensureSpec.WorkspaceID)
	}
	if mgr.ensureSpec.ConversationKey != conversationProbeKey {
		t.Errorf("Ensure conversation key = %q, want %q", mgr.ensureSpec.ConversationKey, conversationProbeKey)
	}
	// The probe exists to prove Delete is reached; a probe that ensured and
	// dispatched but never deleted would leave a conversation behind on every
	// run and would not exercise the cleanup path at all.
	if mgr.deleteCalls != 1 {
		t.Errorf("Delete calls = %d, want 1", mgr.deleteCalls)
	}
	if mgr.deleteWS != "ws-probe" || mgr.deleteConvKey != conversationProbeKey {
		t.Errorf("Delete(%q, %q), want (ws-probe, %q)", mgr.deleteWS, mgr.deleteConvKey, conversationProbeKey)
	}

	out := probeResult(t, resp)
	if out["deleted_count"] != float64(1) {
		t.Errorf("deleted_count = %v, want 1", out["deleted_count"])
	}
	if out["ensure_status"] != "created" {
		t.Errorf("ensure_status = %v, want created", out["ensure_status"])
	}
	if out["task_id"] != "task-conv-1" {
		t.Errorf("task_id = %v, want task-conv-1", out["task_id"])
	}
}

func TestConversationEnsure_RetainsConversationForWorkspaceAgentChat(t *testing.T) {
	p, mgr := newConvProbePlugin(t)

	resp, err := p.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{
		ActionKey: conversationEnsureActionKey,
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "ws-chat"},
	})
	if err != nil {
		t.Fatalf("conversations.ensure: %v", err)
	}
	if mgr.ensureSpec.WorkspaceID != "ws-chat" {
		t.Errorf("Ensure workspace = %q, want ws-chat", mgr.ensureSpec.WorkspaceID)
	}
	if mgr.ensureSpec.ConversationKey != conversationProbeKey {
		t.Errorf("Ensure conversation key = %q, want %q", mgr.ensureSpec.ConversationKey, conversationProbeKey)
	}
	if mgr.deleteCalls != 0 {
		t.Errorf("Delete calls = %d, want 0 for a retained chat conversation", mgr.deleteCalls)
	}

	out := probeResult(t, resp)
	if out["session_id"] != "session-conv-1" {
		t.Errorf("session_id = %v, want session-conv-1", out["session_id"])
	}
	if out["conversation_key"] != conversationProbeKey {
		t.Errorf("conversation_key = %v, want %s", out["conversation_key"], conversationProbeKey)
	}
}

// Each probe run must claim a distinct occurrence key, otherwise a second run
// would be deduplicated as a repeat of the first and silently dispatch nothing.
func TestConversationProbe_UsesAFreshOccurrenceKeyPerRun(t *testing.T) {
	p, mgr := newConvProbePlugin(t)
	req := &pluginsdk.PluginActionRequest{
		ActionKey: conversationProbeActionKey,
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "ws-probe"},
	}

	if _, err := p.HandleAction(context.Background(), req); err != nil {
		t.Fatalf("first probe: %v", err)
	}
	first := mgr.dispatchOcc
	if _, err := p.HandleAction(context.Background(), req); err != nil {
		t.Fatalf("second probe: %v", err)
	}
	if first == "" || mgr.dispatchOcc == first {
		t.Errorf("occurrence key repeated across runs (%q then %q)", first, mgr.dispatchOcc)
	}
}

// A failure at any step is a failed probe. Reporting success would make the
// e2e assertion this fixture exists to support pass against a broken host.
func TestConversationProbe_ReportsStepFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name  string
		apply func(*recordingConvManager)
	}{
		{"ensure fails", func(m *recordingConvManager) { m.ensureErr = boom }},
		{"dispatch fails", func(m *recordingConvManager) { m.dispatchErr = boom }},
		{"delete fails", func(m *recordingConvManager) { m.deleteErr = boom }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, mgr := newConvProbePlugin(t)
			tc.apply(mgr)
			resp, err := p.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{
				ActionKey: conversationProbeActionKey,
				Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "ws-probe"},
			})
			if err == nil {
				t.Fatalf("probe reported success despite a failing step (resp=%v)", resp)
			}
			if !errors.Is(err, boom) {
				t.Errorf("error did not wrap the underlying failure: %v", err)
			}
		})
	}
}

// A host that does not offer agent conversations (capability not declared)
// must produce a clear error, not a nil dereference.
func TestConversationProbe_HostWithoutConversationsErrors(t *testing.T) {
	p := &fixturePlugin{dataDir: t.TempDir()}
	p.SetHost(&fakeHost{})

	if _, err := p.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{
		ActionKey: conversationProbeActionKey,
		Context:   pluginsdk.VerifiedActionContext{WorkspaceID: "ws-probe"},
	}); err == nil {
		t.Fatal("expected an error when the host does not support agent conversations")
	}
}
