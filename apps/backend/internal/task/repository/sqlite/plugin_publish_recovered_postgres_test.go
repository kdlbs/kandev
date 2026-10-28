package sqlite

import "testing"

func TestPluginPublishIsGuardedAndIdempotentPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exercisePublishIsGuardedAndIdempotent(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestPluginPublishMismatchPublishesNothingPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exercisePublishMismatchPublishesNothing(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestPluginPublishRefusesWhenTheEnvelopeIsNotReadyPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exercisePublishRefusesWhenTheEnvelopeIsNotReady(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestPluginPublishChecksTheRepositoryInventoryPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exercisePublishChecksTheRepositoryInventory(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestPluginPublishNeedsTheHeldClaimPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exercisePublishNeedsTheHeldClaim(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestPluginPublishRacePostgres(t *testing.T) {
	repo, second := newDescriptorPostgresRepos(t)
	exercisePublishRace(t, repo, second, seedDescriptorEnvironmentPostgres)
}

func TestPluginArchiveRacingRecoveryHoldsOneClaimPostgres(t *testing.T) {
	repo, second := newDescriptorPostgresRepos(t)
	exerciseArchiveRacingRecoveryHoldsOneClaim(t, repo, second, seedDescriptorEnvironmentPostgres)
}
