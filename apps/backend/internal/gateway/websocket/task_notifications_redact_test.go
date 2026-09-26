package websocket

import (
	"reflect"
	"testing"
)

func TestRedactRemediationURLRemovesOnlyTheLinkKey(t *testing.T) {
	metadata := map[string]interface{}{
		"last_agent_error": map[string]interface{}{
			"message":         "5-hour usage limit reached",
			"remediation_url": "https://opencode.ai/workspace/wrk_123/go",
		},
		"other_key": "kept",
	}
	redacted := redactRemediationURL(metadata).(map[string]interface{})
	lastErr := redacted["last_agent_error"].(map[string]interface{})
	if _, ok := lastErr["remediation_url"]; ok {
		t.Fatal("remediation_url still present in the redacted echo")
	}
	if lastErr["message"] != "5-hour usage limit reached" {
		t.Fatalf("message was dropped: %#v", lastErr)
	}
	if redacted["other_key"] != "kept" {
		t.Fatalf("unrelated metadata was dropped: %#v", redacted)
	}
	// The original event payload must be untouched.
	original := metadata["last_agent_error"].(map[string]interface{})
	if _, ok := original["remediation_url"]; !ok {
		t.Fatal("redaction mutated the source payload")
	}
}

func TestRedactRemediationURLPassesThroughUnexpectedShapes(t *testing.T) {
	for _, metadata := range []interface{}{
		"not a map",
		nil,
		map[string]interface{}{"last_agent_error": "not a map"},
		map[string]interface{}{},
	} {
		if got := redactRemediationURL(metadata); !reflect.DeepEqual(got, metadata) {
			t.Fatalf("redactRemediationURL(%#v) changed the value: %#v", metadata, got)
		}
	}
}
