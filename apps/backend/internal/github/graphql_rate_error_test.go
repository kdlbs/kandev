package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-RATE-001.1
// @covers AC-INTEGRATIONS-GITHUB-RATE-001.4
// @covers AC-INTEGRATIONS-GITHUB-RATE-001.6
// @covers AC-INTEGRATIONS-GITHUB-RATE-004.1
// @covers AC-INTEGRATIONS-GITHUB-RATE-004.2
func TestGraphQLRatePayloadClassificationAcrossClients(t *testing.T) {
	for _, clientKind := range []string{"PAT", "CLI"} {
		t.Run(clientKind, func(t *testing.T) {
			t.Run("primary payload retains reset and later Retry-After", func(t *testing.T) {
				now := time.Now().UTC().Truncate(time.Second)
				resetAt := now.Add(30 * time.Minute)
				laterRetry := now.Add(time.Hour)
				body := graphQLRateErrorPayload(t, 0, resetAt.Format(time.RFC3339), "RATE_LIMITED", "API rate limit exceeded")
				headers := make(http.Header)
				if clientKind == "PAT" {
					headers.Set("Retry-After", laterRetry.Format(http.TimeFormat))
				}
				tracker, admission, _, err := executeGraphQLPayload(t, clientKind, http.StatusOK, headers, body, 1)

				apiErr := requireGraphQLRateError(t, err, FailurePrimaryRateLimit)
				wantRetry := resetAt
				wantSource := RetrySourcePrimaryReset
				if clientKind == "PAT" {
					wantRetry = laterRetry
					wantSource = RetrySourceRetryAfter
				}
				if apiErr.Resource != ResourceGraphQL || apiErr.RetrySource != wantSource || !apiErr.RetryAt.Equal(wantRetry) {
					t.Fatalf("API error = %+v, want GraphQL retry %s from %s", apiErr, wantRetry, wantSource)
				}
				if strings.Contains(apiErr.Error(), "private-fragment") {
					t.Fatalf("GraphQL error exposed partial response data: %q", apiErr.Error())
				}
				primary := tracker.PrimaryRetry(ResourceGraphQL)
				if primary.RetrySource != wantSource || !primary.RetryAt.Equal(wantRetry) {
					t.Fatalf("tracked primary retry = %+v, want %s from %s", primary, wantRetry, wantSource)
				}
				assertAdmissionRetry(t, admission, wantRetry)
			})

			t.Run("invalid reset keeps primary evidence and uses fallback", func(t *testing.T) {
				started := time.Now().UTC()
				body := graphQLRateErrorPayload(t, 0, "invalid-reset", "RATE_LIMITED", "API rate limit exceeded")
				tracker, admission, _, err := executeGraphQLPayload(t, clientKind, http.StatusOK, nil, body, 1)

				apiErr := requireGraphQLRateError(t, err, FailurePrimaryRateLimit)
				if apiErr.RetrySource != RetrySourceConservativeFallback || apiErr.RetryAt.Before(started.Add(50*time.Second)) {
					t.Fatalf("invalid reset retry = %s from %s, want a future conservative boundary", apiErr.RetryAt, apiErr.RetrySource)
				}
				snapshot, ok := tracker.Snapshot(ResourceGraphQL)
				if !ok || !snapshot.RemainingObserved || snapshot.Remaining != 0 {
					t.Fatalf("tracked quota = %+v, present=%v, want observed zero", snapshot, ok)
				}
				assertAdmissionRetry(t, admission, apiErr.RetryAt)
			})

			t.Run("message-only secondary error updates admission", func(t *testing.T) {
				body := `{"data":null,"errors":[{"message":"You have exceeded a secondary rate limit"}]}`
				tracker, admission, _, err := executeGraphQLPayload(t, clientKind, http.StatusOK, nil, body, 1)

				apiErr := requireGraphQLRateError(t, err, FailureSecondaryRateLimit)
				if apiErr.RetrySource != RetrySourceConservativeFallback || !apiErr.RetryAt.After(time.Now()) {
					t.Fatalf("secondary retry = %s from %s, want future fallback", apiErr.RetryAt, apiErr.RetrySource)
				}
				if !tracker.Secondary(ResourceGraphQL).Active {
					t.Fatal("message-only GraphQL rate error did not activate secondary throttle")
				}
				assertAdmissionRetry(t, admission, apiErr.RetryAt)
			})

			t.Run("partial non-rate data remains available", func(t *testing.T) {
				body := `{"data":{"viewer":{"login":"partial-user"}},"errors":[{"type":"GRAPHQL_VALIDATION_FAILED","message":"unknown field"}]}`
				_, _, out, err := executeGraphQLPayload(t, clientKind, http.StatusOK, nil, body, 0)
				if err != nil {
					t.Fatalf("ExecuteGraphQL returned error for partial non-rate data: %v", err)
				}
				data, ok := out["data"].(map[string]any)
				if !ok {
					t.Fatalf("partial response has no data object: %#v", out)
				}
				viewer, ok := data["viewer"].(map[string]any)
				if !ok || viewer["login"] != "partial-user" {
					t.Fatalf("partial data = %#v, want viewer login", out)
				}
			})

			t.Run("successful zero remainder is not an operation failure", func(t *testing.T) {
				resetAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
				body, err := json.Marshal(map[string]any{
					"data": map[string]any{"rateLimit": map[string]any{
						"limit": 5000, "remaining": 0, "resetAt": resetAt.Format(time.RFC3339), "cost": 1,
					}},
				})
				if err != nil {
					t.Fatal(err)
				}
				tracker, admission, _, err := executeGraphQLPayload(t, clientKind, http.StatusOK, nil, string(body), 0)
				if err != nil {
					t.Fatalf("successful zero-remainder query returned error: %v", err)
				}
				snapshot, ok := tracker.Snapshot(ResourceGraphQL)
				if !ok || !snapshot.RemainingObserved || snapshot.Remaining != 0 || !snapshot.ResetAt.Equal(resetAt) {
					t.Fatalf("tracked successful quota = %+v, present=%v", snapshot, ok)
				}
				assertAdmissionRetry(t, admission, resetAt)
			})
		})
	}
}

