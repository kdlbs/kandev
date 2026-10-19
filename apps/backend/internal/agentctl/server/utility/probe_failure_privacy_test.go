package utility

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestProbeFailureLogRedactsProviderDiagnostics(t *testing.T) {
	const opaque = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefgh"
	const detail = "npm error code E401; opaque " + opaque +
		"; account https://example.test/accounts/private-account?access=private-query" +
		"; workspace /home/private-user/private-workspace; cache /tmp/private-cache"
	for _, profile := range []bool{false, true} {
		t.Run(fmt.Sprintf("profile-%t", profile), func(t *testing.T) {
			response, entries := failedProbePrivacyFixture(t, profile, detail, "upstream refused Bearer "+opaque)
			fields, err := json.Marshal(entries)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{opaque, "private-account", "private-query", "private-user", "private-workspace", "private-cache"} {
				if strings.Contains(string(fields), secret) {
					t.Errorf("probe failure log exposed %q: %s", secret, fields)
				}
				if strings.Contains(response.Error, secret) {
					t.Errorf("probe failure response exposed %q: %s", secret, response.Error)
				}
			}
			if !strings.Contains(entries["stderr"].(string), "npm error code E401") {
				t.Fatalf("probe failure log lost the diagnostic: %#v", entries)
			}
			if profile && (response.Error != "profile capability probe failed" || response.FailureCode != "") {
				t.Fatalf("profile response changed: %#v", response)
			}
		})
	}
}

func TestProbeFailureLogDiscardsTruncatedCredential(t *testing.T) {
	const fragment = "fragmentSecret"
	const cause = "npm error code ETARGET\nnpm error notarget No matching version found for @scope/managed-acp@1.2.3.\n"
	diagnostics := cause + strings.Repeat(".", stderrTailLimit-len(fragment+"\n")-len(cause))
	stderr := "api_key=" + strings.Repeat("a", stderrTailLimit) + fragment + "\n" + diagnostics
	for _, profile := range []bool{false, true} {
		t.Run(fmt.Sprintf("profile-%t", profile), func(t *testing.T) {
			response, fields := failedProbePrivacyFixture(t, profile, stderr, "")
			logged := fields["stderr"].(string)
			if strings.Contains(logged, fragment) {
				t.Fatalf("probe failure log exposed truncated credential fragment: %q", logged)
			}
			if !strings.Contains(logged, "npm error code ETARGET") || len(logged) > stderrTailLimit {
				t.Fatalf("probe failure log lost bounded diagnostics: %q", logged)
			}
			code := response.FailureCode
			if profile {
				code = ProbeFailureCode(fields["managed_failure_code"].(string))
			}
			if code != ProbeFailureManagedRuntimeNPMResolution {
				t.Fatalf("managed classification = %q, want %q", code, ProbeFailureManagedRuntimeNPMResolution)
			}
		})
	}
}

func TestProbeFailureLogDiscardsTruncatedUnterminatedLine(t *testing.T) {
	_, fields := failedProbePrivacyFixture(t, true, "api_key="+strings.Repeat("a", stderrTailLimit+1), "")
	if fields["stderr"] != "" {
		t.Fatalf("probe failure log retained an incomplete credential line: %#v", fields)
	}
}

func TestProbeFailureLogKeepsCompleteBoundaryLine(t *testing.T) {
	const cause = "npm error code E401\n"
	diagnostics := cause + strings.Repeat(".", stderrTailLimit-len(cause))
	stderr := strings.Repeat("a", stderrTailLimit) + "\n" + diagnostics
	_, fields := failedProbePrivacyFixture(t, true, stderr, "")
	if !strings.HasPrefix(fields["stderr"].(string), cause) {
		t.Fatalf("probe failure log discarded a complete boundary line: %#v", fields)
	}
}

func TestProfileProbeFailureLogRetainsProtocolErrorWithoutStderr(t *testing.T) {
	_, fields := failedProbePrivacyFixture(t, true, "", "upstream unavailable")
	if !strings.Contains(fmt.Sprint(fields["error"]), "upstream unavailable") {
		t.Fatalf("profile failure log lost the protocol cause without stderr: %#v", fields)
	}
}

