//go:build copilot_native

package copilotplugins_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/acpdbg"
)

func TestNativeVersionOutputParser(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		version string
		want    bool
	}{
		{
			name:    "exact managed package output",
			output:  "GitHub Copilot CLI 1.0.88.\nRun 'copilot update' to check for updates.\n",
			version: "1.0.88",
			want:    true,
		},
		{
			name:    "bare exact version",
			output:  "1.0.104\n",
			version: "1.0.104",
			want:    true,
		},
		{
			name:    "different version",
			output:  "GitHub Copilot CLI 1.0.105.\n",
			version: "1.0.104",
			want:    false,
		},
		{
			name:    "version embedded in unrelated text",
			output:  "Copilot CLI 1.0.104 is running\n",
			version: "1.0.104",
			want:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := containsExactVersion([]byte(test.output), test.version); got != test.want {
				t.Fatalf("containsExactVersion() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestNativePermissionInputGuard(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  bool
	}{
		{name: "explicit empty object", input: map[string]any{}, want: true},
		{name: "missing input", input: nil, want: false},
		{name: "nonempty object", input: map[string]any{"path": "outside"}, want: false},
		{name: "non-object input", input: "unexpected", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nativeInputIsEmpty(test.input); got != test.want {
				t.Fatalf("nativeInputIsEmpty() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestNativePolicyServerIdentity(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "exact managed server label", value: `server "wo03_plugin_target_server" blocked`, want: true},
		{name: "exact MCP tool namespace", value: "mcp__wo03_plugin_target_server__acpdbg_sentinel", want: true},
		{name: "similar longer label", value: "wo03_plugin_target_server_shadow", want: false},
		{name: "similar prefixed label", value: "other_wo03_plugin_target_server", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nativeServerMentioned(test.value, nativeTargetServer); got != test.want {
				t.Fatalf("nativeServerMentioned() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestNativeEnvironmentKeyAllowlist(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{key: "PATH", want: true},
		{key: "COPILOT_HOME", want: true},
		{key: "LC_CTYPE", want: true},
		{key: "GH_TOKEN", want: false},
		{key: "ANTHROPIC_API_KEY", want: false},
		{key: "COPILOT_ALLOW_ALL", want: false},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			if got := nativeEnvironmentKeyAllowed(test.key); got != test.want {
				t.Fatalf("nativeEnvironmentKeyAllowed(%q) = %t, want %t", test.key, got, test.want)
			}
		})
	}
}

func TestNativeGlobalArgsMatchCandidateCLIOptions(t *testing.T) {
	tests := []struct {
		version              string
		wantRemoteDisable    bool
		wantTelemetryDisable bool
	}{
		{version: "1.0.88", wantRemoteDisable: true},
		{version: "1.0.104", wantTelemetryDisable: true},
	}

	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			args := " " + strings.Join(nativeGlobalArgs(test.version, nativeRunPaths{logRoot: "private-logs"}), " ") + " "
			hasRemoteDisable := strings.Contains(args, " --no-remote ") &&
				strings.Contains(args, " --no-remote-export ")
			if hasRemoteDisable != test.wantRemoteDisable {
				t.Fatalf("remote-disable flags present = %t, want %t", hasRemoteDisable, test.wantRemoteDisable)
			}
			if got := strings.Contains(args, " --disable-client-telemetry "); got != test.wantTelemetryDisable {
				t.Fatalf("telemetry-disable flag present = %t, want %t", got, test.wantTelemetryDisable)
			}
		})
	}
}

func TestNativeACPCommandUsesCopilotServerToolIdentifier(t *testing.T) {
	args := nativeACPArgs(
		"1.0.104",
		nativeRunPaths{logRoot: "private-logs"},
		[]string{"copilot"},
		nativePluginFixture{targetRoot: "target-root", siblingRoot: "sibling-root"},
		&nativeTestSentinels{},
		"",
		false,
		[]string{nativeTargetServer},
	)

	availableTools := -1
	for index, arg := range args {
		if arg == "--available-tools" {
			availableTools = index
			break
		}
	}
	if availableTools < 0 {
		t.Fatal("native ACP command omitted --available-tools")
	}
	got := args[availableTools+1:]
	const want = "wo03_plugin_target_server-acpdbg_sentinel"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("native ACP --available-tools selectors = %q, want exact server-tool identifier %q", got, want)
	}
}

func TestNativePermissionRejectsUnannotatedSentinelDespiteReadKind(t *testing.T) {
	recorder, err := acpdbg.NewRecorder(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })

	sentinels := newNativeTestSentinels(recorder)
	t.Cleanup(sentinels.close)
	sentinels.servers[nativeTargetServer].Close()
	sentinels.servers[nativeTargetServer] = acpdbg.NewMCPSentinel(recorder)

	var output bytes.Buffer
	driver := &nativeACPDriver{
		framer:   acpdbg.NewFramer(&output, bytes.NewReader(nil)),
		recorder: recorder,
		toolCalls: map[string]*nativeToolCall{
			"call-1": {
				name:     nativeTargetServer + "-" + nativeSentinelToolName,
				kind:     "read",
				rawInput: map[string]any{},
			},
		},
		knownServers:   []string{nativeTargetServer},
		allowedServers: map[string]bool{nativeTargetServer: true},
		approved:       make(map[string]int),
		rejected:       make(map[string]int),
		sentinels:      sentinels,
	}
	request := acpdbg.Frame{
		"jsonrpc": "2.0",
		"id":      1,
		"params": map[string]any{
			"toolCall": map[string]any{"toolCallId": "call-1"},
			"options": []any{
				map[string]any{"kind": "allow_always", "optionId": "always"},
				map[string]any{"kind": "allow_once", "optionId": "allow"},
				map[string]any{"kind": "reject_once", "optionId": "reject"},
			},
		},
	}
	if err := driver.handlePermissionRequest(request); err != nil {
		t.Fatal(err)
	}
	if driver.approved[nativeTargetServer] != 0 || driver.rejected[nativeTargetServer] != 1 {
		t.Fatalf("unannotated sentinel permission counts = approved:%d rejected:%d, want approved:0 rejected:1",
			driver.approved[nativeTargetServer], driver.rejected[nativeTargetServer])
	}

	var response acpdbg.Frame
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	result, _ := response["result"].(map[string]any)
	outcome, _ := result["outcome"].(map[string]any)
	if outcome["outcome"] != "selected" || outcome["optionId"] != "reject" {
		t.Fatalf("unannotated sentinel response = %v, want reject_once", outcome)
	}
}

func TestNativePermissionAllowsAnnotatedSentinelWithoutACPKind(t *testing.T) {
	recorder, err := acpdbg.NewRecorder(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })

	sentinel := newNativeMCPSSESentinel(recorder)
	t.Cleanup(sentinel.Close)
	list := postNativeHTTPMCPRequest(t, &http.Client{}, sentinel.HTTPURL(), map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	})
	if list["result"] == nil {
		t.Fatalf("read-only fixture tools/list omitted its result: %v", list)
	}
	sentinels := &nativeTestSentinels{
		servers: map[string]nativeTestSentinel{nativeTargetServer: sentinel},
	}

	var output bytes.Buffer
	driver := &nativeACPDriver{
		framer:         acpdbg.NewFramer(&output, bytes.NewReader(nil)),
		recorder:       recorder,
		toolCalls:      make(map[string]*nativeToolCall),
		knownServers:   []string{nativeTargetServer},
		allowedServers: map[string]bool{nativeTargetServer: true},
		approved:       make(map[string]int),
		rejected:       make(map[string]int),
		sentinels:      sentinels,
	}
	request := acpdbg.Frame{
		"jsonrpc": "2.0",
		"id":      1,
		"params": map[string]any{
			"toolCall": map[string]any{
				"toolCallId": "call-1",
				"name":       nativeTargetServer + "-" + nativeSentinelToolName,
				"rawInput":   map[string]any{},
			},
			"options": []any{
				map[string]any{"kind": "allow_always", "optionId": "always"},
				map[string]any{"kind": "allow_once", "optionId": "allow"},
				map[string]any{"kind": "reject_once", "optionId": "reject"},
			},
		},
	}
	if err := driver.handlePermissionRequest(request); err != nil {
		t.Fatal(err)
	}
	if driver.approved[nativeTargetServer] != 1 || driver.rejected[nativeTargetServer] != 0 {
		t.Fatalf("annotated sentinel permission counts = approved:%d rejected:%d, want approved:1 rejected:0",
			driver.approved[nativeTargetServer], driver.rejected[nativeTargetServer])
	}

	var response acpdbg.Frame
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	result, _ := response["result"].(map[string]any)
	outcome, _ := result["outcome"].(map[string]any)
	if outcome["outcome"] != "selected" || outcome["optionId"] != "allow" {
		t.Fatalf("annotated sentinel response = %v, want allow_once", outcome)
	}

	driver.toolCalls["call-2"] = &nativeToolCall{
		name:     nativeTargetServer + "-" + nativeSentinelToolName,
		rawInput: map[string]any{},
	}
	request["id"] = 2
	params := request["params"].(map[string]any)
	toolCall := params["toolCall"].(map[string]any)
	toolCall["toolCallId"] = "call-2"
	if err := driver.handlePermissionRequest(request); err != nil {
		t.Fatal(err)
	}
	if driver.approved[nativeTargetServer] != 1 || driver.rejected[nativeTargetServer] != 1 {
		t.Fatalf("repeat sentinel permission counts = approved:%d rejected:%d, want approved:1 rejected:1",
			driver.approved[nativeTargetServer], driver.rejected[nativeTargetServer])
	}
	lines := bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("permission responses = %d, want one allow-once and one rejection", len(lines))
	}
	if err := json.Unmarshal(lines[1], &response); err != nil {
		t.Fatal(err)
	}
	result, _ = response["result"].(map[string]any)
	outcome, _ = result["outcome"].(map[string]any)
	if outcome["outcome"] != "selected" || outcome["optionId"] != "reject" {
		t.Fatalf("repeated sentinel response = %v, want reject_once", outcome)
	}
}
