package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

type recordingSessionUpdater struct {
	mu      sync.Mutex
	updates []acp.SessionNotification
}

func (r *recordingSessionUpdater) SessionUpdate(_ context.Context, update acp.SessionNotification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, update)
	return nil
}

func (r *recordingSessionUpdater) snapshot() []acp.SessionNotification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]acp.SessionNotification(nil), r.updates...)
}

func (r *recordingSessionUpdater) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{}, nil
}

func TestCopilotNativeFixtureEmitsRawVendorFrames(t *testing.T) {
	t.Setenv("KANDEV_MOCK_COPILOT_NATIVE", "true")
	output, err := os.CreateTemp(t.TempDir(), "copilot-native-frames-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = output.Close() })
	updater := &recordingSessionUpdater{}
	agent := &mockAgent{conn: updater, rawOut: newSerializedACPWriter(output), copilotNativeSubscribed: true}

	resp, err, handled := agent.handleCopilotNativeFixture(
		context.Background(), acp.SessionId("mock-native-session"), "/e2e:copilot-native",
	)
	if !handled || err != nil {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if resp.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", resp.StopReason)
	}
	if len(updater.snapshot()) != 1 {
		t.Fatalf("session updates = %d, want the terminal text frame", len(updater.snapshot()))
	}

	if _, err := output.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(output)
	var frames []map[string]any
	for scanner.Scan() {
		var frame map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatalf("decode frame %q: %v", scanner.Text(), err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(frames) != 10 {
		t.Fatalf("frames = %d, want 10", len(frames))
	}
	var completions, children, applied, failed int
	for _, frame := range frames {
		if frame["method"] != copilotNativeFixtureMethod {
			t.Fatalf("method = %v, want %q", frame["method"], copilotNativeFixtureMethod)
		}
		params, _ := frame["params"].(map[string]any)
		if params["sessionId"] != "mock-native-session" {
			t.Fatalf("sessionId = %v", params["sessionId"])
		}
		if params["type"] != "session.compaction_complete" {
			continue
		}
		completions++
		if _, ok := params["agentId"]; ok {
			children++
		}
		data, _ := params["data"].(map[string]any)
		if data["success"] == true && data["outcome"] == "applied" {
			applied++
		}
		if data["success"] == false {
			failed++
		}
	}
	if completions != 3 || children != 1 || applied != 3 || failed != 0 {
		t.Fatalf("completions=%d children=%d applied=%d failed=%d", completions, children, applied, failed)
	}
	// The child frame must be the one carrying an agentId.
	for _, frame := range frames {
		params, _ := frame["params"].(map[string]any)
		if params["type"] == "session.compaction_complete" && params["agentId"] != nil {
			if !strings.Contains(params["agentId"].(string), "child") {
				t.Fatalf("unexpected child identity: %v", params["agentId"])
			}
		}
	}
}

func TestCopilotNativeFixtureRequiresOptIn(t *testing.T) {
	t.Setenv("KANDEV_MOCK_COPILOT_NATIVE", "")
	agent := &mockAgent{conn: &recordingSessionUpdater{}}
	if _, _, handled := agent.handleCopilotNativeFixture(
		context.Background(), acp.SessionId("mock-native-session"), "/e2e:copilot-native",
	); handled {
		t.Fatal("fixture must stay off without the explicit environment opt-in")
	}
}

func TestCopilotNativeFixtureRequiresSubscription(t *testing.T) {
	t.Setenv("KANDEV_MOCK_COPILOT_NATIVE", "true")
	updater := &recordingSessionUpdater{}
	agent := &mockAgent{conn: updater}
	if _, err, handled := agent.handleCopilotNativeFixture(
		context.Background(), acp.SessionId("mock-native-session"), "/e2e:copilot-native",
	); !handled || err == nil {
		t.Fatalf("handled=%v err=%v, want a subscription error", handled, err)
	}
}

func TestCopilotNativeSubscriptionRequested(t *testing.T) {
	caps := acp.ClientCapabilities{Meta: map[string]any{
		copilotCapabilityMetaKey: map[string]any{
			"events": []any{
				copilotNativeTypeCompactionStart,
				copilotNativeTypeCompactionComplete,
				copilotNativeTypeUsage,
			},
		},
	}}
	if !copilotNativeSubscriptionRequested(caps) {
		t.Fatal("expected exact subscription to be recognized")
	}
	caps.Meta[copilotCapabilityMetaKey] = map[string]any{
		"events": []any{copilotNativeTypeUsage},
	}
	if copilotNativeSubscriptionRequested(caps) {
		t.Fatal("partial subscription must not enable the fixture")
	}
}

func TestCopilotNativeLiveFramesAddOneDistinctCompletion(t *testing.T) {
	frames := copilotNativeFixtureFrames(false, true)
	if len(frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(frames))
	}
	completions := 0
	for _, frame := range frames {
		if frame.Type == copilotNativeTypeCompactionComplete {
			completions++
			if frame.Data["compactionAttemptId"] != "e2e-copilot-live-attempt" {
				t.Fatalf("live attempt ID = %v", frame.Data["compactionAttemptId"])
			}
		}
	}
	if completions != 1 {
		t.Fatalf("completions = %d, want 1", completions)
	}
}

func TestCopilotNativeContextTierFlagHistoryAndConfigRejection(t *testing.T) {
	t.Setenv("KANDEV_MOCK_COPILOT_NATIVE", "true")

	tier, err := parseContextTierFlag([]string{"mock-agent", "--acp", "--model", "mock-fast", "--context", "long_context"})
	if err != nil || tier != "long_context" {
		t.Fatalf("parseContextTierFlag(--context long_context) = (%q, %v), want (\"long_context\", nil)", tier, err)
	}
	if _, err := parseContextTierFlag([]string{"mock-agent", "--context=2m"}); err == nil {
		t.Fatal("parseContextTierFlag(--context=2m) must reject unsupported context tiers")
	}

	firstUpdater := &recordingSessionUpdater{}
	firstAgent := &mockAgent{
		conn:        firstUpdater,
		model:       modelFast,
		contextTier: "default",
		sessions:    make(map[acp.SessionId]bool),
	}
	newResp, err := firstAgent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() {
		_, _ = firstAgent.CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: newResp.SessionId})
	})

	_, setErr := firstAgent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: newResp.SessionId,
			ConfigId:  "context_tier",
			Value:     "long_context",
		},
	})
	if setErr == nil || !strings.Contains(setErr.Error(), "-32602") {
		t.Fatalf("SetSessionConfigOption(context_tier) err = %v, want -32602 Invalid params", setErr)
	}

	zeroTurnAgent := &mockAgent{
		conn:        &recordingSessionUpdater{},
		model:       modelFast,
		contextTier: "long_context",
		sessions:    make(map[acp.SessionId]bool),
	}
	if _, loadZeroErr := zeroTurnAgent.LoadSession(context.Background(), acp.LoadSessionRequest{
		SessionId: newResp.SessionId,
		Cwd:       t.TempDir(),
	}); loadZeroErr == nil || !strings.Contains(loadZeroErr.Error(), "-32602") {
		t.Fatalf("LoadSession(0-turn session) err = %v, want -32602 session not found", loadZeroErr)
	}

	if _, err := firstAgent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("<kandev-system>task envelope</kandev-system>\n\n/e2e:copilot-context-info")},
	}); err != nil {
		t.Fatalf("first Prompt: %v", err)
	}
	firstText := collectRecordedAgentMessageText(firstUpdater.snapshot())
	if !strings.Contains(firstText, "Copilot runtime context_tier=default") ||
		!strings.Contains(firstText, "loads=0 prior_turns=0") {
		t.Fatalf("first prompt text = %q, want default tier with loads=0 prior_turns=0", firstText)
	}

	secondUpdater := &recordingSessionUpdater{}
	secondAgent := &mockAgent{
		conn:        secondUpdater,
		model:       modelFast,
		contextTier: "long_context",
		sessions:    make(map[acp.SessionId]bool),
	}
	t.Cleanup(func() {
		_, _ = secondAgent.CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: newResp.SessionId})
	})
	if _, err := secondAgent.LoadSession(context.Background(), acp.LoadSessionRequest{
		SessionId: newResp.SessionId,
		Cwd:       t.TempDir(),
	}); err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if _, err := secondAgent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("/e2e:copilot-context-info")},
	}); err != nil {
		t.Fatalf("second Prompt: %v", err)
	}
	secondText := collectRecordedAgentMessageText(secondUpdater.snapshot())
	if !strings.Contains(secondText, "Copilot runtime context_tier=long_context") ||
		!strings.Contains(secondText, "loads=1 prior_turns=1") {
		t.Fatalf("second prompt text = %q, want long_context tier with loads=1 prior_turns=1", secondText)
	}
}

func collectRecordedAgentMessageText(updates []acp.SessionNotification) string {
	var builder strings.Builder
	for _, item := range updates {
		if item.Update.AgentMessageChunk != nil && item.Update.AgentMessageChunk.Content.Text != nil {
			builder.WriteString(item.Update.AgentMessageChunk.Content.Text.Text)
		}
	}
	return builder.String()
}
