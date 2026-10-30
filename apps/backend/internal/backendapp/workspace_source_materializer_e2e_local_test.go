package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
)

// The materializer projects an e2e-local repository by its file:// or loopback
// locator only under the e2e profile; every other repository keeps refusing
// a file locator.
func TestRemoteRepositoryLocatorAcceptsE2ELocalOnlyUnderE2E(t *testing.T) {
	const locator = "file:///tmp/repos/widgets.git"
	repo := &models.Repository{Provider: repoclone.ProviderE2ELocal, RemoteURL: locator}
	other := &models.Repository{Provider: "github", RemoteURL: locator}

	t.Setenv("KANDEV_E2E_MOCK", "")
	if got := remoteRepositoryLocator(repo); got != "" {
		t.Fatalf("production projected %q for an e2e-local repository", got)
	}
	t.Setenv("KANDEV_E2E_MOCK", "true")
	if got := remoteRepositoryLocator(repo); got != locator {
		t.Fatalf("e2e profile projected %q, want %q", got, locator)
	}
	if got := remoteRepositoryLocator(other); got != "" {
		t.Fatalf("e2e profile projected %q for a non-e2e-local repository", got)
	}
}
