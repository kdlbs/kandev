package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/auth/authn"
)

// writeKey generates an ed25519 private key at path, optionally encrypted with
// a passphrase, and returns the PEM bytes actually written.
func writeKey(t *testing.T, path, passphrase string) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, "")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	}
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	data := pem.EncodeToMemory(block)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write key %s: %v", path, err)
	}
	return data
}

// newIdentityHome builds a $HOME/.ssh populated with every entry class the
// classifier has to decide about.
func newIdentityHome(t *testing.T) (home string, plainPEM, encryptedPEM []byte) {
	t.Helper()
	home = t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("mkdir .ssh: %v", err)
	}

	plainPEM = writeKey(t, filepath.Join(sshDir, "id_ed25519"), "")
	encryptedPEM = writeKey(t, filepath.Join(sshDir, "id_locked"), "hunter2")

	writeFile := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(sshDir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeFile("id_ed25519.pub", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5 user@host\n")
	writeFile("known_hosts", "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5\n")
	writeFile("known_hosts.old", "old.example.com ssh-ed25519 AAAAC3\n")
	writeFile("authorized_keys", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5 user@host\n")
	writeFile("notes.txt", "this is not a key\n")
	if err := os.MkdirAll(filepath.Join(sshDir, "sockets"), 0o700); err != nil {
		t.Fatalf("mkdir sockets: %v", err)
	}
	return home, plainPEM, encryptedPEM
}

func displayPaths(ids []Identity) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.DisplayPath)
	}
	return out
}

// TestIdentitiesClassifiesSSHDirEntries covers AC-…-001.1 and .2: only regular
// files that parse as private keys are reported, and nothing else in ~/.ssh is.
func TestIdentitiesClassifiesSSHDirEntries(t *testing.T) {
	home, _, _ := newIdentityHome(t)

	got := DiscoverIdentities(home)

	want := []string{"~/.ssh/id_ed25519", "~/.ssh/id_locked"}
	gotPaths := displayPaths(got.Identities)
	if len(gotPaths) != len(want) {
		t.Fatalf("identities = %v, want exactly %v", gotPaths, want)
	}
	for i, w := range want {
		if gotPaths[i] != w {
			t.Errorf("identities[%d] = %q, want %q", i, gotPaths[i], w)
		}
	}

	byPath := map[string]Identity{}
	for _, id := range got.Identities {
		byPath[id.DisplayPath] = id
	}
	if plain := byPath["~/.ssh/id_ed25519"]; plain.Encrypted {
		t.Error("id_ed25519 reported as encrypted, want usable as a file identity")
	} else if plain.KeyType != "ssh-ed25519" {
		t.Errorf("id_ed25519 key_type = %q, want ssh-ed25519", plain.KeyType)
	}
	if locked := byPath["~/.ssh/id_locked"]; !locked.Encrypted {
		t.Error("id_locked reported as unencrypted, want encrypted")
	}
	for _, id := range got.Identities {
		if id.Source != SourceSSHDir {
			t.Errorf("%s source = %q, want %q", id.DisplayPath, id.Source, SourceSSHDir)
		}
	}
}

// TestIdentitiesIncludesSSHConfigReferences covers the second fixed root in
// AC-…-001.1: IdentityFile values named by ~/.ssh/config, wherever they live.
func TestIdentitiesIncludesSSHConfigReferences(t *testing.T) {
	home, _, _ := newIdentityHome(t)
	keyDir := filepath.Join(home, "keys")
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		t.Fatalf("mkdir keys: %v", err)
	}
	writeKey(t, filepath.Join(keyDir, "prod"), "")

	cfg := "Host prod\n  HostName prod.example.com\n  IdentityFile ~/keys/prod\n" +
		"Host dupe\n  IdentityFile ~/.ssh/id_ed25519\n" +
		"Host missing\n  IdentityFile ~/keys/absent\n"
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got := DiscoverIdentities(home)

	gotPaths := displayPaths(got.Identities)
	want := []string{"~/.ssh/id_ed25519", "~/.ssh/id_locked", "~/keys/prod"}
	if strings.Join(gotPaths, ",") != strings.Join(want, ",") {
		t.Fatalf("identities = %v, want %v (ssh_dir first, config-referenced after, no duplicate, no missing file)", gotPaths, want)
	}
	for _, id := range got.Identities {
		if id.DisplayPath == "~/keys/prod" && id.Source != SourceSSHConfig {
			t.Errorf("~/keys/prod source = %q, want %q", id.Source, SourceSSHConfig)
		}
	}
}

