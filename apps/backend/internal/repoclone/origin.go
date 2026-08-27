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
	if !hasSafeRawAuthority(value) {
		return "", fmt.Errorf("provider host must be a credential-free HTTPS URL")
	}
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, ProtocolHTTPS) || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("provider host must be a credential-free HTTPS URL")
	}
	return ProtocolHTTPS + "://" + strings.ToLower(parsed.Host), nil
}

// CanonicalHTTPSCloneURL rewrites an SSH clone URL into its HTTPS equivalent,
// dropping the SSH user and any SSH port. It returns "" for anything that is
// not an SSH clone URL, so callers can fall back to the original value.
//
// A checkout cloned over SSH stores an SSH origin, but the transport is not a
// different repository: managed credentials, and the executors that install
// `url.<https>.insteadOf` rewrites, both work off the HTTPS identity. Callers
// still validate the result against the provider origin.
func CanonicalHTTPSCloneURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		return scpStyleHTTPSCloneURL(value)
	}
	if !hasSafeRawAuthority(value) {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, ProtocolSSH) ||
		parsed.Hostname() == "" || parsed.Path == "" || parsed.Path == "/" {
		return ""
	}
	return ProtocolHTTPS + "://" + strings.ToLower(parsed.Hostname()) + parsed.Path
}

// scpStyleHTTPSCloneURL converts the scp-style "user@host:owner/repo.git" form,
// which carries no scheme and cannot express a port.
func scpStyleHTTPSCloneURL(value string) string {
	user, hostAndPath, found := strings.Cut(value, "@")
	if !found || user == "" || strings.ContainsAny(user, "/:") {
		return ""
	}
	host, path, found := strings.Cut(hostAndPath, ":")
	if !found || host == "" || strings.Contains(host, "/") {
		return ""
	}
	if !IsSafeRawAuthority(user + "@" + host) {
		return ""
	}
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return ""
	}
	return ProtocolHTTPS + "://" + strings.ToLower(host) + "/" + path
}

// ValidateHTTPSCloneOrigin rejects credential-bearing or ambiguous clone URLs
// and requires their origin to match the provider identity that authorized
// credential use.
func ValidateHTTPSCloneOrigin(cloneURL, providerHost string) error {
	value := strings.TrimSpace(cloneURL)
	if !hasSafeRawAuthority(value) {
		return fmt.Errorf("clone URL must be a credential-free HTTPS URL")
	}
	parsed, err := url.Parse(value)
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

func hasSafeRawAuthority(value string) bool {
	authority, found := rawAuthority(value)
	return found && IsSafeRawAuthority(authority)
}

func rawAuthority(value string) (string, bool) {
	if schemeEnd := strings.Index(value, "://"); schemeEnd >= 0 && isURLScheme(value[:schemeEnd]) {
		value = value[schemeEnd+len("://"):]
	}
	if authorityEnd := strings.IndexAny(value, "/?#"); authorityEnd >= 0 {
		value = value[:authorityEnd]
	}
	return value, value != ""
}

// IsSafeRawAuthority reports whether a raw authority stays ASCII-only and
// percent-free before any URL parsing or case folding can reinterpret it.
func IsSafeRawAuthority(authority string) bool {
	for i := 0; i < len(authority); i++ {
		if authority[i] > 0x7f || authority[i] == '%' {
			return false
		}
	}
	return true
}

func isURLScheme(value string) bool {
	if value == "" || !isASCIILetter(value[0]) {
		return false
	}
	for i := 1; i < len(value); i++ {
		if !isASCIILetter(value[i]) && (value[i] < '0' || value[i] > '9') && value[i] != '+' && value[i] != '-' && value[i] != '.' {
			return false
		}
	}
	return true
}

func isASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
