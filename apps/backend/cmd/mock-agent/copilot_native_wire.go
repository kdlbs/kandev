package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

const (
	copilotNativeFixtureMethod          = "github.com/copilot/sessionEvent"
	copilotCapabilityMetaKey            = "github.com/copilot"
	copilotNativeTypeCompactionStart    = "session.compaction_start"
	copilotNativeTypeCompactionComplete = "session.compaction_complete"
	copilotNativeTypeUsage              = "session.usage_info"
)

// copilotNativeFixtureEnabled reports whether this mock process should emit the
// exact Copilot vendor notification method for the raw-wire E2E fixture.
func copilotNativeFixtureEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("KANDEV_MOCK_COPILOT_NATIVE")), "true")
}

type copilotNativeFixtureData struct {
	Data      map[string]any `json:"data"`
	AgentID   string         `json:"agentId,omitempty"`
	Timestamp string         `json:"timestamp"`
	Type      string         `json:"type"`
}

// handleCopilotNativeFixture serializes the exact vendor frames used by the
// desktop and phone counter tests. It deliberately writes raw JSON-RPC through
// the same serialized writer as the ACP SDK, so the real transport parser sees
// the unprefixed method.
func (a *mockAgent) handleCopilotNativeFixture(
	ctx context.Context,
	sid acp.SessionId,
	prompt string,
) (acp.PromptResponse, error, bool) {
	if !copilotNativeFixtureEnabled() || !strings.Contains(prompt, "/e2e:copilot-native") {
		return acp.PromptResponse{}, nil, false
	}
	a.mu.Lock()
	subscribed := a.copilotNativeSubscribed
	a.mu.Unlock()
	if !subscribed {
		return acp.PromptResponse{}, fmt.Errorf("copilot native fixture requires the initialize subscription"), true
	}
	partial := strings.Contains(prompt, "/e2e:copilot-native-partial")
	live := strings.Contains(prompt, "/e2e:copilot-native-live")
	frames := copilotNativeFixtureFrames(partial, live)
	for _, frame := range frames {
		payload := map[string]any{
			"sessionId": string(sid),
			"timestamp": frame.Timestamp,
			"type":      frame.Type,
			"data":      frame.Data,
		}
		if frame.AgentID != "" {
			payload["agentId"] = frame.AgentID
		}
		if err := a.rawOut.writeJSONRPC(map[string]any{
			"jsonrpc": "2.0",
			"method":  copilotNativeFixtureMethod,
			"params":  payload,
		}); err != nil {
			return acp.PromptResponse{}, fmt.Errorf("write Copilot native fixture: %w", err), true
		}
	}
	if err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: sid,
		Update:    acp.UpdateAgentMessageText("Copilot native compaction fixture completed."),
	}); err != nil {
		return acp.PromptResponse{}, err, true
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
}

