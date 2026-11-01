package lifecycle

import "testing"

// Under the e2e profile the provider's agentctl learns the profile, so its
// repository materialization accepts the e2e-local file:// locators the host
// sends; outside it nothing is added.
func TestPluginAgentEnvForwardsTheE2EProfileOnlyUnderE2E(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "")
	env, err := buildPluginAgentEnv(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := env["KANDEV_E2E_MOCK"]; ok {
		t.Fatalf("production agent env carries the e2e profile: %v", env)
	}

	t.Setenv("KANDEV_E2E_MOCK", "true")
	env, err = buildPluginAgentEnv(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if env["KANDEV_E2E_MOCK"] != "true" {
		t.Fatalf("e2e agent env = %v, want KANDEV_E2E_MOCK=true", env)
	}
}
