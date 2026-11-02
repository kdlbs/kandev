package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/acpdbg"
)

func TestProbeRunConfigAdvertisesConfiguredAIRCapabilities(t *testing.T) {
	cfg := probeRunConfig(&sharedFlags{
		workdir:         "/tmp/worktree",
		stderr:          true,
		airCapabilities: "asyncTasks, promptHandoff",
	}, acpdbg.AgentSpec{ID: "claude", Command: []string{"claude", "acp"}})

	if !reflect.DeepEqual(cfg.AIRCapabilities, []string{"asyncTasks", "promptHandoff"}) {
		t.Fatalf("AIR capabilities = %v, want [asyncTasks promptHandoff]", cfg.AIRCapabilities)
	}
	if cfg.AgentID != "claude" || !reflect.DeepEqual(cfg.Command, []string{"claude", "acp"}) {
		t.Fatalf("probe run config lost agent command: %+v", cfg)
	}
}

func TestClientCapabilitiesCLI_MergesWithAIRCapabilitiesInRecordedInitialize(t *testing.T) {
	jsonlPath := filepath.Join(t.TempDir(), "probe.jsonl")
	clientCapabilities, err := json.Marshal(map[string]any{
		"fs":      map[string]any{"readTextFile": true},
		"session": map[string]any{"notices": map[string]any{}, "compaction": map[string]any{}},
		"_meta":   map[string]any{"custom": map[string]any{"enabled": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	args := []string{
		"probe", "--file", jsonlPath,
		"--client-capabilities", string(clientCapabilities),
		"--air-capabilities", "asyncTasks",
		"--exec", cliPeerCommand(t),
	}
	stdout, stderr, err := runCLIHelper(t, args)
	if err != nil {
		t.Fatalf("acpdbg probe failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	capabilities := recordedClientCapabilities(t, jsonlPath)
	if got, ok := capabilities["terminal"].(bool); !ok || got {
		t.Fatalf("base terminal capability = %v, want false", capabilities["terminal"])
	}
	fsCapabilities, ok := capabilities["fs"].(map[string]any)
	if !ok || fsCapabilities["readTextFile"] != true || fsCapabilities["writeTextFile"] != false {
		t.Fatalf("merged fs capabilities = %v", capabilities["fs"])
	}
	sessionCapabilities, ok := capabilities["session"].(map[string]any)
	if !ok || sessionCapabilities["notices"] == nil || sessionCapabilities["compaction"] == nil {
		t.Fatalf("session capabilities = %v", capabilities["session"])
	}
	meta, ok := capabilities["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("client capability metadata = %v", capabilities["_meta"])
	}
	customMeta, ok := meta["custom"].(map[string]any)
	if !ok || customMeta["enabled"] != true {
		t.Fatalf("custom capability metadata = %v", meta["custom"])
	}
	jetbrains, ok := meta["jetbrains"].(map[string]any)
	if !ok {
		t.Fatalf("AIR metadata missing from %v", meta)
	}
	air, ok := jetbrains["air"].(map[string]any)
	if !ok || air["version"] != float64(1) || !reflect.DeepEqual(air["capabilities"], []any{"asyncTasks"}) {
		t.Fatalf("AIR capability metadata = %v", jetbrains["air"])
	}
}

func TestClientCapabilitiesCLI_RejectsInvalidJSONBeforeStartingChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "child-started")
	args := []string{"probe", "--client-capabilities", "[]", "--exec", "touch " + marker}
	_, stderr, err := runCLIHelper(t, args)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("acpdbg exit error = %v, stderr = %s; want flag validation exit 2", err, stderr)
	}
	if !strings.Contains(string(stderr), "JSON object") {
		t.Fatalf("validation error %q does not explain the expected JSON object", stderr)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("agent child started before validation completed (marker stat error: %v)", err)
	}
}

func TestACPDBGCLIHelperProcess(t *testing.T) {
	if os.Getenv("ACPDBG_CLI_PEER") == "1" {
		serveCLIACP(t)
		return
	}
	if os.Getenv("ACPDBG_CLI_HELPER") != "1" {
		return
	}
	var args []string
	if err := json.Unmarshal([]byte(os.Getenv("ACPDBG_CLI_ARGS")), &args); err != nil {
		t.Fatalf("decode CLI args: %v", err)
	}
	if err := os.Setenv("ACPDBG_CLI_PEER", "1"); err != nil {
		t.Fatalf("configure ACP peer: %v", err)
	}
	os.Args = append([]string{"acpdbg"}, args...)
	os.Exit(run())
}

func serveCLIACP(t *testing.T) {
	t.Helper()
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(1)
		}
		result := map[string]any{}
		switch request["method"] {
		case "initialize":
			result = map[string]any{"protocolVersion": 1}
		case "session/new":
			result = map[string]any{"sessionId": "session-1"}
		}
		if err := encoder.Encode(map[string]any{
			"jsonrpc": "2.0", "id": request["id"], "result": result,
		}); err != nil {
			os.Exit(1)
		}
	}
}

func cliPeerCommand(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locate CLI test binary: %v", err)
	}
	return self + " -test.run=^TestACPDBGCLIHelperProcess$"
}

func runCLIHelper(t *testing.T, args []string) ([]byte, []byte, error) {
	t.Helper()
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("encode CLI args: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestACPDBGCLIHelperProcess$")
	cmd.Env = append(os.Environ(), "ACPDBG_CLI_HELPER=1", "ACPDBG_CLI_ARGS="+string(encodedArgs))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func recordedClientCapabilities(t *testing.T, path string) map[string]any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open JSONL: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close JSONL: %v", err)
		}
	}()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry acpdbg.Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decode JSONL entry: %v", err)
		}
		if entry.Direction != acpdbg.DirSent || entry.Frame.Method() != "initialize" {
			continue
		}
		params, ok := entry.Frame["params"].(map[string]any)
		if !ok {
			t.Fatalf("initialize params = %v", entry.Frame["params"])
		}
		capabilities, ok := params["clientCapabilities"].(map[string]any)
		if !ok {
			t.Fatalf("clientCapabilities = %v", params["clientCapabilities"])
		}
		return capabilities
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan JSONL: %v", err)
	}
	t.Fatal("initialize frame was not recorded")
	return nil
}
