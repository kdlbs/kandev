package sqlite

import "testing"

func TestCleanupRequestIsRecordableUnderARecoveryClaimPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseCleanupRequestUnderARecoveryClaim(t, repo, seedDescriptorEnvironmentPostgres)
}
