package azuredevops

import (
	"fmt"
	"strings"
)

// DefaultOAuthClientID is the application (client) ID of Kandev's published
// multi-tenant Microsoft Entra public client. Empty means Microsoft sign-in is
// available only when an operator configures azureDevOps.oauthClientId.
const DefaultOAuthClientID = ""

// OAuthCallbackPath is the Microsoft sign-in redirect route.
const OAuthCallbackPath = "/api/v1/azure-devops/oauth/callback"

// OAuthSettings is the Entra application identity used for Microsoft sign-in.
type OAuthSettings struct {
	ClientID    string
	RedirectURL string
}

// Available reports whether Microsoft sign-in can start.
func (s OAuthSettings) Available() bool {
	return s.ClientID != "" && s.RedirectURL != ""
}

// ResolveOAuthSettings applies the built-in application and the local
// callback to empty operator configuration.
func ResolveOAuthSettings(clientID, redirectURL string, serverPort int) OAuthSettings {
	settings := OAuthSettings{
		ClientID:    strings.TrimSpace(clientID),
		RedirectURL: strings.TrimSpace(redirectURL),
	}
	if settings.ClientID == "" {
		settings.ClientID = DefaultOAuthClientID
	}
	if settings.RedirectURL == "" {
		settings.RedirectURL = fmt.Sprintf("http://localhost:%d%s", serverPort, OAuthCallbackPath)
	}
	return settings
}

// ConfigureOAuth sets the Entra application identity. The E2E mock keeps its
// own identity so tests never reach Microsoft.
func (s *Service) ConfigureOAuth(settings OAuthSettings) {
	if s == nil || s.mock != nil {
		return
	}
	s.oauth = settings
}

// OAuthDefaults returns the installation's Entra application, which the
// settings page offers as the default for a workspace.
func (s *Service) OAuthDefaults() OAuthSettings {
	if s == nil {
		return OAuthSettings{}
	}
	return s.oauth
}

// OAuthAvailable reports whether the installation has a default Entra
// application. A workspace can still sign in with its own application.
func (s *Service) OAuthAvailable() bool {
	return s != nil && s.entra != nil && s.oauth.Available()
}
