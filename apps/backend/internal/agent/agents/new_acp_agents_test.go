package agents

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

// acpAgentSpec captures the full contract each newly-added ACP agent must
// honor. Pinning every command surface (BuildCommand, Runtime.Cmd,
// InferenceConfig.Command, PassthroughCmd) keeps regressions like one path
// drifting to npx while the rest stay native (or vice versa) loud rather
// than silent.
type acpAgentSpec struct {
	id          string
	displayName string
	// detectBinaries lists every binary name IsInstalled accepts via
	// WithCommand. Used by TestNewACPAgents_DetectionRequiresGlobalBinary
	// to skip the test whenever ANY of these is on PATH — otherwise an
	// agent with multiple WithCommand fallbacks (e.g. Pi accepts both
	// `pi-acp` and `pi`) flakes when the secondary binary is present.
	detectBinaries  []string
	expectedArgv    []string // BuildCommand and Runtime.Cmd
	inferenceArgv   []string // InferenceConfig.Command
	passthroughArgv []string // PassthroughCmd (zero-args allowed)
	installViaNpm   bool     // InstallScript starts with "npm install -g"
}

var newACPAgentSpecs = []struct {
	new  func() Agent
	spec acpAgentSpec
}{
	{func() Agent { return NewQwenACP() }, acpAgentSpec{
		id: "qwen-acp", displayName: "Qwen", detectBinaries: []string{"qwen"},
		expectedArgv:    []string{"npx", "-y", "@qwen-code/qwen-code", "--acp"},
		inferenceArgv:   []string{"npx", "-y", "@qwen-code/qwen-code", "--acp"},
		passthroughArgv: []string{"npx", "-y", "@qwen-code/qwen-code"},
		installViaNpm:   true,
	}},
	{func() Agent { return NewIFlowACP() }, acpAgentSpec{
		id: "iflow-acp", displayName: "iFlow (beta)", detectBinaries: []string{"iflow"},
		expectedArgv:    []string{"npx", "-y", "@iflow-ai/iflow-cli", "--experimental-acp"},
		inferenceArgv:   []string{"npx", "-y", "@iflow-ai/iflow-cli", "--experimental-acp"},
		passthroughArgv: []string{"npx", "-y", "@iflow-ai/iflow-cli"},
		installViaNpm:   true,
	}},
	{func() Agent { return NewDroidACP() }, acpAgentSpec{
		id: "droid-acp", displayName: "Droid", detectBinaries: []string{"droid"},
		expectedArgv:    []string{"npx", "-y", "droid", "exec", "--output-format", "acp"},
		inferenceArgv:   []string{"npx", "-y", "droid", "exec", "--output-format", "acp"},
		passthroughArgv: []string{"npx", "-y", "droid"},
		installViaNpm:   true,
	}},
	{func() Agent { return NewKilocodeACP() }, acpAgentSpec{
		id: "kilocode-acp", displayName: "Kilocode", detectBinaries: []string{"kilo", "kilocode"},
		expectedArgv:    []string{"npx", "-y", "@kilocode/cli", "acp"},
		inferenceArgv:   []string{"npx", "-y", "@kilocode/cli", "acp"},
		passthroughArgv: []string{"npx", "-y", "@kilocode/cli"},
		installViaNpm:   true,
	}},
	{func() Agent { return NewPiACP() }, acpAgentSpec{
		id: "pi-acp", displayName: "Pi", detectBinaries: []string{"pi-acp", "pi"},
		expectedArgv:    []string{"npx", "-y", "pi-acp"},
		inferenceArgv:   []string{"npx", "-y", "pi-acp"},
		passthroughArgv: []string{"npx", "-y", "pi-acp"},
		installViaNpm:   true,
	}},
	{func() Agent { return NewCursorACP() }, acpAgentSpec{
		id: "cursor-acp", displayName: "Cursor", detectBinaries: []string{"cursor-agent"},
		expectedArgv:    []string{"cursor-agent", "acp"},
		inferenceArgv:   []string{"cursor-agent", "acp"},
		passthroughArgv: []string{"cursor-agent"},
		installViaNpm:   false,
	}},
	{func() Agent { return NewKimiACP() }, acpAgentSpec{
		id: "kimi-acp", displayName: "Kimi", detectBinaries: []string{"kimi"},
		expectedArgv:    []string{"kimi", "acp"},
		inferenceArgv:   []string{"kimi", "acp"},
		passthroughArgv: []string{"kimi"},
		installViaNpm:   false,
	}},
	{func() Agent { return NewKiroACP() }, acpAgentSpec{
		id: "kiro-acp", displayName: "Kiro", detectBinaries: []string{"kiro-cli-chat"},
		expectedArgv:    []string{"kiro-cli-chat", "acp"},
		inferenceArgv:   []string{"kiro-cli-chat", "acp"},
		passthroughArgv: []string{"kiro-cli-chat"},
		installViaNpm:   false,
	}},
	{func() Agent { return NewQoderACP() }, acpAgentSpec{
		id: "qoder-acp", displayName: "Qoder", detectBinaries: []string{"qodercli"},
		expectedArgv:    []string{"qodercli", "--acp"},
		inferenceArgv:   []string{"qodercli", "--acp"},
		passthroughArgv: []string{"qodercli"},
		installViaNpm:   false,
	}},
	{func() Agent { return NewTraeACP() }, acpAgentSpec{
		id: "trae-acp", displayName: "Trae", detectBinaries: []string{"traecli"},
		expectedArgv:    []string{"traecli", "acp", "serve"},
		inferenceArgv:   []string{"traecli", "acp", "serve"},
		passthroughArgv: []string{"traecli"},
		installViaNpm:   false,
	}},
}

