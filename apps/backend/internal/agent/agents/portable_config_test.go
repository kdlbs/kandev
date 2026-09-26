package agents

import "testing"

func TestInitialPortableConfigDeclarations(t *testing.T) {
	tests := []struct {
		name      string
		agent     Agent
		bundleID  string
		filePaths []string
	}{
		{name: "claude", agent: NewClaudeACP(), bundleID: "claude.settings", filePaths: []string{".claude/settings.json"}},
		{name: "codex", agent: NewCodexACP(), bundleID: "codex.config", filePaths: []string{".codex/config.toml"}},
		{name: "opencode", agent: NewOpenCodeACP(), bundleID: "opencode.config", filePaths: []string{".config/opencode/opencode.json", ".config/opencode/opencode.jsonc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.agent.(PortableConfigAgent).PortableConfig()
			if config == nil || len(config.Bundles) != 1 {
				t.Fatalf("portable config = %+v, want one bundle", config)
			}
			bundle := config.Bundles[0]
			if bundle.ID != tt.bundleID {
				t.Fatalf("bundle ID = %q, want %q", bundle.ID, tt.bundleID)
			}
			if len(bundle.Files) != len(tt.filePaths) {
				t.Fatalf("bundle files = %+v, want %v", bundle.Files, tt.filePaths)
			}
			for index, file := range bundle.Files {
				if file.TargetPath != tt.filePaths[index] {
					t.Errorf("file %d target = %q, want %q", index, file.TargetPath, tt.filePaths[index])
				}
			}
		})
	}
}

func TestCodexRemoteAuthContainsAuthOnly(t *testing.T) {
	auth := NewCodexACP().RemoteAuth()
	if auth == nil || len(auth.Methods) == 0 {
		t.Fatal("Codex RemoteAuth must declare auth")
	}
	for _, method := range auth.Methods {
		for _, paths := range method.SourceFiles {
			for _, path := range paths {
				if path == ".codex/config.toml" {
					t.Fatal("Codex config.toml must be independent from authentication")
				}
			}
		}
	}
}

func TestMockPortableConfigUsesUniqueBundleIDsForAliases(t *testing.T) {
	defaultBundle := NewMockAgent().PortableConfig().Bundles[0]
	aliasBundle := NewMockAgentWithID("codex-acp", "Mock Codex", "Mock Codex").PortableConfig().Bundles[0]

	if defaultBundle.ID != "mock.settings" {
		t.Fatalf("default mock bundle ID = %q, want mock.settings", defaultBundle.ID)
	}
	if aliasBundle.ID != "codex-acp.settings" {
		t.Fatalf("aliased mock bundle ID = %q, want codex-acp.settings", aliasBundle.ID)
	}
}
