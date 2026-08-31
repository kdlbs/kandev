package agents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// agent with multiple WithCommand fallbacks flakes when a secondary binary
	// is present.
	detectBinaries  []string
	expectedArgv    []string // BuildCommand and Runtime.Cmd
	inferenceArgv   []string // InferenceConfig.Command
	passthroughArgv []string // PassthroughCmd (zero-args allowed)
	installViaNpm   bool     // InstallScript starts with "npm install -g"
	installScript   string   // expected InstallScript() value (empty = unchecked)
	stripEnv        []string // expected Runtime().StripEnv (nil = unchecked)
	// sessionDirTemplate is the agent's on-disk session root. Pinned per
	// agent because it is derived from what the CLI actually writes (see the
	// evidence comment on each Runtime()), not from a naming convention —
	// Trae and Devin both diverge from the "{home}/.<agent>" shape, and a
	// wrong value silently bind-mounts the wrong directory into the
	// container.
	sessionDirTemplate string
}

var newACPAgentSpecs = []struct {
	new  func() Agent
	spec acpAgentSpec
}{
	{func() Agent { return NewQwenACP() }, acpAgentSpec{
		id: "qwen-acp", displayName: "Qwen", detectBinaries: []string{"qwen"},
		expectedArgv:       []string{"npx", "-y", "@qwen-code/qwen-code", "--acp"},
		inferenceArgv:      []string{"npx", "-y", "@qwen-code/qwen-code", "--acp"},
		passthroughArgv:    []string{"npx", "-y", "@qwen-code/qwen-code"},
		installViaNpm:      true,
		sessionDirTemplate: "{home}/.qwen",
	}},
	{func() Agent { return NewIFlowACP() }, acpAgentSpec{
		id: "iflow-acp", displayName: "iFlow (beta)", detectBinaries: []string{"iflow"},
		expectedArgv:       []string{"npx", "-y", "@iflow-ai/iflow-cli", "--experimental-acp"},
		inferenceArgv:      []string{"npx", "-y", "@iflow-ai/iflow-cli", "--experimental-acp"},
		passthroughArgv:    []string{"npx", "-y", "@iflow-ai/iflow-cli"},
		installViaNpm:      true,
		sessionDirTemplate: "{home}/.iflow",
	}},
	{func() Agent { return NewDroidACP() }, acpAgentSpec{
		id: "droid-acp", displayName: "Droid", detectBinaries: []string{"droid"},
		expectedArgv:       []string{"npx", "-y", "droid", "exec", "--output-format", "acp"},
		inferenceArgv:      []string{"npx", "-y", "droid", "exec", "--output-format", "acp"},
		passthroughArgv:    []string{"npx", "-y", "droid"},
		installViaNpm:      true,
		sessionDirTemplate: "{home}/.factory",
	}},
	{func() Agent { return NewKilocodeACP() }, acpAgentSpec{
		id: "kilocode-acp", displayName: "Kilocode", detectBinaries: []string{"kilo", "kilocode"},
		expectedArgv:       []string{"npx", "-y", "@kilocode/cli", "acp"},
		inferenceArgv:      []string{"npx", "-y", "@kilocode/cli", "acp"},
		passthroughArgv:    []string{"npx", "-y", "@kilocode/cli"},
		installViaNpm:      true,
		sessionDirTemplate: "{home}/.kilocode",
	}},
	{func() Agent { return NewPiACP() }, acpAgentSpec{
		id: "pi-acp", displayName: "Pi", detectBinaries: []string{"pi"},
		expectedArgv:       []string{"npx", "-y", "pi-acp"},
		inferenceArgv:      []string{"npx", "-y", "pi-acp"},
		passthroughArgv:    []string{"pi"},
		installViaNpm:      true,
		installScript:      "npm install -g --ignore-scripts @earendil-works/pi-coding-agent",
		sessionDirTemplate: "{home}/.pi",
	}},
	{func() Agent { return NewCursorACP() }, acpAgentSpec{
		id: "cursor-acp", displayName: "Cursor", detectBinaries: []string{"cursor-agent"},
		expectedArgv:    []string{"cursor-agent", "acp"},
		inferenceArgv:   []string{"cursor-agent", "acp"},
		passthroughArgv: []string{"cursor-agent"},
		installViaNpm:   false,
		// Multi-line installer: pulls the script to a tempfile, executes it,
		// then exports + persists PATH so subsequent prepare-script steps see
		// cursor-agent. Matches the script in CursorACP.InstallScript().
		installScript: `set -e
tmp="$(mktemp)"
curl -fsS https://cursor.com/install -o "$tmp"
bash "$tmp"
rm -f "$tmp"
export PATH="$HOME/.local/bin:$PATH"
grep -qxF 'export PATH="$HOME/.local/bin:$PATH"' "$HOME/.bashrc" 2>/dev/null || echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.bashrc"`,
		sessionDirTemplate: "{home}/.cursor",
	}},
	{func() Agent { return NewKimiACP() }, acpAgentSpec{
		id: "kimi-acp", displayName: "Kimi", detectBinaries: []string{"kimi"},
		expectedArgv:       []string{"kimi", "acp"},
		inferenceArgv:      []string{"kimi", "acp"},
		passthroughArgv:    []string{"kimi"},
		installViaNpm:      false,
		sessionDirTemplate: "{home}/.kimi",
	}},
	{func() Agent { return NewKiroACP() }, acpAgentSpec{
		id: "kiro-acp", displayName: "Kiro", detectBinaries: []string{"kiro-cli-chat"},
		expectedArgv:       []string{"kiro-cli-chat", "acp"},
		inferenceArgv:      []string{"kiro-cli-chat", "acp"},
		passthroughArgv:    []string{"kiro-cli-chat"},
		installViaNpm:      false,
		sessionDirTemplate: "{home}/.kiro",
	}},
	{func() Agent { return NewQoderACP() }, acpAgentSpec{
		id: "qoder-acp", displayName: "Qoder", detectBinaries: []string{"qodercli"},
		expectedArgv:       []string{"qodercli", "--acp"},
		inferenceArgv:      []string{"qodercli", "--acp"},
		passthroughArgv:    []string{"qodercli"},
		installViaNpm:      false,
		sessionDirTemplate: "{home}/.qoder",
	}},
	{func() Agent { return NewTraeACP() }, acpAgentSpec{
		id: "trae-acp", displayName: "Trae", detectBinaries: []string{"traecli"},
		expectedArgv:       []string{"traecli", "acp", "serve"},
		inferenceArgv:      []string{"traecli", "acp", "serve"},
		passthroughArgv:    []string{"traecli"},
		installViaNpm:      false,
		sessionDirTemplate: "{home}/.cache/trae-cli",
	}},
	{func() Agent { return NewOmpACP() }, acpAgentSpec{
		id: "omp-acp", displayName: "omp", detectBinaries: []string{"omp"},
		expectedArgv:       []string{"omp", "acp"},
		inferenceArgv:      []string{"omp", "acp"},
		passthroughArgv:    []string{"omp"},
		installViaNpm:      false,
		sessionDirTemplate: "{home}/.omp",
	}},
	{func() Agent { return NewDevinACP() }, acpAgentSpec{
		id: "devin-acp", displayName: "Devin", detectBinaries: []string{"devin"},
		expectedArgv:       []string{"devin", "acp"},
		inferenceArgv:      []string{"devin", "acp"},
		passthroughArgv:    []string{"devin"},
		installViaNpm:      false,
		stripEnv:           []string{"ACP_BACKEND"},
		sessionDirTemplate: "{home}/.local/share/devin",
	}},
	{func() Agent { return NewHermesACP() }, acpAgentSpec{
		id: "hermes-acp", displayName: "Hermes", detectBinaries: []string{"hermes"},
		expectedArgv:       []string{"hermes", "acp"},
		inferenceArgv:      []string{"hermes", "acp"},
		passthroughArgv:    []string{"hermes", "chat"},
		installViaNpm:      false,
		sessionDirTemplate: "{home}/.hermes",
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

			// RuntimeConfig.StripEnv is the single source of truth for the
			// persistent session path; inference derives from it separately.
			if tc.spec.stripEnv != nil {
				if !slices.Equal(rt.StripEnv, tc.spec.stripEnv) {
					t.Errorf("Runtime.StripEnv = %v, want %v", rt.StripEnv, tc.spec.stripEnv)
				}
			}

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

// TestAuggieRuntime_DeclaresServerNamespacedMCPTools covers
// AC-TASKS-MCP-TOOL-NAMES-001.1 and .2.
func TestAuggieRuntime_DeclaresServerNamespacedMCPTools(t *testing.T) {
	tests := []struct {
		name string
		new  func() Agent
		want bool
	}{
		{name: "auggie", new: func() Agent { return NewAuggie() }, want: true},
		{name: "cursor", new: func() Agent { return NewCursorACP() }, want: false},
		{name: "codex", new: func() Agent { return NewCodexACP() }, want: false},
		{name: "claude", new: func() Agent { return NewClaudeACP() }, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.new().Runtime().NamespacesMCPToolsByServer
			if got != tt.want {
				t.Errorf("NamespacesMCPToolsByServer = %t, want %t", got, tt.want)
			}
		})
	}
}

// TestNewACPAgents_SessionDirTemplate pins the session root each agent
// declares. The lifecycle manager turns SessionDirTemplate into the
// bind-mount source under <kandev-home>/agent-sessions/<instance>/, so an
// unset value means an agent's own session state is quietly lost on every
// container restart, and a wrong value mounts a directory the CLI never
// reads.
func TestNewACPAgents_SessionDirTemplate(t *testing.T) {
	for _, tc := range newACPAgentSpecs {
		t.Run(tc.spec.id, func(t *testing.T) {
			rt := tc.new().Runtime()
			if rt == nil {
				t.Fatalf("Runtime() returned nil")
			}
			if got := rt.SessionConfig.SessionDirTemplate; got != tc.spec.sessionDirTemplate {
				t.Errorf("SessionDirTemplate = %q, want %q", got, tc.spec.sessionDirTemplate)
			}
			if !strings.HasPrefix(tc.spec.sessionDirTemplate, "{home}/") {
				t.Errorf("SessionDirTemplate %q must be {home}-relative; SessionDirHostPath only trims that prefix",
					tc.spec.sessionDirTemplate)
			}
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
			if tc.spec.installScript != "" && got != tc.spec.installScript {
				t.Errorf("InstallScript() = %q, want %q", got, tc.spec.installScript)
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
// fallbacks skip the test
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

func TestPiACPDetectionRejectsPiBinaryWithoutVersionSupport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell executable fixture is Unix-specific")
	}

	binDir := t.TempDir()
	piPath := filepath.Join(binDir, "pi")
	if err := os.WriteFile(piPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake pi executable: %v", err)
	}
	t.Setenv("PATH", binDir)

	result, err := NewPiACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Fatalf("Available=true for pi executable that fails --version")
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