func graphQLRateErrorPayload(t *testing.T, remaining int, resetAt, errorType, message string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"private":   "private-fragment",
			"rateLimit": map[string]any{"limit": 5000, "remaining": remaining, "resetAt": resetAt, "cost": 1},
		},
		"errors": []map[string]string{{"type": errorType, "message": message}},
	})
	if err != nil {
		t.Fatalf("marshal GraphQL fixture: %v", err)
	}
	return string(body)
}

func executeGraphQLPayload(
	t *testing.T,
	clientKind string,
	status int,
	headers http.Header,
	body string,
	cliExit int,
) (*RateTracker, *RateAdmission, map[string]any, error) {
	t.Helper()
	coordinator := NewRateCoordinator(nil, nil)
	tracker, admission := coordinator.coordinate(defaultGitHubHost, AuthPrincipal{
		Kind: AuthPrincipalHuman, Login: "graphql-payload-test-" + clientKind,
	}, nil)
	out := make(map[string]any)
	var err error
	if clientKind == "PAT" {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			for key, values := range headers {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(server.Close)
		client := newPATClientPointingAt(t, server.URL).WithRateTracker(tracker)
		client.withRateAdmission(admission)
		err = client.ExecuteGraphQL(context.Background(), "query { viewer { login } }", nil, &out)
	} else {
		newFakeGH(t, ghResponse{
			Prefix: "api graphql", Stdout: body, Stderr: "GraphQL query failed", Exit: cliExit,
		})
		client := NewGHClient().WithRateTracker(tracker)
		client.withRateAdmission(admission)
		err = client.ExecuteGraphQL(context.Background(), "query { viewer { login } }", nil, &out)
	}
	return tracker, admission, out, err
}

func requireGraphQLRateError(t *testing.T, err error, kind FailureKind) *GitHubAPIError {
	t.Helper()
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("ExecuteGraphQL error = %T %v, want *GitHubAPIError", err, err)
	}
	if apiErr.FailureKind != kind {
		t.Fatalf("GraphQL failure kind = %q, want %q", apiErr.FailureKind, kind)
	}
	return apiErr
}

func assertAdmissionRetry(t *testing.T, admission *RateAdmission, want time.Time) {
	t.Helper()
	ctx := WithNonBlockingGitHubAdmission(WithGitHubWorkClass(context.Background(), WorkClassBackground))
	release, err := admission.acquire(ctx, ResourceGraphQL)
	if release != nil {
		release()
	}
	var deferred *AdmissionDeferredError
	if !errors.As(err, &deferred) {
		t.Fatalf("next background admission error = %v, want a deferred retry", err)
	}
	if !deferred.RetryAt.Equal(want) {
		t.Fatalf("next background retry = %s, want %s", deferred.RetryAt, want)
	}
}
