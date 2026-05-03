package agents

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

// agentSpec captures the contract each newly-added ACP agent must honor.
// Keeping these in a table-driven test pins ID, Display, command shape, and
// protocol so that drift in any of those is loud.
type agentSpec struct {
	id          string
	displayName string
	wantArgv    []string
	// wantInstalledBinary is the primary binary name used by IsInstalled's
	// Detect strategy. Asserted by checking it appears in the script or
	// (for npx-launched agents) is referenced by the InstallScript.
	wantInstalledBinary string
	wantInstallNpm      bool // true when InstallScript should start with "npm install -g"
}

func TestNewACPAgents_Contract(t *testing.T) {
	cases := []struct {
		new  func() Agent
		spec agentSpec
	}{
		{func() Agent { return NewQwenACP() }, agentSpec{
			id: "qwen-acp", displayName: "Qwen",
			wantArgv:            []string{"npx", "-y", "@qwen-code/qwen-code", "--acp"},
			wantInstalledBinary: "qwen", wantInstallNpm: true,
		}},
		{func() Agent { return NewIFlowACP() }, agentSpec{
			id: "iflow-acp", displayName: "iFlow (beta)",
			wantArgv:            []string{"npx", "-y", "@iflow-ai/iflow-cli", "--experimental-acp"},
			wantInstalledBinary: "iflow", wantInstallNpm: true,
		}},
		{func() Agent { return NewDroidACP() }, agentSpec{
			id: "droid-acp", displayName: "Droid",
			wantArgv:            []string{"npx", "-y", "droid", "exec", "--output-format", "acp"},
			wantInstalledBinary: "droid", wantInstallNpm: true,
		}},
		{func() Agent { return NewKilocodeACP() }, agentSpec{
			id: "kilocode-acp", displayName: "Kilocode",
			wantArgv:            []string{"npx", "-y", "@kilocode/cli", "acp"},
			wantInstalledBinary: "kilo", wantInstallNpm: true,
		}},
		{func() Agent { return NewPiACP() }, agentSpec{
			id: "pi-acp", displayName: "Pi",
			wantArgv:            []string{"npx", "-y", "pi-acp"},
			wantInstalledBinary: "pi-acp", wantInstallNpm: true,
		}},
		{func() Agent { return NewCursorACP() }, agentSpec{
			id: "cursor-acp", displayName: "Cursor",
			wantArgv:            []string{"cursor-agent", "acp"},
			wantInstalledBinary: "cursor-agent", wantInstallNpm: false,
		}},
		{func() Agent { return NewKimiACP() }, agentSpec{
			id: "kimi-acp", displayName: "Kimi",
			wantArgv:            []string{"kimi", "acp"},
			wantInstalledBinary: "kimi", wantInstallNpm: false,
		}},
		{func() Agent { return NewKiroACP() }, agentSpec{
			id: "kiro-acp", displayName: "Kiro",
			wantArgv:            []string{"kiro-cli-chat", "acp"},
			wantInstalledBinary: "kiro-cli-chat", wantInstallNpm: false,
		}},
		{func() Agent { return NewQoderACP() }, agentSpec{
			id: "qoder-acp", displayName: "Qoder",
			wantArgv:            []string{"qodercli", "--acp"},
			wantInstalledBinary: "qodercli", wantInstallNpm: false,
		}},
		{func() Agent { return NewTraeACP() }, agentSpec{
			id: "trae-acp", displayName: "Trae",
			wantArgv:            []string{"traecli", "acp", "serve"},
			wantInstalledBinary: "traecli", wantInstallNpm: false,
		}},
	}

	for _, tc := range cases {
		t.Run(tc.spec.id, func(t *testing.T) {
			ag := tc.new()

			if got := ag.ID(); got != tc.spec.id {
				t.Errorf("ID() = %q, want %q", got, tc.spec.id)
			}
			if got := ag.DisplayName(); got != tc.spec.displayName {
				t.Errorf("DisplayName() = %q, want %q", got, tc.spec.displayName)
			}
			if !ag.Enabled() {
				t.Errorf("Enabled() = false, want true")
			}

			argv := ag.BuildCommand(CommandOptions{}).Args()
			if !equalStringSlice(argv, tc.spec.wantArgv) {
				t.Errorf("BuildCommand argv mismatch\n  got:  %#v\n  want: %#v", argv, tc.spec.wantArgv)
			}

			rt := ag.Runtime()
			if rt == nil {
				t.Fatalf("Runtime() returned nil")
			}
			if rt.Protocol != agent.ProtocolACP {
				t.Errorf("Runtime.Protocol = %q, want ACP", rt.Protocol)
			}
			if !equalStringSlice(rt.Cmd.Args(), tc.spec.wantArgv) {
				t.Errorf("Runtime.Cmd argv mismatch\n  got:  %#v\n  want: %#v", rt.Cmd.Args(), tc.spec.wantArgv)
			}

			if got := ag.InstallScript(); tc.spec.wantInstallNpm {
				if !strings.HasPrefix(got, "npm install -g ") {
					t.Errorf("InstallScript() = %q, want npm install -g …", got)
				}
			} else {
				if strings.HasPrefix(got, "npm install -g ") {
					t.Errorf("InstallScript() should NOT use npm for native-binary agent: %q", got)
				}
			}

			// The detection binary should appear in either argv (native-binary
			// agents) or InstallScript (npx-launched agents whose npm package
			// installs that bin). One of the two must reference it; otherwise
			// users land on a "not installed" agent with no actionable hint.
			if !referencesBinary(argv, tc.spec.wantInstalledBinary) &&
				!strings.Contains(ag.InstallScript(), tc.spec.wantInstalledBinary) {
				t.Errorf("detection binary %q not referenced in argv (%v) or InstallScript (%q)",
					tc.spec.wantInstalledBinary, argv, ag.InstallScript())
			}
		})
	}
}

