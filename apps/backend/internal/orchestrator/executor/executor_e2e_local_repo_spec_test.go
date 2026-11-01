package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// A stored e2e-local repository reaches the launch's repository spec with its
// file:// or loopback locator intact, which is what the provider receives.
func TestBuildRepoSpecsCarriesE2ELocalLocator(t *testing.T) {
	for _, remote := range []string{
		"file:///tmp/local-rig/local-rig-file-main.git",
		"http://127.0.0.1:41234/local-rig-http-main.git",
	} {
		specs := buildRepoSpecs([]*repoInfo{{
			RepositoryID: "repo-1", BaseBranch: "main",
			Repository: &models.Repository{ID: "repo-1", Name: "rig", Provider: "e2e-local", RemoteURL: remote},
		}})
		if len(specs) != 1 || specs[0].RepositoryURL != remote {
			t.Fatalf("specs = %+v, want RepositoryURL %q", specs, remote)
		}
	}
}
