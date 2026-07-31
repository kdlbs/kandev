package handlers

import (
	"encoding/json"
	"slices"
	"testing"
)

// TestCreateCustomTUIAgentRequest_MapsCommandArgs pins the wire contract: a
// command_args element containing a space must reach the controller request as
// one argv element. Smuggling it into the space-separated `command` string
// cannot express this — the registry splits that on whitespace.
func TestCreateCustomTUIAgentRequest_MapsCommandArgs(t *testing.T) {
	payload := `{
		"display_name": "Spaced Args",
		"command": "my-cli",
		"command_args": ["--system-prompt", "you are a helpful agent"]
	}`

	var body createCustomTUIAgentRequest
	if err := json.Unmarshal([]byte(payload), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	req := body.toControllerRequest()
	want := []string{"--system-prompt", "you are a helpful agent"}
	if !slices.Equal(req.CommandArgs, want) {
		t.Errorf("CommandArgs = %#v, want %#v", req.CommandArgs, want)
	}
	if req.DisplayName != "Spaced Args" || req.Command != "my-cli" {
		t.Errorf("unexpected mapping: %+v", req)
	}
}

// TestCreateCustomTUIAgentRequest_OmittedCommandArgs keeps the field optional.
func TestCreateCustomTUIAgentRequest_OmittedCommandArgs(t *testing.T) {
	var body createCustomTUIAgentRequest
	if err := json.Unmarshal([]byte(`{"display_name":"Plain","command":"my-cli --verbose"}`), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := body.toControllerRequest().CommandArgs; len(got) != 0 {
		t.Errorf("CommandArgs = %#v, want empty", got)
	}
}
