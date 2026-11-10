package backendapp

import (
	"context"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// @covers AC-TASKS-PORTABLE-PROFILE-001.1
func TestAgentProfileResolverExportsOptions(t *testing.T) {
	repos, db := newMatcherTestRepos(t)
	agent := createMatcherTestAgent(t, repos)
	profile := createMatcherTestProfile(t, repos, agent, "Codex", "luna", "")
	_, err := db.Exec(`UPDATE agent_profiles SET settings = ? WHERE id = ?`, `{"config_options":{"reasoning_effort":"low"}}`, profile.ID)
	require.NoError(t, err)
	portable := buildAgentProfileResolver(repos)(profile.ID)
	encoded, err := yaml.Marshal(portable)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "reasoning_effort: low")
}

// @covers AC-TASKS-PORTABLE-PROFILE-001.2
func TestAgentProfileLookupDistinguishesOptionsAndReportsAmbiguity(t *testing.T) {
	repos, db := newMatcherTestRepos(t)
	agent := createMatcherTestAgent(t, repos)
	low := createMatcherTestProfile(t, repos, agent, "Codex", "luna", "")
	high := createMatcherTestProfile(t, repos, agent, "Codex", "luna", "")
	for id, value := range map[string]string{low.ID: "low", high.ID: "high"} {
		_, err := db.Exec(`UPDATE agent_profiles SET settings = ? WHERE id = ?`, `{"config_options":{"reasoning_effort":"`+value+`"}}`, id)
		require.NoError(t, err)
	}
	lookup := buildAgentProfileLookup(repos)
	options := map[string]string{"reasoning_effort": "low"}
	profile := &wfmodels.AgentProfilePortable{AgentName: "Codex", Model: "luna", ConfigOptions: &options}
	require.Equal(t, wfmodels.ProfileMatch{ID: low.ID}, lookup(profile, ""))
	require.Equal(t, wfmodels.ProfileMatch{ID: low.ID, Reason: "multiple matching profiles"}, lookup(&wfmodels.AgentProfilePortable{AgentName: "Codex", Model: "luna"}, low.ID))
	options["reasoning_effort"] = "missing"
	require.Equal(t, "no matching profile", lookup(profile, low.ID).Reason)
	options["reasoning_effort"] = "low"
	_, err := repos.AgentSettings.UpdateAgentProfileEnabled(context.Background(), low.ID, false)
	require.NoError(t, err)
	require.Equal(t, wfmodels.ProfileMatch{ID: low.ID}, lookup(profile, low.ID))
	require.Equal(t, "no matching profile", lookup(profile, "").Reason)
}

func TestAgentProfileLookupReportsReadFailure(t *testing.T) {
	repos, db := newMatcherTestRepos(t)
	require.NoError(t, db.Close())
	result := buildAgentProfileLookup(repos)(&wfmodels.AgentProfilePortable{AgentName: "Codex"}, "")
	require.Equal(t, "profile lookup failed", result.Reason)
	require.Empty(t, result.ID)
}
