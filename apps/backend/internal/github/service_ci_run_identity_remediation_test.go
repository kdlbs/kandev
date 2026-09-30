package github

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestFreshCIRunPersistsCompleteNonSecretProviderIdentity(t *testing.T) {
	service, client, input := setupCIRunServiceTest(t, false)
	client.principal = TokenPrincipal{
		Kind: TokenCredentialInstallation, PrincipalID: "installation:55", Login: "kandev-ci[bot]",
		InstallationID: 55, AppRegistrationID: "registration-1", AppCredentialGeneration: 7,
	}
	client.mutationMetadata = GitHubRequestMetadata{
		RequestID: "github-request-1",
		URL:       "https://api.github.com/repos/kdlbs/kandev/actions/runs/100/rerun-failed-jobs",
	}
	client.runs = []GitHubActionsRun{*client.run}
	client.runs[0].Attempt = input.ExpectedSourceAttempt + 1

	receipt, err := service.RequestFreshCIRun(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Repository != "kdlbs/kandev" || receipt.ObservedPRHeadSHA != input.ExpectedHeadSHA ||
		receipt.ProviderEvent != "pull_request" || receipt.ProviderRequestID != "github-request-1" ||
		receipt.ProviderURL != client.mutationMetadata.URL || receipt.IdempotencyStatus != "created" {
		t.Fatalf("receipt identity = %+v", receipt)
	}
	if receipt.ProviderPrincipal == nil || receipt.ProviderPrincipal.Kind != AuthPrincipalApp ||
		receipt.ProviderPrincipal.Source != ConnectionSourceGitHubAppInstallation ||
		receipt.ProviderPrincipal.InstallationID != 55 ||
		receipt.ProviderPrincipal.AppRegistrationID != "registration-1" {
		t.Fatalf("receipt principal = %+v", receipt.ProviderPrincipal)
	}
	loaded, err := service.store.GetCIRunRequest(context.Background(), receipt.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CanonicalRepository != receipt.Repository || loaded.ProviderEvent != receipt.ProviderEvent ||
		loaded.ProviderRequestID != receipt.ProviderRequestID || loaded.ProviderPrincipalJSON == "" {
		t.Fatalf("stored identity = %+v", loaded)
	}
	var events []CIRunAuditEvent
	if err := service.store.ro.Select(&events, `SELECT id, request_id, event_type, failure_class,
		details_json, created_at FROM github_ci_run_audit_events ORDER BY created_at, event_type`); err != nil {
		t.Fatal(err)
	}
	var terminal *CIRunAuditEvent
	for index := range events {
		if events[index].EventType == "succeeded" {
			terminal = &events[index]
		}
	}
	if terminal == nil {
		t.Fatalf("audit events = %+v, want succeeded", events)
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(terminal.DetailsJSON), &details); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"canonical_repository": "kdlbs/kandev",
		"provider_event":       "pull_request",
		"provider_request_id":  "github-request-1",
	} {
		if details[key] != want {
			t.Fatalf("audit %s = %#v, want %#v; details=%v", key, details[key], want, details)
		}
	}
	lower := strings.ToLower(terminal.DetailsJSON)
	for _, forbidden := range []string{"token", "authorization", "private_key", "credential"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("audit contains forbidden secret-shaped key %q: %s", forbidden, terminal.DetailsJSON)
		}
	}
}
