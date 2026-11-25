package sshconnections

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// @covers AC-EXECUTORS-SSH-MIGRATION-001.3, AC-EXECUTORS-SSH-MIGRATION-001.4
func TestMigrationBastionTrustProvenance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edSigner, err := ssh.NewSignerFromKey(edKey)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaSigner, err := ssh.NewSignerFromKey(ecdsaKey)
	if err != nil {
		t.Fatal(err)
	}
	ed, alternate := edSigner.PublicKey(), ecdsaSigner.PublicKey()
	otherEd, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sameAlgorithm, err := ssh.NewPublicKey(otherEd)
	if err != nil {
		t.Fatal(err)
	}
	const address = "bastion.invalid:2222"
	record := knownhosts.Line([]string{address}, ed) + "\n"
	for _, test := range []struct {
		name, contents, pin string
		pending             bool
		presented           ssh.PublicKey
	}{
		{"plain", record, ssh.FingerprintSHA256(ed), false, nil},
		{"different port", knownhosts.Line([]string{"bastion.invalid:2244"}, ed), "", false, nil},
		{"hashed", knownhosts.Line([]string{knownhosts.HashHostname(knownhosts.Normalize(address))}, ed), "", true, ed},
		{"two algorithms", record + knownhosts.Line([]string{address}, alternate), "", true, ed},
		{"same algorithm", record + knownhosts.Line([]string{address}, sameAlgorithm), "", true, sameAlgorithm},
		{"revoked", record + "@revoked " + record, "", false, nil},
		{"certificate authority", "@cert-authority " + record, "", false, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, "known_hosts"), []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			store := testStore(t)
			migrationCreateExecutors(t, store, map[string]string{"ssh": `{
				"ssh_host":"offline.invalid","ssh_user":"deploy","ssh_identity_source":"file","ssh_identity_file":"/missing/key",
				"ssh_proxy_jump":"jump@bastion.invalid:2222","ssh_host_fingerprint":"SHA256:saved"
			}`})
			if err := store.MigrateLegacyExecutors(t.Context()); err != nil {
				t.Fatal(err)
			}
			executor := migrationTestConfig(t, store, "ssh")
			destination, err := store.GetConnection(t.Context(), executor[ConnectionConfigKey])
			if err != nil || destination.Fingerprint != "SHA256:saved" || destination.VerifiedRoute != "" {
				t.Fatalf("destination trust: %+v, %v", destination, err)
			}
			bastion, err := store.GetConnection(t.Context(), destination.BastionID)
			if err != nil || bastion.Fingerprint != test.pin || bastion.KnownHostsPending != test.pending || bastion.VerifiedRoute != "" {
				t.Fatalf("bastion trust: %+v, %v", bastion, err)
			}
			if test.pin != "" && bastion.PublicKey != string(ssh.MarshalAuthorizedKey(ed)) {
				t.Fatal("offline pin did not retain the vetted public key")
			}
			var data string
			if err = store.db.Get(&data, `SELECT data FROM ssh_migrations WHERE executor_id='ssh'`); err != nil {
				t.Fatal(err)
			}
			if test.pending {
				cb, err := migrationKnownHostsCallback()
				if err != nil {
					t.Fatal(err)
				}
				if err = cb(address, migrationHostAddr(address), test.presented); err != nil {
					t.Fatal(err)
				}
			}
			if test.name == "revoked" {
				cb, err := migrationKnownHostsCallback()
				if err != nil {
					t.Fatal(err)
				}
				var revoked *knownhosts.RevokedError
				if err = cb(address, migrationHostAddr(address), ed); !errors.As(err, &revoked) {
					t.Fatalf("revocation accepted: %v", err)
				}
			}
			var migration sshMigration
			if err = json.Unmarshal([]byte(data), &migration); err != nil || migration.BastionKnownHosts != test.pending {
				t.Fatalf("first-use host trust: %+v, %v", migration, err)
			}
		})
	}
}
