package sshconnections

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestMigrationCountsReflectDurableState(t *testing.T) {
	store := testStore(t)
	keyFile := filepath.Join(t.TempDir(), "legacy-key")
	if err := os.WriteFile(keyFile, []byte("opaque fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrationCreateExecutors(t, store, map[string]string{
		"blocked":  `{"ssh_host":"offline.invalid","ssh_user":"deploy","ssh_identity_source":"file","ssh_identity_file":"/missing/key"}`,
		"pending":  fmt.Sprintf(`{"ssh_host":"offline.invalid","ssh_user":"deploy","ssh_identity_source":"file","ssh_identity_file":%q,"ssh_host_fingerprint":"SHA256:saved"}`, keyFile),
		"verified": fmt.Sprintf(`{"ssh_host":"offline.invalid","ssh_user":"deploy","ssh_identity_source":"file","ssh_identity_file":%q,"ssh_host_fingerprint":"SHA256:saved"}`, keyFile),
	})
	if err := store.MigrateLegacyExecutors(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.write(t.Context(), func(tx *sqlx.Tx) error {
		migration, err := migrationIn(t.Context(), tx, "verified")
		if err != nil {
			return err
		}
		migration.Verified = true
		return putMigration(t.Context(), tx, "verified", migration)
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(store.db)
	if err != nil {
		t.Fatal(err)
	}
	counts, err := reopened.migrationCounts(t.Context())
	if err != nil || len(counts) != 3 || counts["blocked"] != 1 || counts["pending"] != 1 || counts["verified"] != 1 {
		t.Fatalf("durable migration counts = %v, %v", counts, err)
	}
}
