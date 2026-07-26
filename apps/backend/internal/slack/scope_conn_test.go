package slack

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// TestTestConnection_DeniesForeignWorkspace is the regression for the review's
// P1 credential-exfiltration path: a caller must not cause a foreign/default
// workspace's stored Slack credentials to be used.
func TestTestConnection_DeniesForeignWorkspace(t *testing.T) {
	ctx := context.Background()
	svc := newCopyTestService(t, newFakeSecrets())
	svc.SetWorkspaceAuthorizer(denyOnly("ws-foreign"))

	result, err := svc.TestConnectionForWorkspace(ctx, "ws-foreign", &SetConfigRequest{
		AuthMethod: AuthMethodCookie, Token: "t", Cookie: "c",
	})
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v (result=%+v)", err, result)
	}
}

// TestClientFor_DeniesForeignWorkspace covers the clientFor chokepoint that all
// data-plane reads funnel through.
func TestClientFor_DeniesForeignWorkspace(t *testing.T) {
	ctx := context.Background()
	svc := newCopyTestService(t, newFakeSecrets())
	svc.SetWorkspaceAuthorizer(denyOnly("ws-foreign"))

	if _, err := svc.ClientForWorkspace(ctx, "ws-foreign"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}
