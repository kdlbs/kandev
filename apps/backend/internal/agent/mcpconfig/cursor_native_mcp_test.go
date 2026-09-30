package mcpconfig

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordedNativeMCPCall struct {
	executable string
	args       []string
	workspace  string
	env        map[string]string
}

type fakeNativeMCPRunner struct {
	calls   []recordedNativeMCPCall
	results []NativeMCPCommandResult
	errors  []error
}

func (f *fakeNativeMCPRunner) Run(_ context.Context, executable string, args []string, workspace string, env map[string]string) (NativeMCPCommandResult, error) {
	f.calls = append(f.calls, recordedNativeMCPCall{
		executable: executable,
		args:       append([]string(nil), args...),
		workspace:  workspace,
		env:        cloneStringMap(env),
	})
	index := len(f.calls) - 1
	if index < len(f.errors) && f.errors[index] != nil {
		return NativeMCPCommandResult{}, f.errors[index]
	}
	if index >= len(f.results) {
		return NativeMCPCommandResult{}, errors.New("unexpected native MCP command")
	}
	return f.results[index], nil
}

func TestCursorNativeMCPAdapterEnablesExactIdentityThenVerifiesTools(t *testing.T) {
	runner := &fakeNativeMCPRunner{results: []NativeMCPCommandResult{
		{ExitCode: 0, Stdout: []byte("✓ Enabled and approved MCP server: plugin-MyPlugin-GitHub")},
		{ExitCode: 0, Stdout: []byte("Tools for plugin-MyPlugin-GitHub (2):\n- search ()\n- fetch ()\n")},
	}}
	adapter := CursorNativeMCPAdapter{Executable: "/opt/cursor/cursor-agent", Runner: runner}
	env := map[string]string{"HOME": "/runtime/home", "PATH": "/runtime/bin"}

	result := adapter.EnableAndVerify(context.Background(), "/workspace/task", env, "plugin-MyPlugin-GitHub")

	if result.Status != NativeMCPStatusReady || result.ToolCount == nil || *result.ToolCount != 2 {
		t.Fatalf("EnableAndVerify() = %#v, want ready with two tools", result)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("native command count = %d, want 2", len(runner.calls))
	}
	want := [][]string{
		{"mcp", "enable", "plugin-MyPlugin-GitHub"},
		{"mcp", "list-tools", "plugin-MyPlugin-GitHub"},
	}
	for i := range want {
		if runner.calls[i].executable != "/opt/cursor/cursor-agent" || !equalStrings(runner.calls[i].args, want[i]) {
			t.Fatalf("call %d = %#v, want executable and argv %#v", i, runner.calls[i], want[i])
		}
		if runner.calls[i].workspace != "/workspace/task" || runner.calls[i].env["HOME"] != "/runtime/home" {
			t.Fatalf("call %d lost workspace/runtime environment: %#v", i, runner.calls[i])
		}
	}
}

func TestCursorNativeMCPAdapterReportsApprovalBeforeVerificationFailure(t *testing.T) {
	runner := &fakeNativeMCPRunner{results: []NativeMCPCommandResult{
		{ExitCode: 0, Stdout: []byte("✓ Enabled and approved MCP server: server")},
		{ExitCode: 1, Stderr: []byte("Failed to list tools: Authentication required")},
	}}
	got := (CursorNativeMCPAdapter{Executable: "cursor-agent", Runner: runner}).EnableAndVerify(
		context.Background(), "/workspace", nil, "server")
	if got.Status != NativeMCPStatusAuthenticationRequired || !got.ApprovalSucceeded {
		t.Fatalf("EnableAndVerify() = %#v, want approved with authentication required", got)
	}
}

