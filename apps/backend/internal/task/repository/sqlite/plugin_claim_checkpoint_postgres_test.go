package sqlite

import "testing"

func TestPluginExecutorClaimCheckpointSequencePostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseClaimCheckpointSequence(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestPluginExecutorClaimCheckpointFencingPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseClaimCheckpointFencing(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestPluginExecutorClaimCheckpointRacePostgres(t *testing.T) {
	repo, second := newDescriptorPostgresRepos(t)
	exerciseClaimCheckpointRace(t, repo, second, seedDescriptorEnvironmentPostgres)
}

func TestReacquiredClaimStartsAFreshSequencePostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseReacquiredClaimStartsAFreshSequence(t, repo, seedDescriptorEnvironmentPostgres)
}
