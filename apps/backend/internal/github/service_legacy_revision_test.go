package github

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func setupLegacyRevisionTest(t *testing.T) (*Service, *mockSecretManager) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = legacyIdentityRoundTripper(func(req *http.Request) (*http.Response, error) {
		login := strings.TrimPrefix(req.Header.Get("Authorization"), "token ")
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"login":"` + login + `"}`))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	ctx := context.Background()
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "work")
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{WorkspaceID: "work", GitHubHost: defaultGitHubHost, Source: ConnectionSourceLegacyShared, Status: ConnectionStatusActive, CredentialGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	mgr := newMockSecretManager()
	provider := &mockSecretProvider{mgr: mgr}
	svc := NewService(nil, AuthMethodNone, provider, store, nil, testLogger(t))
	svc.secretManager = mgr
	t.Cleanup(svc.Stop)
	svc.resolver.SetLegacyFactory(func(ctx context.Context) (Client, string, error) {
		id, _ := svc.findGitHubTokenSecret(ctx)
		token, err := provider.Reveal(ctx, id)
		return NewPATClient(token), AuthMethodPAT, err
	})
	return svc, mgr
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-003.8
func TestLegacyFingerprintInvalidatesGenericSecretChanges(t *testing.T) {
	for _, observed := range []bool{false, true} {
		name := "first observation"
		if observed {
			name = "changed revision"
		}
		t.Run(name, func(t *testing.T) {
			svc, mgr := setupLegacyRevisionTest(t)
			ctx := context.Background()
			if err := svc.ConfigureToken(ctx, "alice"); err != nil {
				t.Fatal(err)
			}
			before := ""
			if observed {
				var err error
				before, err = svc.WorkspaceConnectionFingerprint(ctx, "work")
				if err != nil {
					t.Fatal(err)
				}
			}
			request := ResolveCredentialRequest{WorkspaceID: "work", Purpose: CredentialPurposeAutomation}
			if _, err := svc.resolver.Resolve(ctx, request); err != nil {
				t.Fatal(err)
			}

			svc.markRepoAsMissing("acme", "private", svc.repoErrorGenSnapshot())
			progress := svc.batchedPRQueryProgress("retained", "refs")
			svc.finishBatchedPRQueryProgress("retained", progress, true)
			id, _ := svc.findGitHubTokenSecret(ctx)
			if err := mgr.Update(ctx, id, "bob"); err != nil {
				t.Fatal(err)
			}
			after, err := svc.WorkspaceConnectionFingerprint(ctx, "work")
			if err != nil {
				t.Fatal(err)
			}
			if after == before {
				t.Fatal("secret change did not update the fingerprint")
			}

			if svc.isRepoCachedAsMissing("acme", "private") {
				t.Error("old credential's repository denial survived rotation")
			}
			nextProgress := svc.batchedPRQueryProgress("retained", "refs")
			if nextProgress == progress {
				t.Error("old credential's retained PR result survived rotation")
			}
			svc.finishBatchedPRQueryProgress("retained", nextProgress, false)
			next, err := svc.resolver.Resolve(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if next.Principal.Login != "bob" {
				t.Fatalf("resolved principal = %s, want bob", next.Principal.Login)
			}
		})
	}
}
