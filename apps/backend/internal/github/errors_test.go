package github

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// TestErrorsAreClassifiable verifies that the sentinels declared in errors.go
// remain reachable through errors.Is even when callers wrap them, which is
// the contract HTTP handlers and cleanup paths rely on.
func TestErrorsAreClassifiable(t *testing.T) {
	t.Run("isTaskNotFound recognizes sentinel-wrapped errors", func(t *testing.T) {
		wrapped := fmt.Errorf("%w: task task-1", ErrTaskNotFound)
		if !isTaskNotFound(wrapped) {
			t.Errorf("expected sentinel-wrapped error to be classified as task-not-found")
		}
		if isTaskNotFound(errors.New("something else not found")) {
			t.Errorf("expected unrelated 'not found' string to no longer match")
		}
		if isTaskNotFound(nil) {
			t.Errorf("nil error must not classify as not-found")
		}
	})

	t.Run("AssociateExistingPRByURL wraps ErrInvalidPRURL on malformed input", func(t *testing.T) {
		// Service with a stub client so the client-nil guard does not fire
		// before parsePRURL runs.
		svc := &Service{client: NewMockClient()}
		_, err := svc.AssociateExistingPRByURL(context.Background(), "t1", "", "not a pr url")
		if err == nil {
			t.Fatal("expected error for malformed PR URL")
		}
		if !errors.Is(err, ErrInvalidPRURL) {
			t.Errorf("error not classifiable as ErrInvalidPRURL: %v", err)
		}
	})

	t.Run("ErrInvalidToken survives the ConfigureToken wrap pattern", func(t *testing.T) {
		// ConfigureToken wraps the underlying PAT-client error with
		// `fmt.Errorf("%w: %w", ErrInvalidToken, err)`. Verify the wrap
		// pattern preserves errors.Is reachability for both the sentinel
		// and the inner cause. Exercising the full ConfigureToken would
		// require a real HTTP roundtrip — covered by integration tests.
		inner := errors.New("401 Unauthorized")
		wrapped := fmt.Errorf("%w: %w", ErrInvalidToken, inner)
		if !errors.Is(wrapped, ErrInvalidToken) {
			t.Errorf("wrapped error not classifiable as ErrInvalidToken: %v", wrapped)
		}
		if !errors.Is(wrapped, inner) {
			t.Errorf("wrapped error did not preserve inner cause in the chain: %v", wrapped)
		}
	})
}

// TestIsRepoNotResolvableErr pins the wire-format substrings that classify
// as "stop hammering this repo". A regression here re-opens the PR-watch
// storm that motivated the negative cache: with the classifier missing the
// canonical GraphQL "could not resolve to a repository" signal,
// SyncWatchesBatched keeps retrying the dead repo on every 5s frontend
// poll. Bare 404s are intentionally NOT classified — a 404 from
// /repos/{o}/{r}/pulls/{N} can legitimately mean "this PR is gone" while
// the repo is fine, and the negative cache must not poison the repo on
// that signal.
func TestIsRepoNotResolvableErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil err", nil, false},
		{"unrelated error", errors.New("connection refused"), false},
		{"transient 500", errors.New("HTTP 500: Internal Server Error"), false},
		{"graphql repo missing", errors.New(`graphql error: Could not resolve to a Repository with the name 'NBCUDTC/bff'.`), true},
		{"graphql repo missing mixed case", errors.New("graphql error: could not resolve to a repository ..."), true},
		// Bare 404 errors are NOT repo-not-resolvable — too ambiguous. They
		// may indicate a missing PR number on a live repo, and poisoning
		// the repo for 10 minutes on that signal is the bug this guards
		// against. See the docstring on isRepoNotResolvableErr.
		{"bare gh REST 404 is ambiguous", errors.New("gh api: HTTP 404: Not Found"), false},
		{"bare 404 short form is ambiguous", errors.New("status: 404"), false},
		{"bare 404 with body suffix is ambiguous", errors.New("404 Not Found"), false},
		{"github api 404 is ambiguous", &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/x/y/pulls/123", Body: "Not Found"}, false},
		{"wrapped github api 404 is ambiguous", fmt.Errorf("repo: %w", &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/x/y/pulls/123"}), false},
		{"github api 403", &GitHubAPIError{StatusCode: 403, Endpoint: "/repos/x/y", Body: "Forbidden"}, false},
		{"resource not accessible", errors.New("Resource not accessible by integration"), true},
		{"resource not accessible lowercase", errors.New("resource not accessible by integration"), true},
		{"wrapped sentinel", fmt.Errorf("sync: %w", ErrRepoNotResolvable), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRepoNotResolvableErr(tc.err); got != tc.want {
				t.Errorf("isRepoNotResolvableErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
