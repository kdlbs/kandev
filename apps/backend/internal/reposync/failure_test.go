package reposync

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

func TestClassifySyncErr_NilIsNone(t *testing.T) {
	assert.Equal(t, authcircuit.FailureClassNone, classifyError(nil))
}

func TestClassifySyncErr_NotConfiguredIsConfig(t *testing.T) {
	assert.Equal(t, authcircuit.FailureClassConfig, classifyError(github.ErrGitHubHostMismatch))
	assert.Equal(t, authcircuit.FailureClassConfig, classifyError(ErrGitHubClientNotConfigured))
	assert.Equal(t, authcircuit.FailureClassConfig, classifyError(ErrGitLabClientNotConfigured))
	// Wrapped sentinels must still classify correctly — fetchFiles/list
	// helpers wrap these with additional context.
	assert.Equal(t, authcircuit.FailureClassConfig,
		classifyError(fmt.Errorf("listing failed: %w", ErrGitHubClientNotConfigured)))
}

func TestClassifySyncErr_GitHubAPIErrorByStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   authcircuit.FailureClass
	}{
		{"unauthorized", http.StatusUnauthorized, authcircuit.FailureClassAuth},
		{"forbidden", http.StatusForbidden, authcircuit.FailureClassAuth},
		{"not_found", http.StatusNotFound, authcircuit.FailureClassConfig},
		{"unprocessable", http.StatusUnprocessableEntity, authcircuit.FailureClassConfig},
		{"bad_request", http.StatusBadRequest, authcircuit.FailureClassConfig},
		{"server_error", http.StatusInternalServerError, authcircuit.FailureClassTransient},
		{"rate_limited", http.StatusTooManyRequests, authcircuit.FailureClassTransient},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &github.GitHubAPIError{StatusCode: tt.status, Endpoint: "/repos/x/y", Body: "irrelevant"}
			assert.Equal(t, tt.want, classifyError(err))
			// Wrapped through a %w chain (the real call sites always wrap).
			wrapped := fmt.Errorf("failed to list x/y: %w", err)
			assert.Equal(t, tt.want, classifyError(wrapped))
		})
	}
}

func TestClassifySyncErr_GitHubRateLimit403IsTransient(t *testing.T) {
	err := &github.GitHubAPIError{StatusCode: http.StatusForbidden, Endpoint: "/repos/x/y", Body: "API rate limit exceeded for installation"}
	assert.Equal(t, authcircuit.FailureClassTransient, classifyError(err))
}

func TestClassifySyncErr_GitLabAPIErrorByStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   authcircuit.FailureClass
	}{
		{"unauthorized", http.StatusUnauthorized, authcircuit.FailureClassAuth},
		{"forbidden", http.StatusForbidden, authcircuit.FailureClassAuth},
		{"not_found", http.StatusNotFound, authcircuit.FailureClassConfig},
		{"server_error", http.StatusBadGateway, authcircuit.FailureClassTransient},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &gitlab.APIError{StatusCode: tt.status, Endpoint: "/projects/x"}
			assert.Equal(t, tt.want, classifyError(err))
		})
	}
}

func TestClassifySyncErr_SentinelFallbacksAreAuth(t *testing.T) {
	assert.Equal(t, authcircuit.FailureClassAuth, classifyError(github.ErrNoClient))
	assert.Equal(t, authcircuit.FailureClassAuth, classifyError(github.ErrGitHubConnectionInvalid))
	assert.Equal(t, authcircuit.FailureClassAuth, classifyError(github.ErrGitHubNotConfigured))
	assert.Equal(t, authcircuit.FailureClassAuth, classifyError(gitlab.ErrInvalidToken))
}

func TestClassifySyncErr_UnrecognizedErrorIsTransient(t *testing.T) {
	assert.Equal(t, authcircuit.FailureClassTransient, classifyError(errors.New("connection reset by peer")))
}

// TestClassifySyncErr_TypedErrorTakesPrecedenceOverSentinel confirms the
// documented precedence: a typed API error's status code wins even when the
// underlying transport also happens to satisfy one of the coarser sentinel
// checks, because the status code is the more specific signal.
func TestClassifySyncErr_TypedErrorTakesPrecedenceOverSentinel(t *testing.T) {
	err := &github.GitHubAPIError{StatusCode: http.StatusNotFound, Endpoint: "/repos/x/y"}
	assert.Equal(t, authcircuit.FailureClassConfig, classifyError(err))
}
