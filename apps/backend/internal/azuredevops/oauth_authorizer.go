package azuredevops

import (
	"context"
	"strings"
)

const bearerPrefix = "Bearer "

// staticBearerAuthorizer authenticates one request sequence with a known
// access token, as during sign-in before the token is stored.
type staticBearerAuthorizer struct{ token string }

func (a staticBearerAuthorizer) Header(context.Context) (string, error) {
	return bearerPrefix + a.token, nil
}

func (staticBearerAuthorizer) Renew(context.Context, string) (bool, error) { return false, nil }

func (a staticBearerAuthorizer) Redact(text string) string { return redactToken(text, a.token) }

// oauthAuthorizer reads the workspace's current Microsoft access token for
// every request.
type oauthAuthorizer struct {
	service     *Service
	workspaceID string
}

func (a oauthAuthorizer) Header(ctx context.Context) (string, error) {
	token, err := a.service.accessToken(ctx, a.workspaceID)
	if err != nil {
		return "", err
	}
	return bearerPrefix + token, nil
}

func (a oauthAuthorizer) Renew(ctx context.Context, rejectedHeader string) (bool, error) {
	return a.service.renewAccessToken(ctx, a.workspaceID, strings.TrimPrefix(rejectedHeader, bearerPrefix))
}

func (a oauthAuthorizer) Redact(text string) string {
	if token, err := a.service.storedAccessToken(context.Background(), a.workspaceID); err == nil {
		return redactToken(text, token)
	}
	return text
}

func redactToken(text, token string) string {
	if token == "" {
		return text
	}
	return strings.ReplaceAll(text, token, "[REDACTED]")
}