func copilotNativeFixtureFrames(partial, live bool) []copilotNativeFixtureData {
	attemptID := "e2e-copilot-attempt-1"
	childID := "e2e-copilot-child-1"
	now := func() string { return time.Now().UTC().Format(time.RFC3339Nano) }
	if live {
		return []copilotNativeFixtureData{
			{Type: "session.usage_info", Timestamp: now(), Data: map[string]any{
				"currentTokens": 14500, "tokenLimit": 272000,
			}},
			{Type: "session.compaction_start", Timestamp: now(), Data: map[string]any{
				"compactionAttemptId": "e2e-copilot-live-attempt", "trigger": "manual",
				"currentTokens": 14500, "tokenLimit": 272000,
			}},
			{Type: "session.compaction_complete", Timestamp: now(), Data: map[string]any{
				"compactionAttemptId": "e2e-copilot-live-attempt", "trigger": "manual",
				"success": true, "outcome": "applied",
				"preCompactionTokens": 14500, "postCompactionTokens": 8100,
			}},
			{Type: "session.usage_info", Timestamp: now(), Data: map[string]any{
				"currentTokens": 8100, "tokenLimit": 272000,
			}},
		}
	}
	var frames []copilotNativeFixtureData
	if partial {
		frames = append(frames, copilotNativeFixtureData{
			Type: "session.compaction_complete", Timestamp: now(),
			Data: map[string]any{"omitted": "too-large", "bytes": float64(40000)},
		})
	}
	return append(frames, []copilotNativeFixtureData{
		{Type: "session.usage_info", Timestamp: now(), Data: map[string]any{
			"currentTokens": 13210, "tokenLimit": 272000, "isInitial": true,
		}},
		{Type: "session.usage_info", Timestamp: now(), AgentID: childID, Data: map[string]any{
			"currentTokens": 2877, "tokenLimit": 272000, "isInitial": true,
		}},
		{Type: "session.compaction_complete", Timestamp: now(), AgentID: childID, Data: map[string]any{
			"compactionAttemptId": "e2e-copilot-child-attempt", "success": true, "outcome": "applied",
		}},
		{Type: "session.usage_info", Timestamp: now(), Data: map[string]any{
			"currentTokens": 13327, "tokenLimit": 272000,
		}},
		{Type: "session.usage_info", Timestamp: now(), AgentID: childID, Data: map[string]any{
			"currentTokens": 2877, "tokenLimit": 272000, "isInitial": true,
		}},
		{Type: "session.usage_info", Timestamp: now(), Data: map[string]any{
			"currentTokens": 13417, "tokenLimit": 272000,
		}},
		{Type: "session.compaction_start", Timestamp: now(), Data: map[string]any{
			"compactionAttemptId": attemptID, "trigger": "manual", "currentTokens": 13417, "tokenLimit": 272000,
		}},
		{Type: "session.compaction_complete", Timestamp: now(), Data: map[string]any{
			"compactionAttemptId": attemptID, "trigger": "manual", "success": true, "outcome": "applied",
			"preCompactionTokens": 13417, "postCompactionTokens": 9695,
		}},
		{Type: "session.compaction_complete", Timestamp: now(), Data: map[string]any{
			"compactionAttemptId": attemptID, "trigger": "manual", "success": true, "outcome": "applied",
			"preCompactionTokens": 13417, "postCompactionTokens": 9695,
		}},
		{Type: "session.usage_info", Timestamp: now(), Data: map[string]any{
			"currentTokens": 9695, "tokenLimit": 272000,
		}},
	}...)
}

// serializedACPWriter keeps SDK frames and raw vendor frames on one ordered
// writer. The fixture must never interleave two JSON-RPC lines.
type serializedACPWriter struct {
	mu     chan struct{}
	writer *os.File
}

func newSerializedACPWriter(writer *os.File) *serializedACPWriter {
	lock := make(chan struct{}, 1)
	lock <- struct{}{}
	return &serializedACPWriter{mu: lock, writer: writer}
}

func (w *serializedACPWriter) Write(payload []byte) (int, error) {
	<-w.mu
	defer func() { w.mu <- struct{}{} }()
	return w.writer.Write(payload)
}

func (w *serializedACPWriter) writeJSONRPC(payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	_, err = w.Write(encoded)
	return err
}

// copilotNativeSubscriptionRequested verifies the exact vendor event list this
// fixture depends on. The fixture must not emit frames a real client did not
// subscribe to, or the E2E would not prove negotiation.
func copilotNativeSubscriptionRequested(caps acp.ClientCapabilities) bool {
	meta, ok := caps.Meta[copilotCapabilityMetaKey].(map[string]any)
	if !ok {
		return false
	}
	required := map[string]bool{
		copilotNativeTypeCompactionStart:    false,
		copilotNativeTypeCompactionComplete: false,
		copilotNativeTypeUsage:              false,
	}
	switch events := meta["events"].(type) {
	case []string:
		for _, event := range events {
			required[event] = true
		}
	case []any:
		for _, raw := range events {
			if event, ok := raw.(string); ok {
				required[event] = true
			}
		}
	default:
		return false
	}
	for _, found := range required {
		if !found {
			return false
		}
	}
	return true
}
