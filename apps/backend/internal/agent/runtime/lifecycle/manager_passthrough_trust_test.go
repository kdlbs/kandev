package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func readTrustMap(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trust file: %v", err)
	}
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse trust file: %v", err)
	}
	return m
}

func TestSeedTrustedFolder_CreatesFileWhenMissing(t *testing.T) {
	m := &Manager{logger: newTestLogger()}
	path := filepath.Join(t.TempDir(), ".gemini", "trustedFolders.json")
	ws := "/Users/eugenn/.kandev/tasks/ajo/proj"

	added, err := m.seedTrustedFolder(path, ws, "TRUST_FOLDER")
	if err != nil {
		t.Fatalf("seedTrustedFolder() error = %v", err)
	}
	if !added {
		t.Fatal("added = false, want true for a new entry")
	}
	if got := readTrustMap(t, path)[ws]; got != "TRUST_FOLDER" {
		t.Fatalf("trust[%q] = %q, want TRUST_FOLDER", ws, got)
	}
}

func TestSeedTrustedFolder_PreservesExistingEntries(t *testing.T) {
	m := &Manager{logger: newTestLogger()}
	dir := t.TempDir()
	path := filepath.Join(dir, "trustedFolders.json")
	if err := os.WriteFile(path, []byte(`{"/home/user":"TRUST_FOLDER"}`), 0o600); err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	ws := "/work/space"

	added, err := m.seedTrustedFolder(path, ws, "TRUST_FOLDER")
	if err != nil {
		t.Fatalf("seedTrustedFolder() error = %v", err)
	}
	if !added {
		t.Fatal("added = false, want true")
	}
	got := readTrustMap(t, path)
	if got["/home/user"] != "TRUST_FOLDER" || got[ws] != "TRUST_FOLDER" {
		t.Fatalf("trust map = %v, want both entries present", got)
	}
}

func TestSeedTrustedFolder_NoOpWhenAlreadyTrusted(t *testing.T) {
	m := &Manager{logger: newTestLogger()}
	dir := t.TempDir()
	path := filepath.Join(dir, "trustedFolders.json")
	ws := "/work/space"
	if err := os.WriteFile(path, []byte(`{"/work/space":"TRUST_FOLDER"}`), 0o600); err != nil {
		t.Fatalf("seed existing: %v", err)
	}

	added, err := m.seedTrustedFolder(path, ws, "TRUST_FOLDER")
	if err != nil {
		t.Fatalf("seedTrustedFolder() error = %v", err)
	}
	if added {
		t.Fatal("added = true, want false when the folder is already trusted")
	}
}

// TestApplyAndCleanupPassthroughTrust verifies the full launch+teardown cycle:
// launch seeds the workspace into the agent's trusted-folders file and records
// it; cleanup removes only that entry, leaving the user's untouched.
func TestApplyAndCleanupPassthroughTrust(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // windows

	trustPath := filepath.Join(home, ".gemini", "trustedFolders.json")
	if err := os.MkdirAll(filepath.Dir(trustPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(trustPath, []byte(`{"/pre/existing":"TRUST_FOLDER"}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m := &Manager{logger: newTestLogger()}
	ws := filepath.Join(home, ".kandev", "tasks", "abc", "proj")
	execution := &AgentExecution{WorkspacePath: ws, Metadata: map[string]interface{}{}}

	m.applyPassthroughTrust(execution, agents.NewAntigravity())

	got := readTrustMap(t, trustPath)
	if got[filepath.Clean(ws)] != "TRUST_FOLDER" {
		t.Fatalf("workspace not trusted after launch: %v", got)
	}
	if got["/pre/existing"] != "TRUST_FOLDER" {
		t.Fatalf("pre-existing entry lost: %v", got)
	}
	if getPassthroughTrustFile(execution) != trustPath {
		t.Fatalf("trust file not recorded in metadata: %q", getPassthroughTrustFile(execution))
	}

	m.cleanupPassthroughTrust(execution)

	got = readTrustMap(t, trustPath)
	if _, present := got[filepath.Clean(ws)]; present {
		t.Fatalf("workspace entry not removed on cleanup: %v", got)
	}
	if got["/pre/existing"] != "TRUST_FOLDER" {
		t.Fatalf("cleanup clobbered the user's entry: %v", got)
	}
}

// TestApplyPassthroughTrust_NonTrustAgentNoOp ensures agents that don't gate on
// folder trust never touch any trust file.
func TestApplyPassthroughTrust_NonTrustAgentNoOp(t *testing.T) {
	m := &Manager{logger: newTestLogger()}
	execution := &AgentExecution{WorkspacePath: "/work/space", Metadata: map[string]interface{}{}}

	// testAgent does not implement PassthroughTrustAgent.
	m.applyPassthroughTrust(execution, &testAgent{id: "mock"})

	if getPassthroughTrustFile(execution) != "" {
		t.Fatalf("recorded a trust file for a non-trust agent: %q", getPassthroughTrustFile(execution))
	}
}
