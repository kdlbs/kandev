package acp

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// handoffFakeAgent models claude-agent-acp's detached-child behavior: the
// original session/prompt remains open after the human model cycle goes idle,
// then returns only after the bridge receives the next human prompt echo.
// The replacement prompt remains open while child updates continue streaming.
type handoffFakeAgent struct {
	concurrencyFakeAgent
	entered       chan handoffPromptCall
	replacementIn chan struct{}
	release       chan struct{}
	releaseOnce   sync.Once
	promptCount   atomic.Int32
}

type handoffPromptCall struct {
	number int
	text   string
}

func (f *handoffFakeAgent) releasePrompts() {
	f.releaseOnce.Do(func() { close(f.release) })
}

func (f *handoffFakeAgent) Prompt(_ context.Context, request sdk.PromptRequest) (sdk.PromptResponse, error) {
	call := int(f.promptCount.Add(1))
	var text string
	if len(request.Prompt) > 0 && request.Prompt[0].Text != nil {
		text = request.Prompt[0].Text.Text
	}
	f.entered <- handoffPromptCall{number: call, text: text}
	switch call {
	case 1:
		select {
		case <-f.replacementIn:
		case <-f.release:
		}
	case 2:
		close(f.replacementIn)
		<-f.release
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

func (*handoffFakeAgent) NewSession(
	context.Context,
	sdk.NewSessionRequest,
) (sdk.NewSessionResponse, error) {
	return sdk.NewSessionResponse{SessionId: "session-handoff"}, nil
}

func setupHandoffFakeAgent(
	t *testing.T,
) (*Adapter, *handoffFakeAgent, *sdk.AgentSideConnection) {
	t.Helper()

	a := newTestAdapter()
	a.agentID = claudeAgentID
	a.normalizer = NewNormalizer(claudeAgentID)
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()

	fake := &handoffFakeAgent{
		entered:       make(chan handoffPromptCall, 4),
		replacementIn: make(chan struct{}),
		release:       make(chan struct{}),
	}
	if err := a.Connect(clientToAgentW, agentToClientR); err != nil {
		t.Fatalf("connect adapter: %v", err)
	}
	conn := sdk.NewAgentSideConnection(fake, agentToClientW, clientToAgentR)
	t.Cleanup(func() {
		fake.releasePrompts()
		_ = a.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
	})
	return a, fake, conn
}

func sendCapturedUpdate(
	t *testing.T,
	conn *sdk.AgentSideConnection,
	raw string,
) {
	t.Helper()
	var notification sdk.SessionNotification
	if err := json.Unmarshal([]byte(raw), &notification); err != nil {
		t.Fatalf("decode captured notification: %v", err)
	}
	if err := conn.SessionUpdate(context.Background(), notification); err != nil {
		t.Fatalf("send captured notification: %v", err)
	}
}

func waitForEventType(t *testing.T, a *Adapter, eventType string) AgentEvent {
	t.Helper()
	for {
		select {
		case event := <-a.updatesCh:
			if event.Type == eventType {
				return event
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %q event", eventType)
		}
	}
}

func TestHumanPromptHandoffAfterAuthoritativeForegroundIdle(t *testing.T) {
	a, fake, conn := setupHandoffFakeAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("new session: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- a.Prompt(ctx, "launch a detached child", nil, 41)
	}()
	if call := <-fake.entered; call.number != 1 {
		t.Fatalf("first provider prompt call = %d, want 1", call.number)
	}

	sendCapturedUpdate(t, conn, `{
		"sessionId":"session-handoff",
		"update":{
			"sessionUpdate":"tool_call",
			"toolCallId":"subagent-1",
			"title":"Agent",
			"kind":"other",
			"status":"in_progress",
			"rawInput":{"description":"detached child","prompt":"keep working","subagent_type":"general-purpose"},
			"_meta":{"claudeCode":{"toolName":"Agent"}}
		}
	}`)
	subagent := waitForEventType(t, a, streams.EventTypeToolCall)
	if subagent.NormalizedPayload == nil || !subagent.NormalizedPayload.IsActiveBackgroundWork() {
		t.Fatal("captured detached child launch was not adapter-attested as live background work")
	}

	sendCapturedUpdate(t, conn, `{
		"sessionId":"session-handoff",
		"update":{
			"sessionUpdate":"usage_update",
			"size":1000000,
			"used":23638,
			"_meta":{"_claude/origin":{"kind":"human"}}
		}
	}`)
	idle := waitForEventType(t, a, streams.EventTypeForegroundIdle)
	if idle.PromptGeneration != 41 {
		t.Fatalf("foreground-idle generation = %d, want 41", idle.PromptGeneration)
	}
	if handoff, _ := idle.Data[streams.AgentEventDataPromptHandoff].(bool); !handoff {
		t.Fatal("authoritative Claude foreground-idle did not attest prompt handoff")
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- a.Prompt(ctx, "human follow-up", nil, 42)
	}()
	select {
	case call := <-fake.entered:
		if call.number != 2 || call.text != "human follow-up" {
			t.Fatalf("replacement provider prompt = %+v, want call 2 with human follow-up", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("human follow-up did not reach provider after authoritative foreground idle")
	}

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("original prompt returned error after handoff: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("original provider RPC did not release after receiving the follow-up echo")
	}

	sendCapturedUpdate(t, conn, `{
		"sessionId":"session-handoff",
		"update":{
			"sessionUpdate":"tool_call",
			"toolCallId":"child-bash-1",
			"title":"Bash",
			"kind":"execute",
			"status":"in_progress",
			"rawInput":{"command":"printf child-stream"},
			"_meta":{"claudeCode":{"toolName":"Bash","parentToolUseId":"subagent-1"}}
		}
	}`)
	child := waitForEventType(t, a, streams.EventTypeToolCall)
	if child.ParentToolCallID != "subagent-1" {
		t.Fatalf("streaming child parent = %q, want subagent-1", child.ParentToolCallID)
	}

	fake.releasePrompts()
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("follow-up prompt returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follow-up prompt did not complete")
	}

	complete := waitForEventType(t, a, streams.EventTypeComplete)
	if complete.PromptGeneration != 42 {
		t.Fatalf("delivered completion generation = %d, want follow-up generation 42", complete.PromptGeneration)
	}
	a.mu.RLock()
	_, parentStillTracked := a.activeToolCalls["subagent-1"]
	_, childStillTracked := a.activeToolCalls["child-bash-1"]
	a.mu.RUnlock()
	if !parentStillTracked || !childStillTracked {
		t.Fatalf(
			"follow-up completion dropped predecessor background lineage: parent=%t child=%t",
			parentStillTracked,
			childStillTracked,
		)
	}
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeComplete && event.PromptGeneration == 41 {
			t.Fatal("held original RPC emitted a stale completion after handoff")
		}
	}
}

func TestSyntheticWakeupCannotConsumeHumanPromptHandoff(t *testing.T) {
	a, fake, conn := setupHandoffFakeAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	sessionID, err := a.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- a.Prompt(ctx, "first human prompt", nil, 1)
	}()
	if call := <-fake.entered; call.number != 1 {
		t.Fatalf("first provider prompt call = %d, want 1", call.number)
	}

	sendCapturedUpdate(t, conn, `{
		"sessionId":"session-handoff",
		"update":{
			"sessionUpdate":"usage_update",
			"size":1000000,
			"used":23638,
			"_meta":{"_claude/origin":{"kind":"human"}}
		}
	}`)
	_ = waitForEventType(t, a, streams.EventTypeForegroundIdle)

	a.fireWakeup(sessionID, "synthetic wakeup")
	select {
	case call := <-fake.entered:
		t.Fatalf("synthetic prompt consumed human handoff: %+v", call)
	case <-time.After(100 * time.Millisecond):
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- a.Prompt(ctx, "second human prompt", nil, 2)
	}()
	select {
	case call := <-fake.entered:
		if call.number != 2 || call.text != "second human prompt" {
			t.Fatalf("handoff successor = %+v, want second human prompt", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("human successor did not inherit prompt handoff")
	}
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first prompt: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first prompt did not return after human successor entered")
	}

	fake.releasePrompts()
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second prompt: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second prompt did not return")
	}
	select {
	case call := <-fake.entered:
		if call.number != 3 || call.text != "synthetic wakeup" {
			t.Fatalf("serialized wakeup = %+v, want call 3", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("synthetic wakeup did not run after the human successor completed")
	}
}

func TestHumanFollowupWaitsWithoutForegroundHandoff(t *testing.T) {
	a, fake, _ := setupHandoffFakeAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("new session: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- a.Prompt(ctx, "provider-owned foreground", nil, 1)
	}()
	<-fake.entered

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- a.Prompt(ctx, "queued follow-up", nil, 2)
	}()
	select {
	case call := <-fake.entered:
		t.Fatalf("follow-up reached provider without foreground handoff: %+v", call)
	case <-time.After(100 * time.Millisecond):
	}

	fake.releasePrompts()
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first prompt: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first prompt did not return")
	}
	select {
	case call := <-fake.entered:
		if call.number != 2 || call.text != "queued follow-up" {
			t.Fatalf("queued follow-up = %+v, want call 2", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued follow-up did not run after provider released ownership")
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second prompt: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second prompt did not return")
	}
}

func TestPromptHandoffCapabilityIsAdapterAttested(t *testing.T) {
	for _, test := range []struct {
		agentID string
		want    bool
	}{
		{agentID: claudeAgentID, want: true},
		{agentID: mockAgentID, want: true},
		{agentID: codexAgentID, want: false},
		{agentID: "other-acp", want: false},
	} {
		a := newTestAdapter()
		a.agentID = test.agentID
		if got := a.supportsPromptHandoff(); got != test.want {
			t.Fatalf("agent %q handoff support = %t, want %t", test.agentID, got, test.want)
		}
		_ = a.Close()
	}
}
