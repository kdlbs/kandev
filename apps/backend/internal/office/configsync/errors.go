package configsync

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

// Neutral fetch error classes. Every provider-specific error the fetch layer
// can see (currently *github.GitHubAPIError and *gitlab.APIError) is
// converted to exactly one of these before it reaches the reconciler, so the
// rest of this package never branches on a provider type.
var (
	// ErrNotFound means the requested path does not exist at the configured
	// ref. For an optional subdirectory (agents/, skills/, projects/,
	// routines/) this is not fatal — it just means zero entities of that
	// kind. For the configured root path itself it fails the sync.
	ErrNotFound = errors.New("config sync: path not found")
	// ErrUnavailable means the provider could not be reached, is
	// rate-limiting, or refused the request for an auth/permission reason
	// that the workspace owner can plausibly fix (expired token, missing
	// scope). Fails the sync; the poller will retry on its own schedule.
	ErrUnavailable = errors.New("config sync: provider unavailable")
	// ErrUnreadable is the residue class: anything the classifier cannot
	// place in ErrNotFound or ErrUnavailable (malformed response, an
	// unexpected status code, a non-provider transport error). Fails the
	// sync like ErrUnavailable, but is reported distinctly because it is
	// unlikely to self-resolve on retry alone.
	ErrUnreadable = errors.New("config sync: path unreadable")
)

// classifyFetchErr converts a provider-specific listing/content error into
// one of the three neutral classes above. A nil input returns nil.
func classifyFetchErr(err error) error {
	if err == nil {
		return nil
	}
	var ghErr *github.GitHubAPIError
	if errors.As(err, &ghErr) {
		return classifyStatusCode(ghErr.StatusCode, err)
	}
	var glErr *gitlab.APIError
	if errors.As(err, &glErr) {
		return classifyStatusCode(glErr.StatusCode, err)
	}
	// AC-OFFICE-CONFIG-SYNC-002.4a: a timeout or transport failure (DNS,
	// connection reset, TLS, a context deadline) is Unavailable, not residue
	// — the repository could not be reached at all. Both PAT clients' get()
	// wraps a bare http.Client.Do failure in *url.Error, which is what this
	// checks for, rather than matching net.Error's Timeout() alone: a
	// connection-refused error has no timeout but is equally "could not
	// reach the provider".
	var urlErr *url.Error
	if errors.As(err, &urlErr) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return joinNeutral(ErrUnavailable, err)
	}
	// The gh-CLI client reports the same class of failure differently: a
	// non-zero `gh` exit with no HTTP status in stderr, wrapped bare (not as
	// a *url.Error) by GHClient.runGH. github.IsConnectivityError matches
	// that shape by the network-failure text gh CLI prints to stderr.
	if github.IsConnectivityError(err) {
		return joinNeutral(ErrUnavailable, err)
	}
	// Any other error shape the client returned bare — most commonly a
	// response body it received but could not decode. The repository WAS
	// reached, so this is residue rather than Unavailable.
	return joinNeutral(ErrUnreadable, err)
}

// classifyStatusCode maps an HTTP status code to a neutral class. 404 is the
// unambiguous "not found" signal both providers use for a missing path. 401
// (bad/expired credential), 403 (missing scope or, on GitHub, secondary rate
// limiting), and 429 (explicit rate limiting) are all conditions a workspace
// owner can act on by re-authenticating or waiting, so they classify as
// ErrUnavailable together with any 5xx from the provider itself. Everything
// else (400, 422, an unexpected 3xx, ...) is residue: ErrUnreadable.
func classifyStatusCode(statusCode int, err error) error {
	switch {
	case statusCode == http.StatusNotFound:
		return joinNeutral(ErrNotFound, err)
	case statusCode == http.StatusUnauthorized,
		statusCode == http.StatusForbidden,
		statusCode == http.StatusTooManyRequests,
		statusCode >= http.StatusInternalServerError:
		return joinNeutral(ErrUnavailable, err)
	default:
		return joinNeutral(ErrUnreadable, err)
	}
}

// joinNeutral wraps the original error behind the neutral sentinel so
// errors.Is(result, ErrNotFound) (etc.) works while %w / logs still show the
// underlying provider detail.
func joinNeutral(class, err error) error {
	return &neutralError{class: class, cause: err}
}

type neutralError struct {
	class error
	cause error
}

func (e *neutralError) Error() string { return e.class.Error() + ": " + e.cause.Error() }
func (e *neutralError) Is(target error) bool {
	return target == e.class
}
func (e *neutralError) Unwrap() error { return e.cause }
