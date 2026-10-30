package azuredevops

import (
	"context"
	"fmt"
	"net/url"
	"sync"
)

const (
	mockOAuthClientID  = "kandev-mock-client"
	mockOAuthTenant    = "mock-tenant"
	mockOAuthCode      = "mock-authorization-code"
	mockAuthorizePath  = "/api/v1/azure-devops/mock/oauth/authorize"
	mockTokenLifetimeS = 3600
)

// mockEntra stands in for Microsoft in E2E runs. Its authorization URL is a
// relative mock route on the same origin as the app, so popups never leave
// Kandev.
type mockEntra struct {
	mu     sync.Mutex
	issued int
}

func (m *mockEntra) DiscoverTenant(context.Context, string) (string, error) {
	return mockOAuthTenant, nil
}

func (m *mockEntra) AuthorizeURL(req entraAuthorizeRequest) string {
	return mockAuthorizePath + "?" + url.Values{"state": {req.State}}.Encode()
}

func (m *mockEntra) ExchangeCode(_ context.Context, _, _, code, _, _ string) (*entraToken, error) {
	if code != mockOAuthCode {
		return nil, &EntraError{Code: "invalid_grant"}
	}
	return m.issue(), nil
}

func (m *mockEntra) Refresh(context.Context, string, string, string) (*entraToken, error) {
	return m.issue(), nil
}

func (m *mockEntra) issue() *entraToken {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.issued++
	return &entraToken{
		AccessToken:  fmt.Sprintf("mock-access-%d", m.issued),
		RefreshToken: fmt.Sprintf("mock-refresh-%d", m.issued),
		ExpiresIn:    mockTokenLifetimeS,
	}
}

// mockAuthorizeRedirect returns the callback URL Microsoft would redirect to:
// an error when the E2E state requests one, otherwise the mock code.
func mockAuthorizeRedirect(state, oauthError string) string {
	params := url.Values{"state": {state}}
	if oauthError != "" {
		params.Set("error", oauthError)
	} else {
		params.Set("code", mockOAuthCode)
	}
	return OAuthCallbackPath + "?" + params.Encode()
}