func TestNewACPAgents_IDAndDisplay(t *testing.T) {
	for _, tc := range newACPAgentSpecs {
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
		})
	}
}

// TestNewACPAgents_AllCommandSurfaces pins every launch path: BuildCommand,
// Runtime.Cmd, InferenceConfig.Command, and PassthroughCmd. Without this the
// PassthroughCmd field can drift independently of the others (see the
// OpenCode npx regression in this PR's review history).
func TestNewACPAgents_AllCommandSurfaces(t *testing.T) {
	for _, tc := range newACPAgentSpecs {
		t.Run(tc.spec.id, func(t *testing.T) {
			ag := tc.new()
			assertArgvEqual(t, "BuildCommand", ag.BuildCommand(CommandOptions{}).Args(), tc.spec.expectedArgv)

			rt := ag.Runtime()
			if rt == nil {
				t.Fatalf("Runtime() returned nil")
			}
			if rt.Protocol != agent.ProtocolACP {
				t.Errorf("Runtime.Protocol = %q, want ACP", rt.Protocol)
			}
			assertArgvEqual(t, "Runtime.Cmd", rt.Cmd.Args(), tc.spec.expectedArgv)

			ia, ok := ag.(InferenceAgent)
			if !ok {
				t.Fatalf("%s does not implement InferenceAgent", tc.spec.id)
			}
			ic := ia.InferenceConfig()
			if ic == nil || !ic.Supported {
				t.Fatalf("InferenceConfig() = %+v, want Supported=true", ic)
			}
			assertArgvEqual(t, "InferenceConfig.Command", ic.Command.Args(), tc.spec.inferenceArgv)

			pa, ok := ag.(PassthroughAgent)
			if !ok {
				t.Fatalf("%s does not implement PassthroughAgent", tc.spec.id)
			}
			assertArgvEqual(t, "PassthroughCmd", pa.PassthroughConfig().PassthroughCmd.Args(), tc.spec.passthroughArgv)
		})
	}
}

func TestNewACPAgents_InstallScript(t *testing.T) {
	for _, tc := range newACPAgentSpecs {
		t.Run(tc.spec.id, func(t *testing.T) {
			ag := tc.new()
			got := ag.InstallScript()
			hasNpm := strings.HasPrefix(got, "npm install -g ")
			if tc.spec.installViaNpm && !hasNpm {
				t.Errorf("InstallScript() = %q, want npm install -g …", got)
			}
			if !tc.spec.installViaNpm && hasNpm {
				t.Errorf("InstallScript() should NOT use npm for native-binary agent: %q", got)
			}
			// The primary detection binary must be referenced somewhere
			// actionable — either argv (native binaries) or InstallScript
			// (npm install -g <pkg> ships the bin), so users with "Available
			// to Install" status see a hint that resolves to the right
			// command. Only check the first detect binary; secondaries are
			// fallbacks that may not appear in either surface.
			primary := tc.spec.detectBinaries[0]
			argv := ag.BuildCommand(CommandOptions{}).Args()
			if !slices.Contains(argv, primary) && !strings.Contains(got, primary) {
				t.Errorf("detection binary %q not referenced in argv (%v) or InstallScript (%q)",
					primary, argv, got)
			}
		})
	}
}

// TestNewACPAgents_DetectionRequiresGlobalBinary pins the contract that
// agents are not reported as available solely because npx is on PATH. The
// host-utility manager treats Available=true as a green light to spawn and
// probe the agent — claiming availability for an agent the user hasn't
// actually installed triggers unwanted package downloads and produces
// misleading auth_required/failed states for agents the user never asked
// to use. detectBinaries comes from the spec table so npx-launched agents
// (whose argv[0] is "npx") are still verified against their real detection
// targets (qwen, iflow, droid, …), and agents with multiple WithCommand
// fallbacks (e.g. Pi accepts both `pi-acp` and `pi`) skip the test
// whenever ANY of those is on PATH so the test doesn't flake on CI hosts
// that happen to have a generic `pi` (Raspberry Pi tooling) installed.
func TestNewACPAgents_DetectionRequiresGlobalBinary(t *testing.T) {
	for _, tc := range newACPAgentSpecs {
		t.Run(tc.spec.id, func(t *testing.T) {
			for _, binary := range tc.spec.detectBinaries {
				if _, err := exec.LookPath(binary); err == nil {
					t.Skipf("detection binary %q is on PATH; can't verify availability requirement", binary)
				}
			}
			result, err := tc.new().IsInstalled(context.Background())
			if err != nil {
				t.Fatalf("IsInstalled error: %v", err)
			}
			if result.Available {
				t.Errorf("Available=true without any of %v on PATH; detection should require the global binary so the host-utility manager doesn't spawn unwanted npx probes",
					tc.spec.detectBinaries)
			}
		})
	}
}

// TestNewACPAgents_LogosNonEmpty guards against agents shipping with empty
// embedded SVGs (which renders as a broken <img> in the UI).
func TestNewACPAgents_LogosNonEmpty(t *testing.T) {
	for _, tc := range newACPAgentSpecs {
		t.Run(tc.spec.id, func(t *testing.T) {
			ag := tc.new()
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
	}
	for _, tc := range newACPAgentSpecs {
		all = append(all, tc.new())
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

func assertArgvEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s argv mismatch\n  got:  %#v\n  want: %#v", label, got, want)
	}
}
