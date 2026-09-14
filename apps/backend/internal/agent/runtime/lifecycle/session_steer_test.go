package lifecycle

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// These tests exercise the mid-turn steer dispatch path
// (SessionManager.SendPromptSteerWithDispatchCallback). The invariant under test:
// a steer reuses the *active dispatched* prompt generation and never serializes
// behind it — so the operator's input reaches the still-generating turn — while
// a steer with no live turn (idle, pre-dispatch, or already-completed) returns a
// no-dispatch result for ordinary admission to handle in submission order.

type capturedPrompt struct {
	text       string
	generation uint64
	steer      bool
}

type blockingAttachmentReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingAttachmentReader) OpenClaimed(context.Context, string, string, string) (io.ReadCloser, string, string, int64, error) {
	return &blockingAttachmentReadCloser{reader: r, data: strings.NewReader("attachment bytes")}, "bundle.zip", "application/zip", 16, nil
}

type blockingAttachmentReadCloser struct {
	reader *blockingAttachmentReader
	data   *strings.Reader
}

func (r *blockingAttachmentReadCloser) Read(p []byte) (int, error) {
	r.reader.once.Do(func() { close(r.reader.started) })
	<-r.reader.release
	return r.data.Read(p)
}

func (r *blockingAttachmentReadCloser) Close() error {
	return nil
}

// promptProbe captures every agent.prompt frame the mock receives so a test can
// assert the wire-level generation and steer flag.
type promptProbe struct {
	mu           sync.Mutex
	seen         []capturedPrompt
	ch           chan capturedPrompt
	dropSteerAck bool // when set, steer frames get no response (simulates a stalled ack)
}

func newPromptProbe() *promptProbe {
	return &promptProbe{ch: make(chan capturedPrompt, 8)}
}

func (p *promptProbe) setDropSteerAck(drop bool) {
	p.mu.Lock()
	p.dropSteerAck = drop
	p.mu.Unlock()
}

func (p *promptProbe) install(mock *mockAgentServer) {
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.prompt" {
			var payload struct {
				Text             string `json:"text"`
				PromptGeneration uint64 `json:"prompt_generation"`
				Steer            bool   `json:"steer"`
			}
			_ = msg.ParsePayload(&payload)
			cp := capturedPrompt{payload.Text, payload.PromptGeneration, payload.Steer}
			p.mu.Lock()
			drop := p.dropSteerAck && payload.Steer
			p.seen = append(p.seen, cp)
			p.mu.Unlock()
			p.ch <- cp
			if drop {
				return nil // no response — the client's RPC waits until its deadline
			}
		}
		return mock.defaultHandler(msg)
	}
}

func (p *promptProbe) next(t *testing.T) capturedPrompt {
	t.Helper()
	select {
	case cp := <-p.ch:
		return cp
	case <-time.After(2 * time.Second):
		t.Fatal("no agent.prompt frame reached the mock in time")
		return capturedPrompt{}
	}
}

// steerHarness wires a manager, a connected agentctl client, and one tracked
// execution against the mock agent server.
type steerHarness struct {
	mgr   *Manager
	exec  *AgentExecution
	probe *promptProbe
	mock  *mockAgentServer
	ctx   context.Context
}

func newSteerHarness(t *testing.T) *steerHarness {
	t.Helper()
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	probe := newPromptProbe()
	probe.install(mock)

	mgr, _ := createTestManagerWithTracking()
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil); err != nil {
		t.Fatalf("connect stream: %v", err)
	}
	waitForWSConnected(t, mock)

	exec := createTestExecution("exec-steer", "task-steer", "session-steer")
	exec.agentctl = client
	// Attachment materialization is addressed to the ACP session and refuses to
	// run without one, so every steer harness carries a session identity and a
	// reader even when the test under it sends no attachments.
	exec.ACPSessionID = "acp-session-steer"
	mgr.sessionManager.SetAttachmentReader(testAttachmentReader{})
	if err := mgr.executionStore.Add(exec); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	return &steerHarness{mgr: mgr, exec: exec, probe: probe, mock: mock, ctx: ctx}
}

