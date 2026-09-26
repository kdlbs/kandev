package hostnames

import "strings"

// NormalizeHostname returns the first DNS record suitable for display.
func NormalizeHostname(names []string) string {
	for _, name := range names {
		name = strings.TrimSuffix(strings.TrimSpace(name), ".")
		lower := strings.ToLower(name)
		if name == "" || strings.HasSuffix(lower, ".in-addr.arpa") || strings.HasSuffix(lower, ".ip6.arpa") {
			continue
		}
		return name
	}
	return ""
}
