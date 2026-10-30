package azuredevops

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// resolveOAuthApplication picks the Entra application for one sign-in: the
// workspace's own client ID and redirect URL when given, otherwise the
// installation defaults.
func (s *Service) resolveOAuthApplication(req *StartOAuthRequest) (OAuthSettings, error) {
	app := s.oauth
	if clientID := strings.TrimSpace(req.ClientID); clientID != "" {
		if !tenantIDRE.MatchString(clientID) {
			return OAuthSettings{}, fmt.Errorf("%w: clientId must be an application (client) ID GUID", ErrInvalidConfig)
		}
		app.ClientID = strings.ToLower(clientID)
	}
	if redirectURL := strings.TrimSpace(req.RedirectURL); redirectURL != "" {
		if err := validateOAuthRedirectURL(redirectURL); err != nil {
			return OAuthSettings{}, fmt.Errorf("%w: %s", ErrInvalidConfig, err.Error())
		}
		app.RedirectURL = redirectURL
	}
	if s.entra == nil || !app.Available() {
		return OAuthSettings{}, ErrOAuthUnavailable
	}
	return app, nil
}

// validateOAuthRedirectURL accepts absolute HTTPS URLs and loopback HTTP URLs
// without credentials, query, or fragment. Microsoft still checks the URL
// against the application's registration.
func validateOAuthRedirectURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return errors.New("redirectUrl must be an absolute URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return errors.New("redirectUrl must not include credentials, query, or fragment")
	}
	if parsed.Scheme == "https" || (parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return nil
	}
	return errors.New("redirectUrl must use HTTPS outside localhost")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// oauthClientID returns the application that must renew a connection's
// tokens: the one that signed in, else the installation default.
func (s *Service) oauthClientID(cfg *Config) string {
	if cfg.OAuthClientID != "" {
		return cfg.OAuthClientID
	}
	return s.oauth.ClientID
}
