package utility

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestProfileProbeForwardsEnvironmentFlagsAndPrefixToChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX shell scripts")
	}

	tmp := t.TempDir()
	profilePath := filepath.Join(tmp, "mock-agent")
	prefixPath := filepath.Join(tmp, "npx")
	writeProfileProbeFixture(t, profilePath, `#!/bin/sh
printf '%s' "$PROFILE_VALUE" > "$PROFILE_ENV_CAPTURE"
printf '%s\n' "$@" > "$PROFILE_ARGV_CAPTURE"
read -r INITIALIZE
INITIALIZE_ID=$(printf '%s' "$INITIALIZE" | sed -n 's/.*"id":\([^,}]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{}}}\n' "$INITIALIZE_ID"
read -r NEW_SESSION
SESSION_ID=$(printf '%s' "$NEW_SESSION" | sed -n 's/.*"id":\([^,}]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"test","configOptions":[]}}\n' "$SESSION_ID"
read -r EXIT_REQUEST
`)
	writeProfileProbeFixture(t, prefixPath, `#!/bin/sh
printf '%s\n' "$@" > "$PROFILE_PREFIX_CAPTURE"
if [ "$1" = "--" ]; then shift; fi
exec "$@"
`)
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	envCapture := filepath.Join(tmp, "profile-env")
	argvCapture := filepath.Join(tmp, "profile-argv")
	prefixCapture := filepath.Join(tmp, "prefix-argv")

	executor := NewACPInferenceExecutor(zap.NewNop())
	response, err := executor.Probe(context.Background(), &ProbeRequest{
		AgentID: "mock-agent",
		InferenceConfig: &InferenceConfigDTO{
			Command:       []string{"mock-agent", "--acp"},
			WorkDir:       tmp,
			Env:           map[string]string{"PROFILE_VALUE": "profile-env", "PROFILE_ENV_CAPTURE": envCapture, "PROFILE_ARGV_CAPTURE": argvCapture, "PROFILE_PREFIX_CAPTURE": prefixCapture},
			CLIFlags:      []string{"--profile-token", "profile-flag-secret"},
			CommandPrefix: []string{"npx", "--"},
		},
	})
	if err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
	if !response.Success {
		t.Fatalf("Probe failed: %s", response.Error)
	}

	gotEnv, err := os.ReadFile(envCapture)
	if err != nil {
		t.Fatalf("read child environment evidence: %v", err)
	}
	if string(gotEnv) != "profile-env" {
		t.Fatalf("child PROFILE_VALUE = %q, want profile-env", gotEnv)
	}
	gotPrefix, err := os.ReadFile(prefixCapture)
	if err != nil {
		t.Fatalf("read command-prefix evidence: %v", err)
	}
	if !strings.Contains(string(gotPrefix), "mock-agent") {
		t.Fatalf("prefix argv = %q, want wrapped mock-agent command", gotPrefix)
	}
	gotArgv, err := os.ReadFile(argvCapture)
	if err != nil {
		t.Fatalf("read child argv evidence: %v", err)
	}
	if !strings.Contains(string(gotArgv), "--profile-token\nprofile-flag-secret") {
		t.Fatalf("child argv = %q, want the ordered profile flags", gotArgv)
	}
}

func writeProfileProbeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write probe fixture %s: %v", path, err)
	}
}