func TestProbeFailureLogKeepsDiagnosticsInRelayMessage(t *testing.T) {
	const cause = "npm error code ETARGET\nnpm error notarget No matching version found for @scope/managed-acp@1.2.3.\n"
	for _, profile := range []bool{false, true} {
		t.Run(fmt.Sprintf("profile-%t", profile), func(t *testing.T) {
			_, fields := failedProbePrivacyFixture(t, profile, cause, "upstream unavailable")
			message := fields["log_message"].(string)
			for _, detail := range []string{"upstream unavailable", "npm error code ETARGET", string(ProbeFailureManagedRuntimeNPMResolution)} {
				if !strings.Contains(message, detail) {
					t.Errorf("relay message lost safe diagnostic %q: %q", detail, message)
				}
			}
		})
	}
}

func TestProbeModelDiscoveryFailureLogRedactsProviderError(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("fixture uses a POSIX shell script")
	}
	binDir := t.TempDir()
	writeProfileProbeFixture(t, filepath.Join(binDir, "opencode"), "#!/bin/sh\nprintf '%s' 'npm error code E401; Bearer ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789' >&2\nexit 1\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	core, observed := observer.New(zapcore.WarnLevel)
	executor := NewACPInferenceExecutor(zap.New(core))
	if models := executor.loadOpenCodeModels(context.Background(), "opencode", t.TempDir(), false, false); len(models) != 0 {
		t.Fatalf("failed model discovery returned models: %#v", models)
	}
	entries := observed.FilterMessageSnippet("ACP probe: failed to list opencode models").All()
	if len(entries) != 1 {
		t.Fatalf("model discovery failure entries = %d, want 1", len(entries))
	}
	fields, err := json.Marshal(entries[0].ContextMap())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fields), "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") || !strings.Contains(string(fields), "npm error code E401") {
		t.Fatalf("model discovery failure log is unsafe or lost diagnostics: %s", fields)
	}
}

func failedProbePrivacyFixture(t *testing.T, profile bool, stderr, rpcError string) (*ProbeResponse, map[string]interface{}) {
	t.Helper()
	if runtime.GOOS == windowsGOOS {
		t.Skip("fixture uses a POSIX shell script")
	}
	binDir := t.TempDir()
	fixture := "#!/bin/sh\nprintf '%s' '" + stderr + "' >&2\n"
	if rpcError == "" {
		fixture += "exit 1\n"
	} else {
		fixture += "read -r INITIALIZE\nINITIALIZE_ID=$(printf '%s' \"$INITIALIZE\" | sed -n 's/.*\"id\":\\([^,}]*\\).*/\\1/p')\n" +
			"printf '{\"jsonrpc\":\"2.0\",\"id\":%s,\"error\":{\"code\":-32000,\"message\":\"" + rpcError + "\"}}\\n' \"$INITIALIZE_ID\"\nread -r EXIT_REQUEST\n"
	}
	writeProfileProbeFixture(t, filepath.Join(binDir, "npx"), fixture)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	core, observed := observer.New(zapcore.ErrorLevel)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, err := NewACPInferenceExecutor(zap.New(core)).Probe(ctx, &ProbeRequest{
		AgentID: "managed-acp", ProfileContext: profile,
		InferenceConfig: &InferenceConfigDTO{
			Command: []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@scope/managed-acp@1.2.3"},
			WorkDir: t.TempDir(), Env: map[string]string{"HOME": t.TempDir()},
		},
	})
	if err != nil || response == nil || response.Success {
		t.Fatalf("expected failed probe response, got %#v, %v", response, err)
	}
	message := "ACP probe failed"
	if profile {
		message = "ACP profile probe failed"
	}
	entries := observed.FilterMessageSnippet(message).All()
	if len(entries) != 1 {
		t.Fatalf("probe failure entries = %d, want 1: %#v", len(entries), observed.All())
	}
	fields := entries[0].ContextMap()
	fields["log_message"] = entries[0].Message
	return response, fields
}
