package configsync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

func TestClassifyFetchErr_GitHub(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"404 is not found", http.StatusNotFound, ErrNotFound},
		{"401 is unavailable", http.StatusUnauthorized, ErrUnavailable},
		{"403 is unavailable", http.StatusForbidden, ErrUnavailable},
		{"429 is unavailable", http.StatusTooManyRequests, ErrUnavailable},
		{"500 is unavailable", http.StatusInternalServerError, ErrUnavailable},
		{"422 is unreadable residue", http.StatusUnprocessableEntity, ErrUnreadable},
		{"400 is unreadable residue", http.StatusBadRequest, ErrUnreadable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &github.GitHubAPIError{StatusCode: tt.status, Endpoint: "/repos/o/r/contents/x"}
			got := classifyFetchErr(src)
			if !errors.Is(got, tt.want) {
				t.Fatalf("classifyFetchErr(%d) = %v, want class %v", tt.status, got, tt.want)
			}
			if !errors.Is(got, src) {
				t.Errorf("classifyFetchErr(%d) lost the underlying cause via Unwrap", tt.status)
			}
		})
	}
}

func TestClassifyFetchErr_GitLab(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"404 is not found", http.StatusNotFound, ErrNotFound},
		{"403 is unavailable", http.StatusForbidden, ErrUnavailable},
		{"418 is unreadable residue", http.StatusTeapot, ErrUnreadable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &gitlab.APIError{StatusCode: tt.status, Endpoint: "/projects/1/repository/tree"}
			got := classifyFetchErr(src)
			if !errors.Is(got, tt.want) {
				t.Fatalf("classifyFetchErr(%d) = %v, want class %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestClassifyFetchErr_TransportFailureIsUnavailable(t *testing.T) {
	// Both PAT clients' get() wraps a bare http.Client.Do failure (DNS,
	// connection refused, TLS, timeout) as fmt.Errorf("request %s: %w", ...)
	// around the *url.Error http.Client.Do itself returns. Reproduce that
	// exact shape rather than a bare errors.New, so this test would have
	// caught the misclassification it once let through. This covers only the
	// PAT-client transport shape — the gh-CLI client reports the same class
	// of failure differently (see
	// TestClassifyFetchErr_GHClientTransportFailureIsUnavailable below).
	urlErr := &url.Error{Op: "Get", URL: "https://api.github.com/x", Err: errors.New("dial tcp: connection refused")}
	src := fmt.Errorf("request %s: %w", "/repos/o/r/contents/x", urlErr)
	got := classifyFetchErr(src)
	if !errors.Is(got, ErrUnavailable) {
		t.Fatalf("classifyFetchErr(transport) = %v, want ErrUnavailable", got)
	}
	if !errors.Is(got, src) {
		t.Errorf("classifyFetchErr(transport) lost the underlying cause via Unwrap")
	}
}

func TestClassifyFetchErr_GHClientTransportFailureIsUnavailable(t *testing.T) {
	// GHClient.runGH wraps a non-timeout `gh` CLI failure bare (not as a
	// *url.Error): fmt.Errorf("gh %s: %w: %s", firstArg(args), runErr,
	// stderr.String()). A real network failure has no "HTTP nnn" / "status:
	// nnn" text for ghClassifyContentsErr to match, so ListRepoDirectory and
	// GetRepoFileContent fall back to this bare shape, and the stderr text is
	// where the network-failure signal actually lives.
	runErr := errors.New("exit status 1")
	src := fmt.Errorf("gh %s: %w: %s", "repos/o/r/contents/agents", runErr,
		"dial tcp: lookup api.github.com: no such host")
	got := classifyFetchErr(src)
	if !errors.Is(got, ErrUnavailable) {
		t.Fatalf("classifyFetchErr(gh-CLI transport) = %v, want ErrUnavailable", got)
	}
	if !errors.Is(got, src) {
		t.Errorf("classifyFetchErr(gh-CLI transport) lost the underlying cause via Unwrap")
	}
}

func TestClassifyFetchErr_ContextDeadlineIsUnavailable(t *testing.T) {
	urlErr := &url.Error{Op: "Get", URL: "https://gitlab.example.com/x", Err: context.DeadlineExceeded}
	got := classifyFetchErr(fmt.Errorf("request %s: %w", "/projects/1/repository/tree", urlErr))
	if !errors.Is(got, ErrUnavailable) {
		t.Fatalf("classifyFetchErr(deadline) = %v, want ErrUnavailable", got)
	}
}

func TestClassifyFetchErr_UntypedDecodeFailureIsResidue(t *testing.T) {
	// A response body the provider client returns but cannot decode (e.g. a
	// malformed JSON payload) is an untyped error too, but the repository
	// WAS reached — this must stay ErrUnreadable, not ErrUnavailable.
	src := errors.New("invalid character 'x' looking for beginning of value")
	got := classifyFetchErr(src)
	if !errors.Is(got, ErrUnreadable) {
		t.Fatalf("classifyFetchErr(decode failure) = %v, want ErrUnreadable", got)
	}
	if !errors.Is(got, src) {
		t.Errorf("classifyFetchErr(decode failure) lost the underlying cause via Unwrap")
	}
}

func TestClassifyFetchErr_Nil(t *testing.T) {
	if got := classifyFetchErr(nil); got != nil {
		t.Fatalf("classifyFetchErr(nil) = %v, want nil", got)
	}
}
