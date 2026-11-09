package github

import (
	"context"
	"errors"
	"testing"
)

func TestEnterpriseWorkspaceHostCommitRejectsReplacement(t *testing.T) {
	connection := &WorkspaceConnection{WorkspaceID: "workspace", GitHubHost: "git.example.com", CredentialGeneration: 1}
	svc := NewService(nil, AuthMethodNone, nil, newTestStore(t), nil, testLogger(t))
	svc.resolver = NewCredentialResolver(&fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{"workspace": connection}}, nil)
	ctx, err := svc.BindWorkspaceGitHubHost(context.Background(), "workspace", "git.example.com")
	if err != nil {
		t.Fatal(err)
	}
	written := false
	apply := func(context.Context) error { written = true; return nil }
	if err := svc.CommitWorkspaceGitHubHost(ctx, "workspace", apply); err != nil || !written {
		t.Fatalf("unchanged binding failed: %v", err)
	}
	written = false
	connection.CredentialGeneration++
	if err := svc.CommitWorkspaceGitHubHost(ctx, "workspace", apply); !errors.Is(err, ErrGitHubHostMismatch) || written {
		t.Fatalf("replaced connection applied old data: %v", err)
	}
	if _, err := svc.BindWorkspaceGitHubHost(context.Background(), "workspace", "github.com"); !errors.Is(err, ErrGitHubHostMismatch) {
		t.Fatalf("foreign host binding accepted: %v", err)
	}
	if err := svc.CommitWorkspaceGitHubHost(context.Background(), "workspace", apply); !errors.Is(err, ErrGitHubHostMismatch) || written {
		t.Fatalf("missing binding accepted: %v", err)
	}
}
