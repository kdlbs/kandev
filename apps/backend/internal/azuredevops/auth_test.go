package azuredevops

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestPATAuthorizerSendsBasicHeaderAndNeverRenews(t *testing.T) {
	auth := NewPATAuthorizer("secret-pat")
	header, err := auth.Header(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := "Basic " + base64.StdEncoding.EncodeToString([]byte(":secret-pat")); header != want {
		t.Fatalf("header = %q, want %q", header, want)
	}
	renewed, err := auth.Renew(context.Background(), header)
	if err != nil || renewed {
		t.Fatalf("PAT renew = %v, %v; want false, nil", renewed, err)
	}
	if got := auth.Redact("token secret-pat leaked"); got != "token [REDACTED] leaked" {
		t.Fatalf("redact = %q", got)
	}
	if got := NewPATAuthorizer("").Redact("body"); got != "body" {
		t.Fatalf("empty PAT redact = %q, want body unchanged", got)
	}
}

func TestResolveAzureDevOpsCloneCredentialsIsWorkspaceScoped(t *testing.T) {
	svc, _, _ := newTestService(t, nil)
	ctx := context.Background()
	for workspace, pat := range map[string]string{"ws-a": "pat-a", "ws-b": "pat-b"} {
		if _, err := svc.SetConfigForWorkspace(ctx, workspace, &SetConfigRequest{
			OrganizationURL: "https://dev.azure.com/acme", PAT: pat,
		}); err != nil {
			t.Fatal(err)
		}
	}
	username, password, err := svc.ResolveAzureDevOpsCloneCredentials(ctx, "ws-a")
	if err != nil || username != "kandev" || password != "pat-a" {
		t.Fatalf("ws-a credentials = %q/%q, %v", username, password, err)
	}
	if _, _, err := svc.ResolveAzureDevOpsCloneCredentials(ctx, "ws-missing"); err == nil {
		t.Fatal("unconfigured workspace must not resolve credentials")
	}
	if _, _, err := svc.ResolveAzureDevOpsCloneCredentials(ctx, " "); err == nil {
		t.Fatal("blank workspace must not resolve credentials")
	}
}

// authorizerPAT exposes the PAT carried by a test-observed authorizer.
func authorizerPAT(auth Authorizer) string {
	if pat, ok := auth.(patAuthorizer); ok {
		return pat.pat
	}
	return ""
}
