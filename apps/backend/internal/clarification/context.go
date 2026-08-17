package clarification

import "strings"

// NormalizeContext canonicalizes escaped paragraph separators emitted by agent
// clients before clarification metadata is persisted and shared with UI consumers.
func NormalizeContext(value string) string {
	value = strings.ReplaceAll(value, `\r\n\r\n`, "\n\n")
	value = strings.ReplaceAll(value, `\n\n`, "\n\n")
	return strings.ReplaceAll(value, `\r\r`, "\n\n")
}
