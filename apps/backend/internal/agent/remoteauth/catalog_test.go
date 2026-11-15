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

func TestBuildCatalogClaudeAPIKeyAndOAuth(t *testing.T) {
	cat := BuildCatalogForHost([]agents.Agent{agents.NewClaudeACP()}, "linux", "")

	apiKeyID := "agent:claude-acp:env:ANTHROPIC_API_KEY"
	apiKey, ok := cat.FindMethod(apiKeyID)
	if !ok {
		t.Fatalf("Claude API-key method %q missing from catalog", apiKeyID)
	}
	if apiKey.Type != "env" || apiKey.EnvVar != "ANTHROPIC_API_KEY" {
		t.Fatalf("Claude API-key method = %+v, want env ANTHROPIC_API_KEY", apiKey)
	}
	if apiKey.SetupScript != "" || apiKey.SetupHint != "" || apiKey.Label != "" {
		t.Fatalf("Claude API-key method must not carry setup script, hint, or label: %+v", apiKey)
	}

	oauthID := "agent:claude-acp:env:CLAUDE_CODE_OAUTH_TOKEN"
	oauth, ok := cat.FindMethod(oauthID)
	if !ok {
		t.Fatalf("Claude OAuth method %q missing from catalog", oauthID)
	}
	if oauth.SetupScript == "" {
		t.Fatal("Claude OAuth method lost its setup script")
	}
	if apiKey.MethodID == oauth.MethodID {
		t.Fatal("Claude API-key and OAuth methods must keep distinct identities")
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
