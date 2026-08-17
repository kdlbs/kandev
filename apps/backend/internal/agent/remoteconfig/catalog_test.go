package remoteconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestBuildCatalogForHostUsesStableBundleIDsAndAvailability(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("model = \"private\""), 0o600); err != nil {
		t.Fatal(err)
	}

	first := BuildCatalogForHost([]agents.Agent{agents.NewOpenCodeACP(), agents.NewCodexACP(), agents.NewClaudeACP()}, "linux", home)
	second := BuildCatalogForHost([]agents.Agent{agents.NewClaudeACP(), agents.NewCodexACP(), agents.NewOpenCodeACP()}, "linux", home)

	firstIDs := bundleIDs(first.Bundles)
	secondIDs := bundleIDs(second.Bundles)
	slices.Sort(firstIDs)
	slices.Sort(secondIDs)
	if !slices.Equal(firstIDs, secondIDs) {
		t.Fatalf("bundle IDs depend on declaration order: %v vs %v", firstIDs, secondIDs)
	}

	codex, ok := first.FindBundle("codex.config")
	if !ok || !codex.Available {
		t.Fatalf("codex bundle = %+v, want available stable codex.config bundle", codex)
	}
	if len(codex.Files) != 1 || codex.Files[0].SourcePath != ".codex/config.toml" {
		t.Fatalf("codex bundle files = %+v, want config.toml only", codex.Files)
	}

	claude, ok := first.FindBundle("claude.settings")
	if !ok || claude.Available {
		t.Fatalf("claude bundle = %+v, want unavailable without settings.json", claude)
	}
}

func TestBuildCatalogForHostDoesNotExposeFileData(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	secret := "host-only-secret"
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}

	catalog := BuildCatalogForHost([]agents.Agent{agents.NewCodexACP()}, "linux", home)
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("catalog leaked file data: %s", encoded)
	}
	if strings.Contains(string(encoded), home) {
		t.Fatalf("catalog leaked host home path: %s", encoded)
	}
}

func TestBuildCatalogForHostSkipsAgentsWithoutDeclarations(t *testing.T) {
	catalog := BuildCatalogForHost([]agents.Agent{
		agents.NewTUIAgent(agents.TUIAgentConfig{
			AgentID:   "plain-tui",
			AgentName: "Plain TUI",
			Command:   "plain-tui",
			Desc:      "test agent without portable configuration",
		}),
	}, "linux", "")
	if len(catalog.Bundles) != 0 {
		t.Fatalf("bundles = %+v, want empty catalog", catalog.Bundles)
	}
}

func bundleIDs(bundles []Bundle) []string {
	ids := make([]string, 0, len(bundles))
	for _, bundle := range bundles {
		ids = append(ids, bundle.ID)
	}
	return ids
}
