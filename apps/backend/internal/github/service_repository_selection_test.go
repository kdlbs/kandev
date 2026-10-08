package github

import (
	"errors"
	"net/http"
	"testing"
)

func TestClassifyRepositoryRootReadError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantAbsent bool
	}{
		{name: "exact repository not found", statusCode: http.StatusNotFound, wantAbsent: true},
		{name: "authorization failure remains unavailable", statusCode: http.StatusForbidden},
		{name: "transient failure remains unavailable", statusCode: http.StatusBadGateway},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := &GitHubAPIError{StatusCode: test.statusCode, Endpoint: "/repos/owner/name"}
			got := classifyRepositoryRootReadError(err)
			if errors.Is(got, ErrRepoNotResolvable) != test.wantAbsent {
				t.Fatalf("errors.Is(err, ErrRepoNotResolvable) = %v, want %v", errors.Is(got, ErrRepoNotResolvable), test.wantAbsent)
			}
			var gotAPIError *GitHubAPIError
			if !errors.As(got, &gotAPIError) {
				t.Fatalf("errors.As(err, *GitHubAPIError) = false, want original provider error preserved")
			}
			if gotAPIError.StatusCode != test.statusCode {
				t.Fatalf("status code = %d, want %d", gotAPIError.StatusCode, test.statusCode)
			}
		})
	}
}
