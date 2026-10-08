package mcpconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const cursorAuthProvenanceTestFilename = "kandev-mcp-auth-unified.json.provenance"

func TestAggregateCursorMCPAuthPreservesNativeRefreshWhileSourceIsUnchanged(t *testing.T) {
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	source := writeCursorAuth(t, projects, "trusted-source", `{"github":{"tokens":{"access_token":"source-access","refresh_token":"source-refresh"},"clientInfo":{"client_id":"client-1"},"opaque":{"keep":true}}}`, time.Now())
	if err := AggregateCursorMCPAuth(cursorHome); err != nil {
		t.Fatalf("initial aggregate: %v", err)
	}

	master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
	writeNativeMaster(t, master, `{"github":{"tokens":{"access_token":"rotated-access","refresh_token":"rotated-refresh"},"clientInfo":{"client_id":"client-1"},"opaque":{"keep":true},"cursor_extension":{"keep":true}}}`)
	for range 2 {
		if err := AggregateCursorMCPAuth(cursorHome); err != nil {
			t.Fatalf("refresh aggregate: %v", err)
		}
		got := readCursorAuth(t, master)["github"]
		if got["tokens"].(map[string]any)["access_token"] != "rotated-access" {
			t.Fatalf("native access token was replaced: %#v", got)
		}
		if got["cursor_extension"] == nil {
			t.Fatalf("native whole server object was not retained: %#v", got)
		}
	}
	masterData, err := os.ReadFile(master)
	if err != nil {
		t.Fatal(err)
	}
	var projected map[string]json.RawMessage
	if err := json.Unmarshal(masterData, &projected); err != nil {
		t.Fatalf("decode projected master: %v", err)
	}
	var parsedProvenance cursorMCPAuthProvenance
	if err := json.Unmarshal(mustReadFile(t, filepath.Join(cursorHome, cursorAuthProvenanceTestFilename)), &parsedProvenance); err != nil {
		t.Fatalf("decode provenance: %v", err)
	}
	entry := parsedProvenance.Servers["github"]
	if !entry.NativeUpdated || entry.ProjectedFingerprint != cursorAuthFingerprint(projected["github"]) {
		t.Fatalf("provenance did not record last native projection: %#v", entry)
	}

	provenance, err := os.ReadFile(filepath.Join(cursorHome, cursorAuthProvenanceTestFilename))
	if err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	info, err := os.Stat(filepath.Join(cursorHome, cursorAuthProvenanceTestFilename))
	if err != nil {
		t.Fatalf("stat provenance: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("provenance permissions = %04o, want 0600", mode)
	}
	for _, secret := range []string{"source-access", "source-refresh", "rotated-access", "rotated-refresh"} {
		if strings.Contains(string(provenance), secret) {
			t.Fatalf("provenance contains credential value %q", secret)
		}
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source unexpectedly disappeared: %v", err)
	}
}

func TestAggregateCursorMCPAuthPreservesNativeLoginFromRegistrationOnlySource(t *testing.T) {
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	writeCursorAuth(t, projects, "trusted-source", `{"github":{"clientInfo":{"client_id":"registered-client"},"opaque":{"keep":true}}}`, time.Now())
	if err := AggregateCursorMCPAuth(cursorHome); err != nil {
		t.Fatalf("initial aggregate: %v", err)
	}

	master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
	writeNativeMaster(t, master, `{"github":{"tokens":{"access_token":"login-access","refresh_token":"login-refresh"},"clientInfo":{"client_id":"rotated-client"},"opaque":{"keep":true},"native":{"whole_object":true}}}`)
	if err := AggregateCursorMCPAuth(cursorHome); err != nil {
		t.Fatalf("aggregate after native login: %v", err)
	}
	for range 2 {
		if err := AggregateCursorMCPAuth(cursorHome); err != nil {
			t.Fatalf("refresh aggregate: %v", err)
		}
		got := readCursorAuth(t, master)["github"]
		if got["tokens"].(map[string]any)["access_token"] != "login-access" ||
			got["clientInfo"].(map[string]any)["client_id"] != "rotated-client" || got["native"] == nil {
			t.Fatalf("native login object was not preserved: %#v", got)
		}
	}
}

func TestAggregateCursorMCPAuthChangedOrRemovedSourceInvalidatesNativeRefresh(t *testing.T) {
	t.Run("changed source is reselected", func(t *testing.T) {
		cursorHome := t.TempDir()
		projects := filepath.Join(cursorHome, "projects")
		source := writeCursorAuth(t, projects, "trusted-source", `{"github":{"tokens":{"access_token":"source-access"},"clientInfo":{"client_id":"client-1"}}}`, time.Now())
		if err := AggregateCursorMCPAuth(cursorHome); err != nil {
			t.Fatalf("initial aggregate: %v", err)
		}
		master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
		writeNativeMaster(t, master, `{"github":{"tokens":{"access_token":"native-refresh"},"clientInfo":{"client_id":"client-1"}}}`)
		if err := os.WriteFile(source, []byte(`{"github":{"tokens":{"access_token":"new-source"},"clientInfo":{"client_id":"client-2"}}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := AggregateCursorMCPAuth(cursorHome); err != nil {
			t.Fatalf("aggregate after source change: %v", err)
		}
		got := readCursorAuth(t, master)["github"]
		if got["tokens"].(map[string]any)["access_token"] != "new-source" || got["clientInfo"].(map[string]any)["client_id"] != "client-2" {
			t.Fatalf("changed source was not reselected: %#v", got)
		}
	})

	t.Run("removed source is dropped", func(t *testing.T) {
		cursorHome := t.TempDir()
		projects := filepath.Join(cursorHome, "projects")
		removed := writeCursorAuth(t, projects, "removed-source", `{"github":{"tokens":{"access_token":"source-access"},"clientInfo":{"client_id":"client-1"}}}`, time.Now())
		writeCursorAuth(t, projects, "remaining-source", `{"calendar":{"tokens":{"access_token":"calendar-access"},"clientInfo":{"client_id":"calendar-1"}}}`, time.Now().Add(time.Second))
		if err := AggregateCursorMCPAuth(cursorHome); err != nil {
			t.Fatalf("initial aggregate: %v", err)
		}
		master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
		writeNativeMaster(t, master, `{"github":{"tokens":{"access_token":"native-refresh"},"clientInfo":{"client_id":"client-1"}},"calendar":{"tokens":{"access_token":"calendar-access"},"clientInfo":{"client_id":"calendar-1"}}}`)
		if err := os.Remove(removed); err != nil {
			t.Fatal(err)
		}
		if err := AggregateCursorMCPAuth(cursorHome); err != nil {
			t.Fatalf("aggregate after source removal: %v", err)
		}
		got := readCursorAuth(t, master)
		if _, exists := got["github"]; exists {
			t.Fatalf("removed source server survived in master: %#v", got)
		}
		if _, exists := got["calendar"]; !exists {
			t.Fatalf("remaining server was lost: %#v", got)
		}
	})
}

func TestAggregateCursorMCPAuthRejectsUnrelatedMasterAndInvalidProvenance(t *testing.T) {
	for _, tc := range []struct {
		name         string
		provenance   string
		changeSource bool
	}{
		{name: "invalid sidecar", provenance: "not-json"},
		{name: "partial publication", provenance: `{"version":1,"state":"pending","servers":{}}`},
		{name: "source changed", changeSource: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cursorHome := t.TempDir()
			projects := filepath.Join(cursorHome, "projects")
			writeCursorAuth(t, projects, "trusted-source", `{"github":{"tokens":{"access_token":"source-access"},"clientInfo":{"client_id":"client-1"}}}`, time.Now())
			if err := AggregateCursorMCPAuth(cursorHome); err != nil {
				t.Fatalf("initial aggregate: %v", err)
			}
			master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
			if tc.changeSource {
				if err := os.WriteFile(filepath.Join(projects, "trusted-source", cursorMCPAuthFilename), []byte(`{"github":{"tokens":{"access_token":"new-source"},"clientInfo":{"client_id":"client-2"}}}`), 0o600); err != nil {
					t.Fatal(err)
				}
				writeNativeMaster(t, master, `{"github":{"tokens":{"access_token":"unrelated-secret"},"clientInfo":{"client_id":"attacker-client"}}}`)
			} else {
				if err := os.WriteFile(filepath.Join(cursorHome, cursorAuthProvenanceTestFilename), []byte(tc.provenance), 0o600); err != nil {
					t.Fatal(err)
				}
				writeNativeMaster(t, master, `{"github":{"tokens":{"access_token":"unrelated-secret"},"clientInfo":{"client_id":"attacker-client"}}}`)
			}
			if err := AggregateCursorMCPAuth(cursorHome); err != nil {
				t.Fatalf("aggregate with invalid authorization: %v", err)
			}
			got := readCursorAuth(t, master)["github"]
			want := "source-access"
			if tc.changeSource {
				want = "new-source"
			}
			if got["tokens"].(map[string]any)["access_token"] != want {
				t.Fatalf("unrelated master was accepted: %#v", got)
			}
		})
	}
}

func TestAggregateCursorMCPAuthSidecarFailureLeavesMasterUnchanged(t *testing.T) {
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	writeCursorAuth(t, projects, "trusted-source", `{"github":{"tokens":{"access_token":"source-access"},"clientInfo":{"client_id":"client-1"}}}`, time.Now())
	master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
	before := []byte(`{"github":{"tokens":{"access_token":"prior-master"},"clientInfo":{"client_id":"client-1"}}}`)
	if err := os.WriteFile(master, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(cursorHome, cursorAuthProvenanceTestFilename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AggregateCursorMCPAuth(cursorHome); err == nil {
		t.Fatal("aggregate succeeded with an unreplaceable provenance path")
	}
	after, err := os.ReadFile(master)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("master changed before sidecar publication succeeded: %s", after)
	}
}

func TestAggregateCursorMCPAuthLastSourceRemovalBreaksExistingBridgeLinks(t *testing.T) {
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	source := writeCursorAuth(t, projects, "trusted-source", `{"github":{"tokens":{"access_token":"source-access"},"clientInfo":{"client_id":"client-1"}}}`, time.Now())
	workspace := t.TempDir()
	if err := LinkCursorMCPAuth(workspace, cursorHome); err != nil {
		t.Fatalf("initial link: %v", err)
	}
	destination := cursorAuthDestinationForTest(t, cursorHome, workspace)
	if _, err := os.Readlink(destination); err != nil {
		t.Fatalf("workspace auth is not linked: %v", err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := AggregateCursorMCPAuth(cursorHome); err != nil {
		t.Fatalf("aggregate after last source removal: %v", err)
	}
	master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
	if _, err := os.Lstat(master); !os.IsNotExist(err) {
		t.Fatalf("last source master remains available: %v", err)
	}
	if _, err := os.Lstat(master + ".provenance"); !os.IsNotExist(err) {
		t.Fatalf("provenance remains after last source removal: %v", err)
	}
	if _, err := os.ReadFile(destination); !os.IsNotExist(err) {
		t.Fatalf("existing bridge link still exposes removed credentials, read error = %v", err)
	}
}

func TestAggregateCursorMCPAuthPublicationDoesNotOverwriteConcurrentNativeRefresh(t *testing.T) {
	cursorHome := t.TempDir()
	projects := filepath.Join(cursorHome, "projects")
	writeCursorAuth(t, projects, "trusted-source", `{"github":{"tokens":{"access_token":"source-access"},"clientInfo":{"client_id":"client-1"}}}`, time.Now())
	if err := AggregateCursorMCPAuth(cursorHome); err != nil {
		t.Fatalf("initial aggregate: %v", err)
	}
	snapshot, err := aggregateCursorMCPAuth(cursorHome)
	if err != nil {
		t.Fatalf("prepare aggregate snapshot: %v", err)
	}
	master := filepath.Join(cursorHome, cursorMCPAuthUnifiedFilename)
	writeErr := publishCursorMCPAuthWithHook(cursorHome, snapshot, func() {
		writeNativeMaster(t, master, `{"github":{"tokens":{"access_token":"concurrent-refresh"},"clientInfo":{"client_id":"rotated-client"}}}`)
	})
	if !errors.Is(writeErr, errCursorMCPAuthMasterChanged) {
		t.Fatalf("publish after concurrent refresh error = %v, want master-changed", writeErr)
	}
	if got := readCursorAuth(t, master)["github"]["tokens"].(map[string]any)["access_token"]; got != "concurrent-refresh" {
		t.Fatalf("concurrent refresh was overwritten: %v", got)
	}
	if err := AggregateCursorMCPAuth(cursorHome); err != nil {
		t.Fatalf("aggregate retry after concurrent refresh: %v", err)
	}
	if got := readCursorAuth(t, master)["github"]["tokens"].(map[string]any)["access_token"]; got != "concurrent-refresh" {
		t.Fatalf("retry failed to preserve concurrent refresh: %v", got)
	}
}

func writeNativeMaster(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
