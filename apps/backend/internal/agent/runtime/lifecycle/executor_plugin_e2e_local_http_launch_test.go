package lifecycle

import (
	"context"
	"testing"
)

// Each e2e-local locator form an e2e run creates tasks with (file and loopback
// http) reaches the provider's provision request unchanged, and the e2e
// profile selector is forwarded so the provider's agentctl accepts it.
func TestE2ELocalLocatorsReachTheProvisionRequestWithProfile(t *testing.T) {
	for name, locator := range map[string]string{
		"file":          "file:///tmp/e2e-local/e2e-local-file-main.git",
		"http loopback": "http://127.0.0.1:41234/e2e-local-http-main.git",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(pluginAgentEnvE2EProfile, boolStringTrue)
			harness := preparedHarness(t, preparedResource(repositoryState("repo-1", testInventoryRoot+"/e2e", "main")))
			launch := &LaunchRequest{Repositories: []RepoLaunchSpec{{
				RepositoryID: "repo-1", RepositoryURL: locator, CheckoutBranch: "main", BaseBranch: "main",
			}}}
			if err := attachPluginRepositoryInputs(harness.request, launch, nil); err != nil {
				t.Fatalf("attachPluginRepositoryInputs(): %v", err)
			}
			if _, err := harness.runtime.CreateInstance(context.Background(), harness.request); err != nil {
				t.Fatalf("CreateInstance(): %v", err)
			}
			sent := harness.operations.provisionRequest.GetRepositories()
			if len(sent) != 1 || sent[0].GetLocator() != locator {
				t.Fatalf("provision repositories = %v, want locator %s", sent, locator)
			}
			if got := harness.operations.provisionRequest.GetAgentEnv()[pluginAgentEnvE2EProfile]; got != boolStringTrue {
				t.Fatalf("provision agent env %s = %q, want %q", pluginAgentEnvE2EProfile, got, boolStringTrue)
			}
		})
	}
}