// TestIdentitiesIgnoreKeysOutsideBothRoots pins the outer boundary of
// AC-…-001.1: the reported set is exactly the union of ~/.ssh and the keys
// ~/.ssh/config names. A real, readable, perfectly valid key that neither root
// reaches is not discoverable, because discovery never searches the
// filesystem.
func TestIdentitiesIgnoreKeysOutsideBothRoots(t *testing.T) {
	home, _, _ := newIdentityHome(t)

	elsewhere := filepath.Join(home, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o700); err != nil {
		t.Fatalf("mkdir elsewhere: %v", err)
	}
	writeKey(t, filepath.Join(elsewhere, "unreferenced"), "")

	// A key filed into a subdirectory of ~/.ssh is equally out of reach: the
	// .ssh listing is deliberately not recursive.
	nested := filepath.Join(home, ".ssh", "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	writeKey(t, filepath.Join(nested, "buried"), "")

	got := displayPaths(DiscoverIdentities(home).Identities)

	want := []string{"~/.ssh/id_ed25519", "~/.ssh/id_locked"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("identities = %v, want %v; a key in neither root must not be discoverable", got, want)
	}
}

// TestIdentitiesNeverLeakKeyMaterial covers AC-…-001.3. The endpoint's whole
// value is answering "which files would work" without becoming a way to read
// the keys, so the serialized response must contain none of their bytes.
func TestIdentitiesNeverLeakKeyMaterial(t *testing.T) {
	home, plainPEM, encryptedPEM := newIdentityHome(t)

	payload, err := json.Marshal(DiscoverIdentities(home))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(payload)

	for name, keyPEM := range map[string][]byte{"unencrypted": plainPEM, "encrypted": encryptedPEM} {
		for _, line := range strings.Split(strings.TrimSpace(string(keyPEM)), "\n") {
			line = strings.TrimSpace(line)
			// Skip the PEM banners: they are constants, not key material.
			if line == "" || strings.HasPrefix(line, "-----") {
				continue
			}
			if strings.Contains(body, line) {
				t.Fatalf("%s key material leaked into the response: %q", name, line)
			}
		}
	}
}

// TestIdentitiesMissingSSHDir covers the AC-…-001.5 degradation rule: an
// absent or unreadable ~/.ssh is an empty list, not an error, so "no keys" and
// "cannot look" stay indistinguishable to the caller.
func TestIdentitiesMissingSSHDir(t *testing.T) {
	got := DiscoverIdentities(t.TempDir())

	if len(got.Identities) != 0 {
		t.Fatalf("identities = %v, want empty for a home with no .ssh", displayPaths(got.Identities))
	}
	if got.Identities == nil {
		t.Error("identities is nil, want an empty array so the JSON field is [] and not null")
	}
}

// TestIdentitiesRouteRequiresAdmin covers AC-…-001.5. Discovery lists files
// under the backend user's home, a strictly larger disclosure than the other
// /api/v1/ssh routes make, so it is gated even though they are not.
func TestIdentitiesRouteRequiresAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		identity *authn.Identity
		want     int
	}{
		{name: "unauthenticated", identity: nil, want: http.StatusUnauthorized},
		{name: "member", identity: &authn.Identity{UserID: "user-1", Role: authn.RoleMember}, want: http.StatusForbidden},
		{name: "admin", identity: &authn.Identity{UserID: "user-2", Role: authn.RoleAdmin}, want: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			if tc.identity != nil {
				router.Use(func(c *gin.Context) {
					authn.SetOnGin(c, *tc.identity)
					c.Next()
				})
			}
			h := &Handler{}
			h.registerHTTP(router)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ssh/identities", nil))

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
