package workflowsync

import (
	"errors"
	"net/http"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

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
