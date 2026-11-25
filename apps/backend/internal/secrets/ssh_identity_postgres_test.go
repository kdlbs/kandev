package secrets

import (
	"github.com/kandev/kandev/internal/testutil"
	"testing"
)

func TestPostgresSSHIdentityVault(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	key, err := NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := Provide(db, db, key)
	if err != nil {
		t.Fatal(err)
	}
	testSSHIdentityVault(t, store)
}
