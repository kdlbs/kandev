package sqlite

import "testing"

func TestUnrecordedPluginExecutorSurvivesAndIsListedByEnvironmentPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseUnrecordedPluginExecutorRoundTrip(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestEnvironmentDeleteIsRefusedWhileAHandoffRemainsPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseHandoffGuard(t, repo, seedDescriptorEnvironmentPostgres)
}

func TestListAllUnrecordedPluginExecutorsSpansEnvironmentsPostgres(t *testing.T) {
	repo, _ := newDescriptorPostgresRepos(t)
	exerciseListAllUnrecorded(t, repo, seedDescriptorEnvironmentPostgres)
}
