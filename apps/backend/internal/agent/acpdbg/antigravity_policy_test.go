package acpdbg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/antigravityruntime"
)

func TestAntigravityDebuggerPolicy(t *testing.T) {
	params := initializeParamsForAgent("antigravity-acp", nil, nil)
	info, ok := params["clientInfo"].(map[string]any)
	if !ok || info["name"] != "zed" || info["version"] != "0.0.0" {
		t.Fatalf("antigravity clientInfo = %#v", params["clientInfo"])
	}
	other := initializeParamsForAgent("claude-acp", nil, nil)
	otherInfo := other["clientInfo"].(map[string]any)
	if otherInfo["name"] != "acpdbg" {
		t.Fatalf("other agent clientInfo = %#v", otherInfo)
	}
}

func TestAntigravityDebuggerManagedRuntime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KANDEV_HOME_DIR", home)
	artifact, err := antigravityruntime.ArtifactFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	dest := antigravityruntime.ManagedDestination(home, artifact)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	server := filepath.Join(dest, artifact.ServerName)
	harness := filepath.Join(dest, artifact.HarnessNames[0])
	if err := os.WriteFile(server, []byte("server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(harness, []byte("harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := antigravityruntime.Manifest{
		Version: artifact.Version, OS: artifact.OS, Arch: artifact.Arch,
		Digest: artifact.SHA256, Server: artifact.ServerName, Harness: artifact.HarnessNames[0],
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dest, antigravityruntime.ManifestName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	command, env, err := resolveAgentLaunch(agents.NewAntigravityACP(), []string{"agy_acp_server.par"})
	if err != nil {
		t.Fatalf("resolveAgentLaunch: %v", err)
	}
	if command[0] != server {
		t.Fatalf("debugger command = %v, want managed %s", command, server)
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "ANTIGRAVITY_HARNESS_PATH="+harness) {
		t.Fatalf("debugger env missing pinned harness: %s", joined)
	}
	if !strings.Contains(joined, "antigravity-noop-browser") {
		t.Fatalf("debugger env missing browser suppression: %s", joined)
	}
}
