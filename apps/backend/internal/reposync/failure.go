package reposync

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

// GitHubConnectionFailureMessage is the sanitized message for a workspace
// without a usable GitHub connection.
const GitHubConnectionFailureMessage = "GitHub connection is not configured"

// ErrGitHubClientNotConfigured and ErrGitLabClientNotConfigured mark a
// workspace without a usable client for the configured provider. They are a
// configuration failure: retrying cannot fix a connection nobody created.
var (
	ErrGitHubClientNotConfigured = errors.New("GitHub is not authenticated; configure a GitHub connection for the workspace")
	ErrGitLabClientNotConfigured = errors.New("GitLab is not authenticated; configure a GitLab connection for the workspace")
)

// classifyError maps a sync failure to an authcircuit.FailureClass. Typed
// provider API errors carry the HTTP status, which is the boundary between a
// bad credential, a bad configuration, and a temporary provider failure.
func classifyError(err error) authcircuit.FailureClass {
	if err == nil {
		return authcircuit.FailureClassNone
	}
	if errors.Is(err, github.ErrGitHubHostMismatch) || errors.Is(err, ErrGitHubClientNotConfigured) || errors.Is(err, ErrGitLabClientNotConfigured) {
		return authcircuit.FailureClassConfig
	}

	var ghErr *github.GitHubAPIError
	if errors.As(err, &ghErr) {
		return classifyGitHubAPIError(err, ghErr)
	}
	var glErr *gitlab.APIError
	if errors.As(err, &glErr) {
		return classifyStatusCode(glErr.StatusCode)
	}
	if errors.Is(err, github.ErrNoClient) || errors.Is(err, github.ErrGitHubConnectionInvalid) || errors.Is(err, github.ErrGitHubNotConfigured) {
		return authcircuit.FailureClassAuth
	}
	if errors.Is(err, gitlab.ErrInvalidToken) {
		return authcircuit.FailureClassAuth
	}
	return authcircuit.FailureClassTransient
}

func classifyGitHubAPIError(err error, ghErr *github.GitHubAPIError) authcircuit.FailureClass {
	switch github.FailureKindOf(err) {
	case github.FailurePrimaryRateLimit, github.FailureSecondaryRateLimit:
		return authcircuit.FailureClassTransient
	case github.FailureInvalidCredentials:
		return authcircuit.FailureClassAuth
	case github.FailureMissingResource:
		return authcircuit.FailureClassConfig
	}
	if ghErr.StatusCode == http.StatusForbidden && githubRateLimitBody(ghErr.Body) {
		return authcircuit.FailureClassTransient
	}
	return classifyStatusCode(ghErr.StatusCode)
}

func githubRateLimitBody(body string) bool {
	body = strings.ToLower(body)
	return strings.Contains(body, "rate limit") || strings.Contains(body, "abuse detection")
}

func classifyStatusCode(status int) authcircuit.FailureClass {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return authcircuit.FailureClassAuth
	case http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusBadRequest:
		// The configured repository, branch, or path is not reachable with the
		// current valid credential: a configuration problem.
		return authcircuit.FailureClassConfig
	default:
		return authcircuit.FailureClassTransient
	}
}

// SafeErrorMessage removes provider response bodies before a sync failure is
// persisted or returned. generic is the feature's fallback summary.
func SafeErrorMessage(err error, generic string) string {
	if errors.Is(err, ErrGitHubClientNotConfigured) || errors.Is(err, github.ErrNoClient) ||
		errors.Is(err, github.ErrGitHubNotConfigured) || errors.Is(err, github.ErrGitHubConnectionInvalid) {
		return GitHubConnectionFailureMessage
	}
	var ghErr *github.GitHubAPIError
	if errors.As(err, &ghErr) {
		return fmt.Sprintf("GitHub request failed with HTTP status %d", ghErr.StatusCode)
	}
	var glErr *gitlab.APIError
	if errors.As(err, &glErr) {
		return fmt.Sprintf("GitLab request failed with HTTP status %d", glErr.StatusCode)
	}
	return generic
}

// SafeStoredErrorMessage sanitizes an error value read back from storage,
// keeping only the exact safe summaries SafeErrorMessage writes.
func SafeStoredErrorMessage(message, generic string) string {
	if message == "" || message == generic || message == GitHubConnectionFailureMessage {
		return message
	}
	for _, prefix := range []string{
		"GitHub request failed with HTTP status ",
		"GitLab request failed with HTTP status ",
	} {
		statusText, ok := strings.CutPrefix(message, prefix)
		if !ok {
			continue
		}
		status, err := strconv.Atoi(statusText)
		if err == nil && status >= 100 && status <= 599 && strconv.Itoa(status) == statusText {
			return message
		}
	}
	return generic
}
