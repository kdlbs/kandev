package utility

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestProfileProbeForwardsEnvironmentFlagsAndPrefixToChild(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
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

// A sanitized profile-probe response must not also sanitize the log beyond
// usefulness: the operator still needs the real child-side cause and the
// managed-runtime classification the client payload cannot carry.
func TestProfileProbeFailureLogCarriesScrubbedStderr(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("fixture uses a POSIX shell script")
	}

	binDir := t.TempDir()
	writeProfileProbeFixture(t, filepath.Join(binDir, "npx"), `#!/bin/sh
printf '%s\n' 'npm error code ETARGET' 'npm error notarget No matching version found for @scope/managed-acp@1.2.3.' 'cache location: /tmp/npm-cache-9f2c' >&2
exit 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	core, observed := observer.New(zapcore.DebugLevel)
	executor := NewACPInferenceExecutor(zap.New(core))
	response, err := executor.Probe(context.Background(), &ProbeRequest{
		AgentID:        "managed-acp",
		ProfileContext: true,
		InferenceConfig: &InferenceConfigDTO{
			Command: []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@scope/managed-acp@1.2.3"},
			WorkDir: t.TempDir(),
			Env:     map[string]string{"HOME": t.TempDir()},
		},
	})
	if err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
	if response.Success {
		t.Fatal("Probe unexpectedly succeeded")
	}
	if response.Error != "profile capability probe failed" {
		t.Fatalf("client error = %q, want the sanitized message", response.Error)
	}
	if response.FailureCode != "" {
		t.Fatalf("client failure code = %q, want the sanitized empty code", response.FailureCode)
	}

	entries := observed.FilterMessageSnippet("ACP profile probe failed").All()
	if len(entries) != 1 {
		t.Fatalf("profile probe failure entries = %d, want 1: %#v", len(entries), observed.All())
	}
	entry := entries[0]
	if entry.Level != zapcore.ErrorLevel {
		t.Fatalf("profile probe failure level = %v, want ERROR", entry.Level)
	}
	fields := make(map[string]string)
	for _, field := range entry.Context {
		fields[field.Key] = field.String
	}
	if stderr := fields["stderr"]; !strings.Contains(stderr, "npm error code ETARGET") ||
		strings.Contains(stderr, "/tmp/npm-cache-9f2c") || !strings.Contains(stderr, "<path>") {
		t.Fatalf("logged stderr = %q, want the npm cause with the tmp path scrubbed", stderr)
	}
	if code := fields["managed_failure_code"]; code != string(ProbeFailureManagedRuntimeNPMResolution) {
		t.Fatalf("managed failure code = %q, want %q", code, ProbeFailureManagedRuntimeNPMResolution)
	}
	if code := fields["failure_code"]; code != "" {
		t.Fatalf("client-visible failure code = %q, want the sanitized empty code", code)
	}
}

// A failing child that echoes its auth configuration must not write a live
// credential to the backend log, while the diagnostic text that explains the
// failure stays intact.
func TestProfileProbeFailureLogMasksCredentialShapedStderr(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("fixture uses a POSIX shell script")
	}

	binDir := t.TempDir()
	writeProfileProbeFixture(t, filepath.Join(binDir, "npx"), `#!/bin/sh
printf '%s\n' 'npm error code E401' 'npm error Incorrect or missing password: Bearer sk-abcdef0123456789abcdef0123456789' 'retry with api_key=ghp_abcdefghijklmnopqrstuvwxyz0123456789ABCDEF failed' 'cache location: /tmp/npm-cache-9f2c' >&2
exit 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	core, observed := observer.New(zapcore.DebugLevel)
	executor := NewACPInferenceExecutor(zap.New(core))
	response, err := executor.Probe(context.Background(), &ProbeRequest{
		AgentID:        "managed-acp",
		ProfileContext: true,
		InferenceConfig: &InferenceConfigDTO{
			Command: []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@scope/managed-acp@1.2.3"},
			WorkDir: t.TempDir(),
			Env:     map[string]string{"HOME": t.TempDir()},
		},
	})
	if err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
	if response.Success {
		t.Fatal("Probe unexpectedly succeeded")
	}

	entries := observed.FilterMessageSnippet("ACP profile probe failed").All()
	if len(entries) != 1 {
		t.Fatalf("profile probe failure entries = %d, want 1: %#v", len(entries), observed.All())
	}
	fields := make(map[string]string)
	for _, field := range entries[0].Context {
		fields[field.Key] = field.String
	}
	if stderr := fields["stderr"]; !strings.Contains(stderr, "npm error code E401") ||
		strings.Contains(stderr, "sk-abcdef0123456789") ||
		strings.Contains(stderr, "ghp_abcdefghijklmnopqrstuvwxyz") ||
		!strings.Contains(stderr, "***") {
		t.Fatalf("logged stderr = %q, want credentials masked with diagnostic text intact", stderr)
	}
	if stderr := fields["stderr"]; strings.Contains(stderr, "/tmp/npm-cache-9f2c") {
		t.Fatalf("logged stderr = %q, want the tmp path scrubbed", stderr)
	}
}

// The direct (non-profile) probe failure log carries the same child stderr
// tail and therefore the same credential exposure as the profile site.
func TestProbeFailureLogMasksCredentialShapedStderr(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("fixture uses a POSIX shell script")
	}

	binDir := t.TempDir()
	writeProfileProbeFixture(t, filepath.Join(binDir, "npx"), `#!/bin/sh
printf '%s\n' 'npm error code E401' 'npm error Incorrect or missing password: Bearer sk-abcdef0123456789abcdef0123456789' 'retry with api_key=ghp_abcdefghijklmnopqrstuvwxyz0123456789ABCDEF failed' 'cache location: /tmp/npm-cache-9f2c' >&2
exit 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	core, observed := observer.New(zapcore.DebugLevel)
	executor := NewACPInferenceExecutor(zap.New(core))
	response, err := executor.Probe(context.Background(), &ProbeRequest{
		AgentID: "managed-acp",
		InferenceConfig: &InferenceConfigDTO{
			Command: []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@scope/managed-acp@1.2.3"},
			WorkDir: t.TempDir(),
			Env:     map[string]string{"HOME": t.TempDir()},
		},
	})
	if err != nil {
		t.Fatalf("Probe returned error: %v", err)
	}
	if response.Success {
		t.Fatal("Probe unexpectedly succeeded")
	}

	entries := observed.FilterMessageSnippet("ACP probe failed").All()
	if len(entries) != 1 {
		t.Fatalf("probe failure entries = %d, want 1: %#v", len(entries), observed.All())
	}
	fields := make(map[string]string)
	for _, field := range entries[0].Context {
		fields[field.Key] = field.String
	}
	if stderr := fields["stderr"]; !strings.Contains(stderr, "npm error code E401") ||
		strings.Contains(stderr, "sk-abcdef0123456789") ||
		strings.Contains(stderr, "ghp_abcdefghijklmnopqrstuvwxyz") ||
		!strings.Contains(stderr, "***") {
		t.Fatalf("logged stderr = %q, want credentials masked with diagnostic text intact", stderr)
	}
	if stderr := fields["stderr"]; strings.Contains(stderr, "/tmp/npm-cache-9f2c") {
		t.Fatalf("logged stderr = %q, want the tmp path scrubbed", stderr)
	}
}
