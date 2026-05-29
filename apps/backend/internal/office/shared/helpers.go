package shared

import (
	"regexp"
	"strings"
)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// GenerateSlug converts a name to a lowercase hyphenated slug suitable for
// use as a URL or filesystem identifier.
func GenerateSlug(name string) string {
	lower := strings.ToLower(name)
	slug := slugRe.ReplaceAllString(lower, "-")
	return strings.Trim(slug, "-")
}
