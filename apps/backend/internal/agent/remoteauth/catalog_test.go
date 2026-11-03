package remoteauth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

type openCodeV2Selection struct{}

func (openCodeV2Selection) GetOpenCodeSelection(context.Context) (managedruntime.OpenCodeSelection, bool, error) {
	return managedruntime.OpenCodeSelection{Family: managedruntime.OpenCodeFamilyV2, SelectedVersion: "2.1.2"}, true, nil
}

func TestBuildCatalogOpenCodeV2PreservesSelectionWithoutCopyingLegacyFiles(t *testing.T) {
	ag := agents.NewOpenCodeACP()
	ag.SetOpenCodeSelectionReader(openCodeV2Selection{})
	catalog := BuildCatalogForHost([]agents.Agent{ag}, "linux", t.TempDir())
	method, found := catalog.FindMethod("agent:opencode-acp:files:0")
	if !found || !method.HasLocalFiles || len(method.SourceFiles) != 0 {
		t.Fatalf("V2 must remain selectable without legacy credential copies: %+v", method)
	}
}

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

func TestBuildCatalog_GitHubCLIOnlyOffersExplicitSecret(t *testing.T) {
	cat := BuildCatalogForHost(nil, "linux", "")

	var githubCLI *Spec
	for i := range cat.Specs {
		if cat.Specs[i].ID == ghCLISpecID {
			githubCLI = &cat.Specs[i]
			break
		}
	}
	if githubCLI == nil {
		t.Fatal("GitHub CLI auth spec missing")
	}
	if len(githubCLI.Methods) != 1 {
		t.Fatalf("GitHub CLI methods = %+v, want only explicit secret method", githubCLI.Methods)
	}
	method := githubCLI.Methods[0]
	if method.MethodID != ghCLIEnvMethodID || method.Type != "env" || method.EnvVar != ghCLIEnvVar {
		t.Fatalf("GitHub CLI method = %+v, want explicit GITHUB_TOKEN secret", method)
	}
	if _, ok := cat.FindMethod("gh_cli_token"); ok {
		t.Fatal("host-global gh CLI token method must not be selectable")
	}
}
