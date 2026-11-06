package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

type recordingSessionUpdater struct {
	updates []acp.SessionNotification
}

func (r *recordingSessionUpdater) SessionUpdate(_ context.Context, update acp.SessionNotification) error {
	r.updates = append(r.updates, update)
	return nil
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
	if len(updater.updates) != 1 {
		t.Fatalf("session updates = %d, want the terminal text frame", len(updater.updates))
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
