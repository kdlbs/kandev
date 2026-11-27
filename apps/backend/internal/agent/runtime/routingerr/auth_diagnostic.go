package routingerr

import "strings"

var authenticationDiagnosticPatterns = []string{
	"authentication_error",
	"authentication required",
	"token has expired",
	"failed to authenticate",
	"authorization_error",
	"invalid_api_key",
	"invalid api key",
}

// IsAuthenticationFailureDiagnostic reports whether diagnostic text matches
// the authentication signals used by launch recovery.
func IsAuthenticationFailureDiagnostic(diagnostic string) bool {
	lower := strings.ToLower(diagnostic)
	for _, pattern := range authenticationDiagnosticPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
