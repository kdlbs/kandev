package acp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// awaitPromptCall reads the next provider prompt call with a bound. A bare
// channel read here would hang the whole package for the go-test timeout when a
// regression stops the steer from transferring the token, instead of failing this
// one test fast.
func awaitPromptCall(t *testing.T, fake *handoffFakeAgent, what string) handoffPromptCall {
	t.Helper()
	select {
	case call := <-fake.entered:
		return call
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s to reach the provider", what)
		return handoffPromptCall{}
	}
}

// awaitPromptErr reads a prompt goroutine's result with a bound.
func awaitPromptErr(t *testing.T, done <-chan error, what string) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s to settle", what)
		return nil
	}
}

// TestSteerReachesProviderWhileTurnGenerating is the core contract: a steer must
// reach the agent while the predecessor's session/prompt is still open, with no
// provider foreground-idle event involved. The handoff fake blocks its first
// prompt until a second one arrives, so the second prompt reaching it at all
// proves the token transferred mid-turn.
func TestSteerReachesProviderWhileTurnGenerating(t *testing.T) {
	a, fake, _ := setupHandoffFakeAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("new session: %v", err)
	}
	if !a.SupportsSteering() {
		t.Fatal("advertised agent does not report steering support")
	}

	firstDone := make(chan error, 1)
	go func() { firstDone <- a.Prompt(ctx, "long running work", nil, 71) }()
	if call := <-fake.entered; call.number != 1 {
		t.Fatalf("first provider prompt call = %d, want 1", call.number)
	}

	steerDone := make(chan error, 1)
	go func() { steerDone <- a.PromptSteer(ctx, "change of plan", nil, 72) }()

	select {
	case call := <-fake.entered:
		if call.number != 2 || call.text != "change of plan" {
			t.Fatalf("steer provider prompt = %+v, want call 2 with the steer text", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("steer did not reach the provider while the predecessor turn was open")
	}

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("predecessor prompt errored after steer handoff: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("predecessor RPC never released after the steer arrived")
	}

	fake.releasePrompts()
	select {
	case err := <-steerDone:
		if err != nil {
			t.Fatalf("steer prompt returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("steer prompt did not complete")
	}
}

// TestSteerAttributesCompletionToSuccessor pins the attribution rule. The
// predecessor's early settlement must not emit a completion of its own, and the
// completion the client sees must carry the steer's generation.
func TestSteerAttributesCompletionToSuccessor(t *testing.T) {
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
	go func() { firstDone <- a.Prompt(ctx, "predecessor", nil, 81) }()
	awaitPromptCall(t, fake, "predecessor prompt")

	steerDone := make(chan error, 1)
	go func() { steerDone <- a.PromptSteer(ctx, "steer", nil, 82) }()
	awaitPromptCall(t, fake, "steer")
	if err := awaitPromptErr(t, firstDone, "predecessor prompt"); err != nil {
		t.Fatalf("predecessor prompt errored after handoff: %v", err)
	}

	fake.releasePrompts()
	if err := <-steerDone; err != nil {
		t.Fatalf("steer prompt: %v", err)
	}

	complete := waitForEventType(t, a, streams.EventTypeComplete)
	if complete.PromptGeneration != 82 {
		t.Fatalf("completion generation = %d, want the steer's 82", complete.PromptGeneration)
	}
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeComplete && event.PromptGeneration == 81 {
			t.Fatal("predecessor emitted a completion for a turn the steer took over")
		}
	}
}

// TestSteerPreservesPredecessorBackgroundWork proves the boundary does not sweep
// work that was live when the turn was handed off. This is required, not
// cosmetic: a backgrounded workload has been observed outliving both prompts and
// reporting only afterwards.
func TestSteerPreservesPredecessorBackgroundWork(t *testing.T) {
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
	go func() { firstDone <- a.Prompt(ctx, "spawn a child then keep working", nil, 91) }()
	awaitPromptCall(t, fake, "predecessor prompt")

	sendCapturedUpdate(t, conn, `{
		"sessionId":"session-handoff",
		"update":{
			"sessionUpdate":"tool_call",
			"toolCallId":"subagent-steer",
			"title":"Agent",
			"kind":"other",
			"status":"in_progress",
			"rawInput":{"description":"child","prompt":"keep working","subagent_type":"general-purpose"},
			"_meta":{"claudeCode":{"toolName":"Agent"}}
		}
	}`)
	launch := waitForEventType(t, a, streams.EventTypeToolCall)
	if launch.NormalizedPayload == nil || !launch.NormalizedPayload.IsActiveBackgroundWork() {
		t.Fatal("child launch was not attested as live background work")
	}

	steerDone := make(chan error, 1)
	go func() { steerDone <- a.PromptSteer(ctx, "steer mid-turn", nil, 92) }()
	awaitPromptCall(t, fake, "steer")
	if err := awaitPromptErr(t, firstDone, "predecessor prompt"); err != nil {
		t.Fatalf("predecessor prompt errored after handoff: %v", err)
	}

	fake.releasePrompts()
	if err := <-steerDone; err != nil {
		t.Fatalf("steer prompt: %v", err)
	}
	waitForEventType(t, a, streams.EventTypeComplete)

	a.mu.RLock()
	_, stillTracked := a.activeToolCalls["subagent-steer"]
	a.mu.RUnlock()
	if !stillTracked {
		t.Fatal("steer handoff swept background work that was live at the boundary")
	}
}

// TestSteerOnIdleSessionActsAsOrdinaryPrompt covers the specified idle case: with
// nothing in flight there is nothing to hand off, and the steer must still be
// delivered rather than erroring or hanging.
func TestSteerOnIdleSessionActsAsOrdinaryPrompt(t *testing.T) {
	a, fake, _ := setupHandoffFakeAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("new session: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- a.PromptSteer(ctx, "first thing said", nil, 101) }()
	select {
	case call := <-fake.entered:
		if call.number != 1 || call.text != "first thing said" {
			t.Fatalf("idle steer reached provider as %+v, want call 1", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("steer on an idle session never reached the provider")
	}
	fake.releasePrompts()
	if err := <-done; err != nil {
		t.Fatalf("idle steer returned error: %v", err)
	}
}

// TestSteerRequiresNegotiatedCapability proves an agent that never advertised
// prompt queueing cannot be steered: the steer must wait for the gate like any
// ordinary prompt instead of transferring the token.
func TestSteerRequiresNegotiatedCapability(t *testing.T) {
	a, fake, _ := setupHandoffFakeAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("new session: %v", err)
	}

	// Revoke the advertisement after initialize to isolate the gate from the
	// fake's wire behavior.
	a.mu.Lock()
	a.promptQueueing = false
	a.mu.Unlock()
	if a.SupportsSteering() {
		t.Fatal("adapter reports steering support without the advertisement")
	}

	firstDone := make(chan error, 1)
	go func() { firstDone <- a.Prompt(ctx, "predecessor", nil, 111) }()
	awaitPromptCall(t, fake, "predecessor prompt")

	steerDone := make(chan error, 1)
	go func() { steerDone <- a.PromptSteer(ctx, "should queue", nil, 112) }()

	select {
	case call := <-fake.entered:
		t.Fatalf("steer reached provider as %+v despite no advertisement; it must wait", call)
	case <-time.After(300 * time.Millisecond):
	}

	fake.releasePrompts()
	if err := awaitPromptErr(t, firstDone, "predecessor prompt"); err != nil {
		t.Fatalf("predecessor prompt: %v", err)
	}
	select {
	case err := <-steerDone:
		if err != nil {
			t.Fatalf("queued steer returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued steer never ran after the predecessor released the gate")
	}
}

// TestSteerCancelReleasesGate covers the cancel scenario: cancelling during an
// in-flight steer must end both prompts and leave the gate free, so the session
// does not stay falsely busy.
func TestSteerCancelReleasesGate(t *testing.T) {
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
	go func() { firstDone <- a.Prompt(ctx, "predecessor", nil, 121) }()
	awaitPromptCall(t, fake, "predecessor prompt")

	steerDone := make(chan error, 1)
	go func() { steerDone <- a.PromptSteer(ctx, "steer", nil, 122) }()
	awaitPromptCall(t, fake, "steer")
	if err := awaitPromptErr(t, firstDone, "predecessor prompt"); err != nil {
		t.Fatalf("predecessor prompt errored after handoff: %v", err)
	}

	// Cancel while the steer owns the turn.
	_ = a.Cancel(ctx)
	fake.releasePrompts()
	select {
	case <-steerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("steer prompt did not settle after cancel")
	}

	// The gate must be free: a following prompt has to be admissible.
	nextDone := make(chan error, 1)
	go func() { nextDone <- a.Prompt(ctx, "after cancel", nil, 123) }()
	select {
	case call := <-fake.entered:
		if call.text != "after cancel" {
			t.Fatalf("post-cancel prompt = %+v, want the follow-up text", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt gate was not released after cancelling a steer")
	}
	fake.releasePrompts()
	select {
	case <-nextDone:
	case <-time.After(2 * time.Second):
		t.Fatal("post-cancel prompt did not settle")
	}
}
