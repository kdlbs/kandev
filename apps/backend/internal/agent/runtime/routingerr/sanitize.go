package routingerr

import "github.com/kandev/kandev/internal/common/securityutil"

// MaxRawExcerptBytes caps the sanitized excerpt before persistence.
const MaxRawExcerptBytes = securityutil.MaxRawExcerptBytes

// Sanitize redacts provider diagnostics before they cross runtime boundaries.
func Sanitize(s string) string {
	return securityutil.Sanitize(s)
}

// SanitizeCredentials preserves user-authored text apart from credentials.
func SanitizeCredentials(s string) string {
	return securityutil.SanitizeCredentials(s)
}

// SanitizeCredentialsUnbounded preserves complete user-authored text.
func SanitizeCredentialsUnbounded(s string) string {
	return securityutil.SanitizeCredentialsUnbounded(s)
}

// SanitizeFullUnbounded redacts diagnostics before a caller-owned tail cut.
func SanitizeFullUnbounded(s string) string {
	return securityutil.SanitizeFullUnbounded(s)
}

// SanitizeError preserves error identity while redacting its displayed message.
func SanitizeError(err error) error {
	return securityutil.SanitizeError(err)
}
