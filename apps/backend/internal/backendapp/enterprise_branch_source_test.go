package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/github"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestEnterpriseBranchSourceRetainsRepositoryHost(t *testing.T) {
	svc := github.NewService(nil, github.AuthMethodNone, nil, nil, nil, testLogger(t))
	t.Cleanup(svc.Stop)
	adapter := codeHostBranchListerAdapter{github: svc}
	_, err := adapter.ListRepoBranches(context.Background(), taskservice.RemoteBranchSource{
		Provider: "github", ProviderHost: "https://git.example.com", WorkspaceID: "workspace", Owner: "acme", Name: "widget",
	})
	if !errors.Is(err, github.ErrGitHubHostMismatch) {
		t.Fatalf("foreign repository source did not fail its host boundary: %v", err)
	}
}
