package wakeup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/office/wakeup"
)

// erroringAgentReader is a wakeup.AgentReader double that always fails
// with a non-ErrAgentNotFound error, letting a test drive
// checkPauseGate's middle branch (a transient agent-lookup fault, not "no
// such agent") independently of the sentinel-error and clean-read paths.
type erroringAgentReader struct {
	err error
}

func (e *erroringAgentReader) GetAgentInstance(context.Context, string) (*officemodels.AgentInstance, error) {
	return nil, e.err
}

// fakeWakeupPauseGate is a scripted shared.PauseGate double with
// pop-front semantics, mirroring the convention used by this
// capability's other gate-site tests (office/service, office/scheduler,
// office/routines). calls records every workspaceID the gate was asked
// about, in order — AC-001.7 requires the gate be consulted with the
// derivation site's actual workspace, not just called at all.
type fakeWakeupPauseGate struct {
	active []*officemodels.WorkspacePause
	errs   []error
	calls  []string
}

func (f *fakeWakeupPauseGate) PauseState(_ context.Context, workspaceID string) (*officemodels.WorkspacePause, error) {
	f.calls = append(f.calls, workspaceID)
	var active *officemodels.WorkspacePause
	if len(f.active) > 0 {
		active, f.active = f.active[0], f.active[1:]
	}
	var err error
	if len(f.errs) > 0 {
		err, f.errs = f.errs[0], f.errs[1:]
	}
	return active, err
}

// TestDispatch_BlockedByPause_MarksSkippedWithWorkspacePausedReason
// proves createFreshRun's gate: a confirmed pause marks the
// wakeup-request skipped (reason "workspace_paused", matching the
// literal every other gate site's skip/cancel record uses) instead of
// creating a run, and returns shared.ErrWorkspacePaused.
func TestDispatch_BlockedByPause_MarksSkippedWithWorkspacePausedReason(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedWakeup("w-1", wakeup.SourceSelf, `{"reason":"test"}`)
	gate := &fakeWakeupPauseGate{active: []*officemodels.WorkspacePause{
		{ID: "pause-1", WorkspaceID: "ws-1"},
	}}
	h.dispatcher.SetPauseGate(gate)

	err := h.dispatcher.Dispatch(context.Background(), "w-1")
	if !errors.Is(err, shared.ErrWorkspacePaused) {
		t.Fatalf("err = %v, want shared.ErrWorkspacePaused", err)
	}
	if len(gate.calls) != 1 || gate.calls[0] != "ws-1" {
		t.Fatalf("gate.calls = %v, want [ws-1] — the gate must be asked about the agent's own workspace", gate.calls)
	}

	got, gErr := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if gErr != nil {
		t.Fatalf("get wakeup request: %v", gErr)
	}
	if got.Status != officesqlite.WakeupStatusSkipped {
		t.Errorf("status = %q, want skipped", got.Status)
	}
	if got.Reason != "workspace_paused" {
		t.Errorf("reason = %q, want workspace_paused", got.Reason)
	}
	if got.RunID != "" {
		t.Errorf("run_id = %q, want empty (no run created)", got.RunID)
	}
}

// TestDispatch_PauseGateError_LeavesRequestQueued proves a gate-read
// error fails closed (shared.ErrPauseGateUnavailable) without marking
// the request skipped or claimed — nothing has been written yet, so the
// request is simply left queued for the next dispatch attempt.
func TestDispatch_PauseGateError_LeavesRequestQueued(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedWakeup("w-1", wakeup.SourceSelf, `{"reason":"test"}`)
	h.dispatcher.SetPauseGate(&fakeWakeupPauseGate{errs: []error{errors.New("db unavailable")}})

	err := h.dispatcher.Dispatch(context.Background(), "w-1")
	if !errors.Is(err, shared.ErrPauseGateUnavailable) {
		t.Fatalf("err = %v, want shared.ErrPauseGateUnavailable", err)
	}

	got, gErr := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if gErr != nil {
		t.Fatalf("get wakeup request: %v", gErr)
	}
	if got.Status != officesqlite.WakeupStatusQueued {
		t.Errorf("status = %q, want queued (untouched)", got.Status)
	}
}

// TestDispatch_PauseGateAgentLookupError_FailsClosed proves checkPauseGate's
// middle branch: a non-ErrAgentNotFound failure resolving the wakeup
// request's agent (a transient DB fault, not "this agent doesn't exist")
// fails closed exactly like a PauseState error, distinct from
// ErrAgentNotFound's deliberate ungated-proceed exception one test below.
// This is the specific inversion system-design-02.md's Surface decision
// cites as the reason a governance-table implementation was rejected: a
// gate that fails open on a transient DB fault.
func TestDispatch_PauseGateAgentLookupError_FailsClosed(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedWakeup("w-1", wakeup.SourceSelf, `{"reason":"test"}`)
	dispatcher := wakeup.NewDispatcher(h.repo, &erroringAgentReader{err: errors.New("db unavailable")}, logger.Default())
	dispatcher.SetPauseGate(&fakeWakeupPauseGate{})

	err := dispatcher.Dispatch(context.Background(), "w-1")
	if !errors.Is(err, shared.ErrPauseGateUnavailable) {
		t.Fatalf("err = %v, want shared.ErrPauseGateUnavailable", err)
	}

	got, gErr := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if gErr != nil {
		t.Fatalf("get wakeup request: %v", gErr)
	}
	if got.Status != officesqlite.WakeupStatusQueued {
		t.Errorf("status = %q, want queued (untouched)", got.Status)
	}
}

// TestDispatch_PauseGateAgentNotFound_ProceedsUngated is F41's one
// reachable site: createFreshRun resolves the request's workspace via
// GetAgentInstance directly (it bypasses GetAgentFromConfig), so it is
// the only wakeup-dispatch call that can observe the wrapped
// ErrAgentNotFound sentinel. An agent the dispatcher can no longer find
// must not block indefinitely on a workspace it cannot resolve — launch
// behaviour is unchanged (proceeds ungated), exactly as it was before
// this gate existed.
func TestDispatch_PauseGateAgentNotFound_ProceedsUngated(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	if err := h.repo.CreateWakeupRequest(context.Background(), &officesqlite.WakeupRequest{
		ID: "w-1", AgentProfileID: "agent-does-not-exist", Source: wakeup.SourceSelf, Payload: `{}`,
	}); err != nil {
		t.Fatalf("seed wakeup: %v", err)
	}
	h.dispatcher.SetPauseGate(&fakeWakeupPauseGate{active: []*officemodels.WorkspacePause{
		{ID: "pause-1", WorkspaceID: "ws-1"},
	}})

	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	got, gErr := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if gErr != nil {
		t.Fatalf("get wakeup request: %v", gErr)
	}
	if got.Status != officesqlite.WakeupStatusClaimed {
		t.Errorf("status = %q, want claimed (ungated launch)", got.Status)
	}
}

// TestDispatch_NoPauseGateWired_Unaffected proves the nil-gate default
// (every existing dispatcher test, and any deployment not yet wiring
// SetPauseGate) keeps dispatching exactly as before the kill switch
// existed.
func TestDispatch_NoPauseGateWired_Unaffected(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedWakeup("w-1", wakeup.SourceSelf, `{"reason":"test"}`)

	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	got, gErr := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if gErr != nil {
		t.Fatalf("get wakeup request: %v", gErr)
	}
	if got.Status != officesqlite.WakeupStatusClaimed {
		t.Errorf("status = %q, want claimed", got.Status)
	}
}