func TestCursorNativeMCPAdapterClassifiesOnlyAllowlistedReadinessOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		serverID string
		result   NativeMCPCommandResult
		want     NativeMCPStatus
		wantCode string
	}{
		{name: "ready", serverID: "plugin-kandevfixture-fixture", result: NativeMCPCommandResult{ExitCode: 0, Stdout: readCursorNativeFixture(t, "list-tools-ready.txt")}, want: NativeMCPStatusReady},
		{name: "empty ready inventory", result: NativeMCPCommandResult{ExitCode: 0, Stdout: []byte("Tools for server (0):\n")}, want: NativeMCPStatusReady},
		{name: "success tool names are not failure markers", serverID: "plugin-disabled-401", result: NativeMCPCommandResult{ExitCode: 0, Stdout: []byte("Tools for plugin-disabled-401 (2):\n- handle_401 ()\n- is_disabled ()\n")}, want: NativeMCPStatusReady},
		{name: "unknown success output is not readiness", result: NativeMCPCommandResult{ExitCode: 0, Stdout: []byte("connected; no parseable tool listing")}, want: NativeMCPStatusConnectionFailed, wantCode: "connection_failed"},
		{name: "authentication", result: NativeMCPCommandResult{ExitCode: 1, Stderr: []byte("Failed to list tools: Authentication required: https://private.example/oauth?token=secret")}, want: NativeMCPStatusAuthenticationRequired, wantCode: "authentication_required"},
		{name: "native login guidance", serverID: "plugin-kandevfixture-fixture", result: NativeMCPCommandResult{ExitCode: 1, Stderr: readCursorNativeFixture(t, "list-tools-authentication-required.stderr")}, want: NativeMCPStatusAuthenticationRequired, wantCode: "authentication_required"},
		{name: "native login guidance for another server is not accepted", serverID: "plugin-other-server", result: NativeMCPCommandResult{ExitCode: 1, Stderr: readCursorNativeFixture(t, "list-tools-authentication-required.stderr")}, want: NativeMCPStatusConnectionFailed, wantCode: "connection_failed"},
		{name: "approval", serverID: "plugin-kandevfixture-fixture", result: NativeMCPCommandResult{ExitCode: 1, Stderr: readCursorNativeFixture(t, "list-tools-unapproved.stderr")}, want: NativeMCPStatusApprovalFailed, wantCode: "approval_failed"},
		{name: "unknown failure", result: NativeMCPCommandResult{ExitCode: 7, Stderr: []byte("provider says something private")}, want: NativeMCPStatusConnectionFailed, wantCode: "connection_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeNativeMCPRunner{results: []NativeMCPCommandResult{tt.result}}
			serverID := tt.serverID
			if serverID == "" {
				serverID = "server"
			}
			got := (CursorNativeMCPAdapter{Executable: "cursor-agent", Runner: runner}).Verify(context.Background(), "/workspace", nil, serverID)
			if got.Status != tt.want || got.ReasonCode != tt.wantCode {
				t.Fatalf("Verify() = %#v, want status %q and reason %q", got, tt.want, tt.wantCode)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal readiness result: %v", err)
			}
			if strings.Contains(string(encoded), "private.example") || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "provider says") {
				t.Fatalf("readiness result contains raw output: %s", encoded)
			}
		})
	}
}

func TestCursorNativeMCPAdapterDoesNotVerifyWhenEnableFails(t *testing.T) {
	runner := &fakeNativeMCPRunner{results: []NativeMCPCommandResult{{ExitCode: 1, Stderr: []byte("approval denied: sensitive details")}}}
	got := (CursorNativeMCPAdapter{Executable: "cursor-agent", Runner: runner}).EnableAndVerify(context.Background(), "/workspace", nil, "server")
	if got.Status != NativeMCPStatusApprovalFailed || got.ReasonCode != "approval_failed" {
		t.Fatalf("EnableAndVerify() = %#v, want approval_failed", got)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("native command count = %d, want only enable", len(runner.calls))
	}
}

func TestCursorNativeMCPAdapterReportsUnavailableOnRunnerFailure(t *testing.T) {
	runner := &fakeNativeMCPRunner{errors: []error{ErrNativeMCPExecutableUnavailable}}
	got := (CursorNativeMCPAdapter{Executable: "cursor-agent", Runner: runner}).Verify(context.Background(), "/workspace", nil, "server")
	if got.Status != NativeMCPStatusUnavailable || got.ReasonCode != "unavailable" {
		t.Fatalf("Verify() = %#v, want unavailable", got)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func readCursorNativeFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "cursor-agent-2026.09.26-dd393fe", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Cursor native fixture %q: %v", path, err)
	}
	return data
}
