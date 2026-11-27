package routingerr

import (
	"strings"
	"testing"
)

const cursorPromptStreamClosedTestMessage = "Cursor prompt stream closed: WritableIterable is closed"

func TestCursorPromptStreamClosedRule(t *testing.T) {
	for _, tc := range []struct {
		name       string
		providerID string
		message    string
		wantMatch  bool
	}{
		{name: "exact diagnostic", providerID: cursorRetriableProviderID, message: cursorPromptStreamClosedTestMessage, wantMatch: true},
		{name: "trimmed diagnostic", providerID: cursorRetriableProviderID, message: "  " + cursorPromptStreamClosedTestMessage + "\n", wantMatch: true},
		{name: "diagnostic in prose", providerID: cursorRetriableProviderID, message: "The provider reported " + cursorPromptStreamClosedTestMessage},
		{name: "other provider", providerID: "claude-acp", message: cursorPromptStreamClosedTestMessage},
		{name: "different case", providerID: cursorRetriableProviderID, message: strings.ToLower(cursorPromptStreamClosedTestMessage)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(Input{Phase: PhasePromptSend, ProviderID: tc.providerID, Stderr: tc.message})
			matched := got.ClassifierRule == "cursor.prompt_stream_closed.v1"
			if matched != tc.wantMatch {
				t.Fatalf("matched = %v, want %v (error: %+v)", matched, tc.wantMatch, got)
			}
			if !tc.wantMatch {
				return
			}
			if got.Code != CodeAgentTransportLost || got.Confidence != ConfHigh ||
				!got.AutoRetryable || got.FallbackAllowed {
				t.Fatalf("classification = %+v, want high-confidence Cursor transport loss", got)
			}
		})
	}
}
