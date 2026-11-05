package lifecycle

import (
	"context"
	"strings"
	"testing"
)

// An e2e-local repository (a file:// remote, no credential and no broker
// environment) is projected into the launch's repository intent and reaches the
// provider's provision request with its locator intact.
func TestE2ELocalRepositoryReachesTheProvisionRequest(t *testing.T) {
	const locator = "file:///tmp/repos/widgets.git"
	harness := preparedHarness(t, preparedResource(repositoryState("repo-1", testInventoryRoot+"/widgets", "main")))
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
	if len(sent) != 1 || sent[0].GetId() != "repo-1" || sent[0].GetLocator() != locator {
		t.Fatalf("provision repositories = %v, want repo-1 at %s", sent, locator)
	}
	for name := range harness.operations.provisionRequest.GetAgentEnv() {
		for _, fragment := range pluginAgentEnvSecretFragments {
			if strings.Contains(strings.ToUpper(name), fragment) {
				t.Fatalf("provision agent env carries a credential-like name %q", name)
			}
		}
	}
}