// TestNewACPAgents_NpxLaunchedAgentsHaveNpxFallback verifies that agents
// launched via `npx -y <pkg>` report Available=true through the npx fallback
// when node/npx is on PATH but the global binary isn't. Without this contract,
// users with Node but no `npm install -g …` see these agents in "Available to
// Install" even though `npx -y` would launch them just fine.
func TestNewACPAgents_NpxLaunchedAgentsHaveNpxFallback(t *testing.T) {
	if !nodeOrNpxOnPath() {
		t.Skip("node/npx not on PATH; skipping npx-fallback contract")
	}

	npxAgents := []Agent{
		NewQwenACP(), NewIFlowACP(), NewDroidACP(), NewKilocodeACP(), NewPiACP(),
	}
	for _, ag := range npxAgents {
		t.Run(ag.ID(), func(t *testing.T) {
			argv := ag.BuildCommand(CommandOptions{}).Args()
			if len(argv) == 0 || argv[0] != "npx" {
				t.Fatalf("expected npx-launched agent; argv=%v", argv)
			}

			result, err := ag.IsInstalled(context.Background())
			if err != nil {
				t.Fatalf("IsInstalled error: %v", err)
			}
			// node/npx is on PATH, so even if the agent's global binary isn't
			// installed, the fallback should make it Available.
			if !result.Available {
				t.Errorf("Available=false despite node/npx on PATH; agent should fall back to npx")
			}
		})
	}
}

// TestNewACPAgents_NativeBinaryAgentsHaveNoNpxFallback pins the inverse: the
// agents that aren't on npm (Cursor, Kimi, Kiro, Qoder, Trae) must NOT
// claim availability via npx — they need the upstream binary on PATH.
func TestNewACPAgents_NativeBinaryAgentsHaveNoNpxFallback(t *testing.T) {
	nativeAgents := []Agent{
		NewCursorACP(), NewKimiACP(), NewKiroACP(), NewQoderACP(), NewTraeACP(),
	}
	for _, ag := range nativeAgents {
		t.Run(ag.ID(), func(t *testing.T) {
			argv := ag.BuildCommand(CommandOptions{}).Args()
			if len(argv) == 0 || argv[0] == "npx" {
				t.Fatalf("expected native-binary agent; argv=%v", argv)
			}
			// We can't directly assert "no npx fallback" without inspecting
			// internals, so we assert the indirect contract: when the agent's
			// binary isn't on PATH, the agent is NOT available. This would
			// fail loudly if someone copy-pasted WithNpxRunnable into a
			// native-binary agent's IsInstalled.
			if _, err := exec.LookPath(argv[0]); err == nil {
				t.Skipf("upstream binary %q is on PATH; can't verify fallback absence", argv[0])
			}
			result, err := ag.IsInstalled(context.Background())
			if err != nil {
				t.Fatalf("IsInstalled error: %v", err)
			}
			if result.Available {
				t.Errorf("Available=true for native-binary agent without binary on PATH; should not have npx fallback")
			}
		})
	}
}

// TestNewACPAgents_LogosNonEmpty guards against agents shipping with empty
// embedded SVGs (which renders as a broken <img> in the UI).
func TestNewACPAgents_LogosNonEmpty(t *testing.T) {
	all := []Agent{
		NewQwenACP(), NewIFlowACP(), NewDroidACP(), NewKilocodeACP(), NewPiACP(),
		NewCursorACP(), NewKimiACP(), NewKiroACP(), NewQoderACP(), NewTraeACP(),
	}
	for _, ag := range all {
		t.Run(ag.ID(), func(t *testing.T) {
			if len(ag.Logo(LogoLight)) == 0 {
				t.Errorf("Logo(LogoLight) is empty")
			}
			if len(ag.Logo(LogoDark)) == 0 {
				t.Errorf("Logo(LogoDark) is empty")
			}
		})
	}
}

// TestNewACPAgents_DisplayOrderUnique ensures the new agents don't collide
// with each other or with the existing built-ins (Claude=1..Amp=7).
func TestNewACPAgents_DisplayOrderUnique(t *testing.T) {
	all := []Agent{
		NewClaudeACP(), NewCodexACP(), NewAuggie(), NewOpenCodeACP(),
		NewGemini(), NewCopilotACP(), NewAmpACP(),
		NewQwenACP(), NewIFlowACP(), NewDroidACP(), NewKilocodeACP(), NewPiACP(),
		NewCursorACP(), NewKimiACP(), NewKiroACP(), NewQoderACP(), NewTraeACP(),
	}
	seen := map[int]string{}
	for _, ag := range all {
		order := ag.DisplayOrder()
		if other, exists := seen[order]; exists {
			t.Errorf("DisplayOrder %d collision: %s and %s", order, other, ag.ID())
		}
		seen[order] = ag.ID()
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func referencesBinary(argv []string, bin string) bool {
	return slices.Contains(argv, bin)
}
