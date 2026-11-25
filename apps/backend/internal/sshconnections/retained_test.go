package sshconnections

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/sshtransport"
	"golang.org/x/crypto/ssh"
)

func TestRetainedSSHUsesOriginalDirectEvidenceAndRejectsUnpinnedJump(t *testing.T) {
	s, ctx := identityService(t)
	identity, key := retainedTestIdentity(t)
	host := newWireHost(t, identity)
	replacement := newWireHost(t, identity)
	if _, err := s.store.db.Exec(`CREATE TABLE executors_running (session_id TEXT PRIMARY KEY, runtime TEXT, metadata TEXT)`); err != nil {
		t.Fatal(err)
	}
	target := sshtransport.SSHTarget{
		Host: "127.0.0.1", Port: host.port(), User: "fixture", PinnedFingerprint: host.fingerprint,
		IdentitySource: sshtransport.SSHIdentitySourceFile, IdentityFile: key,
	}
	record := map[string]string{
		"ssh_host": target.Host, "ssh_port": strconv.Itoa(target.Port), "ssh_user": target.User,
		"ssh_host_fingerprint": target.PinnedFingerprint, "ssh_identity_source": string(target.IdentitySource), "ssh_identity_file": target.IdentityFile,
	}
	writeRetainedSSHRecord(t, s, record, false)
	other := target
	other.Port, other.PinnedFingerprint = replacement.port(), replacement.fingerprint
	if _, err := s.DialRetainedLegacySession(ctx, "session", other); !errors.Is(err, ErrConflict) || replacement.keyCalls.Load() != 0 {
		t.Fatalf("replacement executor authenticated: %v", err)
	}
	client, err := s.DialRetainedLegacySession(ctx, "session", target)
	if err != nil {
		t.Fatalf("original record could not reconnect: %v", err)
	}
	_ = client.Close()
	before := host.keyCalls.Load()
	target.ProxyJump = "unknown-bastion"
	record["ssh_proxy_jump"] = target.ProxyJump
	writeRetainedSSHRecord(t, s, record, true)
	if _, err := s.DialRetainedLegacySession(ctx, "session", target); !errors.Is(err, ErrInvalidRoute) || host.keyCalls.Load() != before {
		t.Fatalf("original record with missing bastion proof authenticated: %v", err)
	}
}

func retainedTestIdentity(t *testing.T) (string, string) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(file, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	return string(ssh.MarshalAuthorizedKey(signer.PublicKey())), file
}

func writeRetainedSSHRecord(t *testing.T, s *Service, record map[string]string, update bool) {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	query := `INSERT INTO executors_running VALUES ('session','ssh',?)`
	if update {
		query = `UPDATE executors_running SET metadata=? WHERE session_id='session'`
	}
	if _, err := s.store.db.Exec(query, string(data)); err != nil {
		t.Fatal(err)
	}
}