// startForegroundTurn dispatches an ordinary prompt and blocks it in flight
// (its SendPrompt holds promptMu and waits for completion), returning a channel
// that receives when that predecessor turn finally settles. It waits until the
// generation is marked dispatched so a steer can legitimately reuse it.
func (h *steerHarness) startForegroundTurn(t *testing.T, wantGen uint64) chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := h.mgr.sessionManager.SendPrompt(h.ctx, h.exec, "foreground", false, nil, false)
		done <- err
	}()
	// The predecessor's frame reaches the mock, then markPromptDispatched runs.
	h.probe.next(t)
	waitForActiveGeneration(t, h.mgr, h.exec.ID, wantGen)
	return done
}

func waitForActiveGeneration(t *testing.T, mgr *Manager, execID string, want uint64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mgr.executionStore.ActivePromptGeneration(execID) == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("active prompt generation never reached %d (got %d)",
		want, mgr.executionStore.ActivePromptGeneration(execID))
}

// TestSendPromptSteer_ReusesActiveGenerationWithoutBlocking is the core proof:
// a steer delivered while the foreground turn is still generating must NOT wait
// on the predecessor's promptMu, must reach agentctl as a steer carrying the
// predecessor's generation, must not begin a new generation, and the single
// completion for that generation must wake the predecessor's waiter exactly once.
func TestSendPromptSteer_ReusesActiveGenerationWithoutBlocking(t *testing.T) {
	h := newSteerHarness(t)
	firstDone := h.startForegroundTurn(t, 1)

	steerRes := make(chan *PromptResult, 1)
	steerErr := make(chan error, 1)
	go func() {
		res, err := h.mgr.sessionManager.SendPromptSteerWithDispatchCallback(
			h.ctx, h.exec, "steer now", false, nil, true, nil,
		)
		if err != nil {
			steerErr <- err
			return
		}
		steerRes <- res
	}()

	// The predecessor still holds promptMu; the steer must return regardless.
	select {
	case res := <-steerRes:
		if res == nil || res.StopReason != PromptStopReasonDispatched {
			t.Fatalf("steer result = %+v, want dispatched", res)
		}
	case err := <-steerErr:
		t.Fatalf("steer errored: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("steer blocked behind the predecessor's promptMu — mid-turn delivery defeated")
	}

	// It reached agentctl as a steer on the predecessor's generation.
	sp := h.probe.next(t)
	if !sp.steer {
		t.Fatalf("steer frame carried steer=%v, want true", sp.steer)
	}
	if sp.generation != 1 {
		t.Fatalf("steer frame generation = %d, want 1 (reuse the predecessor)", sp.generation)
	}

	// No new generation was begun by the steer.
	if !h.mgr.executionStore.OwnsPromptGeneration(h.exec.SessionID, h.exec.ID, 1) {
		t.Fatal("steer changed the active generation instead of reusing it")
	}

	// One completion for generation 1 wakes the predecessor's waiter exactly once.
	h.mgr.handleAgentEvent(h.exec, agentctl.AgentEvent{
		Type:             streams.EventTypeComplete,
		PromptGeneration: 1,
		Data:             map[string]any{"stop_reason": "end_turn"},
	})
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("predecessor turn returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the single generation-1 completion did not wake the predecessor waiter")
	}
}

