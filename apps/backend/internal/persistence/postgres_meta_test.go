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

	nightlyCheckedAt := checkedAt.Add(time.Hour)
	if err := WriteLatestNightlyVersion(
		db,
		"v1.2.4-nightly.shaabc123def456",
		"https://example.test/nightly",
		nightlyCheckedAt,
	); err != nil {
		t.Fatalf("write latest nightly version: %v", err)
	}
	nightlyVersion, nightlyURL, gotNightlyCheckedAt, err := ReadLatestNightlyVersion(db)
	if err != nil {
		t.Fatalf("read latest nightly version: %v", err)
	}
	if nightlyVersion != "v1.2.4-nightly.shaabc123def456" ||
		nightlyURL != "https://example.test/nightly" ||
		!gotNightlyCheckedAt.Equal(nightlyCheckedAt) {
		t.Fatalf("nightly meta round-trip = (%q, %q, %s)", nightlyVersion, nightlyURL, gotNightlyCheckedAt)
	}

	version, url, gotCheckedAt, err = ReadLatestVersion(db)
	if err != nil {
		t.Fatalf("read stable version after nightly write: %v", err)
	}
	if version != "v1.2.3" || url != "https://example.test/release" || !gotCheckedAt.Equal(checkedAt) {
		t.Fatalf("stable meta changed after nightly write = (%q, %q, %s)", version, url, gotCheckedAt)
	}
}
