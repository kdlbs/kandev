package sqlite

import "testing"

func TestUnrecordedPluginExecutorSurvivesAndIsListedByEnvironmentPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseUnrecordedPluginExecutorRoundTrip(t, repo, seedDescriptorEnvironmentPostgres)
}
