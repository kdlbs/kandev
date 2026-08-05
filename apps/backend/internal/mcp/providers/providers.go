// Package providers defines the provider capabilities that can affect the
// task-scoped MCP tool surface.
package providers

import "strings"

const (
	GitHub = "github"
	GitLab = "gitlab"
)

var supported = [...]string{GitHub, GitLab}

// Normalize returns the unique, canonical supported providers in stable order.
// Unknown and empty values are intentionally discarded so callers fail closed.
func Normalize(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if isSupported(value) {
			seen[value] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for _, value := range supported {
		if _, ok := seen[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

// Contains reports whether values include the supported provider.
func Contains(values []string, provider string) bool {
	normalized := Normalize([]string{provider})
	if len(normalized) == 0 {
		return false
	}
	for _, value := range Normalize(values) {
		if value == normalized[0] {
			return true
		}
	}
	return false
}

func isSupported(value string) bool {
	return value == GitHub || value == GitLab
}
