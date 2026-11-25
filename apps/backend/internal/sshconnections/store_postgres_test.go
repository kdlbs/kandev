package sshconnections

import (
	"github.com/kandev/kandev/internal/testutil"
	"testing"
)

func TestPostgresConnectionVersionsAndSingleHopGuards(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	s, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	testConnectionVersions(t, s)
}
