package shared

import (
	"encoding/json"
	"regexp"
	"strings"
)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// IsValidPathComponent reports whether s is safe to use as a filesystem path
// component: non-empty, no slashes, no backslashes, no ".." traversal.
func IsValidPathComponent(s string) bool {
	return s != "" &&
		!strings.Contains(s, "/") &&
		!strings.Contains(s, "\\") &&
		!strings.Contains(s, "..")
}

// GenerateSlug converts a name to a lowercase hyphenated slug suitable for
// use as a URL or filesystem identifier.
func GenerateSlug(name string) string {
	lower := strings.ToLower(name)
	slug := slugRe.ReplaceAllString(lower, "-")
	return strings.Trim(slug, "-")
}

// MustJSON marshals v to a JSON string, returning "{}" on any error.
// Intended for struct fields that must always contain valid JSON.
func MustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
