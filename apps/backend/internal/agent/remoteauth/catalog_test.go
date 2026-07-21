package remoteauth

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

// Agents that declare RemoteAuth with no Methods (e.g. mock) must still
// serialize "methods": [] rather than "methods": null — the frontend reads
// spec.methods.length and crashes on null.
func TestBuildCatalog_MockSpecSerializesEmptyMethodsAsArray(t *testing.T) {
	mockAgent := agents.NewMockAgent()
	cat := BuildCatalogForHost([]agents.Agent{mockAgent}, "linux", "")

	var mock *Spec
	for i, s := range cat.Specs {
		if s.ID == mockAgent.ID() {
			mock = &cat.Specs[i]
			break
		}
	}
	if mock == nil {
		t.Fatal("mock spec missing from catalog")
	}
	if mock.Methods == nil {
		t.Fatal("mock.Methods is nil; should be a non-nil empty slice for stable JSON shape")
	}

	out, err := json.Marshal(mock)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"methods":[]`) {
		t.Errorf("expected `\"methods\":[]` in JSON, got: %s", out)
	}
}
