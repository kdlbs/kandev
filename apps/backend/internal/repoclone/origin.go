package repoclone

import (
	"fmt"
	"net/url"
	"strings"
)

// HTTPSProviderOrigin validates a provider host and returns its canonical
// HTTPS origin. Provider hosts may include a product context path, but Git
// credentials are scoped only to the exact scheme, host, and port.
func HTTPSProviderOrigin(providerHost string) (string, error) {
	value := strings.TrimSpace(providerHost)
	if value == "" {
		return "", fmt.Errorf("provider host is required")
	}
	if !strings.Contains(value, "://") {
		value = ProtocolHTTPS + "://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, ProtocolHTTPS) || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("provider host must be a credential-free HTTPS URL")
	}
	return ProtocolHTTPS + "://" + strings.ToLower(parsed.Host), nil
}

// ValidateHTTPSCloneOrigin rejects credential-bearing or ambiguous clone URLs
// and requires their origin to match the provider identity that authorized
// credential use.
func ValidateHTTPSCloneOrigin(cloneURL, providerHost string) error {
	parsed, err := url.Parse(strings.TrimSpace(cloneURL))
	if err != nil || !strings.EqualFold(parsed.Scheme, ProtocolHTTPS) || parsed.Host == "" ||
		parsed.User != nil || parsed.Path == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("clone URL must be a credential-free HTTPS URL")
	}
	wantOrigin, err := HTTPSProviderOrigin(providerHost)
	if err != nil {
		return err
	}
	gotOrigin := ProtocolHTTPS + "://" + strings.ToLower(parsed.Host)
	if gotOrigin != wantOrigin {
		return fmt.Errorf("clone URL origin %q does not match provider origin %q", gotOrigin, wantOrigin)
	}
	return nil
}
