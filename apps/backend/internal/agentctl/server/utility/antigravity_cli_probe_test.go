package utility

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"go.uber.org/zap"
)

func TestRealUtilityProbeAdmitsQualifiedAntigravityCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX ACP executable")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	dir := filepath.Join(t.TempDir(), "host CLI with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cliPath := filepath.Join(dir, "agy")
	marker := filepath.Join(t.TempDir(), "acp-invoked")
	writeProfileProbeFixture(t, cliPath, `#!/bin/sh
if [ "$1" = "--version" ]; then echo 'agy 1.5.15'; exit 0; fi
if [ "$1" = "acp" ] && [ "$2" = "--help" ]; then printf 'Usage: agy acp [flags]\n\nFlags:\n  --process-per-session  Serve each session from an isolated worker process\n'; exit 0; fi
if [ "$1" != "acp" ]; then exit 2; fi
case "$*" in *"--process-per-session=false --process-per-session -- after-delimiter"*) ;; *) exit 3 ;; esac
printf 'invoked' > "$ANTIGRAVITY_CLI_MARKER"
while IFS= read -r line; do
 id=$(printf '%s' "$line" | sed -n 's/.*"id":\([^,}]*\).*/\1/p')
 case "$line" in
 *'"initialize"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{},"agentInfo":{"name":"agy-fixture","version":"1.5.15"}}}\n' "$id" ;;
 *'"session/new"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"fixture-session","configOptions":[]}}\n' "$id" ;;
 esac
done
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ANTIGRAVITY_CLI_MARKER", marker)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := NewACPInferenceExecutor(zap.NewNop()).Probe(ctx, &ProbeRequest{
		AgentID: acpcompat.AntigravityAgentID,
		InferenceConfig: &InferenceConfigDTO{
			Command:  []string{cliPath, "acp", "--process-per-session"},
			WorkDir:  t.TempDir(),
			CLIFlags: []string{"--profile", "value with spaces", "--process-per-session=false", "--", "after-delimiter"},
			AuthPolicy: acpcompat.AntigravityAuthPolicyForRuntime(
				acpcompat.AntigravityRuntimeCLI, "1.5.15", false,
			),
		},
	})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !response.Success {
		t.Fatalf("Probe failed before/after ACP startup: %s", response.Error)
	}
	if response.AgentName != "agy-fixture" {
		t.Fatalf("agent name = %q, want fake ACP response", response.AgentName)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "invoked" {
		t.Fatalf("fake ACP executable evidence = %q, %v", got, err)
	}
	if _, errText := resolveSpawnCommandContext(ctx, acpcompat.AntigravityAgentID, &InferenceConfigDTO{
		Command: []string{cliPath, "acp"},
		AuthPolicy: acpcompat.AntigravityAuthPolicyForRuntime(
			acpcompat.AntigravityRuntimeCLI, "1.5.15", false,
		),
	}); errText == "" {
		t.Fatal("utility admitted a CLI command without the qualified worker-mode base arguments")
	}
}
