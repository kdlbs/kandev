package workflowsync

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

const genericSyncFailureMessage = "Workflow sync failed"

// classifySyncErr maps a sync failure to an authcircuit.FailureClass so the
// caller can decide whether to back off on the short transient schedule or
// the long permanent one that only a credential/config change shortens.
//
// GitHub and GitLab clients report auth/permission failures as typed API
// errors carrying the HTTP status; this package has no other way to learn
// "the token is bad" versus "GitHub had a bad minute", so those status codes
// are the classification boundary. Client-provider-not-configured errors
// (returned by listGitHubEntries/listGitLabEntries when the workspace has no
// connection at all) are a config failure: retrying on any schedule cannot
// fix "nobody has connected this integration yet".
func classifySyncErr(err error) authcircuit.FailureClass {
	if err == nil {
		return authcircuit.FailureClassNone
	}
	if errors.Is(err, errGitHubClientNotConfigured) || errors.Is(err, errGitLabClientNotConfigured) {
		return authcircuit.FailureClassConfig
	}

	var ghErr *github.GitHubAPIError
	if errors.As(err, &ghErr) {
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
	var glErr *gitlab.APIError
	if errors.As(err, &glErr) {
		return classifyStatusCode(glErr.StatusCode)
	}
	if errors.Is(err, github.ErrGitHubConnectionInvalid) || errors.Is(err, github.ErrGitHubNotConfigured) {
		return authcircuit.FailureClassAuth
	}
	if errors.Is(err, gitlab.ErrInvalidToken) {
		return authcircuit.FailureClassAuth
	}
	return authcircuit.FailureClassTransient
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
		// The configured repo/project/branch/path does not exist or is not
		// reachable with the current (valid) credential — a configuration
		// problem, not a credential one.
		return authcircuit.FailureClassConfig
	default:
		return authcircuit.FailureClassTransient
	}
}

// safeSyncErrorMessage removes provider response bodies before a sync failure
// is persisted or returned. The remaining provider and status identify the
// failure without retaining arbitrary upstream content.
func safeSyncErrorMessage(err error) string {
	var ghErr *github.GitHubAPIError
	if errors.As(err, &ghErr) {
		return fmt.Sprintf("GitHub request failed with HTTP status %d", ghErr.StatusCode)
	}
	var glErr *gitlab.APIError
	if errors.As(err, &glErr) {
		return fmt.Sprintf("GitLab request failed with HTTP status %d", glErr.StatusCode)
	}
	return genericSyncFailureMessage
}

// safeStoredSyncErrorMessage sanitizes historical error values read from the
// database. Rows written before provider errors were sanitized can contain
// arbitrary upstream response bodies, so only retain the exact safe summaries
// written by the current code.
func safeStoredSyncErrorMessage(message string) string {
	if message == "" || message == genericSyncFailureMessage {
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
	return genericSyncFailureMessage
}
