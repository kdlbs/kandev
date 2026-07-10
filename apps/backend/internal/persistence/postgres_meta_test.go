package persistence

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresLatestVersionMetaRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	if err := ensureMetaTable(db); err != nil {
		t.Fatalf("ensure meta table: %v", err)
	}

	checkedAt := time.Unix(123, 0).UTC()
	if err := WriteLatestVersion(db, "v1.2.3", "https://example.test/release", checkedAt); err != nil {
		t.Fatalf("write latest version: %v", err)
	}
	version, url, gotCheckedAt, err := ReadLatestVersion(db)
	if err != nil {
		t.Fatalf("read latest version: %v", err)
	}
	if version != "v1.2.3" || url != "https://example.test/release" || !gotCheckedAt.Equal(checkedAt) {
		t.Fatalf("meta round-trip = (%q, %q, %s)", version, url, gotCheckedAt)
	}
}
