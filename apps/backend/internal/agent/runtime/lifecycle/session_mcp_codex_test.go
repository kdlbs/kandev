package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/sessionmcp"
)

func resetCodexVerified() {
	codexVerified.mu.Lock()
	codexVerified.entries = map[string]time.Time{}
	codexVerified.mu.Unlock()
}

func codexArgv(first ...string) []string {
	return append(first, "-c", sessionmcp.CodexTokenArg)
}

func TestCheckCodexCarrier(t *testing.T) {
	npx := codexArgv("npx", "-y", "@openai/codex")
	cases := []struct {
		name    string
		argv    []string
		wrapper launchwrapper.Spec
		out     string
		probe   error
		cause   string
		probed  string
	}{
		{name: "npx at minimum", argv: npx, out: "codex-cli 0.47.0", probed: "'npx' '-y' '@openai/codex' '--version'"},
		{name: "codex binary newer", argv: codexArgv("/usr/local/bin/codex"), out: "codex-cli 1.2.3", probed: "'/usr/local/bin/codex' '--version'"},
		{name: "below minimum", argv: npx, out: "codex-cli 0.46.9", cause: CauseCodexVersion},
		{name: "unparseable", argv: npx, out: "hello", cause: CauseCodexProbeFailed},
		{name: "probe error", argv: npx, probe: errors.New("timeout"), cause: CauseCodexProbeFailed},
		{name: "other command", argv: codexArgv("bash", "-lc", "codex"), cause: CauseCodexWrapper},
		{name: "npx without codex package", argv: codexArgv("npx", "-y", "other"), cause: CauseCodexWrapper},
		{name: "launch wrapper", argv: npx, wrapper: launchwrapper.Profile([]string{"nice"}), cause: CauseCodexWrapper},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetCodexVerified()
			m := newLaunchManager(&keyedStore{key: launchKey()})
			var probed string
			m.codexProbe = func(_ context.Context, _ *AgentExecution, command string) (string, error) {
				probed = command
				return tc.out, tc.probe
			}
			err := m.checkCodexCarrier(t.Context(), carrierExecution("n"), tc.argv, tc.wrapper)
			if tc.cause == "" {
				if err != nil || probed != tc.probed {
					t.Fatalf("err=%v probed=%q want %q", err, probed, tc.probed)
				}
				return
			}
			var refusal *LaunchRefusal
			if !errors.As(err, &refusal) || refusal.Code != CodeMCPTokenCarrierUnavailable || refusal.Cause != tc.cause {
				t.Fatalf("err=%v want cause %s", err, tc.cause)
			}
			if !strings.HasPrefix(err.Error(), CodeMCPTokenCarrierUnavailable+": ") {
				t.Fatalf("text %q", err.Error())
			}
		})
	}
}

func TestCheckCodexCarrierSkipsUnmarkedCommands(t *testing.T) {
	m := newLaunchManager(&keyedStore{key: launchKey()})
	m.codexProbe = func(context.Context, *AgentExecution, string) (string, error) {
		t.Fatal("probe must not run")
		return "", nil
	}
	if err := m.checkCodexCarrier(t.Context(), carrierExecution("n"), []string{"claude"}, launchwrapper.Spec{}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCodexCarrierCachesSuccessOnly(t *testing.T) {
	resetCodexVerified()
	m := newLaunchManager(&keyedStore{key: launchKey()})
	calls := 0
	out := "codex-cli 0.50.0"
	m.codexProbe = func(context.Context, *AgentExecution, string) (string, error) {
		calls++
		return out, nil
	}
	argv := codexArgv("codex")
	for range 2 {
		if err := m.checkCodexCarrier(t.Context(), carrierExecution("n"), argv, launchwrapper.Spec{}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("success must be cached, probe calls = %d", calls)
	}
	resetCodexVerified()
	out = "codex-cli 0.1.0"
	for range 2 {
		_ = m.checkCodexCarrier(t.Context(), carrierExecution("n"), argv, launchwrapper.Spec{})
	}
	if calls != 3 {
		t.Fatalf("failure must not be cached, probe calls = %d", calls)
	}
}
