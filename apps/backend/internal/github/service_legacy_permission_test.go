package github

import (
	"context"
	"errors"
	"testing"
)

type scopeChangedClient struct {
	Client
	denied bool
	calls  *int
}

func (c *scopeChangedClient) GetAuthenticatedUser(context.Context) (string, error) {
	return "alice", nil
}
func (c *scopeChangedClient) GetPRStatus(context.Context, string, string, int) (*PRStatus, error) {
	*c.calls++
	if c.denied {
		return nil, errors.New("replacement token cannot read repository")
	}
	return &PRStatus{PR: &PR{Number: 1, Title: "old-token-visible"}}, nil
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-003.8
func TestLegacyFingerprintDropsOldPermissionCache(t *testing.T) {
	svc, mgr := setupLegacyRevisionTest(t)
	ctx := context.Background()
	if err := svc.ConfigureToken(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	svc.resolver.SetLegacyFactory(func(ctx context.Context) (Client, string, error) {
		id, _ := svc.findGitHubTokenSecret(ctx)
		value, err := svc.secrets.Reveal(ctx, id)
		return &scopeChangedClient{Client: NewMockClient(), denied: value == "replacement", calls: &calls}, AuthMethodPAT, err
	})
	if _, err := svc.WorkspaceConnectionFingerprint(ctx, "work"); err != nil {
		t.Fatal(err)
	}
	before, err := svc.resolveAutomationClient(ctx, "work", "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.getPRStatus(ctx, before.Client, before.CacheScope, "o", "r", 1); err != nil {
		t.Fatal(err)
	}
	id, _ := svc.findGitHubTokenSecret(ctx)
	if err := mgr.Update(ctx, id, "replacement"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WorkspaceConnectionFingerprint(ctx, "work"); err != nil {
		t.Fatal(err)
	}
	after, err := svc.resolveAutomationClient(ctx, "work", "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.getPRStatus(ctx, after.Client, after.CacheScope, "o", "r", 1)
	t.Logf("same scope=%v; replacement denied=%v; provider calls=%d; returned error=%v", before.CacheScope == after.CacheScope, after.Client.(*scopeChangedClient).denied, calls, err)
	if err == nil {
		t.Fatal("old-token PR status survives generic revision recovery and bypasses replacement credential")
	}
}
