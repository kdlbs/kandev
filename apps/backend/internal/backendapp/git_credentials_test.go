package backendapp

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
)

func TestNewGitCredentialBrokerDefaultsToEphemeralReissueSigner(t *testing.T) {
	first := newGitCredentialBroker(nil, nil, nil, "")
	capability, err := first.IssueReissueCapability(gitcredentials.Scope{
		ProviderID: "github", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: "github.com", Path: "/example/widgets.git",
	})
	if err != nil {
		t.Fatalf("IssueReissueCapability() error = %v", err)
	}

	second := newGitCredentialBroker(nil, nil, nil, "")
	_, err = second.Reissue(t.Context(), gitcredentials.ReissueRequest{
		Capability: capability.Token, TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: "github.com", Path: "/example/widgets.git",
	})
	if !errors.Is(err, gitcredentials.ErrReissueCapabilityInvalid) {
		t.Fatalf("Reissue() with another process-local signer error = %v, want invalid capability", err)
	}
}

// This is contract coverage for the existing operator-configured stable-key
// path. The default-signer repair must not make configured capabilities
// process-local.
func TestNewGitCredentialBrokerPreservesConfiguredSigner(t *testing.T) {
	const signingKey = "configured-stable-key"
	first := newGitCredentialBroker(nil, nil, nil, signingKey)
	capability, err := first.IssueReissueCapability(gitcredentials.Scope{
		ProviderID: "github", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: "github.com", Path: "/example/widgets.git",
	})
	if err != nil {
		t.Fatalf("IssueReissueCapability() error = %v", err)
	}

	second := newGitCredentialBroker(nil, nil, nil, signingKey)
	_, err = second.Reissue(t.Context(), gitcredentials.ReissueRequest{
		Capability: capability.Token, TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: "github.com", Path: "/example/widgets.git",
	})
	if !errors.Is(err, gitcredentials.ErrUnsupported) {
		t.Fatalf("Reissue() with configured signer error = %v, want resolver unsupported after capability validation", err)
	}
}
