package backendapp

import (
	"context"
	"testing"
)

func TestRemoteContributionCoordinatorLeavesOrdinaryRepositoriesUnmatched(t *testing.T) {
	coordinator := newRemoteContributionCoordinator(nil, nil)

	for _, rawURL := range []string{
		"https://github.com/acme/pull/repository.git",
		"https://github.com/acme/widget.git",
		"https://gitlab.example.com/group/project.git",
	} {
		resolution, matched, err := coordinator.Resolve(context.Background(), "workspace", "user", rawURL)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", rawURL, err)
		}
		if matched || resolution != nil {
			t.Fatalf("Resolve(%q) = (%#v, %v), want an ordinary repository", rawURL, resolution, matched)
		}
	}
}
