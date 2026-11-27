package utility

import (
	"encoding/json"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

// Published MiniMax Code 0.5.10 uses typed options with provider and variant
// encoded in each opaque model ID, rather than the legacy models carrier.
func TestMiniMaxProbePreservesTypedModelIdentities(t *testing.T) {
	const response = `{"sessionId":"minimax-session","configOptions":[{"type":"select","id":"permissionMode","name":"Permission mode","category":"_permission","currentValue":"default","options":[{"value":"default","name":"Default"}]},{"type":"select","id":"model","name":"Model","category":"model","currentValue":"m:minimax:MiniMax-M3:v:thinking","options":[{"value":"m:minimax:MiniMax-M3:v:thinking","name":"MiniMax-M3 · thinking"},{"value":"m:minimax:MiniMax-M3.1-Flash-Preview:v:thinking","name":"M3.1-Flash-Preview · thinking"},{"value":"m:minimax:MiniMax-M2.7-highspeed:v:thinking","name":"MiniMax-M2.7-highspeed"},{"value":"m:minimax:MiniMax-M2.7:v:thinking","name":"MiniMax-M2.7"}]}]}`
	var session acp.NewSessionResponse
	if err := json.Unmarshal([]byte(response), &session); err != nil {
		t.Fatal(err)
	}
	out := &ProbeResponse{}
	applySessionProbeFields(out, session, "")
	if out.CurrentModelID != "m:minimax:MiniMax-M3:v:thinking" || len(out.Models) != 4 {
		t.Fatalf("models: %+v", out)
	}
	if out.Models[1].ID != "m:minimax:MiniMax-M3.1-Flash-Preview:v:thinking" {
		t.Fatalf("encoded ID changed: %+v", out.Models[1])
	}
}
