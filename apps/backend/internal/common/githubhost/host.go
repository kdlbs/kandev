// Package githubhost owns validated GitHub web authorities and endpoint families.
package githubhost

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

type Family string

const (
	PublicFamily     Family = "github.com"
	CloudFamily      Family = "ghe.com"
	EnterpriseFamily Family = "ghes"
)

// Host is an immutable validated web authority. Its zero value is invalid.
type Host struct {
	authority string
	family    Family
}

var ErrInvalidHost = errors.New("invalid GitHub host")

func Parse(input string) (Host, error) {
	u, err := parseOrigin(input)
	if err != nil {
		return Host{}, err
	}
	name := strings.ToLower(u.Hostname())
	if !validDNSName(name) || net.ParseIP(name) != nil || strings.HasSuffix(u.Host, ":") {
		return Host{}, ErrInvalidHost
	}
	port, err := normalizedPort(u.Port())
	if err != nil {
		return Host{}, err
	}
	family, err := hostFamily(name)
	if err != nil {
		return Host{}, err
	}
	if family != EnterpriseFamily && port != "" {
		return Host{}, ErrInvalidHost
	}
	authority := name
	if port != "" {
		authority += ":" + port
	}
	return Host{authority: authority, family: family}, nil
}

func parseOrigin(input string) (*url.URL, error) {
	if input == "" || strings.ContainsAny(input, "?#\\") || strings.ContainsFunc(input, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return nil, ErrInvalidHost
	}
	raw := input
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, ErrInvalidHost
	}
	if !strings.EqualFold(u.Scheme, "https") || u.User != nil || u.Opaque != "" {
		return nil, ErrInvalidHost
	}
	if (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, ErrInvalidHost
	}
	return u, nil
}

func normalizedPort(port string) (string, error) {
	if port == "" {
		return "", nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", ErrInvalidHost
	}
	if n == 443 {
		return "", nil
	}
	return strconv.Itoa(n), nil
}

func hostFamily(name string) (Family, error) {
	switch {
	case name == "api.github.com" || name == "ghe.com":
		return "", ErrInvalidHost
	case name == "github.com":
		return PublicFamily, nil
	case strings.HasSuffix(name, ".ghe.com"):
		if strings.Contains(strings.TrimSuffix(name, ".ghe.com"), ".") {
			return "", ErrInvalidHost
		}
		return CloudFamily, nil
	default:
		return EnterpriseFamily, nil
	}
}

func validDNSName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !validDNSCharacter(r) {
				return false
			}
		}
	}
	return true
}

func Public() Host            { return Host{authority: "github.com", family: PublicFamily} }
func (h Host) String() string { return h.authority }
func (h Host) Family() Family { return h.family }
func (h Host) IsPublic() bool { return h.family == PublicFamily }
func (h Host) Valid() bool    { return h.authority != "" }
func (h Host) WebURL() string {
	if !h.Valid() {
		return ""
	}
	return "https://" + h.authority
}
func (h Host) RESTURL() string {
	switch h.family {
	case PublicFamily:
		return "https://api.github.com"
	case CloudFamily:
		return "https://api." + h.authority
	default:
		if !h.Valid() {
			return ""
		}
		return h.WebURL() + "/api/v3"
	}
}
func (h Host) GraphQLURL() string {
	if h.family == EnterpriseFamily {
		return h.WebURL() + "/api/graphql"
	}
	if !h.Valid() {
		return ""
	}
	return h.RESTURL() + "/graphql"
}
func (h Host) TokenVariable() string {
	if h.family == EnterpriseFamily {
		return "GH_ENTERPRISE_TOKEN"
	}
	return "GH_TOKEN"
}

func validDNSCharacter(r rune) bool { return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' }