// TestSendPromptSteer_ReturnsNotDispatchedWhenNoTurnInFlight proves an idle
// steer-capable session leaves ordinary prompt admission to the orchestrator.
func TestSendPromptSteer_ReturnsNotDispatchedWhenNoTurnInFlight(t *testing.T) {
	h := newSteerHarness(t)

	if got := h.mgr.executionStore.ActivePromptGeneration(h.exec.ID); got != 0 {
		t.Fatalf("precondition: active generation = %d, want 0", got)
	}

	res, err := h.mgr.sessionManager.SendPromptSteerWithDispatchCallback(
		h.ctx, h.exec, "no turn yet", false, nil, true, nil,
	)
	if !errors.Is(err, ErrSteerNotDispatched) {
		t.Fatalf("steer result = %+v, error = %v, want ErrSteerNotDispatched", res, err)
	}
	select {
	case frame := <-h.probe.ch:
		t.Fatalf("idle steer dispatched a prompt outside ordinary admission: %+v", frame)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestSendPromptSteer_ReturnsNotDispatchedAfterPredecessorCompleted proves a
// steer that arrives after the foreground turn already finished does not attach
// to that dead generation or dispatch around ordinary admission.
func TestSendPromptSteer_ReturnsNotDispatchedAfterPredecessorCompleted(t *testing.T) {
	h := newSteerHarness(t)
	firstDone := h.startForegroundTurn(t, 1)

	h.mgr.handleAgentEvent(h.exec, agentctl.AgentEvent{
		Type:             streams.EventTypeComplete,
		PromptGeneration: 1,
		Data:             map[string]any{"stop_reason": "end_turn"},
	})
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("predecessor turn did not settle")
	}
	if got := h.mgr.executionStore.ActivePromptGeneration(h.exec.ID); got != 0 {
		t.Fatalf("completed generation still reported active: %d", got)
	}

	res, err := h.mgr.sessionManager.SendPromptSteerWithDispatchCallback(
		h.ctx, h.exec, "after done", false, nil, true, nil,
	)
	if !errors.Is(err, ErrSteerNotDispatched) {
		t.Fatalf("steer result = %+v, error = %v, want ErrSteerNotDispatched", res, err)
	}
	select {
	case frame := <-h.probe.ch:
		t.Fatalf("post-completion steer dispatched around ordinary admission: %+v", frame)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestSendPromptSteer_DoesNotOvertakeAdmittedButUndispatchedTurn proves the
// pre-dispatch race is closed with a real predecessor: while a genuine prompt is
// pinned in the admitted-but-undispatched window (generation begun, prompt not
// yet sent), a racing steer must not bind to it or overtake it. The lifecycle
// path returns without sending; the orchestrator owns the ordinary fallback.
func TestSendPromptSteer_DoesNotOvertakeAdmittedButUndispatchedTurn(t *testing.T) {
	h := newSteerHarness(t)

	// Pin the first (predecessor) sendPrompt in the window between BeginPrompt and
	// dispatch.
	var once sync.Once
	predecessorPinned := make(chan struct{})
	releasePredecessor := make(chan struct{})
	h.mgr.sessionManager.beforePromptDispatchHook = func() {
		once.Do(func() {
			close(predecessorPinned)
			<-releasePredecessor
		})
	}

	predecessorDone := make(chan error, 1)
	go func() {
		_, err := h.mgr.sessionManager.SendPrompt(h.ctx, h.exec, "predecessor", false, nil, true)
		predecessorDone <- err
	}()
	<-predecessorPinned // generation 1 admitted, not yet dispatched, promptMu held

	// The steer arrives now. activePromptGeneration is 0 (gen 1 not marked
	// dispatched), so it returns without sending a frame.
	_, steerErr := h.mgr.sessionManager.SendPromptSteerWithDispatchCallback(
		h.ctx, h.exec, "racing steer", false, nil, true, nil,
	)
	if !errors.Is(steerErr, ErrSteerNotDispatched) {
		t.Fatalf("steer error = %v, want ErrSteerNotDispatched", steerErr)
	}

	// No frame of any kind can reach the agent while the predecessor is pinned:
	// the predecessor hasn't dispatched and the steer is blocked behind it.
	select {
	case cp := <-h.probe.ch:
		t.Fatalf("a prompt reached the agent while the predecessor was pinned: %+v", cp)
	case <-time.After(100 * time.Millisecond):
	}

	close(releasePredecessor)
	if err := <-predecessorDone; err != nil {
		t.Fatalf("predecessor returned error: %v", err)
	}

	// The predecessor dispatched its ordinary generation 1 first.
	first := h.probe.next(t)
	if first.steer || first.generation != 1 {
		t.Fatalf("first frame = %+v, want ordinary generation 1 (predecessor)", first)
	}

	// The predecessor left a pending dispatched prompt. No successor was sent by
	// this lifecycle call; the orchestrator must decide when to admit it.
	h.mgr.handleAgentEvent(h.exec, agentctl.AgentEvent{
		Type:             streams.EventTypeComplete,
		PromptGeneration: 1,
		Data:             map[string]any{"stop_reason": "end_turn"},
	})
	select {
	case frame := <-h.probe.ch:
		t.Fatalf("lifecycle sent a successor frame without ordinary admission: %+v", frame)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestSendPromptSteer_GenerationCannotCompleteBetweenReadAndDispatch closes the
// TOCTOU: while the steer holds promptLifecycleMu (after selecting the active
// generation, before the RPC), a completion for that generation must not be
// claimable — so the steer always dispatches against a still-live turn.
func TestSendPromptSteer_GenerationCannotCompleteBetweenReadAndDispatch(t *testing.T) {
	h := newSteerHarness(t)
	firstDone := h.startForegroundTurn(t, 1)

	gateEntered := make(chan struct{})
	release := make(chan struct{})
	h.mgr.sessionManager.beforeSteerDispatchHook = func() {
		close(gateEntered)
		<-release
	}

	steerDone := make(chan struct{})
	go func() {
		_, _ = h.mgr.sessionManager.SendPromptSteerWithDispatchCallback(
			h.ctx, h.exec, "steer", false, nil, true, nil,
		)
		close(steerDone)
	}()
	<-gateEntered // steer holds promptLifecycleMu with generation 1 selected

	// Drive a completion for generation 1 concurrently. It must block on
	// promptLifecycleMu (via claimPromptCompletion) until the steer releases.
	completionApplied := make(chan struct{})
	go func() {
		h.mgr.handleAgentEvent(h.exec, agentctl.AgentEvent{
			Type:             streams.EventTypeComplete,
			PromptGeneration: 1,
			Data:             map[string]any{"stop_reason": "end_turn"},
		})
		close(completionApplied)
	}()
	select {
	case <-completionApplied:
		t.Fatal("completion was claimed while the steer held the lock — TOCTOU not closed")
	case <-time.After(100 * time.Millisecond):
	}

	close(release) // steer sends the RPC and releases the lock
	<-steerDone

	sp := h.probe.next(t)
	if !sp.steer || sp.generation != 1 {
		t.Fatalf("steer frame = %+v, want steer=true gen=1 dispatched before completion", sp)
	}

	<-completionApplied // now the completion proceeds
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("predecessor returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("predecessor waiter did not wake after the completion")
	}
}

// TestSendPromptSteer_StalledAckReleasesLockWithinTimeout proves the steer never
// pins promptLifecycleMu on a stalled agent: with the agent never acknowledging
// the steer, the bounded dispatch returns within steerDispatchTimeout and the
// predecessor's completion (which needs the same lock) is still claimed after.
func TestSendPromptSteer_StalledAckReleasesLockWithinTimeout(t *testing.T) {
	h := newSteerHarness(t)
	firstDone := h.startForegroundTurn(t, 1)

	prev := steerDispatchTimeout
	steerDispatchTimeout = 200 * time.Millisecond
	t.Cleanup(func() { steerDispatchTimeout = prev })
	h.probe.setDropSteerAck(true)

	steerReturned := make(chan error, 1)
	go func() {
		_, err := h.mgr.sessionManager.SendPromptSteerWithDispatchCallback(
			h.ctx, h.exec, "steer", false, nil, true, nil,
		)
		steerReturned <- err
	}()

	select {
	case err := <-steerReturned:
		if err == nil {
			t.Fatal("stalled steer returned nil; expected the bounded dispatch to error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("steer did not return within the dispatch timeout — lifecycle lock pinned")
	}

	// The lock was released: the predecessor's completion can still be claimed.
	h.mgr.handleAgentEvent(h.exec, agentctl.AgentEvent{
		Type:             streams.EventTypeComplete,
		PromptGeneration: 1,
		Data:             map[string]any{"stop_reason": "end_turn"},
	})
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("predecessor returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("predecessor completion wedged — steer did not release promptLifecycleMu")
	}
}

// TestSendPromptSteer_NilAgentctlClientErrors guards the dedicated path's
// precondition without a connected client.
func TestSendPromptSteer_NilAgentctlClientErrors(t *testing.T) {
	sm := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
	exec := &AgentExecution{ID: "exec-1", promptDoneCh: make(chan PromptCompletionSignal, 1)}
	_, err := sm.SendPromptSteerWithDispatchCallback(
		context.Background(), exec, "x", false, nil, true, nil,
	)
	if err == nil {
		t.Fatal("expected an error when the execution has no agentctl client")
	}
}

// claimedAttachment is the bounded descriptor shape the prompt and queue
// protocols carry. Its bytes live in backend storage until materialization
// streams them to agentctl.
func claimedAttachment() []v1.MessageAttachment {
	return []v1.MessageAttachment{
		{AttachmentID: "attachment-steer", Type: "resource", Name: "bundle.zip", SizeBytes: 16},
	}
}

// TestSendPromptSteer_MaterializesAttachmentsBeforeDispatch is the core
// regression: a steer folded into a still-generating turn must stream its
// claimed bytes to agentctl before the prompt frame goes out. Without that, the
// agent receives a reference to a file that was never written into the session
// and the attachment silently reads as empty.
func TestSendPromptSteer_MaterializesAttachmentsBeforeDispatch(t *testing.T) {
	h := newSteerHarness(t)
	done := h.startForegroundTurn(t, 1)

	result, err := h.mgr.sessionManager.SendPromptSteerWithDispatchCallback(
		h.ctx, h.exec, "look at this file", false, claimedAttachment(), false, nil,
	)
	if err != nil {
		t.Fatalf("steer with attachment: %v", err)
	}
	if result.StopReason != PromptStopReasonDispatched {
		t.Fatalf("stop reason = %q, want %q", result.StopReason, PromptStopReasonDispatched)
	}

	steered := h.probe.next(t)
	if !steered.steer {
		t.Fatalf("frame was not a steer: %+v", steered)
	}

	uploads := h.mock.materializedUploads()
	if len(uploads) != 1 {
		t.Fatalf("materialized uploads = %d, want 1", len(uploads))
	}
	if uploads[0].attachmentID != "attachment-steer" {
		t.Errorf("attachment_id = %q", uploads[0].attachmentID)
	}
	if uploads[0].sessionID != "acp-session-steer" {
		t.Errorf("session_id = %q", uploads[0].sessionID)
	}
	if uploads[0].body != "attachment bytes" {
		t.Errorf("uploaded body = %q", uploads[0].body)
	}

	h.mgr.handleAgentEvent(h.exec, agentctl.AgentEvent{
		Type:             streams.EventTypeComplete,
		PromptGeneration: 1,
		Data:             map[string]any{"stop_reason": "end_turn"},
	})
	<-done
}

// TestSendPromptSteer_DoesNotFallbackOutsideAdmission proves that a turn which
// ends while attachment bytes are materializing does not dispatch a low-level
// ordinary prompt after the steer has lost its generation. The caller must
// return a no-dispatch result so the orchestrator can re-enter normal prompt
// admission and preserve ordering with concurrent messages.
func TestSendPromptSteer_DoesNotFallbackOutsideAdmission(t *testing.T) {
	h := newSteerHarness(t)
	done := h.startForegroundTurn(t, 1)
	reader := &blockingAttachmentReader{started: make(chan struct{}), release: make(chan struct{})}
	h.mgr.sessionManager.SetAttachmentReader(reader)

	steerDone := make(chan struct {
		result *PromptResult
		err    error
	}, 1)
	go func() {
		result, err := h.mgr.sessionManager.SendPromptSteerWithDispatchCallback(
			h.ctx, h.exec, "racing attachment", false, claimedAttachment(), true, nil,
		)
		steerDone <- struct {
			result *PromptResult
			err    error
		}{result: result, err: err}
	}()
	<-reader.started

	h.mgr.handleAgentEvent(h.exec, agentctl.AgentEvent{
		Type:             streams.EventTypeComplete,
		PromptGeneration: 1,
		Data:             map[string]any{"stop_reason": "end_turn"},
	})
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("predecessor turn returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("predecessor turn did not settle while attachment materialized")
	}

	close(reader.release)
	select {
	case call := <-steerDone:
		if !errors.Is(call.err, ErrSteerNotDispatched) {
			t.Fatalf("steer result = %+v, error = %v, want ErrSteerNotDispatched", call.result, call.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("steer did not return after attachment materialization")
	}
	select {
	case frame := <-h.probe.ch:
		t.Fatalf("steer sent a fallback prompt outside orchestrator admission: %+v", frame)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestSendPrompt_MaterializesAttachmentsExactlyOnce covers the ordinary route.
// It materializes a claimed descriptor once before dispatching the prompt.
func TestSendPrompt_MaterializesAttachmentsExactlyOnce(t *testing.T) {
	h := newSteerHarness(t)

	if _, err := h.mgr.sessionManager.SendPrompt(
		h.ctx, h.exec, "ordinary prompt", false, claimedAttachment(), true,
	); err != nil {
		t.Fatalf("ordinary prompt: %v", err)
	}

	dispatched := h.probe.next(t)
	if dispatched.steer {
		t.Fatalf("ordinary frame should not be a steer: %+v", dispatched)
	}
	if uploads := h.mock.materializedUploads(); len(uploads) != 1 {
		t.Fatalf("materialized uploads = %d, want exactly 1", len(uploads))
	}
}

// TestSendPromptSteer_MaterializationFailureDoesNotDispatch pins the failure
// contract. A steer that cannot resolve its bytes must surface an error rather
// than deliver a dangling reference, and must not fall through to an ordinary
// prompt carrying the same unresolved descriptor.
func TestSendPromptSteer_MaterializationFailureDoesNotDispatch(t *testing.T) {
	h := newSteerHarness(t)
	done := h.startForegroundTurn(t, 1)
	h.mock.setFailMaterialize(true)

	_, err := h.mgr.sessionManager.SendPromptSteerWithDispatchCallback(
		h.ctx, h.exec, "look at this file", false, claimedAttachment(), false, nil,
	)
	if err == nil {
		t.Fatal("steer with failing materialization returned no error")
	}
	if !errors.Is(err, ErrSteerAttachmentMaterialization) {
		t.Fatalf("error = %v, want ErrSteerAttachmentMaterialization", err)
	}
	if !strings.Contains(err.Error(), "materialize attachment") {
		t.Fatalf("error = %v, want a materialization error", err)
	}

	select {
	case frame := <-h.probe.ch:
		t.Fatalf("a prompt frame dispatched despite the failure: %+v", frame)
	case <-time.After(200 * time.Millisecond):
	}

	h.mgr.handleAgentEvent(h.exec, agentctl.AgentEvent{
		Type:             streams.EventTypeComplete,
		PromptGeneration: 1,
		Data:             map[string]any{"stop_reason": "end_turn"},
	})
	<-done
}
